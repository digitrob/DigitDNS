package dns

import (
	"crypto/tls"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	"github.com/miekg/dns"
)

func freeTCPPort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve TCP port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	return port
}

// doTExchange sends one length-prefixed query over an established TLS
// connection and returns the decoded answer.
func doTExchange(t *testing.T, conn net.Conn, name string) (*dns.Msg, error) {
	t.Helper()
	if err := conn.SetDeadline(time.Now().Add(3 * time.Second)); err != nil {
		return nil, err
	}
	query := new(dns.Msg)
	query.SetQuestion(name, dns.TypeA)
	packed, err := query.Pack()
	if err != nil {
		return nil, err
	}
	var lenBuf [2]byte
	binary.BigEndian.PutUint16(lenBuf[:], uint16(len(packed)))
	if _, err := conn.Write(lenBuf[:]); err != nil {
		return nil, err
	}
	if _, err := conn.Write(packed); err != nil {
		return nil, err
	}
	if _, err := io.ReadFull(conn, lenBuf[:]); err != nil {
		return nil, err
	}
	size := int(binary.BigEndian.Uint16(lenBuf[:]))
	body := make([]byte, size)
	if _, err := io.ReadFull(conn, body); err != nil {
		return nil, err
	}
	var resp dns.Msg
	if err := resp.Unpack(body); err != nil {
		return nil, err
	}
	return &resp, nil
}

func dialDoT(port int, serverName string) (*tls.Conn, error) {
	return tls.DialWithDialer(
		&net.Dialer{Timeout: 3 * time.Second},
		"tcp",
		fmt.Sprintf("127.0.0.1:%d", port),
		&tls.Config{
			InsecureSkipVerify: true,
			NextProtos:         []string{alpnDoT},
			ServerName:         serverName,
		},
	)
}

// TestDoTSNITokenAuth starts a real DoT listener in sni-token mode. The SNI is
// the credential: <token>.<domain> is accepted only when the token is in the
// list; anything else is closed before it reaches the DNS server.
func TestDoTSNITokenAuth(t *testing.T) {
	port := freeTCPPort(t)
	s := &Server{
		handler:   testDoQHandler(t),
		tlsConfig: testDoH3TLSConfig(t),
		bindHost:  "127.0.0.1",
		dotPort:   port,
	}
	t.Cleanup(s.Stop)
	s.SetDoTTokens([]string{"Alpha", " beta "})

	// Force the TLS rebind path that a live save uses.
	s.transportCfg.DoTAuth = "ip"
	if err := s.ApplyTransports(TransportConfig{
		DoTPort:        port,
		DoTAuth:        "sni-token",
		DoTTokenDomain: "dot.example.com",
	}); err != nil {
		t.Fatalf("start DoT with tokens: %v", err)
	}

	conn, err := dialDoT(port, "alpha.dot.example.com")
	if err != nil {
		t.Fatalf("dial valid token: %v", err)
	}
	resp, err := doTExchange(t, conn, "pin.internal.test.")
	_ = conn.Close()
	if err != nil {
		t.Fatalf("valid token exchange: %v", err)
	}
	if len(resp.Answer) != 1 {
		t.Fatalf("valid token answers = %d, want 1", len(resp.Answer))
	}

	conn, err = dialDoT(port, "gamma.dot.example.com")
	if err == nil {
		_, err = doTExchange(t, conn, "pin.internal.test.")
		_ = conn.Close()
	}
	if err == nil {
		t.Fatal("invalid SNI token received a DNS answer")
	}
}

// A token list without a domain — or a mode with neither — must be refused
// before any listener binds, matching DoQ's validation.
func TestDoTSNITokenRequiresConfig(t *testing.T) {
	port := freeTCPPort(t)
	s := &Server{
		handler:   testDoQHandler(t),
		tlsConfig: testDoH3TLSConfig(t),
		bindHost:  "127.0.0.1",
		dotPort:   port,
	}
	t.Cleanup(s.Stop)
	s.transportCfg.DoTAuth = "ip"

	if err := s.ApplyTransports(TransportConfig{
		DoTPort:        port,
		DoTAuth:        "sni-token",
		DoTTokenDomain: "dot.example.com",
	}); err == nil {
		t.Fatal("ApplyTransports accepted sni-token without tokens")
	}

	s.SetDoTTokens([]string{"alpha"})
	if err := s.ApplyTransports(TransportConfig{
		DoTPort: port,
		DoTAuth: "sni-token",
	}); err == nil {
		t.Fatal("ApplyTransports accepted sni-token without a token domain")
	}
}

