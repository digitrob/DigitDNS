package aggregate

import (
	"errors"
	"testing"
	"time"

	"github.com/miekg/dns"
)

func TestSummarySeriesAndTopMergeCurrentHour(t *testing.T) {
	now := time.Date(2026, 11, 4, 11, 30, 0, 0, time.UTC)
	a, _ := openTestAggregator(t, &now, Config{
		FlushInterval: time.Hour,
		TopDomains:    10,
		TopClients:    10,
		CacheTTL:      -1,
	})

	old := testItem(now)
	old.Domain = "old.example."
	a.RecordTotal(now)
	a.RecordDetail(old)
	a.flush()

	now = time.Date(2026, 11, 4, 12, 30, 0, 0, time.UTC)
	current := testItem(now)
	current.Domain = "current.example."
	a.RecordTotal(now)
	a.RecordDetail(current)

	blocked := current
	blocked.Domain = "ads.example."
	blocked.Action = "BLOCK"
	a.RecordTotal(now)
	a.RecordDetail(blocked)

	proxied := current
	proxied.Domain = "video.example."
	proxied.Action = "PROXY"
	proxied.RelayCountry = "DE"
	a.RecordTotal(now)
	a.RecordDetail(proxied)

	summary, err := a.Summary(Range24h)
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if summary.Queries != 4 || summary.Blocked != 1 || summary.Proxied != 1 {
		t.Errorf("summary = %#v", summary)
	}
	if summary.Buckets != 24 {
		t.Errorf("Buckets = %d, want 24", summary.Buckets)
	}
	if summary.ActiveClients != 1 || summary.ActiveClientsApprox {
		t.Errorf("active clients = %d/%v", summary.ActiveClients, summary.ActiveClientsApprox)
	}
	if summary.CacheHitPct != 0 || summary.BlockedPct != 25 {
		t.Errorf("percentages = %#v", summary)
	}

	series, err := a.Series(Range24h, "hour")
	if err != nil {
		t.Fatalf("Series: %v", err)
	}
	if len(series.Points) != 24 || series.Interval != "hour" {
		t.Fatalf("series = %#v", series)
	}
	if got := series.Points[len(series.Points)-1].Queries; got != 3 {
		t.Errorf("current hour point = %d queries, want 3", got)
	}
	if got := series.Points[len(series.Points)-2].Queries; got != 1 {
		t.Errorf("previous hour point = %d queries, want 1", got)
	}

	top, err := a.Top(Range24h, "domain", 10)
	if err != nil {
		t.Fatalf("Top: %v", err)
	}
	if len(top.Items) != 3 {
		t.Errorf("domain top = %#v", top.Items)
	}
	// Blocked names belong only to Top blocked, and the three remaining domains
	// are tied at one; the tie-break is by value.
	if top.Items[0].Value != "current.example" || top.Items[0].Count != 1 {
		t.Errorf("domain top order = %#v", top.Items)
	}
	for _, item := range top.Items {
		if item.Value == "ads.example" {
			t.Errorf("blocked domain leaked into top domains: %#v", top.Items)
		}
	}
	foundCurrent := false
	for _, item := range top.Items {
		if item.Value == "current.example" && item.Count == 1 {
			foundCurrent = true
		}
	}
	if !foundCurrent {
		t.Errorf("current-hour domain missing from top: %#v", top.Items)
	}

	blockedTop, err := a.Top(Range24h, "blocked_domain", 10)
	if err != nil || len(blockedTop.Items) != 1 || blockedTop.Items[0].Value != "ads.example" {
		t.Fatalf("blocked top = %#v, %v", blockedTop, err)
	}

	protoTop, err := a.Top(Range24h, "protocol", 10)
	if err != nil || len(protoTop.Items) != 1 || protoTop.Items[0].Value != "UDP" || protoTop.Items[0].Count != 4 {
		t.Fatalf("protocol top = %#v, %v", protoTop, err)
	}
}

func TestDailyQueriesIncludeTodaysHourlyData(t *testing.T) {
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	a, _ := openTestAggregator(t, &now, Config{FlushInterval: time.Hour, CacheTTL: -1})

	item := testItem(now)
	item.Domain = "today.example."
	a.RecordTotal(now)
	a.RecordDetail(item)

	summary, err := a.Summary(Range30d)
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if summary.Queries != 1 || summary.Buckets != 30 {
		t.Errorf("30d summary = %#v", summary)
	}
	series, err := a.Series(Range7d, "day")
	if err != nil {
		t.Fatalf("Series: %v", err)
	}
	if len(series.Points) != 7 {
		t.Fatalf("series points = %d", len(series.Points))
	}
	if got := series.Points[len(series.Points)-1].Queries; got != 1 {
		t.Errorf("today's daily point = %d, want 1", got)
	}
	top, err := a.Top(Range30d, "domain", 10)
	if err != nil || len(top.Items) != 1 || top.Items[0].Value != "today.example" {
		t.Fatalf("30d top = %#v, %v", top, err)
	}
}

