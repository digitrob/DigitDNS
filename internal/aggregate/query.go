package aggregate

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

var (
	ErrInvalidRange     = errors.New("range must be 1h, 24h, 7d, 30d or 90d")
	ErrInvalidInterval  = errors.New("interval is not valid for this range")
	ErrInvalidDimension = errors.New("unknown top dimension")
	ErrInvalidLimit     = errors.New("limit must be between 1 and 100")
)

const (
	Range1h  = "1h"
	Range24h = "24h"
	Range7d  = "7d"
	Range30d = "30d"
	Range90d = "90d"

	// Interval5m is the 1h range's five-minute bucket size.
	Interval5m = "5m"
)

type Summary struct {
	Range string    `json:"range"`
	From  time.Time `json:"from"`
	To    time.Time `json:"to"`

	Queries uint64 `json:"queries"`
	Blocked uint64 `json:"blocked"`
	Proxied uint64 `json:"proxied"`
	Cached  uint64 `json:"cached"`
	Failed  uint64 `json:"failed"`

	BlockedPct  float64 `json:"blocked_pct"`
	ProxiedPct  float64 `json:"proxied_pct"`
	CacheHitPct float64 `json:"cache_hit_pct"`

	ActiveClients       int     `json:"active_clients"`
	ActiveClientsApprox bool    `json:"active_clients_approx"`
	LatencyAvgMs        float64 `json:"latency_avg_ms"`
	Buckets             int     `json:"buckets"`
}

type SeriesPoint struct {
	Start     time.Time `json:"start"`
	Queries   uint64    `json:"queries"`
	Blocked   uint64    `json:"blocked"`
	Proxied   uint64    `json:"proxied"`
	Cached    uint64    `json:"cached"`
	Failed    uint64    `json:"failed"`
	LatencyMs float64   `json:"latency_ms"`
}

type Series struct {
	Range    string        `json:"range"`
	Interval string        `json:"interval"`
	Points   []SeriesPoint `json:"points"`
}

type TopItem struct {
	Value string `json:"value"`
	Count uint64 `json:"count"`
}

type Top struct {
	Range     string    `json:"range"`
	Dimension string    `json:"dimension"`
	Items     []TopItem `json:"items"`
	Other     uint64    `json:"other_count"`
}

type cacheEntry struct {
	expires time.Time
	value   any
}

type timedBucket struct {
	start  time.Time
	bucket *Bucket
}

type windowSpec struct {
	window string
	hourly bool
	start  time.Time
	end    time.Time
	points int
}

func (a *Aggregator) cacheGet(key string) (any, bool) {
	if a.cfg.CacheTTL < 0 {
		return nil, false
	}
	a.cacheMu.Lock()
	defer a.cacheMu.Unlock()
	entry, ok := a.cache[key]
	if !ok || !a.now().Before(entry.expires) {
		if ok {
			delete(a.cache, key)
		}
		return nil, false
	}
	return entry.value, true
}

func (a *Aggregator) cachePut(key string, value any) {
	if a.cfg.CacheTTL < 0 {
		return
	}
	a.cacheMu.Lock()
	a.cache[key] = cacheEntry{expires: a.now().Add(a.cfg.CacheTTL), value: value}
	a.cacheMu.Unlock()
}

func (a *Aggregator) window(window string) (windowSpec, error) {
	now := a.now().UTC()
	switch window {
	case Range1h:
		// The aggregate stores hourly buckets. "1h" is the bucket for the
		// current hour (from the top of the hour), not a rolling minute window.
		end := now.Truncate(time.Hour)
		return windowSpec{window: window, hourly: true, start: end, end: end, points: 1}, nil
	case Range24h:
		end := now.Truncate(time.Hour)
		return windowSpec{window: window, hourly: true, start: end.Add(-23 * time.Hour), end: end, points: 24}, nil
	case Range7d:
		end := now.Truncate(time.Hour)
		return windowSpec{window: window, hourly: true, start: end.Add(-167 * time.Hour), end: end, points: 168}, nil
	case Range30d:
		end := truncateDay(now)
		return windowSpec{window: window, hourly: false, start: end.AddDate(0, 0, -29), end: end, points: 30}, nil
	case Range90d:
		end := truncateDay(now)
		return windowSpec{window: window, hourly: false, start: end.AddDate(0, 0, -89), end: end, points: 90}, nil
	default:
		return windowSpec{}, ErrInvalidRange
	}
}

