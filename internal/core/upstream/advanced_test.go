package upstream

import (
	"net"
	"path/filepath"
	"testing"
	"time"

	"digitdns/internal/core/resolution"

	"github.com/miekg/dns"
)

func TestWarmTargetsRespectScope(t *testing.T) {
	p := newTestPool(t, []string{
		"1.1.1.1:53",
		"tls://a.example:853",
		"tls://b.example:853",
		"tls://c.example:853",
	}, 2*time.Second, false, "")
	p.SetAdaptiveCurrent("tls://b.example:853")

	if got := p.warmTargetsFor(resolution.KeepWarmOff); len(got) != 0 {
		t.Fatalf("off = %v, want none", got)
	}
	active := p.warmTargetsFor(resolution.KeepWarmActive)
	if len(active) != 1 || active[0] != "tls://b.example:853" {
		t.Fatalf("active = %v, want [b]", active)
	}
	next := p.warmTargetsFor(resolution.KeepWarmActiveNext)
	if len(next) != 2 || next[0] != "tls://b.example:853" || next[1] != "tls://a.example:853" {
		t.Fatalf("active_next = %v, want [b a]", next)
	}
	all := p.warmTargetsFor(resolution.KeepWarmAll)
	if len(all) != 3 {
		t.Fatalf("all = %v, want three encrypted resolvers", all)
	}
}

func TestSetAdvancedPolicyAppliesLive(t *testing.T) {
	p := newTestPool(t, []string{"tls://a.example:853", "tls://b.example:853"}, 2*time.Second, false, "")
	p.SetAdvancedPolicy(AdvancedPolicy{
		KeepWarm:        resolution.KeepWarmOff,
		KeepWarmEvery:   45 * time.Second,
		CircuitCooldown: 90 * time.Second,
		Fallback:        []string{"8.8.8.8", "1.1.1.1:53", "8.8.8.8:53"},
	})
	if p.KeepWarmMode() != resolution.KeepWarmOff {
		t.Fatalf("mode = %q", p.KeepWarmMode())
	}
	if p.KeepWarmInterval() != 45*time.Second {
		t.Fatalf("interval = %s", p.KeepWarmInterval())
	}
	if p.fallback.Load() == nil {
		t.Fatal("fallback pool was not built")
	}
	if got := p.fallback.Load().GetUpstreams(); len(got) != 2 {
		t.Fatalf("fallback pool = %v, want deduplicated 8.8.8.8 and 1.1.1.1", got)
	}
	cooldown := p.circuitOpenFor()
	if cooldown < 90*time.Second || cooldown > 90*time.Second+circuitJitter {
		t.Fatalf("cooldown = %s, want 90s plus jitter", cooldown)
	}

	// Clearing the list removes the fallback pool rather than leaving a stale
	// one that keeps answering after the operator turned it off.
	p.SetAdvancedPolicy(AdvancedPolicy{KeepWarm: resolution.KeepWarmActiveNext})
	if p.fallback.Load() != nil {
		t.Fatal("fallback pool survived an empty list")
	}
	if p.KeepWarmMode() != resolution.KeepWarmActiveNext {
		t.Fatalf("mode = %q", p.KeepWarmMode())
	}
}

// The hedge delay is derived from the active resolver's own median, and the
// aggressive mode halves the head start.
func TestAdaptiveHeadStartFollowsMedianAndMode(t *testing.T) {
	p := newTestPool(t, []string{"tls://a.example:853"}, 2*time.Second, false, "")
	p.SetAdaptiveCurrent("tls://a.example:853")
	st := p.stats["tls://a.example:853"]
	st.mu.Lock()
	for range minLiveLatencySamples {
		st.observeLatency(20 * time.Millisecond)
	}
	st.samples = minLiveLatencySamples
	st.mu.Unlock()

	p.SetAdvancedPolicy(AdvancedPolicy{Hedge: resolution.HedgeBalanced})
	p.hedgeUpdated.Store(0)
	if got := p.adaptiveHeadStartDuration(); got != 40*time.Millisecond {
		t.Fatalf("balanced head start = %s, want 2x the 20 ms median", got)
	}

	p.SetAdvancedPolicy(AdvancedPolicy{Hedge: resolution.HedgeAggressive})
	p.hedgeUpdated.Store(0)
	if got := p.adaptiveHeadStartDuration(); got != 20*time.Millisecond {
		t.Fatalf("aggressive head start = %s, want the 20 ms median", got)
	}
}

