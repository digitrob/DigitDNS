package database

import "testing"

func TestCertificateSettingsDefaultsAndSnapshotMasksToken(t *testing.T) {
	s := DefaultCertificateSettings()
	s.Normalize()
	if got := s.Get(CertPurposePanel); got.Source != CertSourceACME || got.Challenge != CertChallengeHTTP01 || !got.AutoRenew {
		t.Fatalf("panel defaults = %#v", got)
	}
	if err := s.Set(CertPurposeDoT, CertificateConfig{
		Source: CertSourceACME, Domain: "dns.example.com", CertName: "*.example.com",
		Challenge: CertChallengeDNS01, DNSProvider: "cloudflare", DNSZone: "example.com",
		DNSAPIToken: "secret-token", AutoRenew: true,
	}, nil); err != nil {
		t.Fatalf("Set: %v", err)
	}
	view := s.Snapshot().DoT
	if view.CertName != "*.example.com" || view.Challenge != CertChallengeDNS01 || !view.DNSAPITokenSet {
		t.Fatalf("dot view = %#v", view)
	}
}

func TestCertificateSettingsNormalizeLegacyEmptyRecord(t *testing.T) {
	var s CertificateSettings
	s.Normalize()
	for _, purpose := range []string{CertPurposePanel, CertPurposeDoT, CertPurposeSubscription} {
		got := s.Get(purpose)
		if got.Source != CertSourceACME || got.Challenge != CertChallengeHTTP01 || !got.AutoRenew {
			t.Errorf("%s normalized = %#v", purpose, got)
		}
	}
}

func TestCertificateSettingsSetRollsBackOnPersistFailure(t *testing.T) {
	s := DefaultCertificateSettings()
	before := s.Get(CertPurposePanel)
	err := s.Set(CertPurposePanel, CertificateConfig{
		Source: CertSourceFiles, Domain: "panel.example", CertPath: "/tmp/c", KeyPath: "/tmp/k",
	}, func(*CertificateSettings) error { return errPersist })
	if err == nil {
		t.Fatal("expected the persist error")
	}
	if got := s.Get(CertPurposePanel); got != before {
		t.Fatalf("panel changed despite failed persist: %#v", got)
	}
}

var errPersist = &persistError{}

type persistError struct{}

func (*persistError) Error() string { return "persist failed" }