func (a *Aggregator) seriesWindow(window, interval string) (windowSpec, error) {
	spec, err := a.window(window)
	if err != nil {
		return windowSpec{}, err
	}
	if interval == "" {
		if window == Range1h {
			interval = Interval5m
		} else if spec.hourly {
			interval = "hour"
		} else {
			interval = "day"
		}
	}
	switch interval {
	case Interval5m:
		if window != Range1h {
			return windowSpec{}, ErrInvalidInterval
		}
		return spec, nil
	case "hour":
		if window == Range1h {
			// Older clients still ask for the hourly interval on 1h. Serve the
			// five-minute points anyway so the graph keeps drawing.
			return spec, nil
		}
		if !spec.hourly {
			return windowSpec{}, ErrInvalidInterval
		}
		return spec, nil
	case "day":
		if window == Range1h || window == Range24h {
			return windowSpec{}, ErrInvalidInterval
		}
		now := a.now().UTC()
		end := truncateDay(now)
		days := 7
		switch window {
		case Range30d:
			days = 30
		case Range90d:
			days = 90
		}
		return windowSpec{window: window, hourly: false, start: end.AddDate(0, 0, -(days - 1)), end: end, points: days}, nil
	default:
		return windowSpec{}, ErrInvalidInterval
	}
}

func (a *Aggregator) Summary(window string) (Summary, error) {
	key := "summary:" + window
	if v, ok := a.cacheGet(key); ok {
		return v.(Summary), nil
	}
	spec, err := a.window(window)
	if err != nil {
		return Summary{}, err
	}
	buckets, err := a.buckets(spec)
	if err != nil {
		return Summary{}, err
	}
	out := Summary{
		Range:   window,
		From:    spec.start,
		To:      spec.end.Add(time.Hour),
		Buckets: len(buckets),
	}
	if !spec.hourly {
		out.To = spec.end.AddDate(0, 0, 1)
	}
	// Distinct clients come from the per-bucket HyperLogLog registers: their
	// union is exact at the register level and unbounded by the top-N cap.
	registers := newHLL()
	hllComplete := true
	legacyClients := make(map[string]struct{})
	for _, tb := range buckets {
		b := tb.bucket
		if b == nil {
			continue
		}
		out.Queries += b.Queries
		out.Blocked += b.Blocked
		out.Proxied += b.Proxied
		out.Cached += b.Cached
		out.Failed += b.Failed
		out.LatencyAvgMs += b.LatencySumMs
		if b.ClientHLLComplete && len(b.ClientHLL) == hllRegisters {
			hllMerge(registers, b.ClientHLL)
		} else {
			// A bucket written before the HLL field existed. The caller gets a
			// clear lower-bound flag rather than a number that looks precise.
			hllComplete = false
			for k := range b.TopClients.Items {
				legacyClients[k] = struct{}{}
			}
		}
	}
	if out.Queries > 0 {
		out.BlockedPct = 100 * float64(out.Blocked) / float64(out.Queries)
		out.ProxiedPct = 100 * float64(out.Proxied) / float64(out.Queries)
		out.CacheHitPct = 100 * float64(out.Cached) / float64(out.Queries)
	}
	var latencyCount uint64
	for _, tb := range buckets {
		if tb.bucket != nil {
			latencyCount += tb.bucket.LatencyCount
		}
	}
	if latencyCount > 0 {
		out.LatencyAvgMs /= float64(latencyCount)
	}
	if hllComplete {
		out.ActiveClients = hllEstimate(registers)
		out.ActiveClientsApprox = false
	} else {
		for _, tb := range buckets {
			if tb.bucket == nil {
				continue
			}
			for k := range tb.bucket.TopClients.Items {
				legacyClients[k] = struct{}{}
			}
		}
		out.ActiveClients = len(legacyClients)
		out.ActiveClientsApprox = true
	}
	a.cachePut(key, out)
	return out, nil
}

