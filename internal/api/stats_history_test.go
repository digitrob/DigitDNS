package api

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"digitdns/internal/aggregate"
	"digitdns/internal/database"
)

func TestV3HistoricalStatsEndpoints(t *testing.T) {
	apiInst, _, _, cleanup := setupTestAPI(t)
	defer cleanup()

	now := time.Now().UTC()
	agg, err := aggregate.Open(filepath.Join(t.TempDir(), "stats.db"), aggregate.Config{
		FlushInterval: time.Hour,
		CacheTTL:      -1,
		Now:           func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("aggregate.Open: %v", err)
	}
	apiInst.stats.SetAggregator(agg)

	agg.RecordTotal(now)
	agg.RecordDetail(database.QueryLogItem{
		Timestamp: now, ClientIP: "192.0.2.20", AccountName: "Alice",
		Protocol: "UDP", Domain: "history.example.", Action: "DIRECT",
		QType: 1, Rcode: 0, Upstream: "1.1.1.1:53", LatencyMs: 8,
	})
	agg.RecordTotal(now)
	agg.RecordDetail(database.QueryLogItem{
		Timestamp: now, ClientIP: "192.0.2.20", AccountName: "Alice",
		Protocol: "UDP", Domain: "ads.example.", Action: "BLOCK",
		QType: 1, Rcode: 0, LatencyMs: 2,
	})

	mux := http.NewServeMux()
	apiInst.RegisterRoutes(mux)

	w := v3Request(mux, http.MethodGet, "/api/v3/stats/summary?range=24h", testKey, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("summary = %d: %s", w.Code, w.Body.String())
	}
	var summary aggregate.Summary
	if err := json.Unmarshal(w.Body.Bytes(), &summary); err != nil {
		t.Fatalf("summary JSON: %v", err)
	}
	if summary.Range != "24h" || summary.Queries != 2 || summary.Blocked != 1 {
		t.Errorf("summary = %#v", summary)
	}
	if summary.Buckets != 24 || summary.ActiveClients != 1 {
		t.Errorf("summary bucket/client counts = %d/%d", summary.Buckets, summary.ActiveClients)
	}

	w = v3Request(mux, http.MethodGet, "/api/v3/stats/series?range=24h&interval=hour", testKey, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("series = %d: %s", w.Code, w.Body.String())
	}
	var series aggregate.Series
	if err := json.Unmarshal(w.Body.Bytes(), &series); err != nil {
		t.Fatalf("series JSON: %v", err)
	}
	if series.Interval != "hour" || len(series.Points) != 24 {
		t.Errorf("series = %#v", series)
	}

	w = v3Request(mux, http.MethodGet, "/api/v3/stats/top?range=24h&dimension=domain&limit=5", testKey, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("top = %d: %s", w.Code, w.Body.String())
	}
	var top aggregate.Top
	if err := json.Unmarshal(w.Body.Bytes(), &top); err != nil {
		t.Fatalf("top JSON: %v", err)
	}
	if top.Dimension != "domain" || len(top.Items) != 2 {
		t.Errorf("top = %#v", top)
	}

	w = v3Request(mux, http.MethodGet, "/api/v3/stats/summary?range=2h", testKey, nil)
	if w.Code != http.StatusBadRequest {
		t.Errorf("invalid range = %d, want 400", w.Code)
	}
	w = v3Request(mux, http.MethodGet, "/api/v3/stats/top?range=24h&dimension=nope", testKey, nil)
	if w.Code != http.StatusBadRequest {
		t.Errorf("invalid dimension = %d, want 400", w.Code)
	}
	w = v3Request(mux, http.MethodGet, "/api/v3/stats/top?range=24h&limit=0", testKey, nil)
	if w.Code != http.StatusBadRequest {
		t.Errorf("invalid limit = %d, want 400", w.Code)
	}
}

func TestV3HistoricalStatsUnavailable(t *testing.T) {
	apiInst, _, _, cleanup := setupTestAPI(t)
	defer cleanup()
	mux := http.NewServeMux()
	apiInst.RegisterRoutes(mux)

	w := v3Request(mux, http.MethodGet, "/api/v3/stats/summary?range=24h", testKey, nil)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("summary without aggregate = %d, want 503", w.Code)
	}
}

