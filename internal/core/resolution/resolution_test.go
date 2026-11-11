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

// The compiled index moves policy lookup off the query path's linear scan. It
// must select exactly what the original Match did, including the exact-versus-
// wildcard precedence and the rule that a plain profile is exact-only.
func TestCompiledProfileIndexMatchesLinearLookup(t *testing.T) {
	profiles := map[string]Profile{
		"example.com":              {Strategy: StrategyFastest},
		"*.example.com":            {Upstream: "tls://dns.google:853"},
		"*.api.example.com":        {Transport: TransportEncrypted},
		"deep.api.example.com":     {Strategy: StrategyHedged},
		"*.game.net":               {Transport: TransportRegular},
		"  *.Media.Example.COM.  ": {Upstream: "https://dns.example/dns-query"},
	}
	idx := CompileProfiles(profiles)

	for _, domain := range []string{
		"example.com",
		"EXAMPLE.COM.",
		"api.example.com",
		"x.api.example.com",
		"deep.api.example.com",
		"game.net",
		"cdn.game.net",
		"media.example.com",
		"cdn.media.example.com",
		"other.net",
		"",
	} {
		want, wantOK := Match(profiles, domain)
		got, gotOK := idx.Match(domain)
		if got != want || gotOK != wantOK {
			t.Errorf("Match(%q) = (%+v, %v), compiled = (%+v, %v)", domain, want, wantOK, got, gotOK)
		}
	}
}

func TestNilProfileIndexIsSafe(t *testing.T) {
	var idx *ProfileIndex
	if p, ok := idx.Match("example.com"); ok {
		t.Fatalf("nil index matched: %+v", p)
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

func TestNormalizedRetiresTheFailureFallback(t *testing.T) {
	p := Profile{Upstream: "tls://dns.google:853", Fallback: FallbackFail}.Normalized()
	if p.Fallback != "" {
		t.Fatalf("retired fallback survived normalization: %+v", p)
	}
	if p.Upstream != "tls://dns.google:853" {
		t.Fatalf("normalization dropped the upstream: %+v", p)
	}
}

func TestValidStrategy(t *testing.T) {
	for _, v := range []string{"", StrategyFastest, StrategySequential, StrategyHedged, StrategyBestKnown, StrategyAdaptive} {
		if !ValidStrategy(v) {
			t.Errorf("%q should be a valid strategy", v)
		}
	}
	for _, v := range []string{"race", "random"} {
		if ValidStrategy(v) {
			t.Errorf("%q should be rejected", v)
		}
	}
}

func TestNormalizeStrategyRetiresTheLegacyValues(t *testing.T) {
	cases := []struct {
		value string
		want  string
	}{
		{"", StrategyAdaptive},
		{StrategyFastest, StrategyHedged},
		{StrategySequential, StrategySequential},
		{StrategyHedged, StrategyHedged},
		{StrategyBestKnown, StrategyBestKnown},
		{StrategyAdaptive, StrategyAdaptive},
		{"  ADAPTIVE  ", StrategyAdaptive},
	}
	for _, c := range cases {
		if got := NormalizeStrategy(c.value); got != c.want {
			t.Errorf("NormalizeStrategy(%q) = %q, want %q", c.value, got, c.want)
		}
	}
}

func TestNormalizeProfileStrategyKeepsInherit(t *testing.T) {
	if got := NormalizeProfileStrategy(""); got != "" {
		t.Fatalf("empty profile strategy = %q, want inherit", got)
	}
	if got := NormalizeProfileStrategy(StrategyFastest); got != StrategyHedged {
		t.Fatalf("fastest profile strategy = %q, want %q", got, StrategyHedged)
	}
	if got := NormalizeProfileStrategy(StrategyAdaptive); got != "" {
		t.Fatalf("adaptive profile strategy = %q, want inherit", got)
	}
}

func TestAdaptiveIntervalChoices(t *testing.T) {
	for _, v := range AdaptiveIntervalChoices {
		if !ValidAdaptiveInterval(v) {
			t.Errorf("%d should be a valid adaptive interval", v)
		}
	}
	for _, v := range []int{-1, 0, 1, 10, 59, 61, 1441} {
		if ValidAdaptiveInterval(v) {
			t.Errorf("%d should be rejected", v)
		}
	}
	if got := NormalizeAdaptiveInterval(0); got != DefaultAdaptiveIntervalMinutes {
		t.Errorf("NormalizeAdaptiveInterval(0) = %d, want %d", got, DefaultAdaptiveIntervalMinutes)
	}
	if got := NormalizeAdaptiveInterval(15); got != 15 {
		t.Errorf("NormalizeAdaptiveInterval(15) = %d, want 15", got)
	}
}

func TestProtocolClassification(t *testing.T) {
	cases := map[string]string{
		"1.1.1.1:53":                    ProtocolPlain,
		"tcp://9.9.9.9:53":              ProtocolPlain,
		"tls://dns.google:853":          ProtocolDoT,
		"https://dns.google/dns-query":  ProtocolDoH,
		"h3://dns.google/dns-query":     ProtocolDoH3,
		"quic://dns.adguard.com:853":    ProtocolDoQ,
		"sdns://AQcAAAAAAAA":            ProtocolDNSCrypt,
		"  TLS://ONE.ONE.ONE.ONE:853  ": ProtocolDoT,
	}
	for address, want := range cases {
		if got := Protocol(address); got != want {
			t.Errorf("Protocol(%q) = %q, want %q", address, got, want)
		}
	}
}

func TestNormalizeProtocolsAndLegacyTransport(t *testing.T) {
	all := NormalizeProtocols(nil)
	if len(all) != len(AllProtocols) {
		t.Fatalf("empty protocols = %v, want all", all)
	}
	got := NormalizeProtocols([]string{"doh", "plain", "doh", "bogus"})
	want := []string{ProtocolPlain, ProtocolDoH}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("NormalizeProtocols = %v, want %v", got, want)
	}
	if !ValidProtocols([]string{"dot", "doh"}) {
		t.Fatal("valid protocol set was rejected")
	}
	if ValidProtocols(nil) || ValidProtocols([]string{"bogus"}) {
		t.Fatal("empty/unknown protocol set was accepted")
	}
	if got := LegacyTransportFor([]string{ProtocolPlain}); got != TransportRegular {
		t.Fatalf("plain legacy transport = %q, want %q", got, TransportRegular)
	}
	if got := LegacyTransportFor([]string{ProtocolDoT, ProtocolDoH}); got != TransportEncrypted {
		t.Fatalf("encrypted legacy transport = %q, want %q", got, TransportEncrypted)
	}
	if got := LegacyTransportFor([]string{ProtocolPlain, ProtocolDoH}); got != TransportAny {
		t.Fatalf("mixed legacy transport = %q, want %q", got, TransportAny)
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
