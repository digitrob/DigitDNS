package service

// Query-latency percentiles.
//
// The mean is the least useful number a resolver can report about itself: a cache
// answering nine queries in ten in forty microseconds pulls the average well
// under a millisecond no matter how badly the tenth behaves, and the tenth is the
// one a game launcher stalls on. Percentiles are what make that tail visible.
//
// This is a fixed-bucket histogram rather than a reservoir of samples, because
// recording a query has to cost one atomic add with no allocation and no lock —
// the whole point of measuring latency is to not become the reason it went up.
// The price is that a reported percentile is only as precise as the bucket it
// lands in, so the edges are dense where the interesting things happen: below a
// millisecond, where a cache hit lives, and through the tens of milliseconds,
// where an upstream round trip does.
//
// Three views are kept:
//
//   - lifetime, cumulative since the daemon started;
//   - 60 one-minute slots, for the 5m/15m/1h range chips;
//   - 288 five-minute slots, for 6h/24h. Five-minute granularity keeps the
//     memory at tens of kilobytes per histogram instead of a full day of
//     minute buckets.
//
// Every range reports the exact peak of its own window and percentiles from the
// same observations, so a p99 can never read above the peak it sits beside.

import (
	"math"
	"sync/atomic"
	"time"
)

// latencyBounds are the inclusive upper edges of each bucket, in milliseconds.
// Anything above the last edge lands in one final overflow bucket.
var latencyBounds = [...]float64{
	0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10, 20, 35, 50, 75, 100, 150, 250, 500, 1000, 2000,
}

// Offered ranges for the resolver-latency card. The keys are the wire values;
// the minutes drive which slot ring is summed.
const (
	LatencyRange5m  = "5m"
	LatencyRange15m = "15m"
	LatencyRange1h  = "1h"
	LatencyRange6h  = "6h"
	LatencyRange24h = "24h"
)

var (
	LatencyRangeChoices = []string{LatencyRange5m, LatencyRange15m, LatencyRange1h, LatencyRange6h, LatencyRange24h}
	latencyRangeMinutes = map[string]int{
		LatencyRange5m:  5,
		LatencyRange15m: 15,
		LatencyRange1h:  60,
		LatencyRange6h:  360,
		LatencyRange24h: 1440,
	}
)

// NormalizeLatencyRange maps an unknown/empty value onto the five-minute
// default. A request with a typo must not turn into an empty card.
func NormalizeLatencyRange(value string) string {
	if _, ok := latencyRangeMinutes[value]; ok {
		return value
	}
	return LatencyRange5m
}

// ValidLatencyRange reports whether value is an offered range.
func ValidLatencyRange(value string) bool {
	_, ok := latencyRangeMinutes[value]
	return ok
}

const (
	latencyMinuteSlots  = 60
	latencyFiveMinSlots = 288 // 24 h, five minutes per slot
)

// latencySlot is one minute (or five-minute) slice of the rolling
// distribution. The token is the slice index; claiming it clears the counters
// the slot held one full rotation ago.
type latencySlot struct {
	token     atomic.Int64
	buckets   [len(latencyBounds) + 1]atomic.Uint64
	count     atomic.Uint64
	sumMicros atomic.Uint64
	maxMicros atomic.Uint64
}

// latencyHistogram counts query latencies. It is safe for concurrent use.
type latencyHistogram struct {
	buckets   [len(latencyBounds) + 1]atomic.Uint64
	count     atomic.Uint64
	sumMicros atomic.Uint64
	maxMicros atomic.Uint64
	minute    [latencyMinuteSlots]latencySlot
	five      [latencyFiveMinSlots]latencySlot
}

// observe records one query's total service time in milliseconds. A negative or
// non-finite reading is dropped rather than folded in: it can only come from a
// clock that went backwards, and it would corrupt every percentile after it.
func (h *latencyHistogram) observe(ms float64) {
	h.observeAt(ms, time.Now())
}

