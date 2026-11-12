package proxy

import (
	"context"
	"crypto/tls"
	"net"
	"testing"

	"digitdns/internal/database"
)

type blockingRelayPolicy struct{}

func (blockingRelayPolicy) Enabled() bool            { return true }
func (blockingRelayPolicy) AllowsClient(net.IP) bool { return true }
func (blockingRelayPolicy) AllowsHost(string) bool   { return false }

func TestRelayClientRefusesHostOutsideHubCatalogue(t *testing.T) {
	s := NewServer(database.SNIProxySettings{}, "127.0.0.1", "", nil)
	s.SetRelayPolicy(blockingRelayPolicy{}, nil)

	hello := clientHello(t, &tls.Config{ServerName: "not-approved.example", MinVersion: tls.VersionTLS12})
	client, server := net.Pipe()
	defer client.Close()
	go func() {
		_, _ = client.Write(hello)
		_ = client.Close()
	}()

	s.handleConnection(context.Background(), server, true, 443)

	if got := s.refusedRelayHost.Load(); got != 1 {
		t.Fatalf("refusedRelayHost = %d, want 1: a hub relay client must not reach an unapproved host", got)
	}
}

type allowingRelayPolicy struct{}

func (allowingRelayPolicy) Enabled() bool            { return true }
func (allowingRelayPolicy) AllowsClient(net.IP) bool { return false }
func (allowingRelayPolicy) AllowsHost(string) bool   { return true }

func TestNonRelayClientKeepsLocalPath(t *testing.T) {
	// A local client is not in the hub list, so the relay host check must not
	// apply to it. With no access provider and no meter, the connection proceeds
	// to the guard; a malformed numeric host is a convenient terminal decision.
	s := NewServer(database.SNIProxySettings{}, "127.0.0.1", "", nil)
	s.SetRelayPolicy(allowingRelayPolicy{}, nil)

	hello := clientHello(t, &tls.Config{ServerName: "2130706433", MinVersion: tls.VersionTLS12})
	client, server := net.Pipe()
	defer client.Close()
	go func() {
		_, _ = client.Write(hello)
		_ = client.Close()
	}()

	s.handleConnection(context.Background(), server, true, 443)

	if got := s.refusedRelayHost.Load(); got != 0 {
		t.Fatalf("refusedRelayHost = %d, want 0: local clients are not governed by the hub host list", got)
	}
}

func TestDualRoleClientCanUseLocalProxiedHost(t *testing.T) {
	// A client that is both a hub relay client and a local client keeps access
	// to this instance's own proxied hosts. The hub host list stays authoritative
	// for hub-only clients, but coexistence must not break local domains.
	s := NewServer(database.SNIProxySettings{}, "127.0.0.1", "", nil)
	s.SetRelayPolicy(blockingRelayPolicy{}, func(host string) bool {
		return host == "2130706433"
	})

	hello := clientHello(t, &tls.Config{ServerName: "2130706433", MinVersion: tls.VersionTLS12})
	client, server := net.Pipe()
	defer client.Close()
	go func() {
		_, _ = client.Write(hello)
		_ = client.Close()
	}()

	s.handleConnection(context.Background(), server, true, 443)

	if got := s.refusedRelayHost.Load(); got != 0 {
		t.Fatalf("refusedRelayHost = %d, want 0: a dual-role client keeps local proxied hosts", got)
	}
}
