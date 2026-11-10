package dns

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/quic-go/quic-go"
)

// testMTLSCA builds a throwaway client CA and one client certificate signed by
// it. The server certificate used by the tests is self-signed and unrelated;
// clients skip verification, as a subscriber pinned to a stamp would not.
func testMTLSCA(t *testing.T) (caPEM string, client tls.Certificate) {
	t.Helper()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate CA key: %v", err)
	}
	now := time.Now()
	caTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(10),
		Subject:               pkix.Name{CommonName: "DigitDNS Test CA"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create CA: %v", err)
	}
	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatalf("parse CA: %v", err)
	}

	clientKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate client key: %v", err)
	}
	clientTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(11),
		Subject:      pkix.Name{CommonName: "subscriber"},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	clientDER, err := x509.CreateCertificate(rand.Reader, clientTmpl, caCert, &clientKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create client cert: %v", err)
	}
	caPEM = string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}))
	return caPEM, tls.Certificate{Certificate: [][]byte{clientDER}, PrivateKey: clientKey}
}

// DoT is the one listener with a hard handshake-time gate: it shares its port
// with nothing, so a client without a certificate never reaches a query.
func TestApplyTransportsDoTMTLSRequiresClientCert(t *testing.T) {
	caPEM, clientCert := testMTLSCA(t)
	port := freeTCPPort(t)
	s := &Server{
		handler:   testDoQHandler(t),
		tlsConfig: testDoH3TLSConfig(t),
		bindHost:  "127.0.0.1",
		dotPort:   port,
	}
	t.Cleanup(s.Stop)
	s.transportCfg.DoTAuth = "sni-token"
	s.transportCfg.DoTTokenDomain = "dot.example.com"

	if err := s.ApplyTransports(TransportConfig{
		DoTPort:     port,
		DoTAuth:     "ip",
		MTLSEnabled: true,
		MTLSCAPEM:   caPEM,
	}); err != nil {
		t.Fatalf("start DoT with mTLS: %v", err)
	}

	dialer := &net.Dialer{Timeout: 3 * time.Second}
	addr := fmt.Sprintf("127.0.0.1:%d", port)

	noCert := &tls.Config{InsecureSkipVerify: true, NextProtos: []string{alpnDoT}, ServerName: "localhost"}
	conn, err := tls.DialWithDialer(dialer, "tcp", addr, noCert)
	if err == nil {
		_, err = doTExchange(t, conn, "pin.internal.test.")
		_ = conn.Close()
	}
	if err == nil {
		t.Fatal("DoT with mTLS answered a client without a certificate")
	}

	withCert := noCert.Clone()
	withCert.Certificates = []tls.Certificate{clientCert}
	conn, err = tls.DialWithDialer(dialer, "tcp", addr, withCert)
	if err != nil {
		t.Fatalf("dial with a signed client certificate: %v", err)
	}
	resp, err := doTExchange(t, conn, "pin.internal.test.")
	_ = conn.Close()
	if err != nil {
		t.Fatalf("exchange with a signed client certificate: %v", err)
	}
	if len(resp.Answer) != 1 {
		t.Fatalf("answers = %d, want 1", len(resp.Answer))
	}
}

// A CA bundle that is empty or unparsable must fail the save rather than
// silently run an mTLS listener that trusts nothing (or everything).
func TestApplyTransportsMTLSRejectsBadCA(t *testing.T) {
	port := freeTCPPort(t)
	s := &Server{
		handler:   testDoQHandler(t),
		tlsConfig: testDoH3TLSConfig(t),
		bindHost:  "127.0.0.1",
		dotPort:   port,
	}
	t.Cleanup(s.Stop)
	s.transportCfg.DoTAuth = "sni-token"
	s.transportCfg.DoTTokenDomain = "dot.example.com"

	if err := s.ApplyTransports(TransportConfig{
		DoTPort:     port,
		DoTAuth:     "ip",
		MTLSEnabled: true,
		MTLSCAPEM:   "not a certificate",
	}); err == nil {
		t.Fatal("ApplyTransports accepted an mTLS bundle with no certificates")
	}
}

