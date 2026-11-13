package blocklist

import (
	"strings"
	"testing"
)

func TestBuildParsesHostsDomainsAndAdblockRules(t *testing.T) {
	idx, stats := Build([]Source{{
		Name:    "test list",
		Enabled: true,
		Text: `
! comment
0.0.0.0 ads.example.com
127.0.0.1 tracker.example
plain.example
||network.example^
|exact.example|
*.sub.example
@@||allowed.example^
example.org##.banner
/.*regex.*/
||modifier.example^$third-party
`,
	}}, nil)
	if stats.Skipped != 3 {
		t.Errorf("skipped = %d, want 3 (cosmetic, regex, unsupported modifier)", stats.Skipped)
	}
	for _, name := range []string{
		"ads.example.com",
		"x.ads.example.com",
		"tracker.example",
		"plain.example",
		"a.plain.example",
		"network.example",
		"a.network.example",
		"exact.example",
		"a.sub.example",
	} {
		if blocked, _, _ := idx.Match(name); !blocked {
			t.Errorf("%s was not blocked", name)
		}
	}
	if blocked, _, _ := idx.Match("sub.example"); blocked {
		t.Error("*.sub.example must not match its apex")
	}
	if blocked, _, _ := idx.Match("a.exact.example"); blocked {
		t.Error("|exact.example| must stay exact")
	}
	if blocked, _, _ := idx.Match("allowed.example"); blocked {
		t.Error("@@ exception did not win")
	}
	if blocked, _, _ := idx.Match("a.allowed.example"); blocked {
		t.Error("@@ exception did not cover subdomains")
	}
}

func TestAllowlistWinsOverBlocklist(t *testing.T) {
	idx, _ := Build([]Source{{
		Name: "list", Enabled: true, Text: "||tracker.example^\n",
	}}, []string{"tracker.example"})
	if blocked, _, _ := idx.Match("tracker.example"); blocked {
		t.Error("operator allowlist did not override the list")
	}
	if blocked, _, _ := idx.Match("a.tracker.example"); blocked {
		t.Error("operator allowlist did not cover subdomains")
	}
}

func TestAnchoredRuleIsExactOnly(t *testing.T) {
	idx, _ := Build([]Source{{Name: "list", Enabled: true, Text: "|exact.example|\n"}}, nil)
	if blocked, _, _ := idx.Match("exact.example"); !blocked {
		t.Fatal("exact rule did not match itself")
	}
	if blocked, _, _ := idx.Match("a.exact.example"); blocked {
		t.Fatal("anchored rule matched a subdomain")
	}
}

func TestAllowlistSupportsWildcards(t *testing.T) {
	idx, _ := Build([]Source{{Name: "list", Enabled: true, Text: "||trusted.example^\n"}}, []string{"*.trusted.example"})
	if blocked, _, _ := idx.Match("trusted.example"); !blocked {
		t.Error("wildcard allowlist must not unblock its apex")
	}
	if blocked, _, _ := idx.Match("a.trusted.example"); blocked {
		t.Error("wildcard allowlist did not cover subdomains")
	}
}

func TestAnalyzeCountsUniqueDomains(t *testing.T) {
	stats := Analyze("0.0.0.0 ads.example\n||ads.example^\n*.wild.example\n")
	if stats.Blocked != 3 {
		t.Errorf("blocked lines = %d, want 3", stats.Blocked)
	}
	if stats.Domains != 2 {
		t.Errorf("unique domains = %d, want 2", stats.Domains)
	}
}

// A plain domain is deliberately stored twice: exact for the apex and suffix
// for subdomains. This is why the compiled rule count is normally about twice
// the unique-domain count.
func TestRuleCountCountsExactAndSuffixEntries(t *testing.T) {
	idx, _ := Build([]Source{{
		Name:    "test",
		Text:    "ads.example\n*.wild.example\n|exact.example|\n",
		Enabled: true,
	}}, nil)
	if got := idx.RuleCount(); got != 4 {
		t.Fatalf("RuleCount = %d, want 4 (plain domain exact+suffix, wildcard suffix, exact-only exact)", got)
	}
	if got := idx.ExactCount(); got != 2 {
		t.Fatalf("ExactCount = %d, want 2", got)
	}
	if got := idx.SuffixCount(); got != 2 {
		t.Fatalf("SuffixCount = %d, want 2", got)
	}
	if got := idx.DomainCount(); got != 3 {
		t.Fatalf("DomainCount = %d, want 3", got)
	}
}

func TestDNSRewriteRulesIndexApexExactSuffixAndMostSpecificAnswer(t *testing.T) {
	idx, stats := Build([]Source{{
		Name: "custom rules",
		Text: strings.Join([]string{
			"||parent.example^$dnsrewrite=192.0.2.1",
			"||child.parent.example^$DNSREWRITE=192.0.2.2",
			"*.wild.example^$dnsrewrite=2001:db8::1",
			"|exact.example|$dnsrewrite=192.0.2.4",
			"||rewrite-cname.example^$dnsrewrite=Alias.Target.Example.",
		}, "\n"),
		Enabled: true,
	}}, nil)

	for _, tc := range []struct {
		domain     string
		wantTarget string
		want       bool
	}{
		{"parent.example", "192.0.2.1", true},
		{"a.parent.example", "192.0.2.1", true},
		{"child.parent.example", "192.0.2.2", true},
		{"a.child.parent.example", "192.0.2.2", true},
		{"wild.example", "", false}, // *.wild.example excludes its apex
		{"a.wild.example", "2001:db8::1", true},
		{"exact.example", "192.0.2.4", true},
		{"a.exact.example", "", false},
		{"rewrite-cname.example", "alias.target.example", true},
		{"child.rewrite-cname.example", "alias.target.example", true},
	} {
		got, ok := idx.MatchRewriteNormalized(tc.domain)
		if got != tc.wantTarget || ok != tc.want {
			t.Errorf("rewrite(%q) = (%q, %v), want (%q, %v)", tc.domain, got, ok, tc.wantTarget, tc.want)
		}
	}
	if stats.Skipped != 0 {
		t.Errorf("skipped = %d, want 0", stats.Skipped)
	}
	if idx.RuleCount() != 8 || idx.DomainCount() != 5 || idx.ExactCount() != 4 || idx.SuffixCount() != 4 {
		t.Errorf("rewrite counts: rules=%d domains=%d exact=%d suffix=%d, want 8/5/4/4",
			idx.RuleCount(), idx.DomainCount(), idx.ExactCount(), idx.SuffixCount())
	}
}

