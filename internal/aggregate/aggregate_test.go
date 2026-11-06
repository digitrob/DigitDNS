package aggregate

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"digitdns/internal/database"

	"github.com/miekg/dns"
)

func testItem(at time.Time) database.QueryLogItem {
	return database.QueryLogItem{
		Timestamp:   at,
		ClientIP:    "192.0.2.10",
		AccountName: "Alice",
		Protocol:    "UDP",
		Domain:      "example.com.",
		Action:      "DIRECT",
		QType:       uint16(dns.TypeA),
		Rcode:       dns.RcodeSuccess,
		Upstream:    "1.1.1.1:53",
		LatencyMs:   12.5,
	}
}

func openTestAggregator(t *testing.T, now *time.Time, cfg Config) (*Aggregator, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "stats.db")
	cfg.Now = func() time.Time { return *now }
	a, err := Open(path, cfg)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(a.Close)
	return a, path
}

// The HTTPS/SVCB sink is a proxied route answered locally, so it must count in
// the proxied total and in the relay-country breakdown like PROXY_IPV6_SINK.
func TestProxyHTTPSinkCountsAsProxied(t *testing.T) {
	now := time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)
	cfg := Config{FlushInterval: time.Hour, TopDomains: 10, TopClients: 10}
	a, _ := openTestAggregator(t, &now, cfg)

	item := testItem(now)
	item.Domain = "chat.example."
	item.Action = "PROXY_HTTPS_SINK"
	item.RelayCountry = "DE"
	a.RecordTotal(now)
	a.RecordDetail(item)

	a.flush()
	b, err := a.store.readHour(hourKey(now))
	if err != nil {
		t.Fatalf("read hour: %v", err)
	}
	if b == nil {
		t.Fatal("hour bucket was not written")
	}
	if b.Proxied != 1 {
		t.Errorf("Proxied = %d, want 1", b.Proxied)
	}
	if b.ByRelayCountry["DE"] != 1 {
		t.Errorf("ByRelayCountry = %#v, want DE=1", b.ByRelayCountry)
	}
	if b.ByAction["PROXY_HTTPS_SINK"] != 1 {
		t.Errorf("ByAction = %#v, want PROXY_HTTPS_SINK=1", b.ByAction)
	}
}

func TestRecordDetailBuildsBoundedTotalsAndTopLists(t *testing.T) {
	now := time.Date(2026, 11, 4, 10, 15, 0, 0, time.UTC)
	cfg := Config{FlushInterval: time.Hour, TopDomains: 3, TopClients: 2}
	a, _ := openTestAggregator(t, &now, cfg)

	a.RecordTotal(now)
	item := testItem(now)
	a.RecordDetail(item)

	blocked := item
	blocked.Domain = "ads.example."
	blocked.Action = "BLOCK"
	blocked.Rcode = dns.RcodeSuccess
	a.RecordTotal(now)
	a.RecordDetail(blocked)

	proxied := item
	proxied.Domain = "video.example."
	proxied.Action = "PROXY"
	proxied.RelayCountry = "DE"
	a.RecordTotal(now)
	a.RecordDetail(proxied)

	failed := item
	failed.Domain = "broken.example."
	failed.Rcode = dns.RcodeServerFailure
	a.RecordTotal(now)
	a.RecordDetail(failed)

	limited := item
	limited.Domain = "noisy.example."
	limited.Action = "RATELIMIT"
	limited.Rcode = dns.RcodeRefused
	a.RecordTotal(now)
	a.RecordDetail(limited)

	a.flush()
	b, err := a.store.readHour(hourKey(now))
	if err != nil {
		t.Fatalf("read hour: %v", err)
	}
	if b == nil {
		t.Fatal("hour bucket was not written")
	}
	if b.Queries != 5 {
		t.Errorf("Queries = %d, want 5", b.Queries)
	}
	if b.Blocked != 1 || b.Proxied != 1 || b.Failed != 1 || b.Cached != 0 {
		t.Errorf("blocked/proxied/failed/cached = %d/%d/%d/%d, want 1/1/1/0",
			b.Blocked, b.Proxied, b.Failed, b.Cached)
	}
	if b.ByAction["DIRECT"] != 2 || b.ByAction["BLOCK"] != 1 || b.ByAction["PROXY"] != 1 || b.ByAction["RATELIMIT"] != 1 {
		t.Errorf("ByAction = %#v", b.ByAction)
	}
	if b.ByProtocol["UDP"] != 5 {
		t.Errorf("ByProtocol = %#v", b.ByProtocol)
	}
	if b.ByRelayCountry["DE"] != 1 {
		t.Errorf("ByRelayCountry = %#v", b.ByRelayCountry)
	}
	if b.ByQType["A"] != 5 || b.ByRcode["NOERROR"] != 3 || b.ByRcode["SERVFAIL"] != 1 || b.ByRcode["REFUSED"] != 1 {
		t.Errorf("qtype/rcode = %#v / %#v", b.ByQType, b.ByRcode)
	}
	if b.TopDomains.Items["example.com"] != 1 {
		t.Errorf("TopDomains = %#v", b.TopDomains)
	}
	if b.TopBlockedDomains.Items["ads.example"] != 1 {
		t.Errorf("TopBlockedDomains = %#v", b.TopBlockedDomains)
	}
	if b.TopClients.Items["192.0.2.10"] != 5 {
		t.Errorf("TopClients = %#v", b.TopClients)
	}
	if b.LatencyCount != 5 || b.LatencySumMs != 62.5 {
		t.Errorf("latency = %v/%d, want 62.5/5", b.LatencySumMs, b.LatencyCount)
	}
}

