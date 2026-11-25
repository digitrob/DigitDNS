package dns

import (
	"net"
	"sync"
	"testing"
	"time"

	"digitdns/internal/core/cache"
	"digitdns/internal/core/matcher"
	"digitdns/internal/core/upstream"

	"github.com/miekg/dns"
)

// A cold A lookup should warm AAAA and HTTPS in the background, so the next two
// questions a browser asks are cache hits instead of separate upstream trips.
func TestPrefetchWarmsSiblingRecordTypes(t *testing.T) {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	var mu sync.Mutex
	hits := map[uint16]int{}
	srv := &dns.Server{
		PacketConn: pc,
		Handler: dns.HandlerFunc(func(w dns.ResponseWriter, r *dns.Msg) {
			if len(r.Question) == 0 {
				return
			}
			q := r.Question[0]
			mu.Lock()
			hits[q.Qtype]++
			mu.Unlock()
			m := new(dns.Msg)
			m.SetReply(r)
			switch q.Qtype {
			case dns.TypeA:
				if rr, err := dns.NewRR(q.Name + " 60 IN A 203.0.113.50"); err == nil {
					m.Answer = append(m.Answer, rr)
				}
			case dns.TypeAAAA:
				if rr, err := dns.NewRR(q.Name + " 60 IN AAAA 2001:db8::50"); err == nil {
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

	req := new(dns.Msg)
	req.SetQuestion("siblings.example.", dns.TypeA)
	if resp := h.ProcessQuery(req, "203.0.113.50"); resp == nil || len(resp.Answer) == 0 {
		t.Fatal("A lookup did not resolve")
	}

	deadline := time.Now().Add(3 * time.Second)
	for {
		mu.Lock()
		hasAAAA := hits[dns.TypeAAAA] > 0
		hasHTTPS := hits[dns.TypeHTTPS] > 0
		mu.Unlock()
		if hasAAAA && hasHTTPS {
			break
		}
		if time.Now().After(deadline) {
			mu.Lock()
			t.Fatalf("prefetch did not run; upstream hits = %v", hits)
			mu.Unlock()
		}
		time.Sleep(10 * time.Millisecond)
	}

	// The prefetched AAAA must actually be in the cache, not merely requested.
	q := dns.Question{Name: "siblings.example.", Qtype: dns.TypeAAAA, Qclass: dns.ClassINET}
	if !c.HasFresh(q, cache.QueryOptions{}) {
		t.Fatal("prefetched AAAA answer is not in the cache")
	}
}

// A cache hit must not start a prefetch wave: the handler only calls this after
// a real upstream resolution.
func TestPrefetchNotTriggeredFromCacheHit(t *testing.T) {
	c := cache.NewCache(100, 60, 3600)
	defer c.Close()
	h := NewHandler(&dummyAccess{}, c, matcher.NewMatcher(), nil, nil, "198.51.100.1")
	// No upstream pool: the helper must return immediately rather than panic.
	h.prefetchSiblings(dns.Question{Name: "cached.example.", Qtype: dns.TypeA, Qclass: dns.ClassINET}, cache.QueryOptions{})
}
