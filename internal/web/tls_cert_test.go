package web

import (
	"fmt"
	"net/http"
	"os"
	"testing"

	"digitdns/internal/database"
)

// TestHandleTLSCertInstallsUploadedPair drives the BYO path end to end: an
// uploaded self-signed pair for the DoH/DoT surface is validated, written,
// applied to the holder, and recorded as the source of truth.
func TestHandleTLSCertInstallsUploadedPair(t *testing.T) {
	ws, _, cleanup := setupTestWebServer(t)
	defer cleanup()

	certPath, keyPath := writeTestCertificatePair(t, "dot.example")
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		t.Fatal(err)
	}
	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		t.Fatal(err)
	}

	ws.tlsSettings.Domain = "panel.example"
	ws.tlsSettings.DoTDomain = "dot.example"

	body := fmt.Sprintf(`{"purpose":"dot","source":"files","domain":"dot.example","cert_pem":%q,"key_pem":%q,"allow_self_signed":true}`,
		string(certPEM), string(keyPEM))
	token := loginTestSession(t, ws)
	w := postJSON(t, ws.buildAdminHandler(), "/api/tls/cert", body, token)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", w.Code, w.Body.String())
	}

	cfg := ws.certConfig(database.CertPurposeDoT)
	if cfg.Source != database.CertSourceFiles || cfg.CertPath == "" || cfg.KeyPath == "" {
		t.Fatalf("stored dot certificate config = %#v", cfg)
	}
	if _, err := os.Stat(cfg.CertPath); err != nil {
		t.Fatalf("stored certificate missing: %v", err)
	}
	info, err := os.Stat(cfg.KeyPath)
	if err != nil {
		t.Fatalf("stored key missing: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("private key mode = %o, want 600", info.Mode().Perm())
	}
	details := ws.dotCertDetails()
	if details == nil || details.Domain != "dot.example" {
		t.Fatalf("dotCertDetails = %#v", details)
	}
}

// TestHandleTLSCertInstallsPanelPair covers the panel surface with a CA-signed
// pair (the strict validator, no self-signed opt-in).
func TestHandleTLSCertInstallsPanelPair(t *testing.T) {
	ws, _, cleanup := setupTestWebServer(t)
	defer cleanup()

	certPath, keyPath, _ := writeTestChainPair(t, "panel.example")
	certPEM, _ := os.ReadFile(certPath)
	keyPEM, _ := os.ReadFile(keyPath)
	ws.tlsSettings.Domain = "panel.example"

	body := fmt.Sprintf(`{"purpose":"panel","source":"files","domain":"panel.example","cert_pem":%q,"key_pem":%q}`,
		string(certPEM), string(keyPEM))
	token := loginTestSession(t, ws)
	w := postJSON(t, ws.buildAdminHandler(), "/api/tls/cert", body, token)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", w.Code, w.Body.String())
	}
	cfg := ws.certConfig(database.CertPurposePanel)
	if cfg.Source != database.CertSourceFiles || cfg.CertPath == "" {
		t.Fatalf("stored panel certificate config = %#v", cfg)
	}
	if details := ws.panelCertDetails(); details == nil || details.Domain != "panel.example" {
		t.Fatalf("panelCertDetails = %#v", details)
	}
}

// TestHandleTLSCertRefusesWrongHostname proves the save-time validator runs
// before anything is applied.
func TestHandleTLSCertRefusesWrongHostname(t *testing.T) {
	ws, _, cleanup := setupTestWebServer(t)
	defer cleanup()

	certPath, keyPath := writeTestCertificatePair(t, "other.example")
	certPEM, _ := os.ReadFile(certPath)
	keyPEM, _ := os.ReadFile(keyPath)
	ws.tlsSettings.Domain = "panel.example"
	ws.tlsSettings.DoTDomain = "dot.example"

	body := fmt.Sprintf(`{"purpose":"dot","source":"files","domain":"dot.example","cert_pem":%q,"key_pem":%q,"allow_self_signed":true}`,
		string(certPEM), string(keyPEM))
	token := loginTestSession(t, ws)
	w := postJSON(t, ws.buildAdminHandler(), "/api/tls/cert", body, token)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body %s", w.Code, w.Body.String())
	}
	if cfg := ws.certConfig(database.CertPurposeDoT); cfg.Source == database.CertSourceFiles {
		t.Fatal("a refused certificate was still recorded as the active source")
	}
}
