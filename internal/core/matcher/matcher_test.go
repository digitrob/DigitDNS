package matcher

import (
	"fmt"
	"sync"
	"testing"
)

// The matcher decides, for every query the daemon answers, whether the client
// reaches the real host, the operator's proxy, or nothing at all. A defect here is
// not a slow path — it is traffic routed somewhere the operator did not intend.
// These tests pin the properties the rest of the daemon relies on: the operator's
// own lists resolve to the action they name, a name the proxy has no path for is
// answered directly whatever the operator typed, the most specific rule indexed
// for a name wins, and a save replaces the index rather than accumulating.
//
// On the race detector: it is unavailable in this environment (no C compiler), so
// TestConcurrentMatchDuringReload is a stress substitute. It can surface a panic or
// a torn read; it cannot certify the matcher race-free.

// mustMatch asserts the action for a domain and returns the rule name, so a test
// can go on to check attribution without repeating the call.
func mustMatch(t *testing.T, m *Matcher, domain string, want Action) string {
	t.Helper()
	got, rule := m.Match(domain)
	if got != want {
		t.Errorf("Match(%q) = %v (%s), want %v", domain, got, rule, want)
	}
	return rule
}

func mustMatchClient(t *testing.T, m *Matcher, domain string, policies []string, want Action) string {
	t.Helper()
	got, rule := m.MatchForClient(domain, policies)
	if got != want {
		t.Errorf("MatchForClient(%q, %v) = %v (%s), want %v", domain, policies, got, rule, want)
	}
	return rule
}

// The operator's own proxied list is the only source of PROXY verdicts. A name no
// rule covers is forwarded, not captured, and the rule label matters: the query log
// distinguishes "matched nothing" from "matched a rule whose action happens to be
// DIRECT".
func TestCustomProxiedListResolvesToProxy(t *testing.T) {
	m := NewMatcher()
	m.SetCustomRules([]string{"game.example", "*.cdn.example"}, nil, nil, nil)

	for _, d := range []string{"game.example", "sub.game.example", "asset.cdn.example"} {
		t.Run(d, func(t *testing.T) { mustMatch(t, m, d, ActionProxy) })
	}

	for _, d := range []string{"example.com", "random-unblocked-site.org", "a.b.internal.example.org"} {
		t.Run(d, func(t *testing.T) {
			if rule := mustMatch(t, m, d, ActionDirect); rule != "Direct" {
				t.Errorf("rule = %q, want %q", rule, "Direct")
			}
		})
	}
}

// A PROXY verdict answers A with the VPS address, and the SNI proxy listens on TCP
// 80/443/2099/5222/5223/8393 and nothing else. So proxying a name whose traffic is
// UDP does not slow it down, it removes it: the client connects to an address where
// nothing is listening, and cannot tell that the name it resolved was substituted.
func TestRealtimePlaneIsForcedDirect(t *testing.T) {
	m := NewMatcher()

	for _, d := range []string{
		"discord.media",
		"voice.discord.media",
		"latency.discord.media", // the region probe
		"vivox.com",
		"bop.vivox.com",
		"v5.vivox.com",
		"bd8-usw2.vivox.com",
	} {
		t.Run(d, func(t *testing.T) {
			if rule := mustMatch(t, m, d, ActionDirect); rule != RuleRealtimeDirect {
				t.Errorf("rule = %q, want %q", rule, RuleRealtimeDirect)
			}
			// A client carrying any policy selection gets the same answer: the
			// guard is not an opt-in category, and this build does not narrow the
			// result per client at all.
			mustMatchClient(t, m, d, []string{"enable_discord", "enable_riot"}, ActionDirect)
		})
	}
}

// The guard corrects the operator's proxy rules, not the operator. Someone who
// types one of these names into their own list has said so deliberately, and the
// rule that a custom entry is the most specific statement applies here too.
func TestOperatorOverridesRealtimePlane(t *testing.T) {
	m := NewMatcher()

	m.SetCustomRules([]string{"discord.media"}, nil, nil, nil)
	if rule := mustMatch(t, m, "discord.media", ActionProxy); rule != RuleCustomProxy {
		t.Errorf("rule = %q, want %q", rule, RuleCustomProxy)
	}
	// Naming the parent lifts the zone with it, the same way indexing it would
	// cover the zone.
	mustMatch(t, m, "voice.discord.media", ActionProxy)

	// Only the pattern they named is lifted, though. An entry for a name *under* a
	// guarded domain does not reach the guard's own key, so it stays direct — the
	// narrow case fails toward the behaviour that works.
	m.SetCustomRules([]string{"voice.discord.media"}, nil, nil, nil)
	if rule := mustMatch(t, m, "voice.discord.media", ActionDirect); rule != RuleRealtimeDirect {
		t.Errorf("rule = %q, want %q", rule, RuleRealtimeDirect)
	}

	// A block outranks the guard: "stop resolving this" is a stronger instruction
	// than "do not proxy this".
	m.SetCustomRules(nil, []string{"vivox.com"}, nil, nil)
	if rule := mustMatch(t, m, "vivox.com", ActionBlock); rule != RuleCustomBlock {
		t.Errorf("rule = %q, want %q", rule, RuleCustomBlock)
	}
	// Cleared again, the guard is back.
	m.SetCustomRules(nil, nil, nil, nil)
	mustMatch(t, m, "vivox.com", ActionDirect)
}