func TestTopNCompactsIntoOther(t *testing.T) {
	now := time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC)
	cfg := Config{FlushInterval: time.Hour, TopDomains: 3, TopClients: 2}
	a, _ := openTestAggregator(t, &now, cfg)

	for i, domain := range []string{"a.example", "b.example", "c.example", "d.example", "e.example"} {
		item := testItem(now)
		item.Domain = domain
		a.RecordDetail(item)
		if i < 3 {
			a.RecordDetail(item)
		}
	}
	a.flush()
	b, err := a.store.readHour(hourKey(now))
	if err != nil || b == nil {
		t.Fatalf("read hour: %v, bucket=%v", err, b)
	}
	if len(b.TopDomains.Items) > 3 {
		t.Fatalf("TopDomains has %d entries, want <=3", len(b.TopDomains.Items))
	}
	if b.TopDomains.Other == 0 {
		t.Fatal("TopDomains.Other is zero; the evicted tail was lost")
	}
	if b.TopDomains.Items["a.example"] < 2 || b.TopDomains.Items["b.example"] < 2 || b.TopDomains.Items["c.example"] < 2 {
		t.Errorf("heavy hitters were not retained: %#v", b.TopDomains.Items)
	}
}

func TestRecordTotalDoesNotInventDetail(t *testing.T) {
	now := time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC)
	a, _ := openTestAggregator(t, &now, Config{FlushInterval: time.Hour})
	a.RecordTotal(now)
	a.RecordTotal(now)
	a.flush()
	b, err := a.store.readHour(hourKey(now))
	if err != nil || b == nil {
		t.Fatalf("read hour: %v, bucket=%v", err, b)
	}
	if b.Queries != 2 {
		t.Errorf("Queries = %d, want 2", b.Queries)
	}
	if b.LatencyCount != 0 || len(b.TopDomains.Items) != 0 {
		t.Errorf("total-only records created detail: %#v", b)
	}
}

func TestRestartMergesTheCurrentHour(t *testing.T) {
	now := time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "stats.db")
	cfg := Config{FlushInterval: time.Hour, Now: func() time.Time { return now }}

	first, err := Open(path, cfg)
	if err != nil {
		t.Fatalf("first Open: %v", err)
	}
	first.RecordTotal(now)
	first.RecordDetail(testItem(now))
	first.flush()
	first.Close()

	second, err := Open(path, cfg)
	if err != nil {
		t.Fatalf("second Open: %v", err)
	}
	defer second.Close()
	second.RecordTotal(now)
	second.RecordDetail(testItem(now))
	second.flush()

	b, err := second.store.readHour(hourKey(now))
	if err != nil || b == nil {
		t.Fatalf("read hour: %v, bucket=%v", err, b)
	}
	if b.Queries != 2 || b.TopDomains.Items["example.com"] != 2 {
		t.Errorf("restart did not merge current hour: queries=%d top=%#v", b.Queries, b.TopDomains)
	}
}

