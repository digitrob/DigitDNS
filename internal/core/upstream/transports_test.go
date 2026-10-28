package upstream

// DoQ, DoH3 and DNSCrypt are exercised against real listeners, the same way
// encrypted_test.go drives DoT and DoH: the pool validates everything it can, and
// each test injects only what a real deployment would already trust (a root CA
// for the TLS transports, the provider public key for DNSCrypt).

import (
	"context"
	"crypto/ed25519"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"io"
	"net"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ameshkov/dnscrypt/v2"
	"github.com/ameshkov/dnsstamps"
	"github.com/miekg/dns"
	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
)

// serveDoQ reads length-prefixed DNS messages off a QUIC stream and writes a
// length-prefixed reply, mirroring RFC 9250 §4.2 framing.
func serveDoQ(t *testing.T, conn *quic.Conn, reply func(string) answer, hits *atomic.Int64, sawNonZeroID *atomic.Bool) {
	t.Helper()
	for {
		stream, err := conn.AcceptStream(context.Background())
		if err != nil {
			return // connection closed
		}
		go func(s *quic.Stream) {
			defer func() { _ = s.Close() }()

			var lenBuf [2]byte
			if _, err := io.ReadFull(s, lenBuf[:]); err != nil {
				return
			}
			body := make([]byte, binary.BigEndian.Uint16(lenBuf[:]))
			if _, err := io.ReadFull(s, body); err != nil {
				return
			}
			m := new(dns.Msg)
			if err := m.Unpack(body); err != nil {
				return
			}
			hits.Add(1)
			if m.Id != 0 {
				sawNonZeroID.Store(true)
			}

			resp := new(dns.Msg)
			resp.SetReply(m)
			if a := reply(m.Question[0].Name); a.ip != "" {
				if rr, err := dns.NewRR(m.Question[0].Name + " 30 IN A " + a.ip); err == nil {
					resp.Answer = append(resp.Answer, rr)
				}
			}
			packed, err := resp.Pack()
			if err != nil {
				return
			}
			out := make([]byte, 2+len(packed))
			binary.BigEndian.PutUint16(out, uint16(len(packed)))
			copy(out[2:], packed)
			_, _ = s.Write(out)
		}(stream)
	}
}

// newDoQResolver starts a DNS-over-QUIC listener on loopback and returns its
// address, the leaf for the client's root pool, a hit counter, and whether any
// query arrived with a nonzero transaction ID.
func newDoQResolver(t *testing.T, reply func(string) answer) (addr string, leaf *x509.Certificate, hits *atomic.Int64, sawNonZeroID *atomic.Bool) {
	t.Helper()
	cert, leaf := newTestCert(t)

	ln, err := quic.ListenAddr("127.0.0.1:0", &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
		NextProtos:   []string{doqALPN},
	}, &quic.Config{})
	if err != nil {
		t.Fatalf("listen quic: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	hits = &atomic.Int64{}
	sawNonZeroID = &atomic.Bool{}
	go func() {
		for {
			conn, err := ln.Accept(context.Background())
			if err != nil {
				return
			}
			go serveDoQ(t, conn, reply, hits, sawNonZeroID)
		}
	}()
	return ln.Addr().String(), leaf, hits, sawNonZeroID
}

func TestDoQUpstreamExchangesOverQUIC(t *testing.T) {
	addr, leaf, hits, sawNonZeroID := newDoQResolver(t, forName(probeName, answer{rcode: dns.RcodeSuccess, ip: "192.0.2.61"}))

	p := newTestPool(t, []string{"quic://" + addr}, 2*time.Second, false, "")
	roots := x509.NewCertPool()
	roots.AddCert(leaf)
	p.quicTLS = &tls.Config{MinVersion: tls.VersionTLS12, NextProtos: []string{doqALPN}, RootCAs: roots}

	resp, _, _, err := p.Exchange(query(probeName))
	if err != nil {
		t.Fatalf("Exchange over DoQ: %v", err)
	}
	if got := firstA(resp); got != "192.0.2.61" {
		t.Fatalf("answer = %q, want the DoQ answer 192.0.2.61", got)
	}
	if hits.Load() == 0 {
		t.Fatal("the DoQ listener was never reached")
	}
	if sawNonZeroID.Load() {
		t.Fatal("a DoQ query must be sent with transaction ID 0 (RFC 9250 §4.2.1)")
	}
}

// TestDoQUpstreamRejectsUntrustedCertificate pins the strict verification: DoQ
// must not disable certificate validation any more than DoT does.
func TestDoQUpstreamRejectsUntrustedCertificate(t *testing.T) {
	addr, _, _, _ := newDoQResolver(t, always(answer{rcode: dns.RcodeSuccess, ip: "192.0.2.61"}))

	// The default pool trusts the system roots, which do not include the test leaf.
	p := newTestPool(t, []string{"quic://" + addr}, 2*time.Second, false, "")

	if _, _, _, err := p.Exchange(query(probeName)); err == nil {
		t.Fatal("a DoQ upstream with an untrusted certificate was accepted")
	}
}

// newDoH3Resolver starts an RFC 8484 endpoint over HTTP/3 and returns its URL, the
// root pool that trusts its certificate, a request counter, and whether a query
// arrived with transaction ID 0.
func newDoH3Resolver(t *testing.T, reply func(string) answer) (url string, roots *x509.CertPool, hits *atomic.Int64, sawZeroID *atomic.Bool) {
	t.Helper()
	cert, leaf := newTestCert(t)
	hits = &atomic.Int64{}
	sawZeroID = &atomic.Bool{}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.Method != http.MethodPost {
			http.Error(w, "method", http.StatusMethodNotAllowed)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "read", http.StatusBadRequest)
			return
		}
		m := new(dns.Msg)
		if err := m.Unpack(body); err != nil {
			http.Error(w, "unpack", http.StatusBadRequest)
			return
		}
		if m.Id == 0 {
			sawZeroID.Store(true)
		}
		resp := new(dns.Msg)
		resp.SetReply(m)
		if a := reply(m.Question[0].Name); a.ip != "" {
			if rr, err := dns.NewRR(m.Question[0].Name + " 30 IN A " + a.ip); err == nil {
				resp.Answer = append(resp.Answer, rr)
			}
		}
		packed, err := resp.Pack()
		if err != nil {
			http.Error(w, "pack", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/dns-message")
		_, _ = w.Write(packed)
	})

	srv := &http3.Server{
		Handler:   handler,
		TLSConfig: &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12},
	}
	pc, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("listen udp: %v", err)
	}
	go func() { _ = srv.Serve(pc) }()
	t.Cleanup(func() {
		_ = srv.Close()
		_ = pc.Close()
	})

	roots = x509.NewCertPool()
	roots.AddCert(leaf)
	return "h3://" + pc.LocalAddr().String() + "/dns-query", roots, hits, sawZeroID
}