// The other half of the forced-direct plane: a name whose transport is TCP but
// whose port is not one the proxy binds. Steam's connection manager is the case —
// the client's control channel for login, friends, chat and presence.
func TestSteamConnectionManagerIsForcedDirect(t *testing.T) {
	m := NewMatcher()

	if rule := mustMatch(t, m, "cm.steampowered.com", ActionDirect); rule != RuleUnproxyableDirect {
		t.Errorf("rule = %q, want %q", rule, RuleUnproxyableDirect)
	}
	// A client carrying a policy selection gets the same answer: this is not an
	// opt-in category.
	mustMatchClient(t, m, "cm.steampowered.com", []string{"enable_steam"}, ActionDirect)

	// An operator who names it themselves has overridden the guard, exactly as
	// with the real-time set.
	m.SetCustomRules([]string{"cm.steampowered.com"}, nil, nil, nil)
	if rule := mustMatch(t, m, "cm.steampowered.com", ActionProxy); rule != RuleCustomProxy {
		t.Errorf("rule = %q, want %q", rule, RuleCustomProxy)
	}
}

// The operator's three lists are admin-wide. This build resolves no per-client
// policy selection, so a domain the operator typed applies to every subscriber
// whatever plan they carry.
func TestCustomListsApplyToClientsWithPolicies(t *testing.T) {
	m := NewMatcher()
	m.SetCustomRules(
		[]string{"relay.example"},
		[]string{"malware.example", "*.ads.example"},
		[]string{"bank.example"},
		nil,
	)
	riotOnly := []string{"enable_riot"}

	tests := []struct {
		domain string
		want   Action
		rule   string
	}{
		{"malware.example", ActionBlock, RuleCustomBlock},
		{"tracker.ads.example", ActionBlock, RuleCustomBlock},
		{"relay.example", ActionProxy, RuleCustomProxy},
		{"bank.example", ActionDirect, RuleCustomDirect},
		{"sub.bank.example", ActionDirect, RuleCustomDirect},
	}
	for _, tc := range tests {
		t.Run(tc.domain, func(t *testing.T) {
			if rule := mustMatchClient(t, m, tc.domain, riotOnly, tc.want); rule != tc.rule {
				t.Errorf("rule = %q, want %q", rule, tc.rule)
			}
			// The global view must agree; both entry points share one path.
			if rule := mustMatch(t, m, tc.domain, tc.want); rule != tc.rule {
				t.Errorf("global rule = %q, want %q", rule, tc.rule)
			}
		})
	}
}

// The operator typed these in by hand, so the list a domain lands in decides its
// action, whichever list that is.
func TestCustomRulesResolveToTheirAction(t *testing.T) {
	m := NewMatcher()
	m.SetCustomRules(nil, []string{"ads.example"}, []string{"direct.example"}, nil)

	if rule := mustMatch(t, m, "ads.example", ActionBlock); rule != RuleCustomBlock {
		t.Errorf("rule = %q, want %q", rule, RuleCustomBlock)
	}
	if rule := mustMatch(t, m, "direct.example", ActionDirect); rule != RuleCustomDirect {
		t.Errorf("rule = %q, want %q", rule, RuleCustomDirect)
	}
}

// Precedence inside one rule set is longest-suffix-wins, so a wildcard entry for a
// deeper zone beats a plain entry for its parent.
func TestLongestSuffixWins(t *testing.T) {
	rs := newRuleSet()
	rs.index("example.com", "Short")
	rs.index("*.deep.example.com", "Long")

	tests := []struct {
		domain string
		want   string
	}{
		{"host.deep.example.com", "Long"},
		{"a.b.host.deep.example.com", "Long"},
		{"deep.example.com", "Short"}, // the wildcard does not match its own parent
		{"other.example.com", "Short"},
		{"example.com", "Short"},
		{"notexample.com", ""},
		{"com", ""},
	}
	for _, tc := range tests {
		t.Run(tc.domain, func(t *testing.T) {
			name, ok := rs.lookup(tc.domain)
			if ok != (tc.want != "") || name != tc.want {
				t.Errorf("lookup(%q) = (%q, %v), want %q", tc.domain, name, ok, tc.want)
			}
		})
	}
}

