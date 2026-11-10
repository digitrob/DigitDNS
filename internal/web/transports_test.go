package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"digitdns/internal/database"
)

func getTransportSettings(t *testing.T, ws *WebServer) transportSettingsView {
	t.Helper()
	w := httptest.NewRecorder()
	ws.handleTransportSettings(w, httptest.NewRequest(http.MethodGet, "/api/settings/transports", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("GET transports = %d: %s", w.Code, w.Body.String())
	}
	var got transportSettingsView
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("GET transports JSON: %v", err)
	}
	return got
}

func postTransportSettings(t *testing.T, ws *WebServer, body string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	ws.handleTransportSettings(w, httptest.NewRequest(http.MethodPost, "/api/settings/transports", strings.NewReader(body)))
	return w
}

func TestTransportSettingsDefaultsAndRoundTrip(t *testing.T) {
	ws, db, cleanup := setupTestWebServer(t)
	defer cleanup()

	initial := getTransportSettings(t, ws)
	if initial.DoQEnabled || initial.DoH3Enabled {
		t.Errorf("QUIC transports should default off: %#v", initial)
	}
	if initial.DoQPort != 853 || initial.DoQAuth != "ip" {
		t.Errorf("defaults = port %d auth %q, want 853/ip", initial.DoQPort, initial.DoQAuth)
	}
	if len(initial.DoQTokens) != 0 {
		t.Errorf("default tokens = %#v, want empty", initial.DoQTokens)
	}

	body := `{"doq_enabled":true,"doq_port":853,"doq_auth":"sni-token","doq_token_domain":"doq.example.com","doq_tokens":["Alpha"," beta ","alpha"],"doh3_enabled":true}`
	w := postTransportSettings(t, ws, body)
	if w.Code != http.StatusOK {
		t.Fatalf("POST transports = %d: %s", w.Code, w.Body.String())
	}

	var saved database.DNSSettings
	if err := db.GetSetting("dns", &saved); err != nil {
		t.Fatalf("GetSetting(dns): %v", err)
	}
	if !saved.DoQEnabled || !saved.DoH3Enabled || saved.DoQPort != 853 || saved.DoQAuth != "sni-token" || saved.DoQTokenDomain != "doq.example.com" {
		t.Errorf("saved DNS settings = %#v", saved)
	}
	acc := ws.loadAccessConfig()
	if len(acc.DoQTokens) != 2 || acc.DoQTokens[0] != "alpha" || acc.DoQTokens[1] != "beta" {
		t.Errorf("saved DoQ tokens = %#v, want [alpha beta]", acc.DoQTokens)
	}

	got := getTransportSettings(t, ws)
	if !got.DoQEnabled || !got.DoH3Enabled || got.DoQAuth != "sni-token" || got.DoQTokenDomain != "doq.example.com" {
		t.Errorf("GET after save = %#v", got)
	}
	if len(got.DoQTokens) != 2 {
		t.Errorf("GET tokens = %#v", got.DoQTokens)
	}

	// A partial save must not reset the fields it omits, including the tokens.
	w = postTransportSettings(t, ws, `{"doh3_enabled":false}`)
	if w.Code != http.StatusOK {
		t.Fatalf("partial POST = %d: %s", w.Code, w.Body.String())
	}
	got = getTransportSettings(t, ws)
	if !got.DoQEnabled || got.DoH3Enabled || got.DoQAuth != "sni-token" || len(got.DoQTokens) != 2 {
		t.Errorf("partial save lost state: %#v", got)
	}
}

func TestTransportSettingsApplier(t *testing.T) {
	ws, _, cleanup := setupTestWebServer(t)
	defer cleanup()

	var gotCfg database.DNSSettings
	var gotTokens []string
	ws.SetTransportApplier(func(cfg database.DNSSettings, tokens []string) error {
		gotCfg, gotTokens = cfg, tokens
		return nil
	})
	w := postTransportSettings(t, ws, `{"doq_enabled":true,"doq_port":8853,"doh3_enabled":true}`)
	if w.Code != http.StatusOK {
		t.Fatalf("POST = %d: %s", w.Code, w.Body.String())
	}
	if !gotCfg.DoQEnabled || !gotCfg.DoH3Enabled || gotCfg.DoQPort != 8853 {
		t.Errorf("applier config = %#v", gotCfg)
	}
	if gotTokens == nil {
		t.Errorf("applier tokens = nil, want an empty slice")
	}

	w = postTransportSettings(t, ws, `{"doq_auth":"sni-token","doq_token_domain":"doq.example.com","doq_tokens":["alpha"]}`)
	if w.Code != http.StatusOK {
		t.Fatalf("POST sni-token = %d: %s", w.Code, w.Body.String())
	}
	if len(gotTokens) != 1 || gotTokens[0] != "alpha" {
		t.Errorf("applier tokens after sni-token save = %#v", gotTokens)
	}

	ws.SetTransportApplier(func(database.DNSSettings, []string) error {
		return errors.New("bind failed")
	})
	w = postTransportSettings(t, ws, `{"doq_port":9853,"doh3_enabled":false}`)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("bind failure = %d, want 500: %s", w.Code, w.Body.String())
	}
	// A failed apply must not leave the saved switches ahead of the listeners.
	got := getTransportSettings(t, ws)
	if !got.DoQEnabled || got.DoQPort != 8853 || !got.DoH3Enabled {
		t.Errorf("state after bind failure = %#v, want the previous applied state", got)
	}
	if !strings.Contains(got.LastError, "bind failed") {
		t.Errorf("last_error = %q", got.LastError)
	}

	ws.SetTransportApplier(func(database.DNSSettings, []string) error { return nil })
	w = postTransportSettings(t, ws, `{"doh3_enabled":true}`)
	if w.Code != http.StatusOK {
		t.Fatalf("recovery POST = %d: %s", w.Code, w.Body.String())
	}
	if got := getTransportSettings(t, ws); got.LastError != "" {
		t.Errorf("last_error was not cleared: %q", got.LastError)
	}
}

