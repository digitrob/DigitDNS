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

func TestEncryptedUpstreamsClassifiesThePool(t *testing.T) {
	list := []string{
		"1.1.1.1:53",
		"tcp://9.9.9.9:53",
		"tls://dns.google:853",
		"https://dns.google/dns-query",
		"quic://dns.adguard.com:853",
		"h3://dns.google/dns-query",
		"sdns://abc",
	}
	want := []string{
		"tls://dns.google:853",
		"https://dns.google/dns-query",
		"quic://dns.adguard.com:853",
		"h3://dns.google/dns-query",
		"sdns://abc",
	}
	got := encryptedUpstreams(list)
	if len(got) != len(want) {
		t.Fatalf("encryptedUpstreams = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("encryptedUpstreams = %v, want %v", got, want)
		}
	}
	if plain := encryptedUpstreams([]string{"1.1.1.1:53"}); len(plain) != 0 {
		t.Fatalf("plain-only pool reported encrypted resolvers: %v", plain)
	}
}

func TestCanonicalUpstreamsMatchThePoolForms(t *testing.T) {
	// The stats list reports the pool's canonical form. A settings record saved
	// before the pool appended the default port must be normalised to the same
	// strings, or the encrypted set and the pickers cannot annotate its rows.
	got := canonicalUpstreams([]string{"tls://one.one.one.one", "1.1.1.1", "https://dns.google/dns-query"})
	want := []string{"tls://one.one.one.one:853", "1.1.1.1:53", "https://dns.google/dns-query"}
	if len(got) != len(want) {
		t.Fatalf("canonicalUpstreams = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("canonicalUpstreams = %v, want %v", got, want)
		}
	}
	if canonicalUpstream("") != "" {
		t.Fatalf("empty default upstream must stay empty")
	}
}

func TestResolutionSaveAcceptsAdaptiveAndNormalizesFastest(t *testing.T) {
	ws, db, cleanup := setupTestWebServer(t)
	defer cleanup()

	post := func(body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/settings/resolution", strings.NewReader(body))
		ws.handleResolutionSettings(rec, req)
		return rec
	}

	rec := post(`{"transport":"any","default_strategy":"adaptive","adaptive_interval_minutes":15,"default_upstream":"","profiles":{}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("adaptive save: %d %s", rec.Code, rec.Body.String())
	}
	var view resolutionView
	if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil {
		t.Fatalf("decode adaptive view: %v", err)
	}
	if view.DefaultStrategy != resolution.StrategyAdaptive || view.AdaptiveInterval != 15 {
		t.Fatalf("adaptive view = %+v", view)
	}

	var stored database.DNSSettings
	if err := db.GetSetting("dns", &stored); err != nil {
		t.Fatalf("read stored dns settings: %v", err)
	}
	if stored.DefaultStrategy != resolution.StrategyAdaptive || stored.AdaptiveIntervalMinutes != 15 {
		t.Fatalf("stored settings = %+v", stored)
	}

	rec = post(`{"transport":"any","default_strategy":"fastest","default_upstream":"","profiles":{}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("legacy fastest save: %d %s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil {
		t.Fatalf("decode legacy view: %v", err)
	}
	if view.DefaultStrategy != resolution.StrategyHedged {
		t.Fatalf("legacy fastest strategy = %q, want %q", view.DefaultStrategy, resolution.StrategyHedged)
	}
}

func TestResolutionSaveAcceptsTransportChips(t *testing.T) {
	ws, db, cleanup := setupTestWebServer(t)
	defer cleanup()

	post := func(body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/settings/resolution", strings.NewReader(body))
		ws.handleResolutionSettings(rec, req)
		return rec
	}

	rec := post(`{"transports":["plain","doh"],"default_strategy":"adaptive","default_upstream":"","profiles":{}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("transport save: %d %s", rec.Code, rec.Body.String())
	}
	var view resolutionView
	if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil {
		t.Fatalf("decode transport view: %v", err)
	}
	if len(view.Transports) != 2 || view.Transports[0] != resolution.ProtocolPlain || view.Transports[1] != resolution.ProtocolDoH {
		t.Fatalf("view transports = %v, want [plain doh]", view.Transports)
	}
	if view.Transport != resolution.TransportAny {
		t.Fatalf("legacy transport = %q, want any for a mixed set", view.Transport)
	}

	var stored database.DNSSettings
	if err := db.GetSetting("dns", &stored); err != nil {
		t.Fatalf("read stored dns settings: %v", err)
	}
	if len(stored.Transports) != 2 || stored.Transports[0] != resolution.ProtocolPlain || stored.Transports[1] != resolution.ProtocolDoH {
		t.Fatalf("stored transports = %v, want [plain doh]", stored.Transports)
	}

	rec = post(`{"transports":[],"default_strategy":"adaptive","default_upstream":"","profiles":{}}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("empty transport save = %d %s, want 400", rec.Code, rec.Body.String())
	}
}

func TestResolutionSaveRejectsAdaptiveProfile(t *testing.T) {
	ws, db, cleanup := setupTestWebServer(t)
	defer cleanup()
	if err := db.SavePolicy(database.Policy{Key: "custom_proxied", CustomDomains: []string{"example.com"}}); err != nil {
		t.Fatalf("seed proxied domain: %v", err)
	}

	body := `{"transport":"any","default_strategy":"hedged","profiles":{"example.com":{"strategy":"adaptive"}}}`
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/settings/resolution", strings.NewReader(body))
	ws.handleResolutionSettings(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("adaptive profile save = %d %s, want 400", rec.Code, rec.Body.String())
	}
}
