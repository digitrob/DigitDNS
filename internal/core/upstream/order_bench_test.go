package upstream

import (
	"testing"
	"time"

	"digitdns/internal/core/resolution"
)

// The rebuild used to call GetUpstreamStats(), so a once-a-second cache miss on
// a query path could allocate and sort a full dashboard snapshot. This pins the
// cheaper ranking to the same number the card shows.
func TestOrderedByLatencyMatchesCard(t *testing.T) {
	const (
		slow = "192.0.2.1:53"
		fast = "192.0.2.2:53"
		mid  = "192.0.2.3:53"
	)
	addrs := []string{slow, fast, mid}
	p := newTestPool(t, addrs, time.Second, false, "")

	for i := 0; i < minLiveLatencySamples; i++ {
		p.recordStat(slow, 60*time.Millisecond, statSuccess, "", false)
		p.recordStat(fast, 5*time.Millisecond, statSuccess, "", false)
		p.recordStat(mid, 20*time.Millisecond, statSuccess, "", false)
	}
	p.orderView.Store(nil)
	got := p.orderedByLatency(addrs)
	want := []string{fast, mid, slow}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v", got, want)
		}
	}

	card := p.GetUpstreamStats()
	for i, s := range card {
		if got[i] != s.Address {
			t.Fatalf("rank %d = %s, card = %s", i, got[i], s.Address)
		}
	}
}

// Adaptive ranks on the probe window. The order cache must follow the same
// rule as the card, not fall back to live p50 behind the operator's back.
func TestOrderedByLatencyUsesProbeWindowForAdaptive(t *testing.T) {
	const (
		slow = "192.0.2.1:53"
		fast = "192.0.2.2:53"
	)
	addrs := []string{slow, fast}
	p := newTestPool(t, addrs, time.Second, false, "")
	setMeasuredLatency(t, p, slow, 5*time.Millisecond)
	setMeasuredLatency(t, p, fast, 10*time.Millisecond)
	setProbeLatency(t, p, slow, 80*time.Millisecond)
	setProbeLatency(t, p, fast, 8*time.Millisecond)
	p.SetPolicy(Policy{DefaultStrategy: resolution.StrategyAdaptive})

	p.orderView.Store(nil)
	got := p.orderedByLatency(addrs)
	if got[0] != fast || got[1] != slow {
		t.Fatalf("adaptive order = %v, want [%s %s]", got, fast, slow)
	}
}

// BenchmarkOrderedByLatencyRebuild is the cost of the first query after the
// one-second order cache expires. Before the focused fix this also built a full
// []UpstreamStat dashboard snapshot on the query goroutine.
func BenchmarkOrderedByLatencyRebuild(b *testing.B) {
	p := NewUpstreamPool([]string{"192.0.2.1:53", "192.0.2.2:53", "192.0.2.3:53"}, time.Second, false, "")
	addrs := p.GetUpstreams()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p.orderView.Store(nil)
		p.orderedByLatency(addrs)
	}
}
