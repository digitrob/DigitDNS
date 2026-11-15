package service

import (
	"path/filepath"
	"testing"
	"time"

	"digitdns/internal/database"
)

// newQueryLogTestService builds a service backed by a real settings database and a
// query-log file in the test's temp directory. Logging is enabled and anonymisation
// is off so assertions can read the stored values.
func newQueryLogTestService(t *testing.T) *QueryLogService {
	t.Helper()
	dir := t.TempDir()
	db, err := database.Open(filepath.Join(dir, "data.db"), nil)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	s := NewQueryLogService(db, filepath.Join(dir, "queries.db"))
	if err := s.SaveSettings(QueryLogSettings{Enabled: true, RetentionDays: 7, MaxMB: 256}); err != nil {
		t.Fatalf("save settings: %v", err)
	}
	t.Cleanup(s.Stop)
	return s
}

func queryLogTestItem(ts time.Time, domain, action string, qtype uint16, rcode int, cached bool) database.QueryLogItem {
	return database.QueryLogItem{
		Timestamp: ts,
		ClientIP:  "192.0.2.10",
		Domain:    domain,
		Action:    action,
		Protocol:  "UDP",
		QType:     qtype,
		Rcode:     rcode,
		Cached:    cached,
	}
}

func seedQueryLog(t *testing.T, s *QueryLogService, items ...database.QueryLogItem) {
	t.Helper()
	s.writeBatch(items)
}

func domainsOf(items []database.QueryLogItem) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, it.Domain)
	}
	return out
}

// The descending walk must break on the From bound and skip rows newer than To.
// The bounds were inverted, which made an upper bound return nothing.
func TestQueryLogSearchPageAppliesBothTimeBounds(t *testing.T) {
	s := newQueryLogTestService(t)
	base := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	seedQueryLog(t, s,
		queryLogTestItem(base.Add(-2*time.Hour), "old.example.", "DIRECT", 1, 0, false),
		queryLogTestItem(base.Add(-1*time.Hour), "middle.example.", "DIRECT", 1, 0, false),
		queryLogTestItem(base.Add(-30*time.Minute), "new.example.", "DIRECT", 1, 0, false),
	)

	page := s.SearchPage(QueryLogFilter{
		From: base.Add(-90 * time.Minute),
		To:   base.Add(-45 * time.Minute),
	})
	if got := domainsOf(page.Items); len(got) != 1 || got[0] != "middle.example." {
		t.Fatalf("window returned %v, want [middle.example.]", got)
	}

	page = s.SearchPage(QueryLogFilter{From: base.Add(-90 * time.Minute)})
	if got := domainsOf(page.Items); len(got) != 2 {
		t.Fatalf("lower bound alone returned %v, want the two newer rows", got)
	}

	page = s.SearchPage(QueryLogFilter{To: base.Add(-90 * time.Minute)})
	if got := domainsOf(page.Items); len(got) != 1 || got[0] != "old.example." {
		t.Fatalf("upper bound alone returned %v, want [old.example.]", got)
	}
}

func TestQueryLogSearchPageCursorWalksEveryRecordOnce(t *testing.T) {
	s := newQueryLogTestService(t)
	base := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		seedQueryLog(t, s, queryLogTestItem(base.Add(time.Duration(i)*time.Minute), "d"+string(rune('0'+i))+".example.", "DIRECT", 1, 0, false))
	}

	var seen []string
	filter := QueryLogFilter{Limit: 2}
	pages := 0
	for {
		page := s.SearchPage(filter)
		pages++
		if len(page.Items) > 2 {
			t.Fatalf("page %d has %d items, over the limit", pages, len(page.Items))
		}
		seen = append(seen, domainsOf(page.Items)...)
		if !page.HasMore {
			if page.NextCursor != "" {
				t.Fatalf("final page offered a cursor")
			}
			break
		}
		if page.NextCursor == "" {
			t.Fatalf("page %d claims more rows but has no cursor", pages)
		}
		filter.Before = page.NextCursor
		if pages > 5 {
			t.Fatalf("paging did not terminate")
		}
	}
	if pages != 3 {
		t.Fatalf("paged %d times, want 3", pages)
	}
	want := []string{"d4.example.", "d3.example.", "d2.example.", "d1.example.", "d0.example."}
	if len(seen) != len(want) {
		t.Fatalf("saw %v, want %v", seen, want)
	}
	for i := range want {
		if seen[i] != want[i] {
			t.Fatalf("saw %v, want %v", seen, want)
		}
	}
}

func TestQueryLogSearchPageMalformedCursorStartsAtNewest(t *testing.T) {
	s := newQueryLogTestService(t)
	base := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	seedQueryLog(t, s,
		queryLogTestItem(base, "a.example.", "DIRECT", 1, 0, false),
		queryLogTestItem(base.Add(time.Minute), "b.example.", "DIRECT", 1, 0, false),
	)
	page := s.SearchPage(QueryLogFilter{Before: "not-a-cursor", Limit: 10})
	if got := domainsOf(page.Items); len(got) != 2 || got[0] != "b.example." {
		t.Fatalf("malformed cursor returned %v, want newest-first page", got)
	}
}

