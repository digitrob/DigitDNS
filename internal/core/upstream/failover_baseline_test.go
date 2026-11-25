package upstream

// Baseline measurements for the DNS speed plan.
//
// These tests do not assert a speed target yet: they record what the current
// implementation does so the failover work has a before/after number. Stage 2
// turns the logged figures into assertions (a hanging first resolver must not
// make the client wait the whole budget, and a slow first resolver must not
// delay an answer that another resolver can give immediately).

import (
	"fmt"
	"testing"
	"time"

	"digitdns/internal/core/resolution"

	"github.com/miekg/dns"
)

// adaptiveWithCurrent installs the production default strategy with the given
// resolver selected as the persisted Adaptive choice.
func adaptiveWithCurrent(t *testing.T, p *UpstreamPool, current string) {
	t.Helper()
	p.SetPolicy(Policy{
		DefaultStrategy: resolution.StrategyAdaptive,
		AdaptiveCurrent: current,
	})
}

// A silent first resolver is the worst case: the client never gets an answer
// from it, so the shared budget is the only thing that ends the wait.
func TestBaselineAdaptiveFirstResolverHangs(t *testing.T) {
	hang := newTestResolver(t, always(answer{silent: true}), nil)
	good := newTestResolver(t, always(answer{rcode: dns.RcodeSuccess, ip: "203.0.113.210"}), nil)

	p := newTestPool(t, []string{hang.addr, good.addr}, 2*time.Second, false, "")
	adaptiveWithCurrent(t, p, hang.addr)

	start := time.Now()
	resp, _, addr, err := p.ExchangeFor(query("baseline-hang.example."), "")
	elapsed := time.Since(start)

	t.Logf("hanging first resolver: elapsed=%s answered_by=%q err=%v answer=%q",
		elapsed, addr, err, firstA(resp))
	t.Logf("healthy second resolver received %d queries", good.udpHits.Load())
}

// Stage 3 acceptance: a slow first resolver is hedged after the head start, so
// the client gets the fast resolver's answer instead of waiting for the slow one.
func TestAdaptiveHedgesSlowFirstResolver(t *testing.T) {
	slow := newTestResolver(t, always(answer{
		rcode: dns.RcodeSuccess,
		ip:    "203.0.113.211",
		delay: 300 * time.Millisecond,
	}), nil)
	fast := newTestResolver(t, always(answer{rcode: dns.RcodeSuccess, ip: "203.0.113.212"}), nil)

	p := newTestPool(t, []string{slow.addr, fast.addr}, 2*time.Second, false, "")
	adaptiveWithCurrent(t, p, slow.addr)

	start := time.Now()
	resp, _, addr, err := p.ExchangeFor(query("stage3-slow.example."), "")
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("hedged query: %v", err)
	}
	if addr != fast.addr {
		t.Fatalf("answered by %q, want the fast resolver %q", addr, fast.addr)
	}
	if elapsed >= 250*time.Millisecond {
		t.Fatalf("hedged query took %s; the slow resolver was not hedged", elapsed)
	}
	if fast.udpHits.Load() == 0 {
		t.Fatal("fast resolver received no hedge query")
	}
	if firstA(resp) != "203.0.113.212" {
		t.Fatalf("answer = %q, want 203.0.113.212", firstA(resp))
	}
	t.Logf("slow first resolver hedged: elapsed=%s answered_by=%q", elapsed, addr)

	// The slow resolver lost the hedge; selection must not send Force back to it.
	d := p.SelectAdaptiveFastestForced(slow.addr)
	if d.Current != fast.addr || d.Fastest != fast.addr {
		t.Fatalf("forced selection chose the hedged-out resolver: %+v", d)
	}
}

// A healthy first resolver answers before the head start, so hedging must not
// send a second query.
func TestAdaptiveHealthyFirstDoesNotHedge(t *testing.T) {
	first := newTestResolver(t, always(answer{rcode: dns.RcodeSuccess, ip: "203.0.113.213"}), nil)
	second := newTestResolver(t, always(answer{rcode: dns.RcodeSuccess, ip: "203.0.113.214"}), nil)

	p := newTestPool(t, []string{first.addr, second.addr}, 2*time.Second, false, "")
	adaptiveWithCurrent(t, p, first.addr)

	start := time.Now()
	_, _, addr, err := p.ExchangeFor(query("stage3-fast.example."), "")
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if addr != first.addr {
		t.Fatalf("answered by %q, want the first resolver %q", addr, first.addr)
	}
	if elapsed >= 50*time.Millisecond {
		t.Fatalf("healthy first resolver took %s", elapsed)
	}
	if second.udpHits.Load() != 0 {
		t.Fatalf("second resolver received %d hedge queries for a fast first answer", second.udpHits.Load())
	}
}

