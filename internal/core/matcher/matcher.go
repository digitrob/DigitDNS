package matcher

import (
	"slices"
	"strings"
	"sync"
)

type Action int

const (
	ActionDirect Action = iota
	ActionProxy
	ActionBlock
)

func (a Action) String() string {
	switch a {
	case ActionProxy:
		return "PROXY"
	case ActionBlock:
		return "BLOCK"
	default:
		return "DIRECT"
	}
}

// Rule names for the operator's own three lists. These are admin-wide: a domain
// typed into one of them applies to every client.
const (
	RuleCustomDirect = "Custom Direct"
	RuleCustomBlock  = "Custom Block"
	RuleCustomProxy  = "Custom Proxy"
)

// Rule names for the forced-direct plane described in realtime.go: names the SNI
// proxy has no path for, which are answered directly no matter what. Neither is a
// category, neither can be switched off, and both apply to every client. They
// appear in the query log so an operator who expects a name to be proxied can see
// which rule answered it directly instead — and which of the two reasons applied.
const (
	RuleRealtimeDirect    = "Realtime Media"
	RuleUnproxyableDirect = "Unproxyable Port"
)

// ruleSet indexes rule domains for one action. exact holds names that match
// themselves; wildcard holds parent domains that match any strict subdomain. A
// plain rule ("riotgames.com") is indexed in both, a wildcard rule
// ("*.riotgames.com") only in wildcard — so "*.foo.com" never matches "foo.com".
//
// Precedence inside a set is longest-suffix-wins: the exact map is probed first,
// then the label walk from most specific parent to least.
type ruleSet struct {
	exact    map[string]string
	wildcard map[string]string
}

func newRuleSet() *ruleSet {
	return &ruleSet{
		exact:    make(map[string]string),
		wildcard: make(map[string]string),
	}
}

// index adds one rule domain. It accepts "foo.com", "*.foo.com" and trailing dots.
func (rs *ruleSet) index(domain, ruleName string) {
	d := normalizeDomain(domain)
	wild := strings.HasPrefix(d, "*.")
	if wild {
		d = d[2:]
	}
	// A pattern the index cannot honor is dropped rather than stored under a
	// literal "*" label, where it would silently never match.
	if d == "" || strings.Contains(d, "*") {
		return
	}
	if !wild {
		rs.exact[d] = ruleName
	}
	rs.wildcard[d] = ruleName
}

// unindex removes one rule domain, accepting the same forms index does. It exists
// so a domain can be lifted out of the forced-direct set when an operator's own
// Custom Proxy entry covers it.
//
// It removes only the exact pattern given: unindexing "voice.example.com" leaves a
// "example.com" entry in place.
func (rs *ruleSet) unindex(domain string) {
	d := normalizeDomain(domain)
	wild := strings.HasPrefix(d, "*.")
	if wild {
		d = d[2:]
	}
	if d == "" || strings.Contains(d, "*") {
		return
	}
	if !wild {
		delete(rs.exact, d)
	}
	delete(rs.wildcard, d)
}

// lookup returns the rule matching d, walking from the most specific candidate to
// the least.
func (rs *ruleSet) lookup(d string) (string, bool) {
	if name, ok := rs.exact[d]; ok {
		return name, true
	}
	// An empty set is the normal state for the operator's custom lists, and the
	// label walk is the expensive half of a lookup — skip it rather than probe an
	// empty map once per label of every queried name.
	if len(rs.wildcard) == 0 {
		return "", false
	}
	// Probe each parent domain in turn: for "a.b.riotgames.com" that is
	// "b.riotgames.com", then "riotgames.com", then "com". The substrings share
	// d's backing array, so the walk allocates nothing.
	for i := 0; i < len(d); {
		dot := strings.IndexByte(d[i:], '.')
		if dot < 0 {
			break
		}
		i += dot + 1
		if i >= len(d) {
			break
		}
		if name, ok := rs.wildcard[d[i:]]; ok {
			return name, true
		}
	}
	return "", false
}

// size is the number of distinct rule domains indexed. Every indexed domain writes
// the wildcard map, so its length is the count.
func (rs *ruleSet) size() int {
	return len(rs.wildcard)
}

// Matcher resolves a domain to an action against four indexed rule sets. Reads take
// the read lock, so rules can be replaced at runtime while queries are served.
type Matcher struct {
	mu            sync.RWMutex
	direct        *ruleSet
	realtime      *ruleSet
	blocked       *ruleSet
	proxied       *ruleSet
	customRecords map[string]string // domain -> IP override

	// The operator's custom rule state, held so it survives a group change:
	// rebuildLocked re-applies groups, then these. Groups first, then the flat
	// lists, so a bare Custom Proxied/Blocked/Direct entry the operator typed still
	// wins over a group — the flat lists are the most specific statement of intent.
	customProxied []string
	customBlocked []string
	customDirect  []string
	customGroups  []CustomGroup
}

// CustomGroup is an operator-defined named policy: a bundle of domains that resolve
// to one action, toggled as a unit. It is the structured form of the flat Custom
// Proxied/Blocked/Direct lists — same precedence tier as its action, but named,
// individually switchable, and editable at runtime. It is decoupled from the
// database row on purpose, so the matcher package keeps no storage dependency.
type CustomGroup struct {
	Name    string
	Action  Action
	Domains []string
	Enabled bool
}

func NewMatcher() *Matcher {
	m := &Matcher{
		direct:        newRuleSet(),
		realtime:      newRuleSet(),
		blocked:       newRuleSet(),
		proxied:       newRuleSet(),
		customRecords: make(map[string]string),
	}
	m.rebuildLocked()
	return m
}

