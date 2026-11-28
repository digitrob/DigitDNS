package netresolve

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/miekg/dns"
)

func TestParseServers(t *testing.T) {
	text := `
# a comment
; another comment
nameserver 1.1.1.1
nameserver 9.9.9.9
nameserver 1.1.1.1
search example.com
options timeout:2
nameserver fd7a:115c:a1e0::53
nameserver
`
	got := parseServers(text)
	want := []string{"1.1.1.1", "9.9.9.9", "fd7a:115c:a1e0::53"}
	if len(got) != len(want) {
		t.Fatalf("parseServers = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("parseServers[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestNormalizeServers(t *testing.T) {
	got := normalizeServers([]string{" 9.9.9.9 ", "9.9.9.9:53", "1.1.1.1:5353", "fd7a:115c:a1e0::53", ""})
	want := []string{"9.9.9.9:53", "1.1.1.1:5353", "[fd7a:115c:a1e0::53]:53"}
	if len(got) != len(want) {
		t.Fatalf("normalizeServers = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("normalizeServers[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestNewOrDefault(t *testing.T) {
	// A configured list is used as given.
	pinned := NewOrDefault([]string{"192.0.2.53"})
	if got := pinned.Servers(); len(got) != 1 || got[0] != "192.0.2.53:53" {
		t.Fatalf("pinned Servers = %v", got)
	}

	// An empty setting means "race the host's nameservers", not "fall back to
	// the sequential system resolver". This is what the relay and agent get
	// when the operator has not pinned a list.
	dir := t.TempDir()
	path := filepath.Join(dir, "resolv.conf")
	if err := os.WriteFile(path, []byte("nameserver 198.51.100.53\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	old := resolvConfPath
	resolvConfPath = path
	t.Cleanup(func() { resolvConfPath = old })

	system := NewOrDefault(nil)
	if got := system.Servers(); len(got) != 1 || got[0] != "198.51.100.53:53" {
		t.Fatalf("default Servers = %v, want the resolv.conf entry", got)
	}
}

func TestSystemServersFromFixture(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "resolv.conf")
	if err := os.WriteFile(path, []byte("nameserver 192.0.2.53\nnameserver 192.0.2.54\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	old := resolvConfPath
	resolvConfPath = path
	t.Cleanup(func() { resolvConfPath = old })

	servers, err := SystemServers()
	if err != nil {
		t.Fatal(err)
	}
	if len(servers) != 2 || servers[0] != "192.0.2.53" || servers[1] != "192.0.2.54" {
		t.Fatalf("SystemServers = %v", servers)
	}
}

// silentServer binds a UDP socket and never answers. Its address is a
// nameserver that accepts a query and then says nothing, which is exactly the
// failure mode that used to refuse a relay under the two-second budget.
func silentServer(t *testing.T) string {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pc.Close() })
	return pc.LocalAddr().String()
}

// answerServer runs a tiny DNS server. answer == nil makes it return SERVFAIL.
func answerServer(t *testing.T, answer net.IP) string {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &dns.Server{
		PacketConn: pc,
		Handler: dns.HandlerFunc(func(w dns.ResponseWriter, req *dns.Msg) {
			reply := new(dns.Msg)
			reply.SetReply(req)
			if answer == nil {
				reply.SetRcode(req, dns.RcodeServerFailure)
				_ = w.WriteMsg(reply)
				return
			}
			if len(req.Question) > 0 && req.Question[0].Qtype == dns.TypeA {
				reply.Answer = append(reply.Answer, &dns.A{
					Hdr: dns.RR_Header{
						Name:   req.Question[0].Name,
						Rrtype: dns.TypeA,
						Class:  dns.ClassINET,
						Ttl:    60,
					},
					A: answer.To4(),
				})
			}
			_ = w.WriteMsg(reply)
		}),
	}
	go func() { _ = srv.ActivateAndServe() }()
	t.Cleanup(func() { _ = srv.Shutdown() })
	return pc.LocalAddr().String()
}

func TestLookupRacesSilentServer(t *testing.T) {
	slow := silentServer(t)
	fast := answerServer(t, net.ParseIP("192.0.2.10"))

	r := New([]string{slow, fast})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	start := time.Now()
	addrs, err := r.LookupNetIP(ctx, "relay.example.")
	if err != nil {
		t.Fatalf("LookupNetIP: %v", err)
	}
	if len(addrs) == 0 || addrs[0].String() != "192.0.2.10" {
		t.Fatalf("addrs = %v, want 192.0.2.10", addrs)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("lookup took %v; the silent server stalled the race", elapsed)
	}
}

func TestLookupSkipsServfailServer(t *testing.T) {
	bad := answerServer(t, nil)
	good := answerServer(t, net.ParseIP("192.0.2.11"))

	r := New([]string{bad, good})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	addrs, err := r.LookupNetIP(ctx, "relay.example.")
	if err != nil {
		t.Fatalf("LookupNetIP: %v", err)
	}
	if len(addrs) == 0 || addrs[0].String() != "192.0.2.11" {
		t.Fatalf("addrs = %v, want 192.0.2.11", addrs)
	}
}

func TestLookupAllSilentRespectsBudget(t *testing.T) {
	r := New([]string{silentServer(t), silentServer(t)})
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	start := time.Now()
	if _, err := r.LookupNetIP(ctx, "relay.example."); err == nil {
		t.Fatal("expected a timeout when every server is silent")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("lookup took %v; it ignored the context budget", elapsed)
	}
}

func TestLookupFallsBackWithoutServers(t *testing.T) {
	// localhost is answered from the hosts file, so this exercises the fallback
	// to net.DefaultResolver without needing a live network.
	r := New(nil)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	addrs, err := r.LookupNetIP(ctx, "localhost")
	if err != nil {
		t.Fatalf("LookupNetIP(localhost): %v", err)
	}
	if len(addrs) == 0 {
		t.Fatal("no addresses for localhost")
	}
}