// Stage 2 acceptance: a hanging first resolver must not consume the whole
// budget, and the healthy resolver behind it must answer.
func TestAdaptiveHangingFirstResolverFailsOverFast(t *testing.T) {
	hang := newTestResolver(t, always(answer{silent: true}), nil)
	good := newTestResolver(t, always(answer{rcode: dns.RcodeSuccess, ip: "203.0.113.230"}), nil)

	p := newTestPool(t, []string{hang.addr, good.addr}, 2*time.Second, false, "")
	adaptiveWithCurrent(t, p, hang.addr)

	start := time.Now()
	resp, _, addr, err := p.ExchangeFor(query("stage2-failover.example."), "")
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("healthy second resolver was not used: %v", err)
	}
	if addr != good.addr {
		t.Fatalf("answered by %q, want the healthy resolver %q", addr, good.addr)
	}
	if firstA(resp) != "203.0.113.230" {
		t.Fatalf("answer = %q, want 203.0.113.230", firstA(resp))
	}
	if elapsed >= 500*time.Millisecond {
		t.Fatalf("failover took %s; the hanging resolver was not hedged", elapsed)
	}
	if good.udpHits.Load() == 0 {
		t.Fatal("healthy resolver received no query")
	}
	t.Logf("hanging first resolver hedged: elapsed=%s answered_by=%q", elapsed, addr)

	d := p.SelectAdaptiveFastestForced(hang.addr)
	if d.Current != good.addr || d.Fastest != good.addr {
		t.Fatalf("forced selection chose the hedged-out resolver: %+v", d)
	}
}

// Stage 2 acceptance: after a resolver times out it is moved behind the healthy
// ones, so the very next query does not pay the same wait again.
func TestAdaptiveDemotesResolverAfterTimeout(t *testing.T) {
	hang := newTestResolver(t, always(answer{silent: true}), nil)
	good := newTestResolver(t, always(answer{rcode: dns.RcodeSuccess, ip: "203.0.113.231"}), nil)

	// Best Known exercises the ordered fallback walk without the Adaptive
	// hedge, so the first resolver's timeout is what records the failure.
	p := newTestPool(t, []string{hang.addr, good.addr}, 2*time.Second, false, "")
	p.SetPolicy(Policy{DefaultStrategy: resolution.StrategyBestKnown})

	if _, _, _, err := p.ExchangeFor(query("stage2-demote-first.example."), ""); err != nil {
		t.Fatalf("first query: %v", err)
	}
	before := hang.udpHits.Load()

	start := time.Now()
	resp, _, addr, err := p.ExchangeFor(query("stage2-demote-second.example."), "")
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("second query: %v", err)
	}
	if addr != good.addr {
		t.Fatalf("second query answered by %q, want the healthy resolver %q", addr, good.addr)
	}
	if elapsed >= 300*time.Millisecond {
		t.Fatalf("second query took %s; the failed resolver was not demoted", elapsed)
	}
	if hang.udpHits.Load() != before {
		t.Fatalf("failed resolver was queried again (%d -> %d)", before, hang.udpHits.Load())
	}
	if firstA(resp) != "203.0.113.231" {
		t.Fatalf("answer = %q, want 203.0.113.231", firstA(resp))
	}
}

// Selection must agree with the query path: a resolver that just timed out is
// not a candidate for the next Force switch until it answers again.
func TestAdaptiveForcedSkipsResolverInProbation(t *testing.T) {
	// A refused port fails immediately, so the failure is recorded even though
	// the Adaptive hedge answers from the healthy resolver right behind it.
	refused := "127.0.0.1:1"
	good := newTestResolver(t, always(answer{rcode: dns.RcodeSuccess, ip: "203.0.113.232"}), nil)

	p := newTestPool(t, []string{refused, good.addr}, 2*time.Second, false, "")
	adaptiveWithCurrent(t, p, refused)

	if _, _, addr, err := p.ExchangeFor(query("stage2-probation.example."), ""); err != nil {
		t.Fatalf("first query: %v", err)
	} else if addr != good.addr {
		t.Fatalf("first query answered by %q, want %q", addr, good.addr)
	}

	d := p.SelectAdaptiveFastestForced(refused)
	if d.Current != good.addr || d.Fastest != good.addr {
		t.Fatalf("forced selection chose the resolver in probation: %+v", d)
	}
}

// All-healthy baseline: the normal cost of a cache miss is one round trip.
func BenchmarkBaselineAdaptiveHealthyMiss(b *testing.B) {
	first := newTestResolver(b, always(answer{rcode: dns.RcodeSuccess, ip: "203.0.113.220"}), nil)
	second := newTestResolver(b, always(answer{rcode: dns.RcodeSuccess, ip: "203.0.113.221"}), nil)
	p := NewUpstreamPool([]string{first.addr, second.addr}, 2*time.Second, false, "")
	p.SetPolicy(Policy{DefaultStrategy: resolution.StrategyAdaptive, AdaptiveCurrent: first.addr})

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, _, err := p.ExchangeFor(query(fmt.Sprintf("bench-healthy-%d.example.", i)), ""); err != nil {
			b.Fatalf("healthy miss %d: %v", i, err)
		}
	}
}

// Hanging-first baseline is intentionally one iteration: at the default budget
// a single query costs the full timeout.
func BenchmarkBaselineAdaptiveHangingFirst(b *testing.B) {
	hang := newTestResolver(b, always(answer{silent: true}), nil)
	good := newTestResolver(b, always(answer{rcode: dns.RcodeSuccess, ip: "203.0.113.222"}), nil)
	p := NewUpstreamPool([]string{hang.addr, good.addr}, 2*time.Second, false, "")
	p.SetPolicy(Policy{DefaultStrategy: resolution.StrategyAdaptive, AdaptiveCurrent: hang.addr})

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _, _ = p.ExchangeFor(query(fmt.Sprintf("bench-hang-%d.example.", i)), "")
	}
}
