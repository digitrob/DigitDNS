package web

// Tests for the server-location endpoints and the country field they feed into
// the settings view.

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"encoding/json"

	"digitdns/internal/database"
	"digitdns/internal/geoip"
)

func geoipFixture(t *testing.T, dir string) {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("..", "geoip", "testdata", "country.mmdb"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, geoip.FileName), body, 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}

func TestGeoIPEndpointsWithoutDatabase(t *testing.T) {
	ws, _, cleanup := setupTestWebServer(t)
	defer cleanup()

	for _, tc := range []struct {
		method, path string
	}{
		{http.MethodGet, "/api/settings/geoip"},
		{http.MethodPost, "/api/settings/geoip/update"},
	} {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		w := httptest.NewRecorder()
		if tc.method == http.MethodGet {
			ws.handleGeoIPStatus(w, req)
		} else {
			ws.handleGeoIPUpdate(w, req)
		}
		if w.Code != http.StatusServiceUnavailable {
			t.Errorf("%s %s = %d, want 503 without a database", tc.method, tc.path, w.Code)
		}
	}
}

func TestGeoIPStatusAndCountryInSettingsView(t *testing.T) {
	ws, _, cleanup := setupTestWebServer(t)
	defer cleanup()

	dir := t.TempDir()
	geoipFixture(t, dir)
	service := geoip.New(dir, "")
	if err := service.Load(); err != nil {
		t.Fatalf("load fixture: %v", err)
	}
	ws.SetGeoIP(service)
	if err := ws.settings.UpdateAndPersist(func(m *database.MutableSettings) {
		m.PublicIP = "203.0.113.5"
	}, nil); err != nil {
		t.Fatalf("set public IP: %v", err)
	}

	w := httptest.NewRecorder()
	ws.handleGeoIPStatus(w, httptest.NewRequest(http.MethodGet, "/api/settings/geoip", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if body["country"] != "NL" {
		t.Errorf("country = %v, want NL", body["country"])
	}
	if body["available"] != true {
		t.Errorf("available = %v, want true", body["available"])
	}
	if body["database_date"] != "22.10.2026" {
		t.Errorf("database_date = %v, want 22.10.2026", body["database_date"])
	}

	// The same lookup feeds the config settings view, which the dashboard and
	// the Android app read.
	if got := ws.settingsView().Country; got != "NL" {
		t.Errorf("settingsView().Country = %q, want NL", got)
	}
}

func TestGeoIPUpdateHandlerDownloads(t *testing.T) {
	ws, _, cleanup := setupTestWebServer(t)
	defer cleanup()

	body, err := os.ReadFile(filepath.Join("..", "geoip", "testdata", "country.mmdb"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	dir := t.TempDir()
	service := geoip.New(dir, srv.URL)
	ws.SetGeoIP(service)
	if err := ws.settings.UpdateAndPersist(func(m *database.MutableSettings) {
		m.PublicIP = "203.0.113.5"
	}, nil); err != nil {
		t.Fatalf("set public IP: %v", err)
	}

	w := httptest.NewRecorder()
	ws.handleGeoIPUpdate(w, httptest.NewRequest(http.MethodPost, "/api/settings/geoip/update", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("update = %d: %s", w.Code, w.Body.String())
	}
	if _, err := os.Stat(filepath.Join(dir, geoip.FileName)); err != nil {
		t.Errorf("database was not written: %v", err)
	}
	if got := ws.countryOfServer(); got != "NL" {
		t.Errorf("country after update = %q, want NL", got)
	}
}