func TestDailyRollupAndRetention(t *testing.T) {
	day1 := time.Date(2026, 11, 3, 23, 0, 0, 0, time.UTC)
	now := day1
	cfg := Config{
		FlushInterval:   time.Hour,
		HourlyRetention: 24 * time.Hour,
		RetentionDays:   2,
		TopDomains:      10,
	}
	a, _ := openTestAggregator(t, &now, cfg)
	a.RecordTotal(day1)
	a.RecordDetail(testItem(day1))
	a.flush()

	day2 := day1.Add(2 * time.Hour)
	now = day2
	if err := a.maybeRollover(day2); err != nil {
		t.Fatalf("rollover: %v", err)
	}
	daily, err := a.store.readDaily(dayKey(day1))
	if err != nil || daily == nil {
		t.Fatalf("read daily: %v, bucket=%v", err, daily)
	}
	if daily.Queries != 1 || daily.TopDomains.Items["example.com"] != 1 {
		t.Errorf("daily rollup = %#v", daily)
	}

	// An ancient hourly and daily bucket must be pruned by the next prune.
	oldHour := day1.Add(-72 * time.Hour)
	if err := a.store.writeHour(hourKey(oldHour), &Bucket{Queries: 1}); err != nil {
		t.Fatal(err)
	}
	oldDay := day1.AddDate(0, 0, -5)
	if err := a.store.writeDaily(dayKey(oldDay), &Bucket{Queries: 1}); err != nil {
		t.Fatal(err)
	}
	if err := a.prune(day2); err != nil {
		t.Fatalf("prune: %v", err)
	}
	if b, _ := a.store.readHour(hourKey(oldHour)); b != nil {
		t.Error("old hourly bucket survived prune")
	}
	if b, _ := a.store.readDaily(dayKey(oldDay)); b != nil {
		t.Error("old daily bucket survived prune")
	}
	if b, _ := a.store.readDaily(dayKey(day1)); b == nil {
		t.Error("retained daily bucket was pruned")
	}
}

func TestConcurrentRecordAndFlush(t *testing.T) {
	now := time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC)
	cfg := Config{FlushInterval: time.Hour, TopDomains: 50, TopClients: 20}
	a, _ := openTestAggregator(t, &now, cfg)

	const goroutines = 8
	const perGoroutine = 100
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perGoroutine; i++ {
				a.RecordTotal(now)
				item := testItem(now)
				item.Domain = "d.example"
				a.RecordDetail(item)
			}
		}()
	}
	wg.Wait()
	a.flush()
	b, err := a.store.readHour(hourKey(now))
	if err != nil || b == nil {
		t.Fatalf("read hour: %v, bucket=%v", err, b)
	}
	if b.Queries != goroutines*perGoroutine {
		t.Errorf("Queries = %d, want %d", b.Queries, goroutines*perGoroutine)
	}
	if b.TopDomains.Items["d.example"] != goroutines*perGoroutine {
		t.Errorf("TopDomains[d.example] = %d, want %d", b.TopDomains.Items["d.example"], goroutines*perGoroutine)
	}
}

func TestAnonymizeClientsHashesTheKey(t *testing.T) {
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	a, _ := openTestAggregator(t, &now, Config{
		FlushInterval:    time.Hour,
		AnonymizeClients: true,
		CacheTTL:         -1,
	})
	item := testItem(now)
	item.ClientIP = "203.0.113.77"
	item.AccountName = ""
	a.RecordDetail(item)
	a.RecordDetail(item)
	a.flush()
	b, err := a.store.readHour(hourKey(now))
	if err != nil || b == nil {
		t.Fatalf("read hour: %v, bucket=%v", err, b)
	}
	if _, ok := b.TopClients.Items["203.0.113.77"]; ok {
		t.Fatal("raw client IP survived anonymization")
	}
	var hashed string
	for key := range b.TopClients.Items {
		hashed = key
	}
	if !strings.HasPrefix(hashed, "anon:") {
		t.Fatalf("client key = %q, want anon: prefix", hashed)
	}
	if b.TopClients.Items[hashed] != 2 {
		t.Errorf("anonymized client count = %d, want 2", b.TopClients.Items[hashed])
	}
}

