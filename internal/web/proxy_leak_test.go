package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func callProxyLeak(t *testing.T, ws *WebServer, method, body string) (*httptest.ResponseRecorder, proxyLeakView) {
	t.Helper()
	req := httptest.NewRequest(method, "/api/settings/proxy-leak", strings.NewReader(body))
	rec := httptest.NewRecorder()
	ws.handleProxyLeakSettings(rec, req)
	var view proxyLeakView
	if rec.Body.Len() > 0 {
		_ = json.Unmarshal(rec.Body.Bytes(), &view)
	}
	return rec, view
}

func TestProxyLeakSettingsRoundTrip(t *testing.T) {
	ws, db, cleanup := setupTestWebServer(t)
	defer cleanup()

	seeded := seedDNSRecord(t, db)
	seeded.ProxyLeakProtection = true
	if err := db.SetSetting("dns", seeded); err != nil {
		t.Fatalf("seeding proxy leak protection: %v", err)
	}

	applied := true
	ws.SetProxyLeakProtectionSetter(func(v bool) { applied = v })

	rec, view := callProxyLeak(t, ws, http.MethodGet, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want 200", rec.Code)
	}
	if !view.Enabled || !view.Recommended {
		t.Fatalf("initial view = %+v, want enabled and recommended", view)
	}

	rec, view = callProxyLeak(t, ws, http.MethodPost, `{"enabled":false}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if view.Enabled {
		t.Fatal("saved view still reports enabled")
	}
	if applied {
		t.Fatal("the live resolver setter was not called with false")
	}

	stored := storedDNS(t, db)
	if stored.ProxyLeakProtection {
		t.Fatal("proxy_leak_protection = true after an explicit false save")
	}
	if stored.Port != seeded.Port || stored.CacheSize != seeded.CacheSize || stored.ServeStaleSeconds != seeded.ServeStaleSeconds {
		t.Errorf("saving proxy leak protection rewrote unrelated DNS settings: %+v", stored)
	}

	_, view = callProxyLeak(t, ws, http.MethodGet, "")
	if view.Enabled {
		t.Fatal("GET returned enabled after disabling")
	}
}

func TestProxyLeakSettingsRequiresEnabled(t *testing.T) {
	ws, db, cleanup := setupTestWebServer(t)
	defer cleanup()
	seedDNSRecord(t, db)

	rec, _ := callProxyLeak(t, ws, http.MethodPost, `{}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for a missing enabled field", rec.Code)
	}
}
