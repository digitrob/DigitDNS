package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"digitdns/internal/core/resolution"
	"digitdns/internal/database"
)

// A record written before the advanced object existed must still read as the
// recommended defaults rather than a disabled feature.
func TestAdvancedSettingsDefaults(t *testing.T) {
	ws, _, cleanup := setupTestWebServer(t)
	defer cleanup()

	rec := httptest.NewRecorder()
	ws.handleResolutionSettings(rec, httptest.NewRequest(http.MethodGet, "/api/settings/resolution", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET: %d %s", rec.Code, rec.Body.String())
	}
	var view resolutionView
	if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil {
		t.Fatalf("decode: %v", err)
	}
	a := view.Advanced
	if a.KeepWarm != resolution.KeepWarmActiveNext {
		t.Errorf("keep_warm = %q, want %q", a.KeepWarm, resolution.KeepWarmActiveNext)
	}
	if a.KeepWarmSeconds != database.DefaultKeepWarmSeconds {
		t.Errorf("keep_warm_seconds = %d", a.KeepWarmSeconds)
	}
	if a.ProbationSeconds != database.DefaultCircuitCooldownSeconds {
		t.Errorf("probation_seconds = %d", a.ProbationSeconds)
	}
	if a.Hedge != resolution.HedgeBalanced {
		t.Errorf("hedge = %q, want balanced", a.Hedge)
	}
	if !a.WarmCacheOnStart {
		t.Error("warm_cache_on_start defaulted off")
	}
	if a.WarmCacheTop != database.DefaultWarmCacheTop {
		t.Errorf("warm_cache_top = %d", a.WarmCacheTop)
	}
	if len(a.BootstrapDNS) != 0 || len(a.FallbackDNS) != 0 {
		t.Errorf("defaults carry servers: %+v", a)
	}
}

func TestAdvancedSettingsSaveAppliesAndPersists(t *testing.T) {
	ws, db, cleanup := setupTestWebServer(t)
	defer cleanup()

	post := func(body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/settings/resolution", strings.NewReader(body))
		ws.handleResolutionSettings(rec, req)
		return rec
	}
	body := `{"transport":"any","default_strategy":"adaptive","profiles":{},"advanced":{` +
		`"bootstrap_dns":["1.1.1.1","9.9.9.9:53"],` +
		`"fallback_dns":["8.8.8.8:53"],` +
		`"keep_warm":"all","keep_warm_seconds":45,"hedge":"aggressive","probation_seconds":60,` +
		`"warm_cache_on_start":false,"warm_cache_top":25}}`
	rec := post(body)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST: %d %s", rec.Code, rec.Body.String())
	}
	var view resolutionView
	if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !view.RestartRequired {
		t.Error("changing bootstrap did not report restart_required")
	}
	if view.Advanced.KeepWarm != resolution.KeepWarmAll || view.Advanced.KeepWarmSeconds != 45 {
		t.Errorf("applied view = %+v", view.Advanced)
	}
	if view.Advanced.Hedge != resolution.HedgeAggressive {
		t.Errorf("hedge = %q, want aggressive", view.Advanced.Hedge)
	}
	if got := view.Advanced.BootstrapDNS; len(got) != 2 || got[0] != "1.1.1.1:53" || got[1] != "9.9.9.9:53" {
		t.Errorf("bootstrap = %v", got)
	}
	if view.Advanced.WarmCacheOnStart {
		t.Error("warm_cache_on_start stayed on")
	}

	var stored database.DNSSettings
	if err := db.GetSetting("dns", &stored); err != nil {
		t.Fatalf("read stored: %v", err)
	}
	if stored.KeepWarmMode != resolution.KeepWarmAll || stored.KeepWarmSeconds != 45 {
		t.Errorf("stored keep-warm = %q/%d", stored.KeepWarmMode, stored.KeepWarmSeconds)
	}
	if stored.HedgeMode != resolution.HedgeAggressive {
		t.Errorf("stored hedge = %q", stored.HedgeMode)
	}
	if !stored.WarmCacheDisabled || stored.WarmCacheTop != 25 {
		t.Errorf("stored cache warm = disabled:%v top:%d", stored.WarmCacheDisabled, stored.WarmCacheTop)
	}
	if len(stored.FallbackDNS) != 1 || stored.FallbackDNS[0] != "8.8.8.8:53" {
		t.Errorf("stored fallback = %v", stored.FallbackDNS)
	}
	if !strings.Contains(rec.Body.String(), `"keep_warm":"all"`) {
		t.Errorf("response did not round-trip the advanced object: %s", rec.Body.String())
	}
}

func TestAdvancedSettingsRejectsBadInput(t *testing.T) {
	ws, _, cleanup := setupTestWebServer(t)
	defer cleanup()

	cases := map[string]string{
		"encrypted bootstrap": `{"transport":"any","default_strategy":"adaptive","profiles":{},"advanced":{"bootstrap_dns":["tls://dns.google"]}}`,
		"bad keep-warm":       `{"transport":"any","default_strategy":"adaptive","profiles":{},"advanced":{"keep_warm":"sometimes"}}`,
		"bad hedge":           `{"transport":"any","default_strategy":"adaptive","profiles":{},"advanced":{"hedge":"sometimes"}}`,
		"interval too small":  `{"transport":"any","default_strategy":"adaptive","profiles":{},"advanced":{"keep_warm_seconds":1}}`,
		"probation too big":   `{"transport":"any","default_strategy":"adaptive","profiles":{},"advanced":{"probation_seconds":900}}`,
		"warm top too big":    `{"transport":"any","default_strategy":"adaptive","profiles":{},"advanced":{"warm_cache_top":101}}`,
		"bad fallback":        `{"transport":"any","default_strategy":"adaptive","profiles":{},"advanced":{"fallback_dns":["https://dns.google/dns-query"]}}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/settings/resolution", strings.NewReader(body))
			ws.handleResolutionSettings(rec, req)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

// Omitting the advanced object keeps the stored maintenance policy: an older
// client saving a strategy chip must not reset keep-warm or the fallback pool.
func TestAdvancedSettingsOmittedKeepsStoredValues(t *testing.T) {
	ws, _, cleanup := setupTestWebServer(t)
	defer cleanup()

	post := func(body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/settings/resolution", strings.NewReader(body))
		ws.handleResolutionSettings(rec, req)
		return rec
	}
	if rec := post(`{"transport":"any","default_strategy":"adaptive","profiles":{},"advanced":{"keep_warm":"off"}}`); rec.Code != http.StatusOK {
		t.Fatalf("first save: %d %s", rec.Code, rec.Body.String())
	}
	rec := post(`{"transport":"any","default_strategy":"sequential","profiles":{}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("second save: %d %s", rec.Code, rec.Body.String())
	}
	var view resolutionView
	if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if view.Advanced.KeepWarm != resolution.KeepWarmOff {
		t.Fatalf("keep_warm = %q after an omitting save", view.Advanced.KeepWarm)
	}
	if view.RestartRequired {
		t.Fatal("an omitting save reported a restart requirement")
	}
}
