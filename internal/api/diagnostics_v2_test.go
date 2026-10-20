package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"digitdns/internal/database"
)

// The v2 diagnostics route used to answer a hardcoded {"ok":true}. v3.1.0 makes
// it run against the operator's configured targets, through the same runner the
// dashboard uses — so the two surfaces cannot disagree on what a reachable target
// or a score means.
func TestV2DiagnosticsRunsConfiguredTargets(t *testing.T) {
	apiInst, db, _, cleanup := setupTestAPI(t)
	defer cleanup()

	// A closed loopback port: refused instantly, so no internet or timeout.
	if err := db.SetSetting("diagnostics", &database.DiagnosticSettings{
		Targets: []database.DiagnosticTarget{{Name: "Closed", Host: "127.0.0.1:1"}},
	}); err != nil {
		t.Fatalf("seeding targets: %v", err)
	}

	mux := http.NewServeMux()
	apiInst.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/v2/diagnostics", nil)
	req.Header.Set("X-API-Key", "digitdns_live_testkey123")
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("v2 diagnostics = %d, want 200 — %s", w.Code, w.Body.String())
	}
	var report struct {
		Total     int `json:"total"`
		Reachable int `json:"reachable"`
		Results   []struct {
			Name   string `json:"name"`
			Target string `json:"target"`
		} `json:"results"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &report); err != nil {
		t.Fatalf("decoding v2 diagnostics: %v", err)
	}
	if report.Total != 1 || len(report.Results) != 1 || report.Results[0].Name != "Closed" {
		t.Fatalf("v2 diagnostics = %+v, want the single configured target", report)
	}
	if report.Reachable != 0 {
		t.Fatalf("reachable = %d, want 0 for a closed port", report.Reachable)
	}
}

func TestV2DiagnosticsEmptyList(t *testing.T) {
	apiInst, _, _, cleanup := setupTestAPI(t)
	defer cleanup()

	mux := http.NewServeMux()
	apiInst.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodPost, "/api/v2/diagnostics", nil)
	req.Header.Set("X-API-Key", "digitdns_live_testkey123")
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("empty v2 diagnostics = %d, want 200 — %s", w.Code, w.Body.String())
	}
	var report struct {
		Total   int   `json:"total"`
		Results []any `json:"results"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &report); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if report.Total != 0 || len(report.Results) != 0 {
		t.Fatalf("empty run = %d total / %d results, want none", report.Total, len(report.Results))
	}
}

func TestV2DiagnosticsRejectsGet(t *testing.T) {
	apiInst, _, _, cleanup := setupTestAPI(t)
	defer cleanup()

	mux := http.NewServeMux()
	apiInst.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v2/diagnostics", nil)
	req.Header.Set("X-API-Key", "digitdns_live_testkey123")
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET v2 diagnostics = %d, want 405", w.Code)
	}
}
