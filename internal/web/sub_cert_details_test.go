package web

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"digitdns/internal/database"
	"digitdns/internal/service/acme"
)

// writeTestCert generates a self-signed certificate for domain, valid in the
// window [notBefore, notAfter], and writes it to dir/<domain>.crt + .key — the
// exact pair layout Persist uses, which is what subscriptionCertDetails reads
// for a domain distinct from the panel's.
func writeTestCert(t *testing.T, dir, domain string, notBefore, notAfter time.Time) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: domain},
		DNSNames:              []string{domain},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	if err := os.WriteFile(filepath.Join(dir, domain+".crt"), certPEM, 0o600); err != nil {
		t.Fatalf("write cert: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, domain+".key"), keyPEM, 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}
}

// A live pair for a subscription domain distinct from the panel's reports its
// window and a false Expired.
func TestSubscriptionCertDetails_ValidDistinctDomain(t *testing.T) {
	ws, _, cleanup := setupTestWebServer(t)
	defer cleanup()

	certDir := t.TempDir()
	ws.acmeManager = acme.NewManager("http://127.0.0.1:0/dir", "", certDir, nil)
	notBefore := time.Now().Add(-24 * time.Hour)
	notAfter := time.Now().Add(60 * 24 * time.Hour)
	writeTestCert(t, certDir, "sub.example.com", notBefore, notAfter)

	ws.SetSubscriptionSettings(&database.SubscriptionSettings{Domain: "sub.example.com"})

	d := ws.subscriptionCertDetails()
	if d == nil {
		t.Fatal("expected certificate details, got nil")
	}
	if d.Domain != "sub.example.com" {
		t.Errorf("Domain = %q, want sub.example.com", d.Domain)
	}
	if d.Subject != "sub.example.com" {
		t.Errorf("Subject = %q, want sub.example.com", d.Subject)
	}
	if d.Expired {
		t.Error("Expired = true, want false for a pair valid for another 60 days")
	}
	// x509 encodes validity at whole-second granularity, so the expected
	// value is truncated the same way before comparison.
	if want := notAfter.Truncate(time.Second); !d.NotAfter.UTC().Equal(want) {
		t.Errorf("NotAfter = %v, want %v", d.NotAfter, want)
	}
}

// An expired pair is still reported — with Expired true. This is the state the
// dashboard needs to rename the Let's Encrypt button to Renew, so it must not
// be filtered out the way ValidatePanelCertificate would filter it.
func TestSubscriptionCertDetails_Expired(t *testing.T) {
	ws, _, cleanup := setupTestWebServer(t)
	defer cleanup()

	certDir := t.TempDir()
	ws.acmeManager = acme.NewManager("http://127.0.0.1:0/dir", "", certDir, nil)
	writeTestCert(t, certDir, "old.example.com", time.Now().Add(-90*24*time.Hour), time.Now().Add(-24*time.Hour))

	ws.SetSubscriptionSettings(&database.SubscriptionSettings{Domain: "old.example.com"})

	d := ws.subscriptionCertDetails()
	if d == nil {
		t.Fatal("expected details for an expired pair, got nil")
	}
	if !d.Expired {
		t.Error("Expired = false, want true")
	}
}

// No certificate on disk reads as nil, not an error: the UI renders "none".
func TestSubscriptionCertDetails_AbsentPair(t *testing.T) {
	ws, _, cleanup := setupTestWebServer(t)
	defer cleanup()

	certDir := t.TempDir()
	ws.acmeManager = acme.NewManager("http://127.0.0.1:0/dir", "", certDir, nil)
	ws.SetSubscriptionSettings(&database.SubscriptionSettings{Domain: "never.example.com"})

	if d := ws.subscriptionCertDetails(); d != nil {
		t.Fatalf("expected nil for a domain with no pair on disk, got %+v", d)
	}
}