func TestSNITokenShape(t *testing.T) {
	tokens := []string{"alpha"}
	cases := []struct {
		sni  string
		want bool
	}{
		{"alpha.dot.example.com", true},
		{"ALPHA.DOT.EXAMPLE.COM", true},
		{"alpha.dot.example.com.", true},
		{"dot.example.com", false},
		{"alpha.", false},
		{"sub.alpha.dot.example.com", false},
		{"alpha.other.example.com", false},
	}
	for _, tc := range cases {
		got := sniTokenAllowed(tc.sni, "dot.example.com", tokens)
		if got != tc.want {
			t.Errorf("sniTokenAllowed(%q) = %v, want %v", tc.sni, got, tc.want)
		}
	}
}

// Changing DoT authentication must rebind the listener; changing an unrelated
// DoQ field must not. A rebind of a listener that is not bound is a no-op, so
// the observable signal is that the DoT listener comes up at all.
func TestApplyTransportsRebindsDoTOnAuthChange(t *testing.T) {
	port := freeTCPPort(t)
	s := &Server{
		handler:   testDoQHandler(t),
		tlsConfig: testDoH3TLSConfig(t),
		bindHost:  "127.0.0.1",
		dotPort:   port,
	}
	t.Cleanup(s.Stop)

	// The previous config claimed sni-token, so switching to ip rebinds DoT and
	// brings the listener up.
	s.transportCfg.DoTAuth = "sni-token"
	s.transportCfg.DoTTokenDomain = "dot.example.com"
	if err := s.ApplyTransports(TransportConfig{DoTPort: port, DoTAuth: "ip"}); err != nil {
		t.Fatalf("start DoT in ip mode: %v", err)
	}
	if s.dotServer == nil {
		t.Fatal("DoT listener did not start")
	}
	before := s.dotServer

	s.SetDoTTokens([]string{"alpha"})
	if err := s.ApplyTransports(TransportConfig{
		DoTPort:        port,
		DoTAuth:        "sni-token",
		DoTTokenDomain: "dot.example.com",
	}); err != nil {
		t.Fatalf("rebind DoT for sni-token: %v", err)
	}
	if s.dotServer == nil {
		t.Fatal("DoT listener was lost across the rebind")
	}
	if s.dotServer == before {
		t.Fatal("DoT listener was not rebound after the authentication mode changed")
	}
}

// A disabled DoT listener must really stop binding 853; re-enabling brings it
// back through the same TLS rebind path a live save uses.
func TestApplyTransportsDisablesAndReenablesDoT(t *testing.T) {
	port := freeTCPPort(t)
	s := &Server{
		handler:   testDoQHandler(t),
		tlsConfig: testDoH3TLSConfig(t),
		bindHost:  "127.0.0.1",
		dotPort:   port,
	}
	t.Cleanup(s.Stop)
	s.transportCfg.DoTAuth = "ip"
	s.transportCfg.DoTDisabled = true

	if err := s.ApplyTransports(TransportConfig{DoTPort: port, DoTAuth: "ip", DoTDisabled: false}); err != nil {
		t.Fatalf("enable DoT: %v", err)
	}
	if s.dotServer == nil {
		t.Fatal("DoT did not start when enabled")
	}

	if err := s.ApplyTransports(TransportConfig{DoTPort: port, DoTAuth: "ip", DoTDisabled: true}); err != nil {
		t.Fatalf("disable DoT: %v", err)
	}
	if s.dotServer != nil {
		t.Fatal("DoT listener survived the disable")
	}
	if _, err := dialDoT(port, "127.0.0.1"); err == nil {
		t.Fatal("disabled DoT still accepted a TLS connection")
	}

	if err := s.ApplyTransports(TransportConfig{DoTPort: port, DoTAuth: "ip", DoTDisabled: false}); err != nil {
		t.Fatalf("re-enable DoT: %v", err)
	}
	if s.dotServer == nil {
		t.Fatal("DoT did not come back when re-enabled")
	}
}