func TestTransportSettingsValidation(t *testing.T) {
	ws, _, cleanup := setupTestWebServer(t)
	defer cleanup()

	for _, body := range []string{
		`{"doq_port":70000}`,
		`{"doq_auth":"token"}`,
		`{"doq_auth":"sni-token","doq_tokens":["alpha"]}`,
		`{"doq_auth":"sni-token","doq_token_domain":"doq.example.com","doq_tokens":[]}`,
		`{"doq_auth":"sni-token","doq_token_domain":"not a domain","doq_tokens":["alpha"]}`,
		`{"doq_tokens":["-bad"]}`,
	} {
		w := postTransportSettings(t, ws, body)
		if w.Code != http.StatusBadRequest {
			t.Errorf("body %s = %d, want 400: %s", body, w.Code, w.Body.String())
		}
	}
}

func TestTransportViewSurfacesListenerFailure(t *testing.T) {
	ws, _, cleanup := setupTestWebServer(t)
	defer cleanup()

	ws.SetTransportStatusSource(func() (bool, bool, string) {
		return false, true, "DoH3 listener stopped: boom"
	})
	got := getTransportSettings(t, ws)
	if got.LastError != "DoH3 listener stopped: boom" {
		t.Errorf("last_error = %q, want the listener failure", got.LastError)
	}
	if !got.DoQListening || got.DoH3Listening {
		t.Errorf("listener state = doh3 %v doq %v", got.DoH3Listening, got.DoQListening)
	}

	// A save failure is more specific and must win over a stale listener error.
	ws.setTransportLastError("apply failed: bind refused")
	if got := getTransportSettings(t, ws); got.LastError != "apply failed: bind refused" {
		t.Errorf("last_error = %q, want the save failure", got.LastError)
	}
}

func TestTransportViewExplainsIPAllowlistSources(t *testing.T) {
	ws, _, cleanup := setupTestWebServer(t)
	defer cleanup()

	// The harness starts in Public Mode, which is exactly the state the card
	// must warn about.
	got := getTransportSettings(t, ws)
	if !got.IPAllowlist.AllowAll {
		t.Fatalf("initial allow_all = %v, want public mode", got.IPAllowlist.AllowAll)
	}

	home, err := ws.clients.CreateClient("Home", 30, "203.0.113.10")
	if err != nil {
		t.Fatalf("create home client: %v", err)
	}
	office, err := ws.clients.CreateClient("Office", 30, "198.51.100.0/24")
	if err != nil {
		t.Fatalf("create office client: %v", err)
	}
	if _, err := ws.clients.SetClientAllowedIPs(office.ID, []string{"198.51.100.0/24", "198.51.100.7"}); err != nil {
		t.Fatalf("set office IPs: %v", err)
	}
	if _, err := ws.clients.SetResolverAllowCIDRs([]string{"192.0.2.0/24"}); err != nil {
		t.Fatalf("set resolver allowlist: %v", err)
	}
	if err := ws.clients.SetAllowAll(false); err != nil {
		t.Fatalf("enable whitelist mode: %v", err)
	}

	got = getTransportSettings(t, ws)
	al := got.IPAllowlist
	if al.AllowAll {
		t.Error("allow_all was not cleared")
	}
	if al.ClientAccounts != 2 {
		t.Errorf("client accounts = %d, want 2 (home %s, office %s)", al.ClientAccounts, home.ID, office.ID)
	}
	if al.ClientEntries != 3 {
		t.Errorf("client entries = %d, want 3 (home exact + office CIDR and exact)", al.ClientEntries)
	}
	if al.ResolverEntries != 1 {
		t.Errorf("resolver entries = %d, want 1", al.ResolverEntries)
	}
}

func TestAccessConfigPreservesDoQTokens(t *testing.T) {
	ws, db, cleanup := setupTestWebServer(t)
	defer cleanup()

	if err := db.SetSetting("access", map[string]any{
		"doh_tokens":  []string{"old"},
		"doq_tokens":  []string{"alpha"},
		"allowed_ips": []string{},
	}); err != nil {
		t.Fatalf("seed access: %v", err)
	}

	w := httptest.NewRecorder()
	ws.handleConfigAccess(w, httptest.NewRequest(http.MethodPost, "/api/config/access",
		strings.NewReader(`{"doh_tokens":["new"],"allowed_ips":[]}`)))
	if w.Code != http.StatusOK {
		t.Fatalf("POST access = %d: %s", w.Code, w.Body.String())
	}

	acc := ws.loadAccessConfig()
	if len(acc.DoHTokens) != 1 || acc.DoHTokens[0] != "new" {
		t.Errorf("DoH tokens = %#v, want [new]", acc.DoHTokens)
	}
	if len(acc.DoQTokens) != 1 || acc.DoQTokens[0] != "alpha" {
		t.Errorf("DoQ tokens were not preserved: %#v", acc.DoQTokens)
	}
}
