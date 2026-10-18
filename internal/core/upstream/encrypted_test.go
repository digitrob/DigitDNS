package upstream

// The encrypted transports (DoT and DoH) live at the socket boundary exactly like
// UDP and TCP do, so these tests drive the real pool against a real TLS listener
// and a real HTTPS handler rather than a mocked client. The pool validates the
// upstream's certificate in production; the tests inject a root that trusts only
// their own freshly minted leaf, which is what keeps the strict verification on
// the production path.

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/miekg/dns"
)

// newTestCert mints a self-signed leaf valid for loopback, so the pool's
// certificate verification accepts a listener on 127.0.0.1.
func newTestCert(t *testing.T) (tls.Certificate, *x509.Certificate) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "digitdns-test"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IPAddresses:           []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback},
		DNSNames:              []string{"localhost"},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse certificate: %v", err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: leaf}, leaf
}

// newDoTResolver starts a DNS server behind TLS on loopback and returns its
// address, its leaf (for the client's root), and a hit counter. Only a TLS
// listener exists, so a probe sent over plain UDP or TCP would fail to connect
// and leave the counter at zero — which is what makes the benchmark test below a
// real assertion about the transport, not just about the answer.
func newDoTResolver(t *testing.T, reply func(string) answer) (addr string, leaf *x509.Certificate, hits *atomic.Int64) {
	t.Helper()
	cert, leaf := newTestCert(t)
	rawLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	ln := tls.NewListener(rawLn, &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12})

	hits = &atomic.Int64{}
	r := &testResolver{}
	srv := &dns.Server{Listener: ln, Handler: dns.HandlerFunc(func(w dns.ResponseWriter, req *dns.Msg) {
		hits.Add(1)
		r.reply(w, req, reply)
	})}
	go func() { _ = srv.ActivateAndServe() }()
	t.Cleanup(func() { _ = srv.Shutdown() })

	return rawLn.Addr().String(), leaf, hits
}

// newDoHResolver starts an RFC 8484 endpoint and returns its URL, the server (whose
// Client() trusts its own certificate), a request counter, and whether a query
// arrived with transaction ID 0.
func newDoHResolver(t *testing.T, reply func(string) answer) (url string, srv *httptest.Server, hits *atomic.Int64, sawZeroID *atomic.Bool) {
	t.Helper()
	hits = &atomic.Int64{}
	sawZeroID = &atomic.Bool{}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.Method != http.MethodPost {
			http.Error(w, "method", http.StatusMethodNotAllowed)
			return
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/dns-message" {
			http.Error(w, "content type", http.StatusUnsupportedMediaType)
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
		a := reply(m.Question[0].Name)
		resp := new(dns.Msg)
		resp.SetReply(m)
		if a.ip != "" {
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
	srv = httptest.NewTLSServer(handler)
	t.Cleanup(srv.Close)
	return srv.URL + "/dns-query", srv, hits, sawZeroID
}

func TestDotUpstreamExchangesOverTLS(t *testing.T) {
	addr, leaf, _ := newDoTResolver(t, forName(probeName, answer{rcode: dns.RcodeSuccess, ip: "192.0.2.41"}))

	p := newTestPool(t, []string{"tls://" + addr}, 2*time.Second, false, "")
	roots := x509.NewCertPool()
	roots.AddCert(leaf)
	p.tlsClient.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots}

	resp, _, _, err := p.Exchange(query(probeName))
	if err != nil {
		t.Fatalf("Exchange over DoT: %v", err)
	}
	if got := firstA(resp); got != "192.0.2.41" {
		t.Fatalf("answer = %q, want the DoT answer 192.0.2.41", got)
	}
}

// TestDotUpstreamRejectsUntrustedCertificate pins the strict verification: the
// pool must not disable TLS validation. A DoT server whose certificate the client
// does not trust has to fail, or "encrypted" would mean encrypted to anyone.
func TestDotUpstreamRejectsUntrustedCertificate(t *testing.T) {
	addr, _, _ := newDoTResolver(t, always(answer{rcode: dns.RcodeSuccess, ip: "192.0.2.41"}))

	// The default pool trusts the system roots, which do not include the test leaf.
	p := newTestPool(t, []string{"tls://" + addr}, 2*time.Second, false, "")

	if _, _, _, err := p.Exchange(query(probeName)); err == nil {
		t.Fatal("a DoT upstream with an untrusted certificate was accepted")
	}
}

func TestBenchmarkProbesDotOverTLS(t *testing.T) {
	addr, leaf, hits := newDoTResolver(t, always(answer{rcode: dns.RcodeSuccess, ip: "192.0.2.51"}))

	p := newTestPool(t, []string{"tls://" + addr}, 2*time.Second, false, "")
	roots := x509.NewCertPool()
	roots.AddCert(leaf)
	p.tlsClient.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots}

	p.BenchmarkAll()

	if hits.Load() == 0 {
		t.Fatal("BenchmarkAll probed the DoT upstream on a transport it does not listen on")
	}
	if statFor(t, p, "tls://"+addr).Hits == 0 {
		t.Fatal("a successful DoT benchmark recorded no hit")
	}
}

func TestDoHUpstreamExchangesOverHTTPS(t *testing.T) {
	url, srv, hits, sawZero := newDoHResolver(t, forName(probeName, answer{rcode: dns.RcodeSuccess, ip: "192.0.2.42"}))

	p := newTestPool(t, []string{url}, 2*time.Second, false, "")
	p.httpClient = srv.Client()

	resp, _, _, err := p.Exchange(query(probeName))
	if err != nil {
		t.Fatalf("Exchange over DoH: %v", err)
	}
	if got := firstA(resp); got != "192.0.2.42" {
		t.Fatalf("answer = %q, want the DoH answer 192.0.2.42", got)
	}
	if hits.Load() == 0 {
		t.Fatal("the DoH endpoint was never reached")
	}
	if !sawZero.Load() {
		t.Fatal("a DoH query must be sent with transaction ID 0 (RFC 8484 §4.1)")
	}
}

func TestBenchmarkProbesDoHOverHTTPS(t *testing.T) {
	url, srv, hits, _ := newDoHResolver(t, always(answer{rcode: dns.RcodeSuccess, ip: "192.0.2.52"}))

	p := newTestPool(t, []string{url}, 2*time.Second, false, "")
	p.httpClient = srv.Client()

	p.BenchmarkAll()

	if hits.Load() == 0 {
		t.Fatal("BenchmarkAll probed the DoH upstream on UDP:53 instead of HTTPS")
	}
	if statFor(t, p, url).Hits == 0 {
		t.Fatal("a successful DoH benchmark recorded no hit")
	}
}