func (a *Aggregator) Series(window, interval string) (Series, error) {
	key := "series:" + window + ":" + interval
	if v, ok := a.cacheGet(key); ok {
		return v.(Series), nil
	}
	spec, err := a.seriesWindow(window, interval)
	if err != nil {
		return Series{}, err
	}
	if window == Range1h {
		out, err := a.oneHourSeries(spec)
		if err != nil {
			return Series{}, err
		}
		a.cachePut(key, out)
		return out, nil
	}
	buckets, err := a.buckets(spec)
	if err != nil {
		return Series{}, err
	}
	out := Series{Range: window, Interval: interval, Points: make([]SeriesPoint, 0, len(buckets))}
	if interval == "" {
		if spec.hourly {
			out.Interval = "hour"
		} else {
			out.Interval = "day"
		}
	}
	for _, tb := range buckets {
		p := SeriesPoint{Start: tb.start}
		if tb.bucket != nil {
			p.Queries = tb.bucket.Queries
			p.Blocked = tb.bucket.Blocked
			p.Proxied = tb.bucket.Proxied
			p.Cached = tb.bucket.Cached
			p.Failed = tb.bucket.Failed
			if tb.bucket.LatencyCount > 0 {
				p.LatencyMs = tb.bucket.LatencySumMs / float64(tb.bucket.LatencyCount)
			}
		}
		out.Points = append(out.Points, p)
	}
	a.cachePut(key, out)
	return out, nil
}

func (a *Aggregator) Top(window, dimension string, limit int) (Top, error) {
	if limit <= 0 || limit > 100 {
		return Top{}, ErrInvalidLimit
	}
	if !validTopDimension(dimension) {
		return Top{}, ErrInvalidDimension
	}
	key := fmt.Sprintf("top:%s:%s:%d", window, dimension, limit)
	if v, ok := a.cacheGet(key); ok {
		return withoutCacheUpstream(v.(Top)), nil
	}
	spec, err := a.window(window)
	if err != nil {
		return Top{}, err
	}
	buckets, err := a.buckets(spec)
	if err != nil {
		return Top{}, err
	}
	acc := make(map[string]uint64)
	blockedDomains := make(map[string]uint64)
	other := uint64(0)
	var blockedOther uint64
	addCounts := func(c Counts) {
		for k, n := range c.Items {
			acc[k] += n
		}
		other += c.Other
	}
	// Ranked domain tables skip the reverse DNS tree. Historical buckets still
	// contain those names until retention drops them, so filtering here makes
	// the correction visible immediately instead of after a stats.db migration.
	addDomainCounts := func(c Counts) {
		for k, n := range c.Items {
			if isReverseDNSName(k) {
				continue
			}
			acc[k] += n
		}
		other += c.Other
	}
	addMap := func(m map[string]uint64) {
		for k, n := range m {
			acc[k] += n
		}
	}
	for _, tb := range buckets {
		if tb.bucket == nil {
			continue
		}
		switch dimension {
		case "domain":
			addDomainCounts(tb.bucket.TopDomains)
			for k, n := range tb.bucket.TopBlockedDomains.Items {
				if isReverseDNSName(k) {
					continue
				}
				blockedDomains[k] += n
			}
			blockedOther += tb.bucket.TopBlockedDomains.Other
		case "blocked_domain":
			addDomainCounts(tb.bucket.TopBlockedDomains)
		case "client":
			addCounts(tb.bucket.TopClients)
		case "upstream":
			addMap(tb.bucket.ByUpstream)
		case "action":
			addMap(tb.bucket.ByAction)
		case "protocol":
			addMap(tb.bucket.ByProtocol)
		case "qtype":
			addMap(tb.bucket.ByQType)
		case "rcode":
			addMap(tb.bucket.ByRcode)
		case "relay_country":
			addMap(tb.bucket.ByRelayCountry)
		default:
			// Unreachable after validTopDimension; kept for the next dimension
			// added to the switch above.
			return Top{}, ErrInvalidDimension
		}
	}
	if dimension == "domain" {
		// A blocked name is stored in both maps so historical buckets can be
		// corrected without a migration. Top domains must show only the
		// queries that were not blocked; Top blocked keeps its own map.
		// Both maps are bounded top-N tables, so this is exact for ranked
		// entries and approximate for the evicted tail; Other is clamped.
		for value, blocked := range blockedDomains {
			current, ok := acc[value]
			if !ok {
				continue
			}
			if blocked >= current {
				delete(acc, value)
			} else {
				acc[value] = current - blocked
			}
		}
		if blockedOther >= other {
			other = 0
		} else {
			other -= blockedOther
		}
	}
	if dimension == "upstream" {
		// Cache hits are logged with the synthetic upstream "cache". They are
		// not resolver traffic, so the Top Upstreams card must not rank them.
		for value := range acc {
			if strings.EqualFold(value, "cache") {
				delete(acc, value)
			}
		}
	}
	items := make([]TopItem, 0, len(acc))
	for value, count := range acc {
		items = append(items, TopItem{Value: value, Count: count})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Count != items[j].Count {
			return items[i].Count > items[j].Count
		}
		return items[i].Value < items[j].Value
	})
	if len(items) > limit {
		for _, item := range items[limit:] {
			other += item.Count
		}
		items = items[:limit]
	}
	out := withoutCacheUpstream(Top{Range: window, Dimension: dimension, Items: items, Other: other})
	a.cachePut(key, out)
	return out, nil
}

