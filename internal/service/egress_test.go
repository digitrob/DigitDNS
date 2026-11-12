package service

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"io"
	"net"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"digitdns/internal/crypto"
	"digitdns/internal/database"
)

// startFakeSOCKS5 accepts one connection and records the CONNECT request.
func startFakeSOCKS5(t *testing.T, requireAuth bool) (string, <-chan string) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	requests := make(chan string, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		head := make([]byte, 2)
		if _, err := io.ReadFull(conn, head); err != nil {
			return
		}
		methods := make([]byte, int(head[1]))
		if _, err := io.ReadFull(conn, methods); err != nil {
			return
		}
		if requireAuth {
			_, _ = conn.Write([]byte{0x05, 0x02})
			authHead := make([]byte, 2)
			if _, err := io.ReadFull(conn, authHead); err != nil {
				return
			}
			user := make([]byte, int(authHead[1]))
			if _, err := io.ReadFull(conn, user); err != nil {
				return
			}
			plen := make([]byte, 1)
			if _, err := io.ReadFull(conn, plen); err != nil {
				return
			}
			pass := make([]byte, int(plen[0]))
			if _, err := io.ReadFull(conn, pass); err != nil {
				return
			}
			_, _ = conn.Write([]byte{0x01, 0x00})
		} else {
			_, _ = conn.Write([]byte{0x05, 0x00})
		}
		reqHead := make([]byte, 4)
		if _, err := io.ReadFull(conn, reqHead); err != nil {
			return
		}
		host := ""
		switch reqHead[3] {
		case 0x01:
			raw := make([]byte, 4)
			_, _ = io.ReadFull(conn, raw)
			host = net.IP(raw).String()
		case 0x03:
			length := make([]byte, 1)
			_, _ = io.ReadFull(conn, length)
			raw := make([]byte, int(length[0]))
			_, _ = io.ReadFull(conn, raw)
			host = string(raw)
		case 0x04:
			raw := make([]byte, 16)
			_, _ = io.ReadFull(conn, raw)
			host = net.IP(raw).String()
		}
		portRaw := make([]byte, 2)
		_, _ = io.ReadFull(conn, portRaw)
		port := binary.BigEndian.Uint16(portRaw)
		requests <- net.JoinHostPort(host, strconv.Itoa(int(port)))
		_, _ = conn.Write([]byte{0x05, 0x00, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		// Keep the connection open long enough for the client to return it.
		time.Sleep(50 * time.Millisecond)
	}()
	return ln.Addr().String(), requests
}

func TestSOCKS5SendsHostnameByDefault(t *testing.T) {
	address, requests := startFakeSOCKS5(t, false)
	cfg := egressConfig{Address: address, Resolve: "remote"}
	conn, err := dialSOCKS5(context.Background(), cfg, "example.com", nil, 443)
	if err != nil {
		t.Fatalf("dialSOCKS5: %v", err)
	}
	_ = conn.Close()
	select {
	case got := <-requests:
		if got != "example.com:443" {
			t.Fatalf("proxy saw %q, want the hostname form", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("proxy never received a CONNECT request")
	}
}

func TestSOCKS5LocalResolveSendsVettedAddress(t *testing.T) {
	address, requests := startFakeSOCKS5(t, false)
	cfg := egressConfig{Address: address, Resolve: "local"}
	addr := net.ParseIP("203.0.113.10")
	conn, err := dialSOCKS5(context.Background(), cfg, "example.com", []net.IP{addr}, 8443)
	if err != nil {
		t.Fatalf("dialSOCKS5: %v", err)
	}
	_ = conn.Close()
	select {
	case got := <-requests:
		if got != "203.0.113.10:8443" {
			t.Fatalf("proxy saw %q, want the vetted address", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("proxy never received a CONNECT request")
	}
}

func TestSOCKS5UsernamePasswordHandshake(t *testing.T) {
	address, _ := startFakeSOCKS5(t, true)
	cfg := egressConfig{Address: address, Resolve: "remote", Username: "alice", Password: "hunter2"}
	conn, err := dialSOCKS5(context.Background(), cfg, "example.com", nil, 443)
	if err != nil {
		t.Fatalf("dialSOCKS5 with auth: %v", err)
	}
	_ = conn.Close()
}

func wireGuardTestKey(fill byte) string {
	raw := make([]byte, 32)
	for i := range raw {
		raw[i] = fill
	}
	return base64.StdEncoding.EncodeToString(raw)
}

func TestParseWireGuardConf(t *testing.T) {
	raw := `# a comment
[Interface]
PrivateKey = ` + wireGuardTestKey(1) + `
Address = 10.9.0.2/32, fd00::2/128
DNS = 10.9.0.1
MTU = 1380

[Peer]
PublicKey = ` + wireGuardTestKey(2) + `
PresharedKey = ` + wireGuardTestKey(3) + `
Endpoint = vpn.example.com:51820
AllowedIPs = 0.0.0.0/0, ::/0
PersistentKeepalive = 25
`
	conf, err := parseWireGuardConf(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if conf.mtu != 1380 || len(conf.addresses) != 2 || len(conf.peers) != 1 {
		t.Fatalf("parsed = %+v", conf)
	}
	if conf.peers[0].endpoint != "vpn.example.com:51820" || len(conf.peers[0].allowedIPs) != 2 {
		t.Fatalf("peer = %+v", conf.peers[0])
	}
	if _, err := parseWireGuardConf("[Interface]\nAddress = 10.0.0.1/32\n"); err == nil {
		t.Fatal("a config without a private key must be rejected")
	}
	if _, err := parseWireGuardConf("[Interface]\nPrivateKey = " + wireGuardTestKey(1) + "\n"); err == nil {
		t.Fatal("a config without an address must be rejected")
	}
}

func egressTestDB(t *testing.T) *database.DB {
	t.Helper()
	dir := t.TempDir()
	cipher, err := crypto.LoadOrGenerateMasterKey(filepath.Join(dir, "e.key"))
	if err != nil {
		t.Fatal(err)
	}
	db, err := database.Open(filepath.Join(dir, "e.db"), cipher)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestRelayRouterEgressFailClosed(t *testing.T) {
	db := egressTestDB(t)
	closed := database.Egress{Kind: database.EgressKindSOCKS5, Name: "closed", Address: "10.0.0.1:1080", Enabled: true, FailClosed: true}
	open := database.Egress{Kind: database.EgressKindSOCKS5, Name: "open", Address: "10.0.0.2:1080", Enabled: true, FailClosed: false}
	if err := db.SaveEgress(&closed); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveEgress(&open); err != nil {
		t.Fatal(err)
	}

	registry := NewEgressRegistry(db)
	if err := registry.Reload(); err != nil {
		t.Fatal(err)
	}
	registry.items[closed.ID].health.Store("offline")
	registry.items[open.ID].health.Store("offline")

	router := NewRelayRouter()
	router.SetEgressRegistry(registry)
	router.SetAgents(nil)
	router.SetAssignments(map[string]string{
		"closed.example": "egress:" + closed.ID,
		"open.example":   "egress:" + open.ID,
	}, nil, nil)

	target, _ := router.LookupFor("closed.example", "")
	if target.Kind != "egress" || !target.FailClosed || target.Healthy {
		t.Fatalf("closed target = %+v, want an unhealthy fail-closed egress", target)
	}
	if target.FallbackReason == "" {
		t.Fatal("a fail-closed refusal must carry a fallback reason")
	}
	target, _ = router.LookupFor("open.example", "")
	if target.Kind != "local" || target.FallbackReason == "" {
		t.Fatalf("open target = %+v, want a local fallback with a reason", target)
	}

	registry.items[closed.ID].health.Store("online")
	target, _ = router.LookupFor("closed.example", "")
	if target.Kind != "egress" || !target.Healthy {
		t.Fatalf("online target = %+v, want a healthy egress", target)
	}

	// Disabling a fail-closed egress must not become a silent local fallback.
	disabled := closed
	disabled.Enabled = false
	if err := db.SaveEgress(&disabled); err != nil {
		t.Fatal(err)
	}
	if err := registry.Reload(); err != nil {
		t.Fatal(err)
	}
	target, _ = router.LookupFor("closed.example", "")
	if target.Kind != "egress" || target.Healthy || !target.FailClosed {
		t.Fatalf("disabled target = %+v, want a fail-closed refusal", target)
	}
}