// A plain rule matches the name itself and every subdomain; a *. rule matches only
// strict subdomains. Keeping the two distinct is what lets an operator proxy a CDN's
// hosts without capturing the apex, and it is the semantics the old leading-dot
// suffix encoding had.
func TestWildcardAndPlainRuleScope(t *testing.T) {
	m := NewMatcher()
	m.SetCustomRules([]string{"*.cdn.example", "plain.example"}, nil, nil, nil)

	mustMatch(t, m, "asset.cdn.example", ActionProxy)
	mustMatch(t, m, "a.b.cdn.example", ActionProxy)
	mustMatch(t, m, "cdn.example", ActionDirect)

	mustMatch(t, m, "plain.example", ActionProxy)
	mustMatch(t, m, "www.plain.example", ActionProxy)

	// A shared suffix is not a label boundary: "notplain.example" must not match a
	// rule for "plain.example".
	mustMatch(t, m, "notplain.example", ActionDirect)
	mustMatch(t, m, "xcdn.example", ActionDirect)
}

// A pattern the label index cannot honor is dropped rather than stored under a
// literal "*" label, where it would sit in the map looking like a rule and never
// match anything.
func TestIndexRejectsUnsupportedPatterns(t *testing.T) {
	rs := newRuleSet()
	for _, pattern := range []string{"", "   ", ".", "*", "*.", "**", "foo.*.com", "*.foo.*", "a*b.com"} {
		rs.index(pattern, "Rejected")
	}
	if n := rs.size(); n != 0 {
		t.Errorf("indexed %d of the unsupported patterns, want 0", n)
	}

	// The forms that are supported, including a trailing root dot and mixed case.
	rs.index("Foo.COM.", "Kept")
	rs.index("*.Bar.Example.", "Kept")
	if n := rs.size(); n != 2 {
		t.Fatalf("index size = %d, want 2", n)
	}
	if name, ok := rs.lookup("host.bar.example"); !ok || name != "Kept" {
		t.Errorf("lookup of a normalized wildcard = (%q, %v), want (\"Kept\", true)", name, ok)
	}
	if name, ok := rs.lookup("foo.com"); !ok || name != "Kept" {
		t.Errorf("lookup of a normalized plain rule = (%q, %v), want (\"Kept\", true)", name, ok)
	}
}

// Queries arrive as wire-format names: case-preserved as the client typed them and
// carrying the root label. The index is keyed without either.
func TestQueryNormalization(t *testing.T) {
	m := NewMatcher()
	m.SetCustomRules([]string{"game.example"}, nil, nil, nil)

	for _, spelling := range []string{
		"game.example",
		"game.example.",
		"GAME.EXAMPLE",
		"Game.Example.",
		"  game.example.  ",
	} {
		t.Run(spelling, func(t *testing.T) { mustMatch(t, m, spelling, ActionProxy) })
	}

	// A name that normalizes to nothing must not panic or match a rule.
	for _, empty := range []string{"", ".", "   ", "  .  "} {
		if action, rule := m.Match(empty); action != ActionDirect || rule != "Direct" {
			t.Errorf("Match(%q) = %v (%s), want DIRECT (Direct)", empty, action, rule)
		}
	}
}

// SetCustomRules replaces the operator's lists; it does not accumulate. Calling it
// twice with the same input must leave the same index, and a domain dropped from the
// list must stop matching — this is the path the dashboard takes on every save.
func TestSetCustomRulesReplacesRatherThanAccumulates(t *testing.T) {
	m := NewMatcher()
	// The direct baseline is not zero: the forced-direct real-time set is counted
	// there too, and it is rebuilt on every save.
	baseDirect, baseBlocked, baseProxied := m.RuleCounts()

	m.SetCustomRules([]string{"a.example", "b.example"}, []string{"bad.example"}, []string{"ok.example"}, nil)
	direct, blocked, proxied := m.RuleCounts()
	if direct != baseDirect+1 {
		t.Errorf("direct count = %d, want %d", direct, baseDirect+1)
	}
	if blocked != baseBlocked+1 {
		t.Errorf("blocked count = %d, want %d", blocked, baseBlocked+1)
	}
	if proxied != baseProxied+2 {
		t.Errorf("proxied count = %d, want %d", proxied, baseProxied+2)
	}

	m.SetCustomRules([]string{"a.example", "b.example"}, []string{"bad.example"}, []string{"ok.example"}, nil)
	if d, b, p := m.RuleCounts(); d != direct || b != blocked || p != proxied {
		t.Errorf("counts after an identical reload = (%d, %d, %d), want (%d, %d, %d)", d, b, p, direct, blocked, proxied)
	}

	m.SetCustomRules(nil, nil, nil, nil)
	if d, b, p := m.RuleCounts(); d != baseDirect || b != baseBlocked || p != baseProxied {
		t.Errorf("counts after clearing = (%d, %d, %d), want (%d, %d, %d)", d, b, p, baseDirect, baseBlocked, baseProxied)
	}
	mustMatch(t, m, "bad.example", ActionDirect)
}

