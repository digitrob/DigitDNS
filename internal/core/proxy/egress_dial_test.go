package proxy

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"testing"
	"time"

	"digitdns/internal/database"
)

type fakeEgressDialer struct {
	called bool
	conn   net.Conn
	err    error
}

func (f *fakeEgressDialer) Dial(context.Context, string, string, []netip.Addr, int) (net.Conn, error) {
	f.called = true
	return f.conn, f.err
}

func TestDialTargetFailClosedEgressRefuses(t *testing.T) {
	s := NewServer(database.SNIProxySettings{}, "127.0.0.1", "", nil)
	dialer := &fakeEgressDialer{}
	s.SetRelayTarget(func(string) RelayDialTarget {
		return RelayDialTarget{
			Kind: "egress", EgressID: "e1", EgressName: "NL",
			FailClosed: true, Healthy: false,
		}
	}, dialer)

	_, err := s.dialTarget(context.Background(), "example.com", []netip.Addr{netip.MustParseAddr("203.0.113.10")}, 443, "198.51.100.7")
	if err == nil {
		t.Fatal("a fail-closed offline egress must refuse the relay")
	}
	if dialer.called {
		t.Fatal("the egress dialer must not be called when the egress is known offline")
	}
}

func TestDialTargetHealthyEgressIsUsed(t *testing.T) {
	s := NewServer(database.SNIProxySettings{}, "127.0.0.1", "", nil)
	server, client := net.Pipe()
	defer server.Close()
	dialer := &fakeEgressDialer{conn: client}
	s.SetRelayTarget(func(string) RelayDialTarget {
		return RelayDialTarget{
			Kind: "egress", EgressID: "e1", EgressName: "NL",
			FailClosed: true, Healthy: true,
		}
	}, dialer)

	conn, err := s.dialTarget(context.Background(), "example.com", []netip.Addr{netip.MustParseAddr("203.0.113.10")}, 443, "198.51.100.7")
	if err != nil {
		t.Fatalf("dialTarget: %v", err)
	}
	_ = conn.Close()
	if !dialer.called {
		t.Fatal("a healthy egress must carry the dial")
	}
}

func TestDialTargetFailOpenEgressFallsBack(t *testing.T) {
	// A local listener stands in for the direct path. The egress dial fails, so
	// a fail-open target must reach the listener instead of refusing.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	accepted := make(chan struct{})
	go func() {
		conn, err := ln.Accept()
		if err == nil {
			_ = conn.Close()
			close(accepted)
		}
	}()
	addr := ln.Addr().(*net.TCPAddr)

	s := NewServer(database.SNIProxySettings{}, "127.0.0.1", "", nil)
	dialer := &fakeEgressDialer{err: errors.New("egress down")}
	s.SetRelayTarget(func(string) RelayDialTarget {
		return RelayDialTarget{
			Kind: "egress", EgressID: "e1", EgressName: "NL",
			FailClosed: false, Healthy: true,
		}
	}, dialer)

	ip, _ := netip.AddrFromSlice(addr.IP)
	conn, err := s.dialTarget(context.Background(), "example.com", []netip.Addr{ip}, addr.Port, "198.51.100.7")
	if err != nil {
		t.Fatalf("dialTarget fallback: %v", err)
	}
	_ = conn.Close()
	select {
	case <-accepted:
	case <-time.After(2 * time.Second):
		t.Fatal("the fail-open fallback did not dial the direct path")
	}
}