// Five consecutive checks of a sustained 2x slowdown move the Adaptive choice
// without waiting for the scheduled benchmark.
func TestMaybeReselectMovesSustainedSlowdown(t *testing.T) {
	p := newTestPool(t, []string{"tls://slow.example:853", "tls://fast.example:853"}, 2*time.Second, false, "")
	setMedian := func(addr string, ms time.Duration) {
		st := p.stats[addr]
		st.mu.Lock()
		for range minLiveLatencySamples {
			st.observeLatency(ms)
		}
		st.samples = minLiveLatencySamples
		st.mu.Unlock()
	}
	setMedian("tls://slow.example:853", 40*time.Millisecond)
	setMedian("tls://fast.example:853", 20*time.Millisecond)
	p.SetAdaptiveCurrent("tls://slow.example:853")

	for range 5 {
		p.maybeReselect("tls://slow.example:853")
	}
	if got := p.AdaptiveCurrent(); got != "tls://fast.example:853" {
		t.Fatalf("Adaptive current = %q, want the healthy resolver after five checks", got)
	}
}

// Last-known latencies survive a restart so the first query is ordered by the
// previous run, not by the configured list order.
func TestLatencySeedsRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "upstream-latency.json")
	servers := []string{"tls://a.example:853", "tls://b.example:853"}

	first := newTestPool(t, servers, 2*time.Second, false, "")
	first.SetLatencySeedPath(path)
	first.stats["tls://a.example:853"].mu.Lock()
	first.stats["tls://a.example:853"].lastProbeLatency = 25 * time.Millisecond
	first.stats["tls://a.example:853"].lastProbeOK = true
	first.stats["tls://a.example:853"].lastProbeAt = time.Now()
	first.stats["tls://a.example:853"].mu.Unlock()
	first.stats["tls://b.example:853"].mu.Lock()
	first.stats["tls://b.example:853"].lastProbeLatency = 45 * time.Millisecond
	first.stats["tls://b.example:853"].lastProbeOK = true
	first.stats["tls://b.example:853"].lastProbeAt = time.Now()
	first.stats["tls://b.example:853"].mu.Unlock()
	first.persistLatencySeeds()

	second := newTestPool(t, servers, 2*time.Second, false, "")
	second.SetLatencySeedPath(path)
	st := second.stats["tls://a.example:853"]
	st.mu.Lock()
	got := st.lastProbeLatency
	ok := st.lastProbeOK
	st.mu.Unlock()
	if !ok || got != 25*time.Millisecond {
		t.Fatalf("restored latency = %s (ok=%v), want 25 ms", got, ok)
	}
	order := second.orderedByLatency(servers)
	if len(order) != 2 || order[0] != "tls://a.example:853" {
		t.Fatalf("seeded order = %v, want the 25 ms resolver first", order)
	}
}

// The fallback pool is a real last resort: a dead primary must hand the query
// to it rather than returning SERVFAIL.
func TestExchangeUsesFallbackPool(t *testing.T) {
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := &dns.Server{
		PacketConn: pc,
		Handler: dns.HandlerFunc(func(w dns.ResponseWriter, r *dns.Msg) {
			m := new(dns.Msg)
			m.SetReply(r)
			if len(r.Question) > 0 {
				if rr, err := dns.NewRR(r.Question[0].Name + " 60 IN A 203.0.113.9"); err == nil {
					m.Answer = append(m.Answer, rr)
				}
			}
			_ = w.WriteMsg(m)
		}),
	}
	go func() { _ = srv.ActivateAndServe() }()
	t.Cleanup(func() { _ = srv.Shutdown() })

	p := newTestPool(t, []string{"127.0.0.1:1"}, 200*time.Millisecond, false, "")
	p.SetAdvancedPolicy(AdvancedPolicy{Fallback: []string{pc.LocalAddr().String()}})
	resp, _, upstream, err := p.Exchange(query("fallback.example."))
	if err != nil {
		t.Fatalf("Exchange: %v", err)
	}
	if got := firstA(resp); got != "203.0.113.9" {
		t.Fatalf("answer = %q from %s, want the fallback", got, upstream)
	}
}