func TestDoH3UpstreamExchangesOverHTTP3(t *testing.T) {
	url, roots, hits, sawZero := newDoH3Resolver(t, forName(probeName, answer{rcode: dns.RcodeSuccess, ip: "192.0.2.62"}))

	p := newTestPool(t, []string{url}, 2*time.Second, false, "")
	p.h3Client = &http.Client{
		Timeout:   2 * time.Second,
		Transport: &http3.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots}},
	}

	resp, _, _, err := p.Exchange(query(probeName))
	if err != nil {
		t.Fatalf("Exchange over DoH3: %v", err)
	}
	if got := firstA(resp); got != "192.0.2.62" {
		t.Fatalf("answer = %q, want the DoH3 answer 192.0.2.62", got)
	}
	if hits.Load() == 0 {
		t.Fatal("the DoH3 endpoint was never reached")
	}
	if !sawZero.Load() {
		t.Fatal("a DoH3 query must be sent with transaction ID 0 (RFC 8484 §4.1)")
	}
}

// dnscryptHandler answers every DNSCrypt query with a fixed A record.
type dnscryptHandler struct{ ip string }

func (h dnscryptHandler) ServeDNS(rw dnscrypt.ResponseWriter, r *dns.Msg) error {
	resp := new(dns.Msg)
	resp.SetReply(r)
	if len(r.Question) > 0 {
		if rr, err := dns.NewRR(r.Question[0].Name + " 30 IN A " + h.ip); err == nil {
			resp.Answer = append(resp.Answer, rr)
		}
	}
	return rw.WriteMsg(resp)
}

// TestDNSCryptUpstreamExchanges runs a real DNSCrypt server on loopback, encodes
// its provider key into an sdns:// stamp, and drives the pool through the stamp.
func TestDNSCryptUpstreamExchanges(t *testing.T) {
	rc, err := dnscrypt.GenerateResolverConfig("2.dnscrypt-cert.example.org", nil)
	if err != nil {
		t.Fatalf("generate resolver config: %v", err)
	}
	cert, err := rc.CreateCert()
	if err != nil {
		t.Fatalf("create resolver cert: %v", err)
	}

	udpConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("listen udp: %v", err)
	}
	srv := &dnscrypt.Server{
		ProviderName: rc.ProviderName,
		ResolverCert: cert,
		Handler:      dnscryptHandler{ip: "192.0.2.63"},
	}
	go func() { _ = srv.ServeUDP(udpConn) }()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
		_ = udpConn.Close()
	})

	priv, err := dnscrypt.HexDecodeKey(rc.PrivateKey)
	if err != nil {
		t.Fatalf("decode resolver key: %v", err)
	}
	pub := ed25519.PrivateKey(priv).Public().(ed25519.PublicKey)

	stamp := dnsstamps.ServerStamp{
		ServerAddrStr: udpConn.LocalAddr().String(),
		ServerPk:      pub,
		ProviderName:  rc.ProviderName,
		Proto:         dnsstamps.StampProtoTypeDNSCrypt,
	}
	stampStr := stamp.String()

	p := newTestPool(t, []string{stampStr}, 2*time.Second, false, "")
	if got := p.GetUpstreams(); len(got) != 1 || got[0] != stampStr {
		t.Fatalf("a DNSCrypt stamp must be kept verbatim: got %v, want [%s]", got, stampStr)
	}

	resp, _, _, err := p.Exchange(query(probeName))
	if err != nil {
		t.Fatalf("Exchange over DNSCrypt: %v", err)
	}
	if got := firstA(resp); got != "192.0.2.63" {
		t.Fatalf("answer = %q, want the DNSCrypt answer 192.0.2.63", got)
	}
}

