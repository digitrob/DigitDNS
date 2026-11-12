package web

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"digitdns/internal/database"
	"digitdns/internal/httpx"
)

// handleTLSCert installs an operator-supplied certificate pair for one TLS
// surface. It accepts either PEM bodies (upload/paste) or paths on the server.
// The pair is validated before it is written, and applied through the same
// purpose switch the ACME path uses, so a BYO certificate hot-swaps exactly
// like an issued one.
func (ws *WebServer) handleTLSCert(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	switch r.Method {
	case http.MethodGet, http.MethodHead:
		_ = json.NewEncoder(w).Encode(map[string]any{
			"certificates": ws.certSettings.Snapshot(),
			"panel_cert":   ws.panelCertDetails(),
			"dot_cert":     ws.dotCertDetails(),
			"sub_cert":     ws.subscriptionCertDetails(),
		})
		return
	case http.MethodPost:
	default:
		httpx.WriteMethodNotAllowed(w, "GET, HEAD, POST")
		return
	}

	var req struct {
		Purpose         string `json:"purpose"`
		Source          string `json:"source"`
		Domain          string `json:"domain"`
		CertPEM         string `json:"cert_pem"`
		KeyPEM          string `json:"key_pem"`
		CertPath        string `json:"cert_path"`
		KeyPath         string `json:"key_path"`
		AllowSelfSigned bool   `json:"allow_self_signed"`
	}
	// A full chain plus a key is still small; 256 KiB leaves room for a
	// generous chain without turning the endpoint into an upload surface.
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 256<<10)).Decode(&req); err != nil {
		httpx.WriteJSONError(w, http.StatusBadRequest, "the request body could not be read as JSON")
		return
	}

	purpose := strings.ToLower(strings.TrimSpace(req.Purpose))
	switch purpose {
	case database.CertPurposePanel, database.CertPurposeDoT, database.CertPurposeSubscription:
	default:
		httpx.WriteJSONError(w, http.StatusBadRequest, "purpose must be panel, dot, or subscription")
		return
	}
	source := strings.ToLower(strings.TrimSpace(req.Source))
	if source == "" {
		source = database.CertSourceFiles
	}
	if source != database.CertSourceFiles {
		httpx.WriteJSONError(w, http.StatusBadRequest, "this endpoint installs files; use /api/tls/issue for Let's Encrypt")
		return
	}

	domain, err := ws.certificateDomain(purpose, req.Domain)
	if err != nil {
		httpx.WriteJSONErrorFor(w, http.StatusBadRequest, err)
		return
	}

	certPEM := strings.TrimSpace(req.CertPEM)
	keyPEM := strings.TrimSpace(req.KeyPEM)
	certPath := strings.TrimSpace(req.CertPath)
	keyPath := strings.TrimSpace(req.KeyPath)

	if certPEM != "" || keyPEM != "" {
		if certPEM == "" || keyPEM == "" {
			httpx.WriteJSONError(w, http.StatusBadRequest, "both the certificate and its private key are required")
			return
		}
		if err := validatePEMPair(certPEM, keyPEM, domain, req.AllowSelfSigned); err != nil {
			httpx.WriteJSONErrorFor(w, http.StatusBadRequest, err)
			return
		}
		certPath, keyPath, err = ws.writeManualPair(purpose, domain, certPEM, keyPEM)
		if err != nil {
			log.Printf("[TLS] storing the uploaded certificate for %s failed: %v", purpose, err)
			httpx.WriteJSONError(w, http.StatusInternalServerError, "the certificate could not be written to disk; see the daemon log")
			return
		}
	} else {
		if certPath == "" || keyPath == "" {
			httpx.WriteJSONError(w, http.StatusBadRequest, "provide PEM bodies, or both cert_path and key_path")
			return
		}
		if err := ValidateCertificateForDomain(certPath, keyPath, domain, req.AllowSelfSigned); err != nil {
			httpx.WriteJSONErrorFor(w, http.StatusBadRequest, err)
			return
		}
	}

	if problem := ws.applyIssuedPurpose(domain, purpose, certPath, keyPath); problem != "" {
		httpx.WriteJSONError(w, http.StatusInternalServerError, problem)
		return
	}
	if err := ws.setCertificateConfig(purpose, database.CertificateConfig{
		Source:          database.CertSourceFiles,
		Domain:          domain,
		CertName:        domain,
		Challenge:       database.CertChallengeHTTP01,
		CertPath:        certPath,
		KeyPath:         keyPath,
		AllowSelfSigned: req.AllowSelfSigned,
		AutoRenew:       false,
	}); err != nil {
		log.Printf("[TLS] certificate applied but the source record could not be saved: %v", err)
		httpx.WriteJSONError(w, http.StatusInternalServerError, "the certificate is live but its source could not be saved; see the daemon log")
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]any{
		"success":   true,
		"purpose":   purpose,
		"domain":    domain,
		"cert_path": certPath,
		"key_path":  keyPath,
		"detail":    "certificate installed and applied.",
	})
}