// rebuildLocked reindexes every rule set from scratch. Caller holds mu.
func (m *Matcher) rebuildLocked() {
	m.direct = newRuleSet()
	m.realtime = newRuleSet()
	m.blocked = newRuleSet()
	m.proxied = newRuleSet()

	for _, g := range forcedDirectGroups {
		for _, d := range g.domains {
			m.realtime.index(d, g.rule)
		}
	}

	m.applyCustomGroupsLocked()
	m.applyCustomListsLocked()
}

// applyCustomGroupsLocked indexes every enabled custom group into the ruleset for
// its action. Caller holds mu and has already rebuilt the base indexes.
func (m *Matcher) applyCustomGroupsLocked() {
	for _, g := range m.customGroups {
		if !g.Enabled {
			continue
		}
		name := "Custom Group: " + g.Name
		var target *ruleSet
		switch g.Action {
		case ActionBlock:
			target = m.blocked
		case ActionDirect:
			target = m.direct
		default:
			target = m.proxied
		}
		for _, d := range g.Domains {
			target.index(d, name)
			if g.Action == ActionProxy {
				// Same override the flat Custom Proxied list performs: a proxied
				// group name must beat the forced-direct guard.
				m.realtime.unindex(d)
			}
		}
	}
}

// applyCustomListsLocked indexes the flat Custom Proxied/Blocked/Direct lists.
// Caller holds mu; the sets were reset by rebuildLocked.
func (m *Matcher) applyCustomListsLocked() {
	for _, d := range m.customDirect {
		m.direct.index(d, RuleCustomDirect)
	}
	for _, d := range m.customBlocked {
		m.blocked.index(d, RuleCustomBlock)
	}
	for _, d := range m.customProxied {
		m.proxied.index(d, RuleCustomProxy)
		m.realtime.unindex(d)
	}
}

// normalizeDomain lowercases a name and strips the trailing root label. DNS names
// are case-insensitive, and the rule index is keyed without the root dot.
func normalizeDomain(domain string) string {
	d := strings.TrimSuffix(strings.TrimSpace(domain), ".")
	return strings.ToLower(d)
}

// Match resolves a domain against the global rule set.
func (m *Matcher) Match(domain string) (Action, string) {
	return m.match(domain, nil)
}

// MatchForClient resolves a domain on behalf of a client. The per-client policy
// selection is accepted for API compatibility but no longer narrows the result:
// the only rules this build indexes are the operator's own admin-wide lists and
// groups, which apply to every client.
func (m *Matcher) MatchForClient(domain string, clientPolicies []string) (Action, string) {
	return m.match(domain, clientPolicies)
}

// match is the single implementation behind both entry points, so the global and
// per-client paths cannot drift apart in precedence or in rule handling.
func (m *Matcher) match(domain string, _ []string) (Action, string) {
	d := normalizeDomain(domain)
	if d == "" {
		return ActionDirect, "Direct"
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	// Precedence: the operator's whitelist, then blocks, then the forced-direct
	// real-time plane, then proxy rules.
	//
	// Blocks outrank the real-time set on purpose. An operator who typed a name
	// into Custom Block asked for it to stop resolving, which is a stronger
	// statement than "do not proxy this".
	if name, ok := m.direct.lookup(d); ok {
		return ActionDirect, name
	}
	if name, ok := m.blocked.lookup(d); ok {
		return ActionBlock, name
	}
	if name, ok := m.realtime.lookup(d); ok {
		return ActionDirect, name
	}
	if name, ok := m.proxied.lookup(d); ok {
		return ActionProxy, name
	}
	return ActionDirect, "Direct"
}

// GetCustomRecord returns the custom DNS answer configured for a domain, if any.
func (m *Matcher) GetCustomRecord(domain string) (string, bool) {
	d := normalizeDomain(domain)
	m.mu.RLock()
	defer m.mu.RUnlock()
	ip, ok := m.customRecords[d]
	return ip, ok
}

// RuleCounts is the number of distinct rule domains indexed per action.
func (m *Matcher) RuleCounts() (direct, blocked, proxied int) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.direct.size() + m.realtime.size(), m.blocked.size(), m.proxied.size()
}

// SetCustomRules atomically rebuilds the operator's custom rule lists and custom A
// records. Safe for concurrent use and can be called repeatedly at runtime.
func (m *Matcher) SetCustomRules(customProxied, customBlocked, customDirect []string, customRecords map[string]string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.customProxied = slices.Clone(customProxied)
	m.customBlocked = slices.Clone(customBlocked)
	m.customDirect = slices.Clone(customDirect)

	m.rebuildLocked()

	m.customRecords = make(map[string]string, len(customRecords))
	for d, ip := range customRecords {
		// The lookup side normalizes the queried name, so the key must be
		// normalized the same way: a configured "pin.example." never matched.
		d = normalizeDomain(d)
		ip = strings.TrimSpace(ip)
		if d != "" && ip != "" {
			m.customRecords[d] = ip
		}
	}
}

// SetCustomGroups replaces the operator's named custom policy groups and reindexes.
func (m *Matcher) SetCustomGroups(groups []CustomGroup) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.customGroups = make([]CustomGroup, len(groups))
	for i, g := range groups {
		m.customGroups[i] = CustomGroup{
			Name:    g.Name,
			Action:  g.Action,
			Domains: slices.Clone(g.Domains),
			Enabled: g.Enabled,
		}
	}
	m.rebuildLocked()
}
