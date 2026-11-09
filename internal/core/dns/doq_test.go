package dns

import (
	"context"
	"crypto/tls"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	"digitdns/internal/core/cache"
	"digitdns/internal/core/matcher"
	"digitdns/internal/core/upstream"
	"github.com/miekg/dns"
	"github.com/quic-go/quic-go"
)

func testDoQHandler(t *testing.T) *Handler {
	t.Helper()
	c := cache.NewCache(1000, 60, 3600)
	t.Cleanup(c.Close)
	m := matcher.NewMatcher()
	m.SetCustomRules(nil, nil, nil, map[string]string{"pin.internal.test": "203.0.113.77"})
	u := upstream.NewUpstreamPool([]string{"192.0.2.1:53"}, 200*time.Millisecond, true, "")
	return NewHandler(&dummyAccess{}, c, m, u, nil, "198.51.100.1")
}

func TestApplyTransportsStartsAndStopsDoQ(t *testing.T) {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve UDP port: %v", err)
	}
	port := pc.LocalAddr().(*net.UDPAddr).Port
	_ = pc.Close()

	s := &Server{
		handler:      testDoQHandler(t),
		tlsConfig:    testDoH3TLSConfig(t),
		bindHost:     "127.0.0.1",
		doqPort:      port,
		transportCfg: TransportConfig{DoQPort: port, DoQEnabled: true, DoQAuth: "ip"},
	}
	t.Cleanup(func() { _ = s.ApplyTransports(TransportConfig{DoQPort: port}) })

	if err := s.ApplyTransports(TransportConfig{DoQPort: port, DoQEnabled: true, DoQAuth: "ip"}); err != nil {
		t.Fatalf("start DoQ: %v", err)
	}
	if s.doqListener == nil {
		t.Fatal("DoQ listener was not created")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tlsConf := &tls.Config{InsecureSkipVerify: true, NextProtos: []string{alpnDoQ}}
	var conn *quic.Conn
	for attempt := 0; attempt < 50; attempt++ {
		conn, err = quic.DialAddr(ctx, fmt.Sprintf("127.0.0.1:%d", port), tlsConf, &quic.Config{})
		if err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("dial DoQ: %v", err)
	}
	defer conn.CloseWithError(0, "")

	stream, err := conn.OpenStreamSync(ctx)
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}
	query := new(dns.Msg)
	query.SetQuestion("pin.internal.test.", dns.TypeA)
	query.Id = 0
	packed, err := query.Pack()
	if err != nil {
		t.Fatalf("pack query: %v", err)
	}
	var lenBuf [2]byte
	binary.BigEndian.PutUint16(lenBuf[:], uint16(len(packed)))
	if _, err := stream.Write(lenBuf[:]); err != nil {
		t.Fatalf("write length: %v", err)
	}
	if _, err := stream.Write(packed); err != nil {
		t.Fatalf("write query: %v", err)
	}
	if err := stream.Close(); err != nil {
		t.Fatalf("close send side: %v", err)
	}

	if _, err := io.ReadFull(stream, lenBuf[:]); err != nil {
		t.Fatalf("read response length: %v", err)
	}
	size := int(binary.BigEndian.Uint16(lenBuf[:]))
	if size == 0 || size > doqMaxMessageSize {
		t.Fatalf("response size = %d", size)
	}
	body := make([]byte, size)
	if _, err := io.ReadFull(stream, body); err != nil {
		t.Fatalf("read response body: %v", err)
	}
	var resp dns.Msg
	if err := resp.Unpack(body); err != nil {
		t.Fatalf("unpack response: %v", err)
	}
	if len(resp.Answer) != 1 {
		t.Fatalf("answers = %d, want 1", len(resp.Answer))
	}
	if a, ok := resp.Answer[0].(*dns.A); !ok || a.A.String() != "203.0.113.77" {
		t.Errorf("answer = %#v", resp.Answer[0])
	}

	if err := s.ApplyTransports(TransportConfig{DoQPort: port}); err != nil {
		t.Fatalf("stop DoQ: %v", err)
	}
	if s.doqListener != nil {
		t.Error("DoQ listener still present after disable")
	}
}

