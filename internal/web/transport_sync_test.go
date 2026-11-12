package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"digitdns/internal/database"
)

// TestResolutionChipsDriveClientTransports proves the Upstream Resolvers
// transport chips and the Client Transports switches are one policy: a chip
// save updates the listener flags and hands the applier the new DNSSettings.
func TestResolutionChipsDriveClientTransports(t *testing.T) {
	ws, db, cleanup := setupTestWebServer(t)
	defer cleanup()

	var applied database.DNSSettings
	ws.SetTransportApplier(func(cfg database.DNSSettings, _ []string) error {
		applied = cfg
		return nil
	})

	req := httptest.NewRequest(http.MethodPost, "/api/settings/resolution", strings.NewReader(`{"transports":["dot"]}`))
	rec := httptest.NewRecorder()
	ws.handleResolutionSettings(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("resolution save = %d: %s", rec.Code, rec.Body.String())
	}

	var saved database.DNSSettings
	if err := db.GetSetting("dns", &saved); err != nil {
		t.Fatalf("GetSetting(dns): %v", err)
	}
	if !saved.PlainDNSDisabled || saved.DoTDisabled || !saved.DoHDisabled || saved.DoQEnabled || saved.DNSCryptEnabled {
		t.Errorf("saved client flags = %#v", saved)
	}
	if !saved.DoH3Disabled {
		t.Errorf("DoH3 must be off when only DoT is selected: %#v", saved)
	}
	if len(saved.Transports) != 1 || saved.Transports[0] != "dot" {
		t.Errorf("saved transports = %#v, want [dot]", saved.Transports)
	}
	if applied.DoTDisabled || applied.PlainDNSDisabled != true {
		t.Errorf("applier did not receive the chip policy: %#v", applied)
	}
}

// TestClientTransportSwitchesDriveResolutionChips is the reverse direction:
// turning DoH3 on in Settings makes the resolver chips show DoH and DoH3.
func TestClientTransportSwitchesDriveResolutionChips(t *testing.T) {
	ws, db, cleanup := setupTestWebServer(t)
	defer cleanup()

	rec := postTransportSettings(t, ws, `{"doh_enabled":true,"doh3_enabled":true}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("transport save = %d: %s", rec.Code, rec.Body.String())
	}
	var saved database.DNSSettings
	if err := db.GetSetting("dns", &saved); err != nil {
		t.Fatalf("GetSetting(dns): %v", err)
	}
	if saved.DoHDisabled || saved.DoH3Disabled {
		t.Errorf("DoH/DoH3 flags = %#v", saved)
	}
	if !contains(saved.Transports, "doh") || !contains(saved.Transports, "doh3") {
		t.Errorf("saved transports = %#v, want doh and doh3", saved.Transports)
	}

	rec = httptest.NewRecorder()
	ws.handleResolutionSettings(rec, httptest.NewRequest(http.MethodGet, "/api/settings/resolution", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("resolution read = %d", rec.Code)
	}
	if body := rec.Body.String(); !strings.Contains(body, `"doh3"`) || !strings.Contains(body, `"doh"`) {
		t.Errorf("resolution view does not carry the client switches: %s", body)
	}
}

func contains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}
