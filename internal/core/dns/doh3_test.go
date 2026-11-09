package dns

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"digitdns/internal/core/cache"
	"digitdns/internal/core/matcher"
	"digitdns/internal/core/upstream"
	"github.com/miekg/dns"
	"github.com/quic-go/quic-go/http3"
)

func testDoH3TLSConfig(t *testing.T) *tls.Config {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		DNSNames:     []string{"localhost"},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	return &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}}
}

func testDoH3Handler(t *testing.T) *DoHHandler {
	t.Helper()
	c := cache.NewCache(1000, 60, 3600)
	t.Cleanup(c.Close)
	m := matcher.NewMatcher()
	m.SetCustomRules(nil, nil, nil, map[string]string{"pin.internal.test": "203.0.113.77"})
	u := upstream.NewUpstreamPool([]string{"192.0.2.1:53"}, 200*time.Millisecond, true, "")
	f := NewHandler(&dummyAccess{}, c, m, u, nil, "198.51.100.1")
	return NewDoHHandler(f)
}

func TestDoHHandlerAltSvcAndDoH3Protocol(t *testing.T) {
	c := cache.NewCache(1000, 60, 3600)
	t.Cleanup(c.Close)
	m := matcher.NewMatcher()
	m.SetCustomRules(nil, nil, nil, map[string]string{"pin.internal.test": "203.0.113.77"})
	u := upstream.NewUpstreamPool([]string{"192.0.2.1:53"}, 200*time.Millisecond, true, "")
	sink := &recordingTelemetry{}
	h := NewDoHHandler(NewHandler(&oneSubscriberAccess{allowed: "203.0.113.50"}, c, m, u, sink, "198.51.100.1"))
	h.SetAltSvc(`h3=":8443"; ma=86400`)

	query := dohQuery(t, "pin.internal.test.")
	req := httptest.NewRequest(http.MethodPost, "/dns-query", bytes.NewReader(query))
	req.Header.Set("Content-Type", "application/dns-message")
	req.RemoteAddr = "203.0.113.50:51000"
	req.Proto = "HTTP/3.0"
	req.ProtoMajor, req.ProtoMinor = 3, 0
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	dohReply(t, rec)
	if got := rec.Header().Get("Alt-Svc"); got != `h3=":8443"; ma=86400` {
		t.Errorf("Alt-Svc = %q", got)
	}
	if len(sink.items) != 1 || sink.items[0].Protocol != "DoH3" {
		t.Fatalf("logged protocol = %#v, want DoH3", sink.items)
	}

	req = httptest.NewRequest(http.MethodPost, "/dns-query", bytes.NewReader(dohQuery(t, "pin.internal.test.")))
	req.Header.Set("Content-Type", "application/dns-message")
	req.RemoteAddr = "203.0.113.50:51000"
	req.Proto = "HTTP/2.0"
	req.ProtoMajor, req.ProtoMinor = 2, 0
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	dohReply(t, rec)
	if len(sink.items) != 2 || sink.items[1].Protocol != "DoH" {
		t.Fatalf("logged protocol = %#v, want DoH", sink.items)
	}
}

func TestApplyTransportsStartsAndStopsDoH3(t *testing.T) {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve UDP port: %v", err)
	}
	port := pc.LocalAddr().(*net.UDPAddr).Port
	_ = pc.Close()

	doh := testDoH3Handler(t)
	s := &Server{
		tlsConfig:  testDoH3TLSConfig(t),
		webMux:     doh,
		dohHandler: doh,
		bindHost:   "127.0.0.1",
		dohPort:    port,
		transportCfg: TransportConfig{
			DoHPort:     port,
			DoH3Enabled: true,
		},
	}
	t.Cleanup(func() { _ = s.ApplyTransports(TransportConfig{DoHPort: port}) })

	if err := s.ApplyTransports(TransportConfig{DoHPort: port, DoH3Enabled: true}); err != nil {
		t.Fatalf("start DoH3: %v", err)
	}
	if doh.altSvcValue() == "" {
		t.Fatal("Alt-Svc was not advertised while DoH3 was running")
	}

	transport := &http3.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
	defer transport.Close()
	client := &http.Client{Transport: transport}
	url := fmt.Sprintf("https://127.0.0.1:%d/dns-query", port)

	var resp *http.Response
	for attempt := 0; attempt < 50; attempt++ {
		req, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(dohQuery(t, "pin.internal.test.")))
		req.Header.Set("Content-Type", "application/dns-message")
		resp, err = client.Do(req)
		if err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("HTTP/3 DoH request: %v", err)
	}
	defer resp.Body.Close()
	if resp.ProtoMajor != 3 {
		t.Errorf("response protocol = %q, want HTTP/3", resp.Proto)
	}
	var body bytes.Buffer
	if _, err := body.ReadFrom(resp.Body); err != nil {
		t.Fatalf("read response: %v", err)
	}
	msg := new(dns.Msg)
	if err := msg.Unpack(body.Bytes()); err != nil {
		t.Fatalf("unpack response: %v", err)
	}
	if len(msg.Answer) != 1 {
		t.Fatalf("answers = %d, want 1", len(msg.Answer))
	}
	if a, ok := msg.Answer[0].(*dns.A); !ok || a.A.String() != "203.0.113.77" {
		t.Errorf("answer = %#v", msg.Answer[0])
	}

	if err := s.ApplyTransports(TransportConfig{DoHPort: port}); err != nil {
		t.Fatalf("stop DoH3: %v", err)
	}
	if doh.altSvcValue() != "" {
		t.Error("Alt-Svc still advertised after DoH3 stopped")
	}
}

func TestApplyTransportsDoQRequiresTLS(t *testing.T) {
	s := &Server{}
	if err := s.ApplyTransports(TransportConfig{DoQEnabled: true, DoQPort: 853}); err == nil {
		t.Fatal("ApplyTransports accepted DoQ without a TLS certificate")
	}
}
