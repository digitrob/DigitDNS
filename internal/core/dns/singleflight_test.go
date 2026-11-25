package dns

import (
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"digitdns/internal/core/cache"
	"digitdns/internal/core/matcher"
	"digitdns/internal/core/upstream"

	"github.com/miekg/dns"
)

// Ten devices opening the same cold name must produce one upstream query, and
// every client must get its own transaction ID back.
func TestConcurrentMissesShareOneUpstreamQuery(t *testing.T) {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	var hits atomic.Int64
	srv := &dns.Server{
		PacketConn: pc,
		Handler: dns.HandlerFunc(func(w dns.ResponseWriter, r *dns.Msg) {
			// Sibling prefetch also asks for AAAA/HTTPS; this test is about the
			// identical A misses sharing one lookup.
			if len(r.Question) > 0 && r.Question[0].Qtype == dns.TypeA {
				hits.Add(1)
			}
			time.Sleep(200 * time.Millisecond)
			m := new(dns.Msg)
			m.SetReply(r)
			if len(r.Question) > 0 {
				if rr, err := dns.NewRR(r.Question[0].Name + " 30 IN A 203.0.113.200"); err == nil {
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

	const clients = 10
	start := make(chan struct{})
	var wg sync.WaitGroup
	errs := make(chan string, clients)
	for i := 0; i < clients; i++ {
		wg.Add(1)
		go func(id uint16) {
			defer wg.Done()
			<-start
			req := new(dns.Msg)
			req.SetQuestion("cold-shared.example.", dns.TypeA)
			req.Id = id
			resp := h.ProcessQuery(req, "203.0.113."+strconv.Itoa(int(id)))
			if resp == nil {
				errs <- "nil response"
				return
			}
			if resp.Id != id {
				errs <- "transaction ID was not restored"
			}
			if got := firstARecord(resp); got != "203.0.113.200" {
				errs <- "answer = " + got
			}
		}(uint16(i + 1))
	}
	close(start)
	wg.Wait()
	close(errs)
	for msg := range errs {
		t.Errorf("%s", msg)
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("upstream saw %d queries, want 1", got)
	}
	if got := pool.Metrics().Coalesced; got == 0 {
		t.Fatal("coalesced counter stayed at zero")
	}
}

// A client that arrives just after the leader's answer lands, but before the
// handler has written it to the cache, must reuse the completed lookup instead
// of starting a second upstream query.
func TestCompletedMissIsReusedWhileCacheWarms(t *testing.T) {
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
				if rr, err := dns.NewRR(r.Question[0].Name + " 30 IN A 203.0.113.201"); err == nil {
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

	key := "warm|1|1|false|false|false"
	first := new(dns.Msg)
	first.SetQuestion("warm.example.", dns.TypeA)
	if _, _, err := h.exchangeCoalesced(first, "warm.example", key); err != nil {
		t.Fatalf("leader: %v", err)
	}

	second := new(dns.Msg)
	second.SetQuestion("warm.example.", dns.TypeA)
	second.Id = 4242
	resp, _, err := h.exchangeCoalesced(second, "warm.example", key)
	if err != nil {
		t.Fatalf("follower: %v", err)
	}
	if got := hits.Load(); got != 1 {
		t.Fatalf("upstream hits = %d, want 1 while the cache is still warming", got)
	}
	if resp == nil || resp.Id != 4242 {
		t.Fatalf("follower response id = %v, want its own id", resp)
	}
}

func firstARecord(m *dns.Msg) string {
	if m == nil {
		return ""
	}
	for _, rr := range m.Answer {
		if a, ok := rr.(*dns.A); ok {
			return a.A.String()
		}
	}
	return ""
}
