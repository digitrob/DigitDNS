package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"digitdns/internal/core/dnssec"
)

func callDNSSEC(t *testing.T, ws *WebServer, method, body string) (*httptest.ResponseRecorder, dnssecView) {
	t.Helper()
	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, "/api/settings/dnssec", reader)
	rec := httptest.NewRecorder()
	ws.handleDNSSECSettings(rec, req)
	var view dnssecView
	if rec.Body.Len() > 0 {
		_ = json.Unmarshal(rec.Body.Bytes(), &view)
	}
	return rec, view
}

func TestDNSSECSettingsRoundTrip(t *testing.T) {
	ws, db, cleanup := setupTestWebServer(t)
	defer cleanup()
	seeded := seedDNSRecord(t, db)
	ws.SetDNSSECStatus(false, nil)

	rec, view := callDNSSEC(t, ws, http.MethodGet, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want 200", rec.Code)
	}
	if view.Enabled || view.Running || view.RestartRequired {
		t.Fatalf("initial view = %+v, want disabled and not running", view)
	}
	if len(view.DefaultAnchors) != 2 {
		t.Fatalf("default anchors = %v, want the two root KSK records", view.DefaultAnchors)
	}
	if view.TrustAnchors == nil {
		t.Fatal("trust_anchors is null; the dashboard expects an array")
	}

	anchor := dnssec.DefaultTrustAnchorRecords()[0]
	body := fmt.Sprintf(`{"enabled":true,"trust_anchors":[%q]}`, anchor)
	rec, view = callDNSSEC(t, ws, http.MethodPost, body)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if !view.Enabled || view.Running || !view.RestartRequired {
		t.Fatalf("saved view = %+v, want enabled, not running, restart required", view)
	}
	if len(view.TrustAnchors) != 1 || view.TrustAnchors[0] != anchor {
		t.Fatalf("saved anchors = %v, want the submitted record", view.TrustAnchors)
	}

	got := storedDNS(t, db)
	if !got.DNSSECValidation || len(got.DNSSECTrustAnchors) != 1 {
		t.Fatalf("stored DNSSEC settings = %+v, want enabled with one anchor", got)
	}
	if got.Port != seeded.Port || got.CacheSize != seeded.CacheSize || got.ServeStaleSeconds != seeded.ServeStaleSeconds {
		t.Errorf("saving DNSSEC rewrote unrelated DNS settings: %+v", got)
	}

	// Simulate the restart the note asks for: the running snapshot catches up.
	ws.SetDNSSECStatus(true, got.DNSSECTrustAnchors)
	_, view = callDNSSEC(t, ws, http.MethodGet, "")
	if view.RestartRequired || !view.Running {
		t.Fatalf("view after restart = %+v, want running with no pending restart", view)
	}
}

func TestDNSSECSettingsRejectsBadAnchor(t *testing.T) {
	ws, db, cleanup := setupTestWebServer(t)
	defer cleanup()
	seedDNSRecord(t, db)

	rec, _ := callDNSSEC(t, ws, http.MethodPost, `{"enabled":true,"trust_anchors":["not a record"]}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body.String())
	}
	got := storedDNS(t, db)
	if got.DNSSECValidation || len(got.DNSSECTrustAnchors) != 0 {
		t.Fatalf("a rejected save changed the record: %+v", got)
	}
}

func TestDNSSECSettingsRejectsNonRootAnchor(t *testing.T) {
	ws, db, cleanup := setupTestWebServer(t)
	defer cleanup()
	seedDNSRecord(t, db)

	rec, _ := callDNSSEC(t, ws, http.MethodPost,
		`{"enabled":true,"trust_anchors":["example.com. 3600 IN DS 1 8 2 AABBCCDD"]}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "root") {
		t.Errorf("error body does not explain the root-only rule: %s", rec.Body.String())
	}
}

func TestDNSSECSettingsSplitsPastedBlock(t *testing.T) {
	ws, db, cleanup := setupTestWebServer(t)
	defer cleanup()
	seedDNSRecord(t, db)

	block := strings.Join(dnssec.DefaultTrustAnchorRecords(), "\n")
	body, err := json.Marshal(map[string]any{"enabled": true, "trust_anchors": []string{block}})
	if err != nil {
		t.Fatal(err)
	}
	rec, view := callDNSSEC(t, ws, http.MethodPost, string(body))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if len(view.TrustAnchors) != 2 {
		t.Fatalf("anchors = %v, want both lines from the pasted block", view.TrustAnchors)
	}
}
