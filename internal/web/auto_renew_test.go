package web

// Tests for the certificate auto-renew toggles (Subscription portal + DoH/DoT)
// and the certificate-detail lines the SSL/TLS Manager card renders.

import (
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"hyperdns/internal/database"
	"hyperdns/internal/service/acme"
)

// hangACMEDir starts a directory server that accepts connections and never
// answers, so an issuance blocks inside Issue and the single-flight gate stays
// observable. Mirrors the helper in acme_gate_test.go.
func hangACMEDir(t *testing.T) (*acme.Manager, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("hang server: %v", err)
	}
	go func() {
		for {
			c, aerr := ln.Accept()
			if aerr != nil {
				return
			}
			go func(c net.Conn) {
				buf := make([]byte, 64)
				for {
					if _, e := c.Read(buf); e != nil {
						return
					}
				}
			}(c)
		}
	}()
	addr := ln.Addr().(*net.TCPAddr)
	mgr := acme.NewManager("http://127.0.0.1:"+strconv.Itoa(addr.Port)+"/dir", "", t.TempDir(), nil)
	return mgr, func() { _ = ln.Close() }
}

// The DoH/DoT auto-renew toggle: a disabled flag stops RenewDOTIfDue before it
// can take the single-flight gate, even when a certificate is due.
func TestRenewDOTIfDueRespectsAutoRenewFlag(t *testing.T) {
	ws, _, cleanup := setupTestWebServer(t)
	defer cleanup()

	mgr, stop := hangACMEDir(t)
	defer stop()
	ws.acmeManager = mgr
	ws.tlsSettings = &database.TLSSettings{DoTDomain: "dot.example.com"}

	// Disabled: no run is started, the gate stays free.
	if err := ws.tlsSettings.SetDotAutoRenew(false, nil); err != nil {
		t.Fatalf("SetDotAutoRenew: %v", err)
	}
	ws.RenewDOTIfDue()
	if ws.acmeRunning.Load() {
		ws.acmeRunning.Store(false)
		t.Fatal("RenewDOTIfDue started an issuance while auto-renew was disabled")
	}
	if mgr.State().Running {
		t.Fatal("RenewDOTIfDue reached the ACME manager while auto-renew was disabled")
	}

	// Enabled: the absent cert is due, so a run starts.
	if err := ws.tlsSettings.SetDotAutoRenew(true, nil); err != nil {
		t.Fatalf("SetDotAutoRenew: %v", err)
	}
	ws.RenewDOTIfDue()
	defer ws.acmeRunning.Store(false)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && !ws.acmeRunning.Load() && !mgr.State().Running {
		time.Sleep(10 * time.Millisecond)
	}
	if !ws.acmeRunning.Load() && !mgr.State().Running {
		t.Fatal("RenewDOTIfDue did not start an issuance for a due certificate with auto-renew enabled")
	}
}

// The subscription toggle: a disabled flag stops RenewSubscriptionIfDue.
func TestRenewSubscriptionIfDueRespectsAutoRenewFlag(t *testing.T) {
	ws, _, cleanup := setupTestWebServer(t)
	defer cleanup()

	mgr, stop := hangACMEDir(t)
	defer stop()
	ws.acmeManager = mgr
	ws.settings.WebPort = 8080
	ws.tlsSettings = &database.TLSSettings{Domain: "panel.example"}
	ws.subSettings = &database.SubscriptionSettings{
		Enabled:             true,
		Domain:              "sub.example.com",
		Port:                8443,
		UsePanelCertificate: false,
		NoAutoRenewACME:     true, // disabled
	}

	ws.RenewSubscriptionIfDue()
	if ws.acmeRunning.Load() || mgr.State().Running {
		ws.acmeRunning.Store(false)
		t.Fatal("RenewSubscriptionIfDue started an issuance while auto-renew was disabled")
	}

	// Enable it and confirm the due subscription cert now starts a run.
	ws.subSettings.NoAutoRenewACME = false
	ws.RenewSubscriptionIfDue()
	defer ws.acmeRunning.Store(false)
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && !ws.acmeRunning.Load() && !mgr.State().Running {
		time.Sleep(10 * time.Millisecond)
	}
	if !ws.acmeRunning.Load() && !mgr.State().Running {
		t.Fatal("RenewSubscriptionIfDue did not start an issuance with auto-renew enabled")
	}
}

