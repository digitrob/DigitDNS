package web

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"digitdns/internal/database"
	dnscryptlib "github.com/ameshkov/dnscrypt/v2"
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
	if !initial.DoTEnabled {
		t.Error("DoT should default to enabled for existing installs")
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
		`{"doq_enabled":true,"doq_auth":"sni-token","doq_tokens":["alpha"]}`,
		`{"doq_enabled":true,"doq_auth":"sni-token","doq_token_domain":"doq.example.com","doq_tokens":[]}`,
		`{"doq_enabled":true,"doq_auth":"sni-token","doq_token_domain":"not a domain","doq_tokens":["alpha"]}`,
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

// testClientCAPEM returns a self-signed CA in PEM, which is what an operator
// pastes into the mTLS field.
func testClientCAPEM(t *testing.T) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("CA GenerateKey: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "DigitDNS mTLS Test CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("CA CreateCertificate: %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

func postTransportJSON(t *testing.T, ws *WebServer, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	return postTransportSettings(t, ws, string(raw))
}

func TestTransportSettingsDNSCryptDoTAndMTLS(t *testing.T) {
	ws, db, cleanup := setupTestWebServer(t)
	defer cleanup()

	var applied database.DNSSettings
	ws.SetTransportApplier(func(cfg database.DNSSettings, tokens []string) error {
		applied = cfg
		return nil
	})
	var installedProvider string
	ws.SetDNSCryptApplier(func(rc *dnscryptlib.ResolverConfig, cert *dnscryptlib.Cert) error {
		installedProvider = rc.ProviderName
		return nil
	})

	ca := testClientCAPEM(t)
	w := postTransportJSON(t, ws, map[string]any{
		"dnscrypt_enabled":       true,
		"dnscrypt_port":          5443,
		"dnscrypt_provider_name": "resolver.example.net",
		"dot_auth":               "sni-token",
		"dot_token_domain":       "dot.example.com",
		"dot_tokens":             []string{"Alpha", " alpha ", "beta"},
		"mtls_enabled":           true,
		"mtls_ca_pem":            ca,
	})
	if w.Code != http.StatusOK {
		t.Fatalf("POST DNSCrypt/DoT/mTLS = %d: %s", w.Code, w.Body.String())
	}

	var saved database.DNSSettings
	if err := db.GetSetting("dns", &saved); err != nil {
		t.Fatalf("GetSetting(dns): %v", err)
	}
	if !saved.DNSCryptEnabled || saved.DNSCryptPort != 5443 {
		t.Errorf("DNSCrypt settings = %#v", saved)
	}
	if saved.DNSCryptProviderName != "2.dnscrypt-cert.resolver.example.net" {
		t.Errorf("provider name = %q", saved.DNSCryptProviderName)
	}
	if saved.DoTAuth != "sni-token" || saved.DoTTokenDomain != "dot.example.com" {
		t.Errorf("DoT settings = auth %q domain %q", saved.DoTAuth, saved.DoTTokenDomain)
	}
	if len(saved.DoTTokens) != 2 || saved.DoTTokens[0] != "alpha" || saved.DoTTokens[1] != "beta" {
		t.Errorf("DoT tokens = %#v, want [alpha beta]", saved.DoTTokens)
	}
	if !saved.MTLSEnabled || saved.MTLSCAPEM != strings.TrimSpace(ca) {
		t.Errorf("mTLS settings were not saved")
	}
	if !applied.DNSCryptEnabled || applied.DoTAuth != "sni-token" || !applied.MTLSEnabled {
		t.Errorf("applied config = %#v", applied)
	}
	if installedProvider != "2.dnscrypt-cert.resolver.example.net" {
		t.Errorf("DNSCrypt applier got provider %q", installedProvider)
	}

	// The keys must be generated and persisted without ever leaving the server.
	var keys database.DNSCryptSettings
	if err := db.GetSetting("dnscrypt", &keys); err != nil {
		t.Fatalf("GetSetting(dnscrypt): %v", err)
	}
	if keys.PublicKey == "" || keys.PrivateKey == "" || keys.CertNotAfter == 0 {
		t.Errorf("DNSCrypt key material = %#v", keys)
	}

	got := getTransportSettings(t, ws)
	if got.DNSCryptStamp == "" {
		t.Error("view did not expose a DNSCrypt stamp")
	}
	if got.DNSCryptCertNotAfter == 0 {
		t.Error("view did not expose the DNSCrypt certificate expiry")
	}
	if got.DNSCryptProviderName != "2.dnscrypt-cert.resolver.example.net" {
		t.Errorf("view provider = %q", got.DNSCryptProviderName)
	}
	if len(got.DoTTokens) != 2 || got.DoTAuth != "sni-token" || !got.MTLSEnabled {
		t.Errorf("view = %#v", got)
	}

	// A partial save keeps the token list and the CA.
	w = postTransportJSON(t, ws, map[string]any{"mtls_enabled": false})
	if w.Code != http.StatusOK {
		t.Fatalf("partial POST = %d: %s", w.Code, w.Body.String())
	}
	got = getTransportSettings(t, ws)
	if len(got.DoTTokens) != 2 || got.MTLSCAPEM == "" {
		t.Errorf("partial save lost state: %#v", got)
	}
}

func TestTransportSettingsDNSCryptDoTMTLSValidation(t *testing.T) {
	ws, _, cleanup := setupTestWebServer(t)
	defer cleanup()

	for _, body := range []map[string]any{
		{"dnscrypt_port": 70000},
		{"dnscrypt_provider_name": "not a domain"},
		{"dot_auth": "token"},
		{"dot_enabled": true, "dot_auth": "sni-token", "dot_tokens": []string{"alpha"}},
		{"dot_enabled": true, "dot_auth": "sni-token", "dot_token_domain": "dot.example.com", "dot_tokens": []string{}},
		{"dot_enabled": true, "dot_auth": "sni-token", "dot_token_domain": "not a domain", "dot_tokens": []string{"alpha"}},
		{"dot_tokens": []string{"-bad"}},
		{"mtls_enabled": true},
		{"mtls_enabled": true, "mtls_ca_pem": "not a certificate"},
	} {
		w := postTransportJSON(t, ws, body)
		if w.Code != http.StatusBadRequest {
			t.Errorf("body %v = %d, want 400: %s", body, w.Code, w.Body.String())
		}
	}
}

// A failed listener apply must roll the DNSCrypt keys and the DNS policy back.
func TestTransportSettingsDNSCryptRollback(t *testing.T) {
	ws, db, cleanup := setupTestWebServer(t)
	defer cleanup()

	ws.SetDNSCryptApplier(func(*dnscryptlib.ResolverConfig, *dnscryptlib.Cert) error { return nil })
	ws.SetTransportApplier(func(database.DNSSettings, []string) error {
		return errors.New("dnscrypt port busy")
	})

	w := postTransportJSON(t, ws, map[string]any{
		"dnscrypt_enabled": true,
		"dnscrypt_port":    5443,
	})
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("failed apply = %d, want 500: %s", w.Code, w.Body.String())
	}
	got := getTransportSettings(t, ws)
	if got.DNSCryptEnabled {
		t.Error("DNSCrypt stayed enabled after the apply failed")
	}
	if !strings.Contains(got.LastError, "dnscrypt port busy") {
		t.Errorf("last_error = %q", got.LastError)
	}
	var keys database.DNSCryptSettings
	if err := db.GetSetting("dnscrypt", &keys); err != nil {
		t.Fatalf("GetSetting(dnscrypt): %v", err)
	}
	if keys.PublicKey != "" {
		t.Error("generated keys survived a rolled-back save")
	}
}

func TestTransportViewSurfacesDNSCryptStatus(t *testing.T) {
	ws, _, cleanup := setupTestWebServer(t)
	defer cleanup()

	ws.SetDNSCryptStatusSource(func() (bool, int64, string) {
		return false, 0, "DNSCrypt UDP listener stopped: boom"
	})
	got := getTransportSettings(t, ws)
	if got.DNSCryptListening {
		t.Error("dnscrypt_listening = true, want false")
	}
	if !strings.Contains(got.LastError, "DNSCrypt UDP listener stopped: boom") {
		t.Errorf("last_error = %q, want the DNSCrypt failure", got.LastError)
	}
}

// DoT is enabled by default and can be switched off without losing its auth
// configuration. An incomplete SNI setup may be stored while DoT is off, but
// turning it back on requires the fields the listener needs.
func TestTransportSettingsDoTToggle(t *testing.T) {
	ws, db, cleanup := setupTestWebServer(t)
	defer cleanup()

	if got := getTransportSettings(t, ws); !got.DoTEnabled {
		t.Fatal("DoT default = disabled, want enabled")
	}

	w := postTransportJSON(t, ws, map[string]any{"dot_enabled": false})
	if w.Code != http.StatusOK {
		t.Fatalf("disable DoT = %d: %s", w.Code, w.Body.String())
	}
	var saved database.DNSSettings
	if err := db.GetSetting("dns", &saved); err != nil {
		t.Fatalf("GetSetting(dns): %v", err)
	}
	if !saved.DoTDisabled {
		t.Error("dot_disabled was not persisted")
	}
	if got := getTransportSettings(t, ws); got.DoTEnabled {
		t.Error("view still reports DoT enabled")
	}

	// With DoT off, an incomplete SNI-token setup is allowed to be staged: the
	// operator can prepare it before switching the listener on.
	w = postTransportJSON(t, ws, map[string]any{
		"dot_auth": "sni-token", "dot_token_domain": "", "dot_tokens": []string{},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("stage SNI while DoT off = %d: %s", w.Code, w.Body.String())
	}

	// Enabling it now must be refused until the token fields are complete.
	w = postTransportJSON(t, ws, map[string]any{"dot_enabled": true})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("enable incomplete DoT = %d, want 400: %s", w.Code, w.Body.String())
	}

	w = postTransportJSON(t, ws, map[string]any{
		"dot_enabled":      true,
		"dot_auth":         "sni-token",
		"dot_token_domain": "dot.example.com",
		"dot_tokens":       []string{"alpha"},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("enable configured DoT = %d: %s", w.Code, w.Body.String())
	}
	if err := db.GetSetting("dns", &saved); err != nil {
		t.Fatalf("GetSetting(dns): %v", err)
	}
	if saved.DoTDisabled {
		t.Error("dot_disabled stayed set after re-enabling")
	}
}