func TestInvalidOrUnsupportedRewriteModifiersAreSkippedNotBlockRules(t *testing.T) {
	idx, stats := Build([]Source{{
		Name: "custom rules",
		Text: strings.Join([]string{
			"||empty.example^$dnsrewrite=",
			"||bad-ip.example^$dnsrewrite=not-an-ip/",
			"||bad-name.example^$dnsrewrite=bad..name",
			"||duplicate.example^$dnsrewrite=192.0.2.1,dnsrewrite=192.0.2.2",
			"@@||rewrite-exception.example^$dnsrewrite=192.0.2.3",
			"||client-scoped.example^$client=192.0.2.4",
			"@@||client-exception.example^$client=192.0.2.5",
			"||denyallow.example^$denyallow=allowed.example",
		}, "\n"),
		Enabled: true,
	}}, nil)

	if stats.Skipped != 8 {
		t.Fatalf("skipped = %d, want 8", stats.Skipped)
	}
	if idx.RuleCount() != 0 || idx.DomainCount() != 0 {
		t.Fatalf("invalid/unsupported modifiers produced rules: rules=%d domains=%d",
			idx.RuleCount(), idx.DomainCount())
	}
	for _, domain := range []string{
		"empty.example", "bad-ip.example", "duplicate.example",
		"bad-name.example",
		"rewrite-exception.example", "client-scoped.example",
		"client-exception.example", "denyallow.example",
	} {
		if blocked, _, _ := idx.Match(domain); blocked {
			t.Errorf("invalid or unsupported rule unexpectedly blocked %q", domain)
		}
		if ip, ok := idx.MatchRewriteNormalized(domain); ok {
			t.Errorf("invalid or unsupported rule unexpectedly rewrote %q to %q", domain, ip)
		}
	}
}

func TestUnsupportedClientExceptionCannotBroadenAllowlist(t *testing.T) {
	idx, stats := Build([]Source{{
		Name:    "custom rules",
		Text:    "||tracker.example^\n@@||tracker.example^$client=192.0.2.10\n",
		Enabled: true,
	}}, nil)

	if stats.Skipped != 1 {
		t.Fatalf("skipped = %d, want 1", stats.Skipped)
	}
	if blocked, _, _ := idx.Match("tracker.example"); !blocked {
		t.Fatal("unsupported client-scoped exception was applied network-wide")
	}
}

func TestRegexRulesAreLimitedToOptedInSourcesAndUseAllowPrecedence(t *testing.T) {
	idx, stats := Build([]Source{
		{
			Name:    "downloaded list",
			Enabled: true,
			Text:    `/^remote-[0-9]+\.example$/` + "\n",
		},
		{
			Name:       "custom rules",
			Enabled:    true,
			AllowRegex: true,
			Text: strings.Join([]string{
				"||ads.example^",
				"/^regex-[0-9]+\\.example$/",
				"@@/^ads\\.example$/",
			}, "\n"),
		},
	}, nil)

	if stats.Skipped != 1 {
		t.Errorf("skipped = %d, want downloaded-list regex skipped", stats.Skipped)
	}
	if blocked, _, _ := idx.Match("remote-1.example"); blocked {
		t.Error("regex from a source without AllowRegex was compiled")
	}
	if blocked, source, _ := idx.Match("regex-42.example"); !blocked || source != "custom rules" {
		t.Errorf("opted-in regex = (%v, %q), want a custom-rules block", blocked, source)
	}
	if blocked, _, _ := idx.Match("ads.example"); blocked {
		t.Error("regex allow exception did not take precedence over the domain block")
	}
	if got := idx.RuleCount(); got != 4 {
		t.Errorf("RuleCount = %d, want 4 compiled entries", got)
	}
	if got := idx.RegexCount(); got != 2 {
		t.Errorf("RegexCount = %d, want 2 opted-in regex entries", got)
	}
}

func TestCustomRegexRuleLimits(t *testing.T) {
	lines := make([]string, maxCustomRegexRules+1)
	for i := range maxCustomRegexRules + 1 {
		lines[i] = "/^regex\\.example$/"
	}
	long := "/" + strings.Repeat("a", maxCustomRegexPatternSize+1) + "/"
	lines = append(lines, long)

	idx, stats := Build([]Source{{
		Name:       "custom rules",
		Enabled:    true,
		AllowRegex: true,
		Text:       strings.Join(lines, "\n"),
	}}, nil)
	if stats.Blocked != maxCustomRegexRules || stats.Skipped != 2 {
		t.Fatalf("regex stats = blocked:%d skipped:%d, want blocked:%d skipped:2",
			stats.Blocked, stats.Skipped, maxCustomRegexRules)
	}
	if got := idx.RuleCount(); got != maxCustomRegexRules {
		t.Fatalf("RuleCount = %d, want %d", got, maxCustomRegexRules)
	}
}
