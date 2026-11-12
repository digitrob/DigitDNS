package acme

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/acme"
)

// DNSProvider creates and removes the TXT record an ACME DNS-01 challenge
// needs. Present must return only after the record is visible to the public
// resolvers the CA is likely to use; CleanUp must be safe to call twice.
type DNSProvider interface {
	Present(ctx context.Context, fqdn, value string) error
	CleanUp(ctx context.Context, fqdn, value string) error
}

// IssueDNS01 obtains (or renews) a certificate for certName using a DNS-01
// challenge. certName may be a wildcard ("*.example.com"); the TXT record is
// created at _acme-challenge.example.com. purpose names the surface the
// certificate is for, exactly as Issue does.
func (m *Manager) IssueDNS01(ctx context.Context, certName, purpose string, provider DNSProvider) (*Result, error) {
	if provider == nil {
		return nil, fmt.Errorf("a DNS provider is required for the DNS-01 challenge")
	}
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return nil, fmt.Errorf("an issuance is already running")
	}
	m.running = true
	m.run = RunState{Running: true, Purpose: purpose, Domain: certName, Stage: "start", Progress: 5, StartedAt: time.Now()}
	m.mu.Unlock()
	defer func() {
		m.mu.Lock()
		m.running = false
		m.run.Running = false
		m.lastRunAt = time.Now()
		m.mu.Unlock()
	}()

	client, _, keyPEM, err := m.ensureClient(ctx)
	if err != nil {
		m.fail("account", 10, fmt.Sprintf("ACME account setup failed: %v", err))
		return nil, err
	}
	m.mu.Lock()
	m.AccountKeyPEM = keyPEM
	m.mu.Unlock()

	order, err := client.AuthorizeOrder(ctx, acme.DomainIDs(certName))
	if err != nil {
		m.fail("order", 25, fmt.Sprintf("order creation failed for %s: %v", certName, err))
		return nil, err
	}

	for _, authzURL := range order.AuthzURLs {
		authz, err := client.GetAuthorization(ctx, authzURL)
		if err != nil {
			m.fail("authorize", 30, fmt.Sprintf("could not read the authorization for %s: %v", certName, err))
			return nil, err
		}
		if authz.Status == acme.StatusValid {
			continue
		}
		var chal *acme.Challenge
		for _, c := range authz.Challenges {
			if c.Type == "dns-01" {
				chal = c
				break
			}
		}
		if chal == nil {
			m.fail("challenge", 35, fmt.Sprintf("the CA offered no dns-01 challenge for %s", certName))
			return nil, fmt.Errorf("no dns-01 challenge offered")
		}

		value, err := client.DNS01ChallengeRecord(chal.Token)
		if err != nil {
			m.fail("challenge", 40, fmt.Sprintf("could not derive the DNS-01 response: %v", err))
			return nil, err
		}
		fqdn := dns01Name(certName)
		m.stage("challenge", 45, fmt.Sprintf("publishing %s TXT for %s", fqdn, certName))
		if err := provider.Present(ctx, fqdn, value); err != nil {
			m.fail("challenge", 48, fmt.Sprintf("could not publish the DNS-01 record: %v", err))
			return nil, err
		}
		cleanup := func() {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			_ = provider.CleanUp(cleanupCtx, fqdn, value)
		}

		m.stage("authorize", 55, fmt.Sprintf("asking the CA to validate %s", certName))
		if _, err := client.Accept(ctx, chal); err != nil {
			cleanup()
			m.fail("challenge", 58, fmt.Sprintf("the CA refused the challenge acceptance: %v", err))
			return nil, err
		}
		if _, err := client.WaitAuthorization(ctx, authz.URI); err != nil {
			cleanup()
			m.fail("authorize", 65, fmt.Sprintf("DNS-01 validation did not complete for %s — check the TXT record and the API token (%v)", certName, err))
			return nil, err
		}
		cleanup()
	}

	if _, err := client.WaitOrder(ctx, order.URI); err != nil {
		m.fail("ready", 75, fmt.Sprintf("the order never became ready: %v", err))
		return nil, err
	}

	csrKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		m.fail("finalize", 80, fmt.Sprintf("key generation failed: %v", err))
		return nil, err
	}
	csr, err := certRequest(csrKey, certName)
	if err != nil {
		m.fail("finalize", 80, fmt.Sprintf("CSR build failed: %v", err))
		return nil, err
	}
	der, _, err := client.CreateOrderCert(ctx, order.FinalizeURL, csr, true)
	if err != nil {
		m.fail("finalize", 85, fmt.Sprintf("the CA refused the certificate request: %v", err))
		return nil, err
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der[0]})
	for _, extra := range der[1:] {
		certPEM = append(certPEM, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: extra})...)
	}
	keyDER, err := x509.MarshalECPrivateKey(csrKey)
	if err != nil {
		m.fail("finalize", 85, fmt.Sprintf("key marshalling failed: %v", err))
		return nil, err
	}
	keyPEMBytes := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	leaf, err := parseLeaf(certPEM)
	if err != nil {
		m.fail("fetched", 90, fmt.Sprintf("the issued certificate did not parse: %v", err))
		return nil, err
	}
	m.stage("fetched", 90, fmt.Sprintf("certificate issued for %s, valid until %s", certName, leaf.NotAfter.Format(time.RFC3339)))
	return &Result{
		Issued:   true,
		Detail:   fmt.Sprintf("certificate issued for %s (expires %s)", certName, leaf.NotAfter.Format("2006-01-02")),
		CertPEM:  certPEM,
		KeyPEM:   keyPEMBytes,
		NotAfter: leaf.NotAfter,
	}, nil
}

// dns01Name returns the TXT owner for a certificate identifier. A wildcard
// certificate for *.example.com is validated at _acme-challenge.example.com,
// not at _acme-challenge.*.example.com.
func dns01Name(certName string) string {
	name := strings.TrimSpace(strings.ToLower(certName))
	name = strings.TrimPrefix(name, "*.")
	return "_acme-challenge." + name
}
