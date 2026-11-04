package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"digitdns/internal/database"
)

func TestStatsSettingsRoundTripAndValidation(t *testing.T) {
	ws, db, cleanup := setupTestWebServer(t)
	defer cleanup()

	w := httptest.NewRecorder()
	ws.handleStatsSettings(w, httptest.NewRequest(http.MethodGet, "/api/stats/settings", nil))
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

	body := `{"enabled":false,"retention_days":30,"top_domains":250,"top_clients":100,"anonymize_clients":true,"exclude_clients":["192.0.2.9"," 192.0.2.9 ","Alice"]}`
	w = httptest.NewRecorder()
	ws.handleStatsSettings(w, httptest.NewRequest(http.MethodPost, "/api/stats/settings", strings.NewReader(body)))
	if w.Code != http.StatusOK {
		t.Fatalf("POST settings = %d: %s", w.Code, w.Body.String())
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

	for _, bad := range []string{
		`{"enabled":true,"retention_days":3,"top_domains":250,"top_clients":100}`,
		`{"enabled":true,"retention_days":30,"top_domains":10,"top_clients":100}`,
		`{"enabled":true,"retention_days":30,"top_domains":250,"top_clients":10}`,
	} {
		w = httptest.NewRecorder()
		ws.handleStatsSettings(w, httptest.NewRequest(http.MethodPost, "/api/stats/settings", strings.NewReader(bad)))
		if w.Code != http.StatusBadRequest {
			t.Errorf("invalid settings %s = %d, want 400", bad, w.Code)
		}
	}
}