// observeAt is observe with an explicit clock, so the ranges can be tested
// without sleeping for hours.
func (h *latencyHistogram) observeAt(ms float64, now time.Time) {
	if ms < 0 || math.IsNaN(ms) || math.IsInf(ms, 0) {
		return
	}
	idx := len(latencyBounds)
	for i, edge := range latencyBounds {
		if ms <= edge {
			idx = i
			break
		}
	}
	micros := uint64(ms * 1000)

	// Lifetime distribution: cumulative since the daemon started.
	h.buckets[idx].Add(1)
	h.count.Add(1)
	h.sumMicros.Add(micros)
	raiseMax(&h.maxMicros, micros)

	minute := now.Unix() / 60
	addToSlot(&h.minute[slotIndex(minute, latencyMinuteSlots)], minute, idx, micros)
	five := minute / 5
	addToSlot(&h.five[slotIndex(five, latencyFiveMinSlots)], five, idx, micros)
}

func slotIndex(token, slots int64) int {
	return int(((token % slots) + slots) % slots)
}

func addToSlot(slot *latencySlot, token int64, idx int, micros uint64) {
	for {
		old := slot.token.Load()
		if old == token {
			break
		}
		if slot.token.CompareAndSwap(old, token) {
			for i := range slot.buckets {
				slot.buckets[i].Store(0)
			}
			slot.count.Store(0)
			slot.sumMicros.Store(0)
			slot.maxMicros.Store(0)
			break
		}
	}
	slot.buckets[idx].Add(1)
	slot.count.Add(1)
	slot.sumMicros.Add(micros)
	raiseMax(&slot.maxMicros, micros)
}

// raiseMax publishes micros as the new maximum unless a larger value already won.
func raiseMax(target *atomic.Uint64, micros uint64) {
	for {
		cur := target.Load()
		if micros <= cur || target.CompareAndSwap(cur, micros) {
			return
		}
	}
}

// LatencySnapshot is a read-only view of a latency distribution. Every figure is
// in milliseconds. P50/P95/P99 and PeakMs describe the selected range;
// SinceStart* and MaxMs are cumulative since the daemon started.
type LatencySnapshot struct {
	Range    string  `json:"range,omitempty"`
	Count    uint64  `json:"count"`
	AvgMs    float64 `json:"avg_ms"`
	P50Ms    float64 `json:"p50_ms"`
	P95Ms    float64 `json:"p95_ms"`
	P99Ms    float64 `json:"p99_ms"`
	MaxMs    float64 `json:"max_ms"`
	PeakMs   float64 `json:"peak_ms"`
	Peak5mMs float64 `json:"peak_5m_ms,omitempty"`

	// WindowCount and WindowAvgMs describe the selected range. A window with no
	// observations reports no percentiles rather than stale lifetime ones.
	WindowCount uint64  `json:"window_count"`
	WindowAvgMs float64 `json:"window_avg_ms"`

	// SinceStart* are the lifetime figures the tooltips show beside the rolling
	// numbers.
	SinceStartP50Ms float64 `json:"since_start_p50_ms"`
	SinceStartP95Ms float64 `json:"since_start_p95_ms"`
	SinceStartP99Ms float64 `json:"since_start_p99_ms"`
}

// snapshot reads the default five-minute window.
func (h *latencyHistogram) snapshot() LatencySnapshot {
	return h.snapshotRangeAt(time.Now(), LatencyRange5m)
}

// snapshotAt is snapshot with an explicit clock.
func (h *latencyHistogram) snapshotAt(now time.Time) LatencySnapshot {
	return h.snapshotRangeAt(now, LatencyRange5m)
}