// withoutCacheUpstream strips the synthetic cache-hit pseudo-upstream from a
// top-upstreams result. It is applied to both fresh and cached results so an
// older cached document cannot reintroduce it.
func withoutCacheUpstream(top Top) Top {
	if top.Dimension != "upstream" {
		return top
	}
	items := top.Items[:0]
	for _, item := range top.Items {
		if !strings.EqualFold(item.Value, "cache") {
			items = append(items, item)
		}
	}
	top.Items = items
	return top
}

func validTopDimension(dimension string) bool {
	switch dimension {
	case "domain", "blocked_domain", "client", "upstream", "action", "protocol", "qtype", "rcode", "relay_country":
		return true
	}
	return false
}

// oneHourSeries splits the current hour into five-minute points. Only the
// slots that have started are returned, so a young hour shows a short growing
// line instead of a flat run of future zeros.
func (a *Aggregator) oneHourSeries(spec windowSpec) (Series, error) {
	buckets, err := a.buckets(spec)
	if err != nil {
		return Series{}, err
	}
	out := Series{Range: Range1h, Interval: Interval5m, Points: []SeriesPoint{}}
	if len(buckets) == 0 || buckets[0].bucket == nil {
		return out, nil
	}
	b := buckets[0].bucket
	hourStart := buckets[0].start
	now := a.now().UTC()
	elapsed := int(now.Sub(hourStart) / (chunkMinutes * time.Minute))
	if elapsed < 0 {
		elapsed = 0
	}
	if elapsed >= chunkSlots {
		elapsed = chunkSlots - 1
	}
	var recorded uint64
	for _, chunk := range b.Chunks {
		recorded += chunk.Queries
	}
	// A bucket written before the five-minute slices existed has only the
	// hourly total. Keep one point rather than showing an hour of zeros.
	if recorded == 0 && b.Queries > 0 {
		out.Points = append(out.Points, SeriesPoint{
			Start:   hourStart,
			Queries: b.Queries,
			Blocked: b.Blocked,
			Proxied: b.Proxied,
			Cached:  b.Cached,
			Failed:  b.Failed,
		})
		return out, nil
	}
	for i := 0; i <= elapsed; i++ {
		chunk := b.Chunks[i]
		point := SeriesPoint{
			Start:   hourStart.Add(time.Duration(i*chunkMinutes) * time.Minute),
			Queries: chunk.Queries,
			Blocked: chunk.Blocked,
			Proxied: chunk.Proxied,
			Cached:  chunk.Cached,
			Failed:  chunk.Failed,
		}
		if chunk.LatencyCount > 0 {
			point.LatencyMs = chunk.LatencySumMs / float64(chunk.LatencyCount)
		}
		out.Points = append(out.Points, point)
	}
	return out, nil
}