func TestOneHourSeriesUsesFiveMinuteChunks(t *testing.T) {
	now := time.Date(2026, 11, 4, 12, 12, 0, 0, time.UTC)
	a, _ := openTestAggregator(t, &now, Config{FlushInterval: time.Hour, CacheTTL: -1})

	first := time.Date(2026, 11, 4, 12, 1, 0, 0, time.UTC)
	second := time.Date(2026, 11, 4, 12, 7, 0, 0, time.UTC)
	a.RecordTotal(first)
	a.RecordTotal(first)
	a.RecordTotal(second)

	blocked := testItem(first)
	blocked.Action = "BLOCK"
	a.RecordDetail(blocked)

	proxied := testItem(second)
	proxied.Action = "PROXY"
	proxied.RelayCountry = "DE"
	a.RecordDetail(proxied)

	series, err := a.Series(Range1h, Interval5m)
	if err != nil {
		t.Fatalf("Series: %v", err)
	}
	if len(series.Points) != 3 {
		t.Fatalf("points = %#v, want the three started slots", series.Points)
	}
	want := []struct {
		start   time.Time
		queries uint64
		blocked uint64
		proxied uint64
	}{
		{time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC), 2, 1, 0},
		{time.Date(2026, 11, 4, 12, 5, 0, 0, time.UTC), 1, 0, 1},
		{time.Date(2026, 11, 4, 12, 10, 0, 0, time.UTC), 0, 0, 0},
	}
	for i, w := range want {
		got := series.Points[i]
		if !got.Start.Equal(w.start) || got.Queries != w.queries || got.Blocked != w.blocked || got.Proxied != w.proxied {
			t.Errorf("point %d = %#v, want %#v", i, got, w)
		}
	}
}

func TestTopDomainsExcludeBlockedCounts(t *testing.T) {
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	a, _ := openTestAggregator(t, &now, Config{FlushInterval: time.Hour, CacheTTL: -1})

	record := func(domain, action string, times int) {
		for i := 0; i < times; i++ {
			item := testItem(now)
			item.Domain = domain
			item.Action = action
			a.RecordTotal(now)
			a.RecordDetail(item)
		}
	}
	record("mixed.example.", "DIRECT", 2)
	record("mixed.example.", "BLOCK", 3)
	record("blocked-only.example.", "BLOCK", 2)
	record("allowed-only.example.", "DIRECT", 1)

	top, err := a.Top(Range24h, "domain", 10)
	if err != nil {
		t.Fatalf("Top domain: %v", err)
	}
	got := make(map[string]uint64, len(top.Items))
	for _, item := range top.Items {
		got[item.Value] = item.Count
	}
	if got["mixed.example"] != 2 || got["allowed-only.example"] != 1 {
		t.Errorf("top domains = %#v, want allowed-only counts", top.Items)
	}
	if _, ok := got["blocked-only.example"]; ok {
		t.Errorf("blocked-only domain leaked into top domains: %#v", top.Items)
	}

	blocked, err := a.Top(Range24h, "blocked_domain", 10)
	if err != nil {
		t.Fatalf("Top blocked: %v", err)
	}
	blockedCounts := make(map[string]uint64, len(blocked.Items))
	for _, item := range blocked.Items {
		blockedCounts[item.Value] = item.Count
	}
	if blockedCounts["mixed.example"] != 3 || blockedCounts["blocked-only.example"] != 2 {
		t.Errorf("top blocked = %#v", blocked.Items)
	}
}

func TestTopDomainsBlockedTailIsClamped(t *testing.T) {
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	a, _ := openTestAggregator(t, &now, Config{FlushInterval: time.Hour, CacheTTL: -1})
	// Deliberately inconsistent tail totals: blocked Other is larger than the
	// domain Other. The subtraction must clamp instead of underflowing.
	if err := a.store.writeHour(hourKey(now), &Bucket{
		TopDomains:        Counts{Items: map[string]uint64{}, Other: 1},
		TopBlockedDomains: Counts{Items: map[string]uint64{}, Other: 5},
	}); err != nil {
		t.Fatalf("writeHour: %v", err)
	}
	top, err := a.Top(Range24h, "domain", 10)
	if err != nil {
		t.Fatalf("Top: %v", err)
	}
	if len(top.Items) != 0 || top.Other != 0 {
		t.Errorf("top = %#v, want an empty clamped result", top)
	}
}