func TestV3HistoricalStatsExport(t *testing.T) {
	apiInst, _, _, cleanup := setupTestAPI(t)
	defer cleanup()
	now := time.Now().UTC()
	agg, err := aggregate.Open(filepath.Join(t.TempDir(), "stats.db"), aggregate.Config{
		FlushInterval: time.Hour, CacheTTL: -1, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("aggregate.Open: %v", err)
	}
	apiInst.stats.SetAggregator(agg)
	agg.RecordTotal(now)
	agg.RecordDetail(database.QueryLogItem{Timestamp: now, ClientIP: "192.0.2.30", Domain: "export.example.", Action: "DIRECT", QType: 1, Protocol: "UDP"})
	mux := http.NewServeMux()
	apiInst.RegisterRoutes(mux)

	w := v3Request(mux, http.MethodGet, "/api/v3/stats/export?range=24h&format=csv", testKey, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("csv export = %d: %s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/csv") {
		t.Errorf("csv content-type = %q", ct)
	}
	if !strings.Contains(w.Body.String(), "time,queries,blocked") {
		t.Errorf("csv body = %q", w.Body.String())
	}
	w = v3Request(mux, http.MethodGet, "/api/v3/stats/export?range=24h&format=json", testKey, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("json export = %d: %s", w.Code, w.Body.String())
	}
	var doc map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &doc); err != nil {
		t.Fatalf("json export: %v", err)
	}
	if doc["summary"] == nil || doc["series"] == nil || doc["tops"] == nil {
		t.Errorf("json export missing sections: %#v", doc)
	}
	w = v3Request(mux, http.MethodGet, "/api/v3/stats/export?range=24h&format=xml", testKey, nil)
	if w.Code != http.StatusBadRequest {
		t.Errorf("invalid export format = %d, want 400", w.Code)
	}
}

func TestV3StatsSettingsRoundTripAndValidation(t *testing.T) {
	apiInst, db, _, cleanup := setupTestAPI(t)
	defer cleanup()
	mux := http.NewServeMux()
	apiInst.RegisterRoutes(mux)

	w := v3Request(mux, http.MethodGet, "/api/v3/stats/settings", testKey, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("GET settings = %d: %s", w.Code, w.Body.String())
	}
	var initial database.StatsSettings
	if err := json.Unmarshal(w.Body.Bytes(), &initial); err != nil {
		t.Fatalf("GET settings JSON: %v", err)
	}
	if !initial.Enabled || initial.RetentionDays != 90 || initial.TopDomains != 1000 || initial.TopClients != 500 {
		t.Errorf("initial settings = %#v", initial)
	}

	body := map[string]any{
		"enabled": false, "retention_days": 30, "top_domains": 250,
		"top_clients": 100, "anonymize_clients": true,
		"exclude_clients": []string{"192.0.2.9", " 192.0.2.9 ", "Alice"},
	}
	w = v3Request(mux, http.MethodPost, "/api/v3/stats/settings", testKey, body)
	if w.Code != http.StatusOK {
		t.Fatalf("POST settings = %d: %s", w.Code, w.Body.String())
	}
	var savedResponse struct {
		Success         bool                   `json:"success"`
		RestartRequired bool                   `json:"restart_required"`
		Settings        database.StatsSettings `json:"settings"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &savedResponse); err != nil {
		t.Fatalf("POST settings JSON: %v", err)
	}
	if !savedResponse.Success || !savedResponse.RestartRequired {
		t.Errorf("save response = %#v", savedResponse)
	}
	saved, err := db.LoadStatsSettings()
	if err != nil {
		t.Fatalf("LoadStatsSettings: %v", err)
	}
	if saved.Enabled || saved.RetentionDays != 30 || saved.TopDomains != 250 || saved.TopClients != 100 || !saved.AnonymizeClients {
		t.Errorf("saved settings = %#v", saved)
	}
	if len(saved.ExcludeClients) != 2 || saved.ExcludeClients[0] != "192.0.2.9" || saved.ExcludeClients[1] != "Alice" {
		t.Errorf("exclude_clients = %#v", saved.ExcludeClients)
	}

	for _, bad := range []map[string]any{
		{"retention_days": 3, "top_domains": 250, "top_clients": 100},
		{"retention_days": 30, "top_domains": 10, "top_clients": 100},
		{"retention_days": 30, "top_domains": 250, "top_clients": 10},
	} {
		w = v3Request(mux, http.MethodPost, "/api/v3/stats/settings", testKey, bad)
		if w.Code != http.StatusBadRequest {
			t.Errorf("invalid settings %v = %d, want 400", bad, w.Code)
		}
	}
}

func TestV3TopClientsResolveNamesFromClientRecords(t *testing.T) {
	apiInst, _, _, cleanup := setupTestAPI(t)
	defer cleanup()

	if _, err := apiInst.clients.CreateClient("Alice", 30, "192.0.2.77"); err != nil {
		t.Fatalf("CreateClient: %v", err)
	}
	now := time.Now().UTC()
	agg, err := aggregate.Open(filepath.Join(t.TempDir(), "stats.db"), aggregate.Config{
		FlushInterval: time.Hour, CacheTTL: -1, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("aggregate.Open: %v", err)
	}
	apiInst.stats.SetAggregator(agg)
	agg.RecordDetail(database.QueryLogItem{
		Timestamp: now, ClientIP: "192.0.2.77", AccountName: "Alice",
		Domain: "named.example.", Action: "DIRECT", QType: 1, Protocol: "UDP",
	})

	mux := http.NewServeMux()
	apiInst.RegisterRoutes(mux)
	w := v3Request(mux, http.MethodGet, "/api/v3/stats/top?range=24h&dimension=client&limit=10", testKey, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("top clients = %d: %s", w.Code, w.Body.String())
	}
	var top aggregate.Top
	if err := json.Unmarshal(w.Body.Bytes(), &top); err != nil {
		t.Fatalf("top JSON: %v", err)
	}
	if len(top.Items) != 1 || top.Items[0].Value != "Alice" || top.Items[0].Count != 1 {
		t.Errorf("top clients = %#v, want Alice/1", top.Items)
	}
}