func (a *Aggregator) buckets(spec windowSpec) ([]timedBucket, error) {
	snapshot := a.snapshot()
	out := make([]timedBucket, 0, spec.points)
	if spec.hourly {
		for t := spec.start; !t.After(spec.end); t = t.Add(time.Hour) {
			key := hourKey(t)
			// Try the plain hour key first (backward compat: first shard).
			b := snapshot[key]
			if b == nil {
				// Try any shard key for this hour.
				for i := 0; i < shardCount; i++ {
					shardKey := key + "-shard-" + string(rune('0'+i))
					b = snapshot[shardKey]
					if b != nil {
						break
					}
				}
			}
			if b == nil {
				var err error
				b, err = a.store.readHour(key)
				if err != nil {
					return nil, err
				}
			}
			if b == nil {
				b = newBucket()
			}
			out = append(out, timedBucket{start: t, bucket: b})
		}
		return out, nil
	}
	for t := spec.start; !t.After(spec.end); t = t.AddDate(0, 0, 1) {
		b, err := a.dayBucket(dayKey(t), snapshot)
		if err != nil {
			return nil, err
		}
		out = append(out, timedBucket{start: t, bucket: b})
	}
	return out, nil
}

func (a *Aggregator) snapshot() map[string]*Bucket {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make(map[string]*Bucket, len(a.pending)+1)
	for _, pair := range a.pending {
		if pair.bucket != nil {
			out[pair.key] = pair.bucket.clone()
		}
	}
	shards := *a.current.Load() // *[]*Bucket → []*Bucket
	if shards != nil {
		key, _ := a.currentKey.Load().(string)
		// Include all 16 shards under distinct keys so the caller can merge
		// them as needed (e.g. dayBucket sums all shards for the day).
		for i, b := range shards {
			if b != nil {
				out[key+"-shard-"+string(rune('0'+i))] = b.clone()
			}
		}
		// Also keep the first shard under the plain hour key for backward compat.
		if b := shards[0]; b != nil {
			out[key] = b.clone()
		}
	}
	return out
}

func (a *Aggregator) dayBucket(day string, snapshot map[string]*Bucket) (*Bucket, error) {
	daily, err := a.store.readDaily(day)
	if err != nil {
		return nil, err
	}
	if daily != nil {
		return daily, nil
	}
	out := newBucket()
	keys, err := a.store.hourKeysForDay(day)
	if err != nil {
		return nil, err
	}
	// First, try to find hour keys from the store.
	for _, key := range keys {
		if snapshot[key] != nil {
			// Already have this hour's data in snapshot; merge it.
			out.merge(snapshot[key], a.cfg)
			continue
		}
		b, err := a.store.readHour(key)
		if err != nil {
			return nil, err
		}
		if b != nil {
			out.merge(b, a.cfg)
		}
	}
	// Now merge all shard entries from the snapshot that belong to this day.
	for key, b := range snapshot {
		if strings.HasPrefix(key, day) {
			out.merge(b, a.cfg)
		}
		// Also handle shard keys like "20261104-shard-0"
		if len(key) > len(day)+6 && key[:len(day)] == day && strings.HasSuffix(key, "-shard-") {
			// Find the shard index
			rest := key[len(day)+7:]
			if n, err := strconv.Atoi(rest); err == nil && n >= 0 && n < shardCount {
				// Look up the shard directly from current
				shards := *a.current.Load()
				if shards != nil && n < len(shards) {
					if sb := shards[n]; sb != nil {
						out.merge(sb, a.cfg)
					}
				}
			}
		}
	}
	return out, nil
}
