package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestConfigExportOmitsSecrets(t *testing.T) {
	ws, _, cleanup := setupTestWebServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/config/export", nil)
	rec := httptest.NewRecorder()
	ws.handleConfigExport(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var export map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &export); err != nil {
		t.Fatalf("decode export: %v", err)
	}
	server, _ := export["server"].(map[string]any)
	for _, key := range []string{"api_key", "admin_password", "admin_path"} {
		if _, ok := server[key]; ok {
			t.Errorf("export includes server.%s", key)
		}
	}
	access, _ := export["access"].(map[string]any)
	for _, key := range []string{"doh_tokens", "doq_tokens"} {
		if _, ok := access[key]; ok {
			t.Errorf("export includes access.%s", key)
		}
	}
	for _, key := range []string{"version", "dns", "rules", "filtering", "filter_lists", "subscription"} {
		if _, ok := export[key]; !ok {
			t.Errorf("export is missing %q", key)
		}
	}
}

func TestConfigExportRejectsNonGet(t *testing.T) {
	ws, _, cleanup := setupTestWebServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPost, "/api/config/export", nil)
	rec := httptest.NewRecorder()
	ws.handleConfigExport(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
}
