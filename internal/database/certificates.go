package database

import (
	"strings"
	"sync"
)

// Certificate sources and challenge types. The values are persisted, so they
// are spelled the way the dashboard and the API present them.
const (
	CertSourceACME  = "acme"
	CertSourceFiles = "files"

	CertChallengeHTTP01 = "http-01"
	CertChallengeDNS01  = "dns-01"

	CertPurposePanel        = "panel"
	CertPurposeDoT          = "dot"
	CertPurposeSubscription = "subscription"
)

// CertificateConfig is the operator's certificate choice for one surface.
//
// Source is "acme" (the embedded Let's Encrypt client) or "files" (a pair the
// operator already holds). For ACME, CertName is the identifier requested from
// the CA — it may be a wildcard such as "*.example.com" — while Domain is the
// name the listener actually serves. A wildcard certificate for
// "*.example.com" therefore covers Domain "dns.example.com".
//
// DNSAPIToken is a secret. It is persisted with the rest of the settings (the
// settings bucket is encrypted), and the snapshot type below deliberately
// exposes only "set/not set" so it never reaches /api/config.
type CertificateConfig struct {
	Source          string `json:"source"`
	Domain          string `json:"domain"`
	CertName        string `json:"cert_name"`
	Challenge       string `json:"challenge"`
	CertPath        string `json:"cert_path"`
	KeyPath         string `json:"key_path"`
	DNSProvider     string `json:"dns_provider"`
	DNSZone         string `json:"dns_zone"`
	DNSAPIToken     string `json:"dns_api_token,omitempty"`
	AllowSelfSigned bool   `json:"allow_self_signed"`
	AutoRenew       bool   `json:"auto_renew"`
}

// CertificateConfigView is the safe, display-only form of CertificateConfig.
// It never carries the DNS API token.
type CertificateConfigView struct {
	Source          string `json:"source"`
	Domain          string `json:"domain"`
	CertName        string `json:"cert_name"`
	Challenge       string `json:"challenge"`
	CertPath        string `json:"cert_path"`
	KeyPath         string `json:"key_path"`
	DNSProvider     string `json:"dns_provider"`
	DNSZone         string `json:"dns_zone"`
	DNSAPITokenSet  bool   `json:"dns_api_token_set"`
	AllowSelfSigned bool   `json:"allow_self_signed"`
	AutoRenew       bool   `json:"auto_renew"`
}

// CertificateSettings is the persisted certificate policy for the three TLS
// surfaces. It is a standalone setting record ("certificates") so the existing
// TLSSettings and SubscriptionSettings schemas keep their historical meaning.
type CertificateSettings struct {
	mu sync.RWMutex

	Panel        CertificateConfig `json:"panel"`
	DoT          CertificateConfig `json:"dot"`
	Subscription CertificateConfig `json:"subscription"`
}

// CertificateSettingsSnapshot is what /api/config and the API expose.
type CertificateSettingsSnapshot struct {
	Panel        CertificateConfigView `json:"panel"`
	DoT          CertificateConfigView `json:"dot"`
	Subscription CertificateConfigView `json:"subscription"`
}

// DefaultCertificateSettings returns the policy a fresh install uses: ACME
// over HTTP-01, automatic renewal on, no custom pair.
func DefaultCertificateSettings() *CertificateSettings {
	return &CertificateSettings{
		Panel:        CertificateConfig{Source: CertSourceACME, Challenge: CertChallengeHTTP01, AutoRenew: true},
		DoT:          CertificateConfig{Source: CertSourceACME, Challenge: CertChallengeHTTP01, AutoRenew: true},
		Subscription: CertificateConfig{Source: CertSourceACME, Challenge: CertChallengeHTTP01, AutoRenew: true},
	}
}

// Normalize fills the defaults a record written before this feature would not
// carry. It is called once at startup and again after every load.
func (s *CertificateSettings) Normalize() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Panel = normalizeCertificateConfig(s.Panel)
	s.DoT = normalizeCertificateConfig(s.DoT)
	s.Subscription = normalizeCertificateConfig(s.Subscription)
}