// The /api/tls/auto-renew handler: verb guard, required field, persistence and
// the echoed state.
func TestHandleTLSAutoRenew(t *testing.T) {
	ws, db, cleanup := setupTestWebServer(t)
	defer cleanup()
	ws.tlsSettings = &database.TLSSettings{}
	h := ws.buildAdminHandler()
	tok := bearerFor(t, h)

	// Verb guard.
	for _, m := range []string{http.MethodGet, http.MethodDelete, http.MethodPut} {
		req := httptest.NewRequest(m, "/api/tls/auto-renew", nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s /api/tls/auto-renew = %d, want 405", m, w.Code)
		}
	}

	// Missing enabled is refused.
	req := httptest.NewRequest(http.MethodPost, "/api/tls/auto-renew", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("POST without enabled = %d, want 400", w.Code)
	}

	// Disabling persists the inverted stored flag and echoes enabled:false.
	req = httptest.NewRequest(http.MethodPost, "/api/tls/auto-renew", strings.NewReader(`{"enabled":false}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tok)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("POST enabled=false = %d — %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"enabled":false`) {
		t.Errorf("response body = %s, want enabled:false", w.Body.String())
	}
	var stored database.TLSSettings
	if err := db.GetSetting("tls", &stored); err != nil {
		t.Fatalf("tls record not persisted: %v", err)
	}
	if !stored.NoDotAutoRenewACME {
		t.Error("stored NoDotAutoRenewACME = false after disabling, want true")
	}

	// Re-enabling clears it.
	req = httptest.NewRequest(http.MethodPost, "/api/tls/auto-renew", strings.NewReader(`{"enabled":true}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+tok)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("POST enabled=true = %d — %s", w.Code, w.Body.String())
	}
	if err := db.GetSetting("tls", &stored); err != nil {
		t.Fatalf("tls record not persisted: %v", err)
	}
	if stored.NoDotAutoRenewACME {
		t.Error("stored NoDotAutoRenewACME = true after enabling, want false")
	}
}

// dotCertDetails reports the DoH/DoT domain's own pair, and falls back to the
// panel pair when no custom name is configured.
func TestDotCertDetails(t *testing.T) {
	ws, _, cleanup := setupTestWebServer(t)
	defer cleanup()

	certDir := t.TempDir()
	ws.acmeManager = acme.NewManager("http://127.0.0.1:0/dir", "", certDir, nil)
	notAfter := time.Now().Add(50 * 24 * time.Hour)
	writeTestCert(t, certDir, "dot.example.com", time.Now().Add(-24*time.Hour), notAfter)
	writeTestCert(t, certDir, "panel.example", time.Now().Add(-24*time.Hour), notAfter)

	// Custom DoH/DoT domain: its own pair.
	ws.tlsSettings = &database.TLSSettings{DoTDomain: "dot.example.com"}
	d := ws.dotCertDetails()
	if d == nil || d.Domain != "dot.example.com" {
		t.Fatalf("dotCertDetails(custom) = %+v, want dot.example.com", d)
	}

	// No custom name: rides the panel pair, resolved by the live domain.
	ws.tlsSettings = &database.TLSSettings{Domain: "panel.example"}
	d = ws.dotCertDetails()
	if d == nil || d.Domain != "panel.example" {
		t.Fatalf("dotCertDetails(panel fallback) = %+v, want panel.example", d)
	}
}

// panelCertDetails reports an expired panel pair (which ValidatePanelCertificate
// would refuse) with Expired true — the state the Renew label flip depends on.
func TestPanelCertDetailsReportsExpired(t *testing.T) {
	ws, _, cleanup := setupTestWebServer(t)
	defer cleanup()

	certDir := t.TempDir()
	ws.acmeManager = acme.NewManager("http://127.0.0.1:0/dir", "", certDir, nil)
	writeTestCert(t, certDir, "panel.example", time.Now().Add(-90*24*time.Hour), time.Now().Add(-24*time.Hour))
	ws.tlsSettings = &database.TLSSettings{Domain: "panel.example"}

	d := ws.panelCertDetails()
	if d == nil {
		t.Fatal("panelCertDetails returned nil for an expired-but-present pair")
	}
	if !d.Expired {
		t.Error("Expired = false, want true for a pair that died a day ago")
	}
}

// handleConfig carries the DoH/DoT auto-renew state and both certificate views.
func TestHandleConfigCarriesDoTCertAndAutoRenew(t *testing.T) {
	ws, _, cleanup := setupTestWebServer(t)
	defer cleanup()
	ws.tlsSettings = &database.TLSSettings{}
	h := ws.buildAdminHandler()
	tok := bearerFor(t, h)

	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/config = %d", w.Code)
	}
	body := w.Body.String()
	for _, key := range []string{`"dot_cert"`, `"panel_cert"`, `"dot_auto_renew_acme"`} {
		if !strings.Contains(body, key) {
			t.Errorf("config body has no %s key", key)
		}
	}
	if !strings.Contains(body, `"dot_auto_renew_acme":true`) {
		t.Errorf("dot_auto_renew_acme should default true: %.200s", body)
	}
}