func TestHyperLogLogCountsBeyondTheClientCap(t *testing.T) {
	now := time.Date(2026, 11, 4, 12, 30, 0, 0, time.UTC)
	a, _ := openTestAggregator(t, &now, Config{
		FlushInterval: time.Hour,
		TopClients:    2,
		CacheTTL:      -1,
	})
	const clients = 400
	for i := 0; i < clients; i++ {
		item := testItem(now)
		item.ClientIP = fmt.Sprintf("198.51.%d.%d", i/250, i%250)
		item.AccountName = ""
		a.RecordDetail(item)
	}
	summary, err := a.Summary(Range24h)
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if summary.ActiveClientsApprox {
		t.Error("active clients fell back to the top-N approximation")
	}
	// 6.5% standard error; allow a little more for the linear-counting boundary.
	if delta := summary.ActiveClients - clients; delta < -40 || delta > 40 {
		t.Errorf("active clients = %d, want about %d", summary.ActiveClients, clients)
	}
	if err := a.flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}
	b, err := a.store.readHour(hourKey(now))
	if err != nil || b == nil {
		t.Fatalf("read hour: %v, bucket=%v", err, b)
	}
	if len(b.TopClients.Items) != 2 {
		t.Errorf("top clients kept %d entries, want the cap of 2", len(b.TopClients.Items))
	}
}

func TestHyperLogLogMergesAcrossBuckets(t *testing.T) {
	now := time.Date(2026, 11, 4, 11, 30, 0, 0, time.UTC)
	a, _ := openTestAggregator(t, &now, Config{
		FlushInterval: time.Hour,
		TopClients:    500,
		CacheTTL:      -1,
	})
	for i := 0; i < 60; i++ {
		item := testItem(now)
		item.ClientIP = fmt.Sprintf("192.0.2.%d", i)
		a.RecordDetail(item)
	}
	now = now.Add(time.Hour)
	for i := 60; i < 120; i++ {
		item := testItem(now)
		item.ClientIP = fmt.Sprintf("192.0.2.%d", i)
		a.RecordDetail(item)
	}
	summary, err := a.Summary(Range24h)
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if summary.ActiveClientsApprox {
		t.Error("active clients fell back to the top-N approximation")
	}
	if delta := summary.ActiveClients - 120; delta < -12 || delta > 12 {
		t.Errorf("merged active clients = %d, want about 120", summary.ActiveClients)
	}
}

func TestExcludedClientsKeepAggregatesButLoseTheirIdentifier(t *testing.T) {
	now := time.Date(2026, 11, 4, 12, 30, 0, 0, time.UTC)
	a, _ := openTestAggregator(t, &now, Config{
		FlushInterval:  time.Hour,
		TopClients:     10,
		ExcludeClients: []string{"192.0.2.66"},
		CacheTTL:       -1,
	})
	excluded := testItem(now)
	excluded.ClientIP = "192.0.2.66"
	excluded.AccountName = ""
	a.RecordTotal(now)
	a.RecordDetail(excluded)

	included := testItem(now)
	included.ClientIP = "192.0.2.67"
	included.AccountName = ""
	a.RecordTotal(now)
	a.RecordDetail(included)

	summary, err := a.Summary(Range24h)
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if summary.Queries != 2 || summary.ActiveClients != 1 {
		t.Errorf("summary = queries %d, clients %d", summary.Queries, summary.ActiveClients)
	}
	top, err := a.Top(Range24h, "client", 10)
	if err != nil {
		t.Fatalf("Top: %v", err)
	}
	for _, item := range top.Items {
		if item.Value == "192.0.2.66" {
			t.Error("excluded client reached the top-client table")
		}
	}
	domains, err := a.Top(Range24h, "domain", 10)
	if err != nil {
		t.Fatalf("Top domains: %v", err)
	}
	if len(domains.Items) != 1 || domains.Items[0].Count != 2 {
		t.Errorf("domain aggregates = %#v, want both queries", domains.Items)
	}
}

func TestSummaryFallsBackForLegacyBucketsWithoutHLL(t *testing.T) {
	now := time.Date(2026, 11, 4, 12, 30, 0, 0, time.UTC)
	a, _ := openTestAggregator(t, &now, Config{
		FlushInterval: time.Hour,
		TopClients:    10,
		CacheTTL:      -1,
	})
	legacy := &Bucket{
		Queries:    5,
		TopClients: Counts{Items: map[string]uint64{"192.0.2.44": 5}},
	}
	if err := a.store.writeHour(hourKey(now.Add(-time.Hour)), legacy); err != nil {
		t.Fatalf("write legacy hour: %v", err)
	}
	summary, err := a.Summary(Range24h)
	if err != nil {
		t.Fatalf("Summary: %v", err)
	}
	if summary.ActiveClients != 1 || !summary.ActiveClientsApprox {
		t.Errorf("legacy active clients = %d/%v, want 1/true", summary.ActiveClients, summary.ActiveClientsApprox)
	}
}