// DoH requests a certificate at handshake time so the subscriber portal on the
// same listener stays reachable; /dns-query then rejects an unverified chain.
func TestDoHMTLSGate(t *testing.T) {
	h := newDoHHandler(t, &dummyAccess{})
	h.SetMTLSRequire(true)

	req := httptest.NewRequest(http.MethodPost, "/dns-query", strings.NewReader(string(dohQuery(t, "pin.internal.test."))))
	req.Header.Set("Content-Type", "application/dns-message")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("no TLS = %d, want 403", rec.Code)
	}

	_, clientCert := testMTLSCA(t)
	leaf, err := x509.ParseCertificate(clientCert.Certificate[0])
	if err != nil {
		t.Fatalf("parse client cert: %v", err)
	}
	req = httptest.NewRequest(http.MethodPost, "/dns-query", strings.NewReader(string(dohQuery(t, "pin.internal.test."))))
	req.Header.Set("Content-Type", "application/dns-message")
	req.TLS = &tls.ConnectionState{VerifiedChains: [][]*x509.Certificate{{leaf}}}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	dohReply(t, rec)

	h.SetMTLSRequire(false)
}

// DoQ's handshake requests a certificate, and the connection is closed before
// any stream is served when the chain is not verified.
func TestApplyTransportsDoQMTLSRejectsWithoutCert(t *testing.T) {
	caPEM, _ := testMTLSCA(t)
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve UDP port: %v", err)
	}
	port := pc.LocalAddr().(*net.UDPAddr).Port
	_ = pc.Close()

	s := &Server{
		handler:   testDoQHandler(t),
		tlsConfig: testDoH3TLSConfig(t),
		bindHost:  "127.0.0.1",
		doqPort:   port,
		dotPort:   freeTCPPort(t),
	}
	t.Cleanup(s.Stop)

	if err := s.ApplyTransports(TransportConfig{
		DoTPort:     s.dotPort,
		DoQPort:     port,
		DoQEnabled:  true,
		DoQAuth:     "ip",
		MTLSEnabled: true,
		MTLSCAPEM:   caPEM,
	}); err != nil {
		t.Fatalf("start DoQ with mTLS: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := quic.DialAddr(ctx, fmt.Sprintf("127.0.0.1:%d", port), &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{alpnDoQ},
	}, &quic.Config{})
	if err != nil {
		// A handshake-time rejection is a valid outcome too.
		return
	}
	_, err = doqExchange(ctx, conn, "pin.internal.test.")
	_ = conn.CloseWithError(0, "")
	if err == nil {
		t.Fatal("DoQ with mTLS answered a client without a certificate")
	}
}

// verifyClientCertIfGiven is the mode DoH/DoH3/DoQ need: a certificate that is
// offered must be verified (so the handler sees VerifiedChains), but its
// absence must not break the portal sharing the listener.
func TestApplyMTLSVerifiesOfferedCertificates(t *testing.T) {
	caPEM, _ := testMTLSCA(t)
	s := &Server{transportCfg: TransportConfig{MTLSEnabled: true, MTLSCAPEM: caPEM}}

	optional := &tls.Config{}
	if err := s.applyMTLS(optional, false); err != nil {
		t.Fatalf("applyMTLS optional: %v", err)
	}
	if optional.ClientAuth != tls.VerifyClientCertIfGiven {
		t.Errorf("optional ClientAuth = %v, want VerifyClientCertIfGiven", optional.ClientAuth)
	}

	required := &tls.Config{}
	if err := s.applyMTLS(required, true); err != nil {
		t.Fatalf("applyMTLS required: %v", err)
	}
	if required.ClientAuth != tls.RequireAndVerifyClientCert {
		t.Errorf("required ClientAuth = %v, want RequireAndVerifyClientCert", required.ClientAuth)
	}
}