func TestQueryLogSearchPageFilters(t *testing.T) {
	s := newQueryLogTestService(t)
	base := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	seedQueryLog(t, s,
		queryLogTestItem(base, "a.example.", "BLOCK", 28, 3, false),
		queryLogTestItem(base.Add(time.Minute), "b.example.", "DIRECT", 1, 0, true),
		queryLogTestItem(base.Add(2*time.Minute), "c.example.", "PROXY", 1, 0, false),
	)

	tests := []struct {
		name   string
		filter QueryLogFilter
		want   []string
	}{
		{"qtype", QueryLogFilter{QTypes: []uint16{28}}, []string{"a.example."}},
		{"rcode", QueryLogFilter{Rcodes: []int{3}}, []string{"a.example."}},
		{"action csv", QueryLogFilter{Action: "Direct,Proxy"}, []string{"c.example.", "b.example."}},
		{"cached true", QueryLogFilter{Cached: boolPtr(true)}, []string{"b.example."}},
		{"cached false", QueryLogFilter{Cached: boolPtr(false)}, []string{"c.example.", "a.example."}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := domainsOf(s.SearchPage(tc.filter).Items)
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Fatalf("got %v, want %v", got, tc.want)
				}
			}
		})
	}
}

func boolPtr(v bool) *bool { return &v }

func TestQueryLogExportStreamsEveryMatchAndReportsTruncation(t *testing.T) {
	s := newQueryLogTestService(t)
	base := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 4; i++ {
		seedQueryLog(t, s, queryLogTestItem(base.Add(time.Duration(i)*time.Minute), "export.example.", "DIRECT", 1, 0, false))
	}
	var seen []database.QueryLogItem
	count, truncated, err := s.Export(QueryLogFilter{}, func(it database.QueryLogItem) error {
		seen = append(seen, it)
		return nil
	})
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if count != 4 || len(seen) != 4 {
		t.Fatalf("exported %d records (%d visited), want 4", count, len(seen))
	}
	if truncated {
		t.Fatalf("export reported truncation below the cap")
	}
	if !seen[0].Timestamp.After(seen[len(seen)-1].Timestamp) {
		t.Fatalf("export is not newest-first")
	}
}

func TestQueryLogUsageCountsLiveRecords(t *testing.T) {
	s := newQueryLogTestService(t)
	seedQueryLog(t, s,
		queryLogTestItem(time.Now(), "one.example.", "DIRECT", 1, 0, false),
		queryLogTestItem(time.Now(), "two.example.", "DIRECT", 1, 0, false),
	)
	usage := s.Usage()
	if usage.Records != 2 {
		t.Fatalf("usage records = %d, want 2", usage.Records)
	}
	if usage.Bytes <= 0 {
		t.Fatalf("usage bytes = %d, want > 0", usage.Bytes)
	}
}

// Clear must go through the writer goroutine while it is running. Clearing
// directly would let a batch that has already been flushed land after the delete.
func TestQueryLogClearThroughWriterRemovesRows(t *testing.T) {
	s := newQueryLogTestService(t)
	seedQueryLog(t, s, queryLogTestItem(time.Now(), "clear.example.", "DIRECT", 1, 0, false))
	if got := len(s.SearchPage(QueryLogFilter{}).Items); got != 1 {
		t.Fatalf("seeded %d rows, want 1", got)
	}
	if err := s.Clear(); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if got := len(s.SearchPage(QueryLogFilter{}).Items); got != 0 {
		t.Fatalf("after clear there are %d rows, want 0", got)
	}
	if usage := s.Usage(); usage.Records != 0 {
		t.Fatalf("usage still reports %d records", usage.Records)
	}
}

func TestQueryLogCleanupDropsExpiredRetention(t *testing.T) {
	s := newQueryLogTestService(t)
	now := time.Now()
	seedQueryLog(t, s,
		queryLogTestItem(now.Add(-10*24*time.Hour), "expired.example.", "DIRECT", 1, 0, false),
		queryLogTestItem(now.Add(-time.Hour), "kept.example.", "DIRECT", 1, 0, false),
	)
	s.cleanup()
	got := domainsOf(s.SearchPage(QueryLogFilter{}).Items)
	if len(got) != 1 || got[0] != "kept.example." {
		t.Fatalf("after retention cleanup got %v, want [kept.example.]", got)
	}
}

// max_mb is a promise about live records, not the bbolt file, which does not shrink
// on delete. The purge must trim oldest-first down to the soft target.
func TestQueryLogCleanupEnforcesSizeCap(t *testing.T) {
	s := newQueryLogTestService(t)
	if err := s.SaveSettings(QueryLogSettings{Enabled: true, RetentionDays: 7, MaxMB: 1}); err != nil {
		t.Fatalf("save settings: %v", err)
	}
	base := time.Now().Add(-time.Hour)
	const records = 6000
	batch := make([]database.QueryLogItem, 0, 500)
	for i := 0; i < records; i++ {
		batch = append(batch, queryLogTestItem(base.Add(time.Duration(i)*time.Millisecond), "size-cap.example.", "DIRECT", 1, 0, false))
		if len(batch) == cap(batch) {
			seedQueryLog(t, s, batch...)
			batch = batch[:0]
		}
	}
	seedQueryLog(t, s, batch...)

	before := s.Usage()
	if before.Bytes <= 1<<20 {
		t.Fatalf("test data is only %d bytes, expected to exceed the 1 MB cap", before.Bytes)
	}
	s.cleanup()
	after := s.Usage()
	if after.Bytes > 1<<20 {
		t.Fatalf("after cleanup usage is %d bytes, still over the 1 MB cap", after.Bytes)
	}
	if after.Records >= before.Records {
		t.Fatalf("cleanup did not delete anything (%d -> %d records)", before.Records, after.Records)
	}
}