func normalizeCertificateConfig(c CertificateConfig) CertificateConfig {
	fresh := strings.TrimSpace(c.Source) == "" &&
		strings.TrimSpace(c.Domain) == "" &&
		strings.TrimSpace(c.CertName) == "" &&
		strings.TrimSpace(c.CertPath) == "" &&
		strings.TrimSpace(c.KeyPath) == ""

	c.Source = strings.ToLower(strings.TrimSpace(c.Source))
	if c.Source != CertSourceFiles {
		c.Source = CertSourceACME
	}
	c.Challenge = strings.ToLower(strings.TrimSpace(c.Challenge))
	if c.Challenge != CertChallengeDNS01 {
		c.Challenge = CertChallengeHTTP01
	}
	c.Domain = strings.TrimSpace(c.Domain)
	c.CertName = strings.TrimSpace(c.CertName)
	if c.CertName == "" {
		c.CertName = c.Domain
	}
	c.CertPath = strings.TrimSpace(c.CertPath)
	c.KeyPath = strings.TrimSpace(c.KeyPath)
	c.DNSProvider = strings.ToLower(strings.TrimSpace(c.DNSProvider))
	c.DNSZone = strings.TrimSpace(c.DNSZone)
	c.DNSAPIToken = strings.TrimSpace(c.DNSAPIToken)
	// A record written before this feature decodes AutoRenew as false. A fresh
	// (all-empty) record is the one case where false is certainly not an
	// operator choice; every record with any content preserves its stored flag.
	if fresh {
		c.AutoRenew = true
	}
	return c
}

// Get returns a copy of the config for one purpose.
func (s *CertificateSettings) Get(purpose string) CertificateConfig {
	if s == nil {
		return CertificateConfig{Source: CertSourceACME, Challenge: CertChallengeHTTP01, AutoRenew: true}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	switch purpose {
	case CertPurposeDoT:
		return s.DoT
	case CertPurposeSubscription:
		return s.Subscription
	default:
		return s.Panel
	}
}

// Set replaces one purpose's config and persists the whole record. A failed
// persist restores the previous value so the in-memory policy never claims a
// state the database refused.
func (s *CertificateSettings) Set(purpose string, next CertificateConfig, persist func(*CertificateSettings) error) error {
	if s == nil {
		return ErrTLSSettingsNil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	next = normalizeCertificateConfig(next)

	var before CertificateConfig
	switch purpose {
	case CertPurposeDoT:
		before = s.DoT
		s.DoT = next
	case CertPurposeSubscription:
		before = s.Subscription
		s.Subscription = next
	default:
		before = s.Panel
		s.Panel = next
	}
	if persist == nil {
		return nil
	}
	if err := persist(s); err != nil {
		switch purpose {
		case CertPurposeDoT:
			s.DoT = before
		case CertPurposeSubscription:
			s.Subscription = before
		default:
			s.Panel = before
		}
		return err
	}
	return nil
}

// Snapshot returns the safe, token-free view of every purpose.
func (s *CertificateSettings) Snapshot() CertificateSettingsSnapshot {
	if s == nil {
		d := DefaultCertificateSettings()
		return d.Snapshot()
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return CertificateSettingsSnapshot{
		Panel:        viewCertificateConfig(s.Panel),
		DoT:          viewCertificateConfig(s.DoT),
		Subscription: viewCertificateConfig(s.Subscription),
	}
}

func viewCertificateConfig(c CertificateConfig) CertificateConfigView {
	return CertificateConfigView{
		Source:          c.Source,
		Domain:          c.Domain,
		CertName:        c.CertName,
		Challenge:       c.Challenge,
		CertPath:        c.CertPath,
		KeyPath:         c.KeyPath,
		DNSProvider:     c.DNSProvider,
		DNSZone:         c.DNSZone,
		DNSAPITokenSet:  c.DNSAPIToken != "",
		AllowSelfSigned: c.AllowSelfSigned,
		AutoRenew:       c.AutoRenew,
	}
}