// certificateDomain resolves the hostname a purpose serves. The operator may
// override it, but an empty value falls back to the configured name rather
// than guessing.
func (ws *WebServer) certificateDomain(purpose, requested string) (string, error) {
	requested = strings.ToLower(strings.TrimSpace(requested))
	if requested != "" {
		return validACMEDomain(requested)
	}
	switch purpose {
	case database.CertPurposePanel:
		if ws.tlsSettings == nil || strings.TrimSpace(ws.tlsSettings.GetDomain()) == "" {
			return "", fmt.Errorf("configure the panel domain before installing a certificate for it")
		}
		return strings.TrimSpace(ws.tlsSettings.GetDomain()), nil
	case database.CertPurposeDoT:
		if ws.tlsSettings == nil {
			return "", fmt.Errorf("TLS settings are not available")
		}
		if d := strings.TrimSpace(ws.tlsSettings.GetDoTDomain()); d != "" {
			return d, nil
		}
		if d := strings.TrimSpace(ws.tlsSettings.GetDomain()); d != "" {
			return d, nil
		}
		return "", fmt.Errorf("configure the DoH/DoT domain before installing a certificate for it")
	case database.CertPurposeSubscription:
		if ws.subSettings == nil || strings.TrimSpace(ws.subSettings.Snapshot().Domain) == "" {
			return "", fmt.Errorf("configure the subscription domain before installing a certificate for it")
		}
		return strings.TrimSpace(ws.subSettings.Snapshot().Domain), nil
	}
	return "", fmt.Errorf("unknown certificate purpose")
}

// validatePEMPair parses an in-memory pair and checks the same properties the
// file validator checks.
func validatePEMPair(certPEM, keyPEM, domain string, allowSelfSigned bool) error {
	pair, err := tls.X509KeyPair([]byte(certPEM), []byte(keyPEM))
	if err != nil {
		return fmt.Errorf("certificate and key do not pair: %w", err)
	}
	if len(pair.Certificate) == 0 {
		return fmt.Errorf("the certificate file contains no certificate")
	}
	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		return fmt.Errorf("the certificate could not be parsed: %w", err)
	}
	return validateLeaf(leaf, domain, allowSelfSigned)
}

// validateCertificateForDomain is ValidatePanelCertificate with an explicit
// self-signed opt-in for private-CA/LAN deployments.
func ValidateCertificateForDomain(certPath, keyPath, domain string, allowSelfSigned bool) error {
	if strings.TrimSpace(certPath) == "" || strings.TrimSpace(keyPath) == "" {
		return fmt.Errorf("certificate and key paths are required")
	}
	pair, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return fmt.Errorf("certificate and key do not pair: %w", err)
	}
	if len(pair.Certificate) == 0 {
		return fmt.Errorf("the certificate file contains no certificate")
	}
	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		return fmt.Errorf("the certificate could not be parsed: %w", err)
	}
	return validateLeaf(leaf, domain, allowSelfSigned)
}

func validateLeaf(leaf *x509.Certificate, domain string, allowSelfSigned bool) error {
	now := time.Now()
	if now.Before(leaf.NotBefore) {
		return fmt.Errorf("certificate is not valid yet (not before %s)", leaf.NotBefore.UTC().Format(time.RFC3339))
	}
	if now.After(leaf.NotAfter) {
		return fmt.Errorf("certificate expired %s", leaf.NotAfter.UTC().Format(time.RFC3339))
	}
	if domain != "" {
		if err := leaf.VerifyHostname(domain); err != nil {
			return fmt.Errorf("certificate does not cover domain %q: %w", domain, err)
		}
		if !allowSelfSigned && string(leaf.RawIssuer) == string(leaf.RawSubject) {
			return fmt.Errorf("certificate for %q is self-signed; tick “allow self-signed” only for a private CA or LAN deployment", domain)
		}
	}
	return nil
}

// writeManualPair stores an uploaded pair under certs/manual/<purpose>/.
// Writes are atomic, the key is 0600, and the certificate is 0644.
func (ws *WebServer) writeManualPair(purpose, domain, certPEM, keyPEM string) (string, string, error) {
	base := strings.ReplaceAll(domain, "*", "_wildcard")
	dir := filepath.Join(filepath.Dir(ws.acmeDir()), "manual", purpose)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", "", err
	}
	certPath := filepath.Join(dir, base+".crt")
	keyPath := filepath.Join(dir, base+".key")
	if err := atomicWriteFile(certPath, []byte(certPEM+"\n"), 0o644); err != nil {
		return "", "", err
	}
	if err := atomicWriteFile(keyPath, []byte(keyPEM+"\n"), 0o600); err != nil {
		return "", "", err
	}
	return certPath, keyPath, nil
}

func atomicWriteFile(path string, data []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".manual-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer func() { _ = os.Remove(name) }()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}

// pemBlockType is used only in tests and diagnostics; keeping it here makes
// the accepted PEM shapes explicit.
func pemBlockType(raw []byte) string {
	block, _ := pem.Decode(raw)
	if block == nil {
		return ""
	}
	return block.Type
}
