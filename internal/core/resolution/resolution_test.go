package resolution

import "testing"

func ptr(v bool) *bool { return &v }

func TestMatchPrefersExactThenLongestSuffix(t *testing.T) {
	profiles := map[string]Profile{
		"example.com":   {Strategy: StrategyFastest},
		"*.example.com": {Upstream: "tls://dns.google:853"},
		"*.game.net":    {Transport: TransportEncrypted},
	}
	cases := []struct {
		domain string
		want   string
	}{
		{"example.com", "fastest"},    // exact beats the wildcard
		{"api.example.com", ""},       // *.example.com (apex wildcard)
		{"deep.api.example.com", ""},  // still the same wildcard
		{"game.net", ""},              // wildcard does not match its apex
		{"cdn.game.net", "encrypted"}, // *.game.net
		{"other.net", ""},             // no match
	}
	for _, c := range cases {
		p, ok := Match(profiles, c.domain)
		got := ""
		switch {
		case !ok:
			got = ""
		case p.Strategy != "":
			got = p.Strategy
		case p.Upstream != "":
			got = ""
		case p.Transport != "":
			got = p.Transport
		}
		if got != c.want {
			t.Errorf("Match(%q) = %q (ok=%v), want %q", c.domain, got, ok, c.want)
		}
	}
}

func TestMatchWildcardDoesNotCoverApex(t *testing.T) {
	p, ok := Match(map[string]Profile{"*.example.com": {Upstream: "x"}}, "example.com")
	if ok {
		t.Fatalf("wildcard matched the apex: %+v", p)
	}
}

func TestEncryptedClassifiesTransports(t *testing.T) {
	for _, u := range []string{"tls://dns.google:853", "https://dns.google/dns-query", "quic://dns.adguard.com:853", "h3://dns.google/dns-query", "sdns://abc"} {
		if !Encrypted(u) {
			t.Errorf("%s should be encrypted", u)
		}
	}
	for _, u := range []string{"1.1.1.1:53", "tcp://1.1.1.1:53"} {
		if Encrypted(u) {
			t.Errorf("%s should be plain", u)
		}
	}
}

func TestTransportForInheritsAndReadsLegacySwitches(t *testing.T) {
	cases := []struct {
		name    string
		profile Profile
		global  string
		want    string
	}{
		{"inherit", Profile{}, TransportEncrypted, TransportEncrypted},
		{"explicit regular beats global", Profile{Transport: TransportRegular}, TransportEncrypted, TransportRegular},
		{"legacy true", Profile{EncryptedOnly: ptr(true)}, TransportAny, TransportEncrypted},
		{"legacy false", Profile{EncryptedOnly: ptr(false)}, TransportEncrypted, TransportAny},
		{"unknown global is any", Profile{}, "bogus", TransportAny},
	}
	for _, c := range cases {
		if got := c.profile.TransportFor(c.global); got != c.want {
			t.Errorf("%s: TransportFor(%q) = %q, want %q", c.name, c.global, got, c.want)
		}
	}
}

func TestNormalizedRewritesTheLegacySwitch(t *testing.T) {
	p := Profile{EncryptedOnly: ptr(true)}.Normalized()
	if p.Transport != TransportEncrypted || p.EncryptedOnly != nil {
		t.Fatalf("legacy true not normalised: %+v", p)
	}
	p = Profile{Transport: TransportRegular, EncryptedOnly: ptr(true)}.Normalized()
	if p.Transport != TransportRegular || p.EncryptedOnly != nil {
		t.Fatalf("explicit transport was overwritten: %+v", p)
	}
}

func TestValidTransport(t *testing.T) {
	for _, v := range []string{"", "any", "encrypted", "regular", " ENCRYPTED "} {
		if !ValidTransport(v) {
			t.Errorf("%q should be valid", v)
		}
	}
	for _, v := range []string{"tls", "plain", "encrypted-only"} {
		if ValidTransport(v) {
			t.Errorf("%q should be rejected", v)
		}
	}
}
