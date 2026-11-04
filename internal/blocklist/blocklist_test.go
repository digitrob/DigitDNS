package blocklist

import "testing"

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