// A custom A record is keyed by the normalized name, because GetCustomRecord
// normalizes the queried name before probing: a configured "pin.example." never
// matched anything.
func TestCustomRecordNormalization(t *testing.T) {
	m := NewMatcher()
	m.SetCustomRules(nil, nil, nil, map[string]string{
		"Pin.Example.":  "10.0.0.1",
		"  spaced.io ":  " 10.0.0.2 ",
		"blank.example": "   ",
		"":              "10.0.0.3",
	})

	for _, probe := range []string{"pin.example", "PIN.EXAMPLE.", "Pin.Example"} {
		if ip, ok := m.GetCustomRecord(probe); !ok || ip != "10.0.0.1" {
			t.Errorf("GetCustomRecord(%q) = (%q, %v), want (\"10.0.0.1\", true)", probe, ip, ok)
		}
	}
	if ip, ok := m.GetCustomRecord("spaced.io"); !ok || ip != "10.0.0.2" {
		t.Errorf("GetCustomRecord(\"spaced.io\") = (%q, %v), want (\"10.0.0.2\", true)", ip, ok)
	}
	// An entry with no address would answer the query with an empty A record.
	if _, ok := m.GetCustomRecord("blank.example"); ok {
		t.Error("an entry with a blank address was kept")
	}
}

// The index is replaced wholesale on every dashboard save while queries are being
// served, so readers must never observe a half-built rule set. The race detector is
// unavailable in this environment (no C compiler), so this is a stress substitute: it
// can surface a panic or a torn read, but it cannot certify the matcher race-free.
func TestConcurrentMatchDuringReload(t *testing.T) {
	const (
		readers = 24
		rounds  = 500
	)
	m := NewMatcher()
	policies := []string{"enable_riot", "enable_steam", "enable_discord"}

	names := make([]string, 32)
	for i := range names {
		names[i] = fmt.Sprintf("host%d.unknown.example", i)
	}

	var wg sync.WaitGroup
	for w := range readers {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for r := range rounds {
				d := names[(w*rounds+r)%len(names)]
				switch r % 4 {
				case 0:
					// The reader's names are covered by no rule, so this must
					// hold whatever the writer is doing to the custom lists.
					if action, rule := m.Match(d); action != ActionDirect {
						t.Errorf("Match(%q) = %v (%s) during a reload, want DIRECT", d, action, rule)
						return
					}
				case 1:
					m.MatchForClient(d, policies)
				case 2:
					m.GetCustomRecord(d)
				default:
					m.RuleCounts()
				}
			}
		}(w)
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := range 200 {
			m.SetCustomRules(
				[]string{fmt.Sprintf("relay%d.example", i), "*.cdn.example"},
				[]string{fmt.Sprintf("bad%d.example", i)},
				[]string{fmt.Sprintf("ok%d.example", i)},
				map[string]string{fmt.Sprintf("pin%d.example", i): "10.0.0.1"},
			)
		}
	}()

	wg.Wait()
}

func TestMatchDetailReturnsTheMatchedPattern(t *testing.T) {
	m := NewMatcher()
	m.SetCustomRules(
		[]string{"api.example.com", "*.parent.test"},
		[]string{"*.ads.test"},
		nil,
		nil,
	)

	cases := []struct {
		domain  string
		action  Action
		rule    string
		pattern string
	}{
		{"api.example.com", ActionProxy, RuleCustomProxy, "api.example.com"},
		{"cdn.parent.test", ActionProxy, RuleCustomProxy, "*.parent.test"},
		{"deep.cdn.parent.test", ActionProxy, RuleCustomProxy, "*.parent.test"},
		{"x.ads.test", ActionBlock, RuleCustomBlock, "*.ads.test"},
		{"unmatched.test", ActionDirect, "Direct", ""},
	}
	for _, c := range cases {
		action, rule, pattern := m.MatchDetail(c.domain)
		if action != c.action || rule != c.rule || pattern != c.pattern {
			t.Errorf("MatchDetail(%q) = (%v, %q, %q), want (%v, %q, %q)",
				c.domain, action, rule, pattern, c.action, c.rule, c.pattern)
		}
	}
}