func doqExchange(ctx context.Context, conn *quic.Conn, name string) (*dns.Msg, error) {
	stream, err := conn.OpenStreamSync(ctx)
	if err != nil {
		return nil, err
	}
	query := new(dns.Msg)
	query.SetQuestion(name, dns.TypeA)
	query.Id = 0
	packed, err := query.Pack()
	if err != nil {
		return nil, err
	}
	var lenBuf [2]byte
	binary.BigEndian.PutUint16(lenBuf[:], uint16(len(packed)))
	if _, err := stream.Write(lenBuf[:]); err != nil {
		return nil, err
	}
	if _, err := stream.Write(packed); err != nil {
		return nil, err
	}
	if err := stream.Close(); err != nil {
		return nil, err
	}
	if _, err := io.ReadFull(stream, lenBuf[:]); err != nil {
		return nil, err
	}
	size := int(binary.BigEndian.Uint16(lenBuf[:]))
	if size == 0 || size > doqMaxMessageSize {
		return nil, fmt.Errorf("response size = %d", size)
	}
	body := make([]byte, size)
	if _, err := io.ReadFull(stream, body); err != nil {
		return nil, err
	}
	var resp dns.Msg
	if err := resp.Unpack(body); err != nil {
		return nil, err
	}
	return &resp, nil
}

func TestDoQSNITokenAuth(t *testing.T) {
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
	}
	s.SetDoQTokens([]string{"Alpha", " beta "})
	cfg := TransportConfig{
		DoQPort:        port,
		DoQEnabled:     true,
		DoQAuth:        "sni-token",
		DoQTokenDomain: "doq.example.com",
	}
	if err := s.ApplyTransports(cfg); err != nil {
		t.Fatalf("start DoQ with tokens: %v", err)
	}
	t.Cleanup(func() { _ = s.ApplyTransports(TransportConfig{DoQPort: port}) })

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	validTLS := &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{alpnDoQ},
		ServerName:         "alpha.doq.example.com",
	}
	conn, err := quic.DialAddr(ctx, fmt.Sprintf("127.0.0.1:%d", port), validTLS, &quic.Config{})
	if err != nil {
		t.Fatalf("dial valid token: %v", err)
	}
	resp, err := doqExchange(ctx, conn, "pin.internal.test.")
	_ = conn.CloseWithError(0, "")
	if err != nil {
		t.Fatalf("valid token exchange: %v", err)
	}
	if len(resp.Answer) != 1 {
		t.Fatalf("valid token answers = %d, want 1", len(resp.Answer))
	}

	invalidTLS := validTLS.Clone()
	invalidTLS.ServerName = "gamma.doq.example.com"
	conn, err = quic.DialAddr(ctx, fmt.Sprintf("127.0.0.1:%d", port), invalidTLS, &quic.Config{})
	if err != nil {
		// A handshake-time rejection is also a valid outcome.
		return
	}
	_, err = doqExchange(ctx, conn, "pin.internal.test.")
	_ = conn.CloseWithError(0, "")
	if err == nil {
		t.Fatal("invalid SNI token received a DNS answer")
	}
}

func TestApplyTransportsDoQSNITokenRequiresConfig(t *testing.T) {
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
	}
	if err := s.ApplyTransports(TransportConfig{
		DoQPort:        port,
		DoQEnabled:     true,
		DoQAuth:        "sni-token",
		DoQTokenDomain: "doq.example.com",
	}); err == nil {
		t.Fatal("ApplyTransports accepted sni-token without tokens")
	}

	s.SetDoQTokens([]string{"alpha"})
	if err := s.ApplyTransports(TransportConfig{
		DoQPort:    port,
		DoQEnabled: true,
		DoQAuth:    "sni-token",
	}); err == nil {
		t.Fatal("ApplyTransports accepted sni-token without a token domain")
	}
}