// snapshotRangeAt sums the slots inside the requested range. Ranges up to an
// hour use the minute ring; longer ranges use the five-minute ring, which is why
// the 6h/24h figures include the current partial five-minute slice.
func (h *latencyHistogram) snapshotRangeAt(now time.Time, rangeKey string) LatencySnapshot {
	rangeKey = NormalizeLatencyRange(rangeKey)

	// Lifetime first: it supplies the since-start figures and the exact max.
	lifetimeCounts := make([]uint64, len(h.buckets))
	var lifetimeTotal uint64
	for i := range h.buckets {
		lifetimeCounts[i] = h.buckets[i].Load()
		lifetimeTotal += lifetimeCounts[i]
	}
	lifetimeMax := float64(h.maxMicros.Load()) / 1000.0
	out := LatencySnapshot{
		Range: rangeKey,
		Count: lifetimeTotal,
		MaxMs: lifetimeMax,
	}
	if lifetimeTotal > 0 {
		out.AvgMs = float64(h.sumMicros.Load()) / 1000.0 / float64(lifetimeTotal)
		out.SinceStartP50Ms = min(percentileFrom(lifetimeCounts, lifetimeTotal, 0.50), lifetimeMax)
		out.SinceStartP95Ms = min(percentileFrom(lifetimeCounts, lifetimeTotal, 0.95), lifetimeMax)
		out.SinceStartP99Ms = min(percentileFrom(lifetimeCounts, lifetimeTotal, 0.99), lifetimeMax)
	}

	windowCounts := make([]uint64, len(latencyBounds)+1)
	var windowTotal, windowSum, windowMaxMicros uint64
	minutes := latencyRangeMinutes[rangeKey]
	nowMinute := now.Unix() / 60
	addSlot := func(slot *latencySlot) {
		for j := range slot.buckets {
			windowCounts[j] += slot.buckets[j].Load()
		}
		windowTotal += slot.count.Load()
		windowSum += slot.sumMicros.Load()
		if value := slot.maxMicros.Load(); value > windowMaxMicros {
			windowMaxMicros = value
		}
	}
	if minutes <= 60 {
		for i := range h.minute {
			slot := &h.minute[i]
			minute := slot.token.Load()
			if minute == 0 || minute > nowMinute || nowMinute-minute >= int64(minutes) {
				continue
			}
			addSlot(slot)
		}
	} else {
		nowFive := nowMinute / 5
		slots := int64(minutes / 5)
		for i := range h.five {
			slot := &h.five[i]
			five := slot.token.Load()
			if five == 0 || five > nowFive || nowFive-five >= slots {
				continue
			}
			addSlot(slot)
		}
	}

	out.WindowCount = windowTotal
	out.PeakMs = float64(windowMaxMicros) / 1000.0
	if rangeKey == LatencyRange5m {
		out.Peak5mMs = out.PeakMs
	}
	if windowTotal == 0 {
		return out
	}
	out.WindowAvgMs = float64(windowSum) / 1000.0 / float64(windowTotal)
	// Cap the estimates at the exact maximum. Interpolation inside a bucket can
	// otherwise report a p99 above the worst real reading, which reads as a
	// contradiction on a card that shows both.
	peak := out.PeakMs
	out.P50Ms = min(percentileFrom(windowCounts, windowTotal, 0.50), peak)
	out.P95Ms = min(percentileFrom(windowCounts, windowTotal, 0.95), peak)
	out.P99Ms = min(percentileFrom(windowCounts, windowTotal, 0.99), peak)
	return out
}

// percentileFrom reads a quantile out of bucket counts, interpolating inside the
// bucket the quantile falls in on the assumption that its observations are spread
// evenly across it. That assumption is why the edges are placed where they are.
//
// The overflow bucket has no upper edge, so a quantile that lands in it is
// reported as the last edge — "at least this" rather than a number invented from
// nothing.
func percentileFrom(counts []uint64, total uint64, q float64) float64 {
	last := latencyBounds[len(latencyBounds)-1]
	if total == 0 {
		return 0
	}
	target := q * float64(total)
	cumulative := 0.0
	for i, c := range counts {
		below := cumulative
		cumulative += float64(c)
		if cumulative < target {
			continue
		}
		if i >= len(latencyBounds) {
			return last
		}
		upper := latencyBounds[i]
		if c == 0 {
			return upper
		}
		lower := 0.0
		if i > 0 {
			lower = latencyBounds[i-1]
		}
		frac := (target - below) / float64(c)
		return lower + (upper-lower)*min(max(frac, 0), 1)
	}
	return last
}