// TestNormalizeStampTranslatesToNativeScheme checks that a DoH/DoT/DoQ stamp is
// translated to the pool's native scheme so it deduplicates with a hand-typed URL.
func TestNormalizeStampTranslatesToNativeScheme(t *testing.T) {
	doq := dnsstamps.ServerStamp{
		ServerAddrStr: "94.140.14.14",
		ServerPk:      make([]byte, 32),
		ProviderName:  "dns.adguard-dns.com",
		Proto:         dnsstamps.StampProtoTypeDoQ,
	}
	got, ok := normalizeUpstream(doq.String())
	if !ok || got != "quic://dns.adguard-dns.com:853" {
		t.Fatalf("normalizeUpstream(DoQ stamp) = %q, %v; want quic://dns.adguard-dns.com:853, true", got, ok)
	}

	doh := dnsstamps.ServerStamp{
		ServerAddrStr: "1.1.1.1",
		ServerPk:      make([]byte, 32),
		ProviderName:  "cloudflare-dns.com",
		Path:          "/dns-query",
		Proto:         dnsstamps.StampProtoTypeDoH,
	}
	got, ok = normalizeUpstream(doh.String())
	if !ok || got != "https://cloudflare-dns.com/dns-query" {
		t.Fatalf("normalizeUpstream(DoH stamp) = %q, %v; want https://cloudflare-dns.com/dns-query, true", got, ok)
	}
}

func TestEligibleUpstreamsFiltersByTransport(t *testing.T) {
	list := []string{"1.1.1.1:53", "tcp://9.9.9.9:53", "tls://dns.google:853", "https://dns.google/dns-query"}

	if got := eligibleUpstreams(list, ""); len(got) != 4 {
		t.Fatalf("empty transport should keep the whole list, got %v", got)
	}
	if got := eligibleUpstreams(list, "any"); len(got) != 4 {
		t.Fatalf("any should keep the whole list, got %v", got)
	}
	enc := eligibleUpstreams(list, "encrypted")
	if len(enc) != 2 || enc[0] != "tls://dns.google:853" || enc[1] != "https://dns.google/dns-query" {
		t.Fatalf("encrypted filter = %v", enc)
	}
	reg := eligibleUpstreams(list, "regular")
	if len(reg) != 2 || reg[0] != "1.1.1.1:53" || reg[1] != "tcp://9.9.9.9:53" {
		t.Fatalf("regular filter = %v", reg)
	}
}

func TestPinCandidateNeverBypassesTheTransport(t *testing.T) {
	plain := "1.1.1.1:53"
	dot := "tls://dns.google:853"
	all := []string{plain, dot}

	if got := pinCandidate(all, plain); len(got) != 1 || got[0] != plain {
		t.Fatalf("an allowed pin should narrow the list, got %v", got)
	}
	// Encrypted Only: the pool was filtered to the DoT resolver, so a plain pin
	// must not be used.
	encrypted := eligibleUpstreams(all, "encrypted")
	if got := pinCandidate(encrypted, plain); len(got) != 1 || got[0] != dot {
		t.Fatalf("a plain pin escaped Encrypted Only: %v", got)
	}
	// Regular DNS: the mirror image.
	regular := eligibleUpstreams(all, "regular")
	if got := pinCandidate(regular, dot); len(got) != 1 || got[0] != plain {
		t.Fatalf("an encrypted pin escaped Regular DNS: %v", got)
	}
}

func TestProtocolOfMapsTheResolutionTransport(t *testing.T) {
	cases := map[string]string{
		"1.1.1.1:53":                   "UDP",
		"tcp://1.1.1.1:53":             "TCP",
		"tls://dns.google:853":         "DoT",
		"https://dns.google/dns-query": "DoH",
		"quic://dns.adguard.com:853":   "DoQ",
		"doq://dns.adguard.com:853":    "DoQ",
		"h3://dns.google/dns-query":    "DoH3",
		"sdns://abc":                   "DNSCrypt",
		"cache":                        "",
		"":                             "",
	}
	for address, want := range cases {
		if got := ProtocolOf(address); got != want {
			t.Errorf("ProtocolOf(%q) = %q, want %q", address, got, want)
		}
	}
}