func TestQueryValidation(t *testing.T) {
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	a, _ := openTestAggregator(t, &now, Config{FlushInterval: time.Hour, CacheTTL: -1})

	if _, err := a.Summary("2h"); !errors.Is(err, ErrInvalidRange) {
		t.Errorf("Summary invalid range = %v", err)
	}
	if _, err := a.Series(Range24h, "day"); !errors.Is(err, ErrInvalidInterval) {
		t.Errorf("24h daily series = %v", err)
	}
	if _, err := a.Series(Range30d, "hour"); !errors.Is(err, ErrInvalidInterval) {
		t.Errorf("30d hourly series = %v", err)
	}
	if _, err := a.Top(Range24h, "nope", 10); !errors.Is(err, ErrInvalidDimension) {
		t.Errorf("invalid dimension = %v", err)
	}
	if _, err := a.Top(Range24h, "domain", 0); !errors.Is(err, ErrInvalidLimit) {
		t.Errorf("invalid limit = %v", err)
	}
	if _, err := a.Top(Range24h, "domain", 101); !errors.Is(err, ErrInvalidLimit) {
		t.Errorf("invalid high limit = %v", err)
	}
}

func TestQueryCacheTTL(t *testing.T) {
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	a, _ := openTestAggregator(t, &now, Config{
		FlushInterval: time.Hour,
		CacheTTL:      30 * time.Second,
	})

	a.RecordTotal(now)
	first, err := a.Summary(Range24h)
	if err != nil {
		t.Fatalf("first Summary: %v", err)
	}
	if first.Queries != 1 {
		t.Fatalf("first queries = %d", first.Queries)
	}

	a.RecordTotal(now)
	second, err := a.Summary(Range24h)
	if err != nil {
		t.Fatalf("second Summary: %v", err)
	}
	if second.Queries != 1 {
		t.Errorf("cached summary = %d queries, want the cached 1", second.Queries)
	}

	now = now.Add(31 * time.Second)
	third, err := a.Summary(Range24h)
	if err != nil {
		t.Fatalf("third Summary: %v", err)
	}
	if third.Queries != 2 {
		t.Errorf("post-TTL summary = %d queries, want 2", third.Queries)
	}
}

func TestTopOtherCount(t *testing.T) {
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	a, _ := openTestAggregator(t, &now, Config{
		FlushInterval: time.Hour,
		TopDomains:    2,
		CacheTTL:      -1,
	})
	for _, domain := range []string{"a.example", "b.example", "c.example"} {
		item := testItem(now)
		item.Domain = domain
		item.QType = uint16(dns.TypeA)
		a.RecordDetail(item)
	}
	top, err := a.Top(Range24h, "domain", 1)
	if err != nil {
		t.Fatalf("Top: %v", err)
	}
	if len(top.Items) != 1 || top.Other == 0 {
		t.Errorf("top/other = %#v", top)
	}
}

func TestTopUpstreamsExcludesCachePseudoUpstream(t *testing.T) {
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	a, _ := openTestAggregator(t, &now, Config{FlushInterval: time.Hour, CacheTTL: -1})

	real := testItem(now)
	real.Upstream = "1.1.1.1:53"
	a.RecordTotal(now)
	a.RecordDetail(real)

	cached := testItem(now)
	cached.Upstream = "cache"
	cached.Cached = true
	a.RecordTotal(now)
	a.RecordDetail(cached)

	top, err := a.Top(Range24h, "upstream", 10)
	if err != nil {
		t.Fatalf("Top: %v", err)
	}
	if len(top.Items) != 1 || top.Items[0].Value != "1.1.1.1:53" {
		t.Fatalf("top upstreams = %#v, want only the real resolver", top.Items)
	}
}

func TestOneHourRangeUsesCurrentHourBucket(t *testing.T) {
	now := time.Date(2026, 11, 4, 12, 37, 0, 0, time.UTC)
	a, _ := openTestAggregator(t, &now, Config{FlushInterval: time.Hour, CacheTTL: -1})

	item := testItem(now)
	item.Domain = "hour.example."
	a.RecordTotal(now)
	a.RecordDetail(item)

	summary, err := a.Summary(Range1h)
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if summary.Queries != 1 || summary.Buckets != 1 {
		t.Errorf("summary = %#v", summary)
	}
	if !summary.From.Equal(time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)) {
		t.Errorf("from = %v, want current hour start", summary.From)
	}
	series, err := a.Series(Range1h, Interval5m)
	if err != nil {
		t.Fatalf("Series: %v", err)
	}
	// 12:37 has started eight five-minute slots (:00 through :35).
	if series.Interval != Interval5m || len(series.Points) != 8 {
		t.Fatalf("series = %#v", series)
	}
	last := series.Points[len(series.Points)-1]
	if !last.Start.Equal(time.Date(2026, 11, 4, 12, 35, 0, 0, time.UTC)) || last.Queries != 1 {
		t.Errorf("last point = %#v, want the 12:35 slot with one query", last)
	}
	// Older clients that still send interval=hour get the same five-minute
	// points instead of a one-point series.
	legacy, err := a.Series(Range1h, "hour")
	if err != nil || len(legacy.Points) != 8 {
		t.Fatalf("legacy hourly series = %#v, %v", legacy, err)
	}
	top, err := a.Top(Range1h, "domain", 10)
	if err != nil || len(top.Items) != 1 || top.Items[0].Value != "hour.example" {
		t.Fatalf("top = %#v, %v", top, err)
	}
}
