package blocklist

import (
	"testing"

	"github.com/miekg/dns"
)

func TestDNSTypeNarrowsARuleToNamedTypes(t *testing.T) {
	idx, stats := Build([]Source{{
		Name:    "google quic guard",
		Enabled: true,
		Text:    "||google.com^$dnstype=HTTPS\n",
	}}, nil)
	if stats.Blocked != 1 || stats.Skipped != 0 {
		t.Fatalf("stats = %+v, want 1 blocked and 0 skipped", stats)
	}

	if blocked, _, _ := idx.MatchQ("gemini.google.com", dns.TypeHTTPS); !blocked {
		t.Error("HTTPS query for a covered subdomain was not blocked")
	}
	if blocked, _, _ := idx.MatchQ("google.com", dns.TypeA); blocked {
		t.Error("A query was blocked by an HTTPS-only rule")
	}
	if blocked, _, _ := idx.MatchQ("google.com", dns.TypeAAAA); blocked {
		t.Error("AAAA query was blocked by an HTTPS-only rule")
	}
	// The type-agnostic form is used by callers with no query; it must not
	// invent a type and apply a typed rule.
	if blocked, _, _ := idx.Match("google.com"); blocked {
		t.Error("the query-type-agnostic Match applied a typed rule")
	}
}

func TestDNSTypeAcceptsPipeListAndCombinedModifiers(t *testing.T) {
	idx, stats := Build([]Source{{
		Name:    "list",
		Enabled: true,
		Text:    "||example.com^$dnstype=HTTPS|SVCB,important\n",
	}}, nil)
	if stats.Blocked != 1 || stats.Skipped != 0 {
		t.Fatalf("stats = %+v, want 1 blocked and 0 skipped", stats)
	}
	for _, qtype := range []uint16{dns.TypeHTTPS, dns.TypeSVCB} {
		if blocked, _, _ := idx.MatchQ("example.com", qtype); !blocked {
			t.Errorf("type %d was not blocked", qtype)
		}
	}
	if blocked, _, _ := idx.MatchQ("example.com", dns.TypeA); blocked {
		t.Error("an unlisted type was blocked")
	}
}

func TestDNSTypeAllowExceptionIsAlsoTyped(t *testing.T) {
	idx, _ := Build([]Source{{
		Name:    "list",
		Enabled: true,
		Text:    "||example.com^$dnstype=HTTPS\n@@||api.example.com^$dnstype=HTTPS\n",
	}}, nil)
	if blocked, _, _ := idx.MatchQ("api.example.com", dns.TypeHTTPS); blocked {
		t.Error("typed allowlist did not suppress the typed block")
	}
	if blocked, _, _ := idx.MatchQ("www.example.com", dns.TypeHTTPS); !blocked {
		t.Error("typed block did not apply to another name")
	}
}

func TestDNSTypeInvalidValueSkipsTheRule(t *testing.T) {
	idx, stats := Build([]Source{{
		Name:    "list",
		Enabled: true,
		Text:    "||example.com^$dnstype=NOT_A_TYPE\n",
	}}, nil)
	if stats.Skipped != 1 || stats.Blocked != 0 {
		t.Fatalf("stats = %+v, want 1 skipped and 0 blocked", stats)
	}
	if blocked, _, _ := idx.MatchQ("example.com", dns.TypeHTTPS); blocked {
		t.Error("an invalid type rule was applied as a plain block")
	}
}

func TestDNSTypeOnRegexRule(t *testing.T) {
	idx, stats := Build([]Source{{
		Name:       "custom",
		Enabled:    true,
		AllowRegex: true,
		Text:       "/^ads[0-9]+\\.example\\.com$/$dnstype=HTTPS\n",
	}}, nil)
	if stats.Blocked != 1 || stats.Skipped != 0 {
		t.Fatalf("stats = %+v, want 1 blocked and 0 skipped", stats)
	}
	if blocked, _, _ := idx.MatchQ("ads12.example.com", dns.TypeHTTPS); !blocked {
		t.Error("typed regex did not block HTTPS")
	}
	if blocked, _, _ := idx.MatchQ("ads12.example.com", dns.TypeA); blocked {
		t.Error("typed regex blocked an unlisted type")
	}
}

func TestDNSTypeSurvivesMerge(t *testing.T) {
	one, _ := ParseSource(Source{Name: "one", Enabled: true, Text: "||a.example^$dnstype=HTTPS\n"})
	two, _ := ParseSource(Source{Name: "two", Enabled: true, Text: "||b.example^$dnstype=SVCB\n"})
	merged := Merge([]*Index{one, two}, nil)
	if blocked, _, _ := merged.MatchQ("a.example", dns.TypeHTTPS); !blocked {
		t.Error("merge lost the first typed rule")
	}
	if blocked, _, _ := merged.MatchQ("b.example", dns.TypeSVCB); !blocked {
		t.Error("merge lost the second typed rule")
	}
	if blocked, _, _ := merged.MatchQ("a.example", dns.TypeA); blocked {
		t.Error("merge widened a typed rule")
	}
}

func TestDNSTypeOnAdGuardHomeQuicRules(t *testing.T) {
	// The exact rules an operator pastes to stop QUIC hints. They are skipped by
	// the generic modifier stripper; $dnstype has to be understood here.
	text := `||gemini.google.com^$dnstype=HTTPS
||googleapis.com^$dnstype=HTTPS
||clients6.google.com^$dnstype=HTTPS
||google.com^$dnstype=HTTPS
`
	idx, stats := Build([]Source{{Name: "quic", Enabled: true, Text: text}}, nil)
	if stats.Blocked != 4 || stats.Skipped != 0 {
		t.Fatalf("stats = %+v, want 4 blocked and 0 skipped", stats)
	}
	for _, name := range []string{"gemini.google.com", "www.googleapis.com", "clients6.google.com", "google.com"} {
		if blocked, _, _ := idx.MatchQ(name, dns.TypeHTTPS); !blocked {
			t.Errorf("%s HTTPS was not blocked", name)
		}
		if blocked, _, _ := idx.MatchQ(name, dns.TypeA); blocked {
			t.Errorf("%s A was blocked by a type-narrowed rule", name)
		}
	}
}
