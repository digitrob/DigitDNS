package dns

import (
	"net"
	"sync/atomic"
	"testing"
	"time"

	"digitdns/internal/core/cache"
	"digitdns/internal/core/matcher"
	"digitdns/internal/core/upstream"

	"github.com/miekg/dns"
)

// Startup warming must populate the cache for the names it resolves, without
// touching the client access path or the query log.
func TestWarmCacheStoresPopularNames(t *testing.T) {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	var hits atomic.Int64
	srv := &dns.Server{
		PacketConn: pc,
		Handler: dns.HandlerFunc(func(w dns.ResponseWriter, r *dns.Msg) {
			hits.Add(1)
			m := new(dns.Msg)
			m.SetReply(r)
			if len(r.Question) > 0 {
				if rr, err := dns.NewRR(r.Question[0].Name + " 60 IN A 203.0.113.7"); err == nil {
					m.Answer = append(m.Answer, rr)
				}
			}
			_ = w.WriteMsg(m)
		}),
	}
	go func() { _ = srv.ActivateAndServe() }()
	t.Cleanup(func() { _ = srv.Shutdown() })

	c := cache.NewCache(1000, 60, 3600)
	defer c.Close()
	pool := upstream.NewUpstreamPool([]string{pc.LocalAddr().String()}, 2*time.Second, false, "")
	h := NewHandler(&dummyAccess{}, c, matcher.NewMatcher(), pool, nil, "198.51.100.1")

	warmed := h.WarmCache([]string{"popular.example", "popular.example.", "not a name!"}, 10)
	if warmed != 1 {
		t.Fatalf("warmed = %d, want 1", warmed)
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("upstream hits = %d, want 1 (duplicates and invalid names skipped)", got)
	}
	q := dns.Question{Name: "popular.example.", Qtype: dns.TypeA, Qclass: dns.ClassINET}
	resp := c.GetWithOptions(q, cache.QueryOptions{EDNS: true})
	if resp == nil {
		t.Fatal("warmed answer is not in the cache")
	}
	if got := firstARecord(resp); got != "203.0.113.7" {
		t.Fatalf("cached answer = %q", got)
	}
}

func TestWarmCacheHonoursLimit(t *testing.T) {
	c := cache.NewCache(100, 60, 3600)
	defer c.Close()
	h := NewHandler(&dummyAccess{}, c, matcher.NewMatcher(), nil, nil, "198.51.100.1")
	// No pool: the function must be a no-op instead of panicking.
	if got := h.WarmCache([]string{"one.example", "two.example"}, 1); got != 0 {
		t.Fatalf("warmed without a pool = %d", got)
	}
}
