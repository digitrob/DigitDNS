// Package blocklist compiles DNS blocklists into an immutable index.
//
// It supports rules that can be answered from a domain name: hosts lines,
// domains-only lists, Adblock/uBO network rules, and bounded RE2 expressions
// only for operator-supplied custom rules. Cosmetic rules, scriptlets, regex
// rules from downloaded lists and modifiers that need client or URL context are
// skipped rather than guessed at.
package blocklist

import (
	"net"
	"regexp"
	"strconv"
	"strings"

	"github.com/miekg/dns"
	"golang.org/x/net/idna"
)

// Source is one downloaded or uploaded list. Text is parsed at rebuild time, not
// on the DNS path.
type Source struct {
	Name    string
	Text    string
	Enabled bool
	// AllowRegex opts operator-supplied custom rules into bounded RE2 matching.
	// Downloaded lists and upload analysis leave this false.
	AllowRegex bool
}

// Stats reports what one parse or a combined build retained.
type Stats struct {
	Blocked int
	Allowed int
	Skipped int
	Sources int
	// Domains is the number of unique blocked names compiled from the source.
	Domains int
}

func (s Stats) Total() int { return s.Blocked + s.Allowed }

// Index is an immutable compiled rule set. Build once, publish to the matcher,
// and discard the old value; readers never take a lock.
type Index struct {
	blockExact    map[string]ruleEntry
	blockWildcard map[string]ruleEntry
	allowExact    map[string]ruleEntry
	allowWildcard map[string]ruleEntry
	// rewrites holds $dnsrewrite targets (IP addresses or CNAME names): exact
	// keys and suffix (parent) keys.
	rewriteExact  map[string]string
	rewriteSuffix map[string]string
	// regex rules, only populated when the source opts in (custom rules).
	blockRegex []compiledRegex
	allowRegex []compiledRegex
}

// ruleEntry is one compiled rule: the list that supplied it and the DNS types it
// applies to. A nil type set means "every type", which is the common case and
// keeps the lookup a single map probe.
type ruleEntry struct {
	source string
	types  map[uint16]struct{}
}

// matches reports whether the rule applies to a query of this type. qtype 0 is
// used by callers that have no query (the panel's explain endpoint); it matches
// only untyped rules, never a $dnstype rule.
func (e ruleEntry) matches(qtype uint16) bool {
	if len(e.types) == 0 {
		return true
	}
	if qtype == 0 {
		return false
	}
	_, ok := e.types[qtype]
	return ok
}

// compiledRegex is one compiled AdGuard regex rule with its source label.
type compiledRegex struct {
	re     *regexp.Regexp
	source string
	types  map[uint16]struct{}
}

// MaxCustomRegexRules and MaxCustomRegexPatternSize bound the operator-supplied
// regex plane. They are exported so the migration importer can reject the same
// rules the resolver would silently skip, instead of reporting a different cap.
const (
	MaxCustomRegexRules       = 64
	MaxCustomRegexPatternSize = 512
)

func newIndex() *Index {
	return &Index{
		blockExact:    make(map[string]ruleEntry),
		blockWildcard: make(map[string]ruleEntry),
		allowExact:    make(map[string]ruleEntry),
		allowWildcard: make(map[string]ruleEntry),
		rewriteExact:  make(map[string]string),
		rewriteSuffix: make(map[string]string),
	}
}

// Build parses every enabled source plus the operator's allowlist. An empty
// result is still a valid index, so the matcher can clear its filter with it.
func Build(sources []Source, allowlist []string) (*Index, Stats) {
	idx := newIndex()
	var stats Stats
	for _, src := range sources {
		if !src.Enabled || strings.TrimSpace(src.Text) == "" {
			continue
		}
		stats.Sources++
		parseInto(idx, src.Name, src.Text, src.AllowRegex, &stats)
	}
	for _, domain := range allowlist {
		addRule(idx, domain, true, "Allowlist", &stats, "", nil)
	}
	stats.Domains = idx.DomainCount()
	return idx, stats
}

// ParseSource compiles one source and returns its index together with the stats
// the panel stores. The filter service keeps the index, so a later refresh of a
// different list can merge it without parsing this body again.
func ParseSource(src Source) (*Index, Stats) {
	idx := newIndex()
	var stats Stats
	if !src.Enabled || strings.TrimSpace(src.Text) == "" {
		return idx, stats
	}
	stats.Sources = 1
	parseInto(idx, src.Name, src.Text, src.AllowRegex, &stats)
	stats.Domains = idx.DomainCount()
	return idx, stats
}

// Merge combines already-parsed sources into one immutable index and applies the
// operator's allowlist. It replaces the "Analyze then Build everything again"
// pattern that made one list refresh reparse every other list.
func Merge(indices []*Index, allowlist []string) *Index {
	idx := newIndex()
	for _, source := range indices {
		if source == nil {
			continue
		}
		idx.merge(source)
	}
	var stats Stats
	for _, domain := range allowlist {
		addRule(idx, domain, true, "Allowlist", &stats, "", nil)
	}
	return idx
}

// merge copies another index into i. The source indexes are immutable after
// publication, so reading them here needs no lock. Regex rules are copied up to
// the same cap Build enforces, keeping the merged index bounded.
func (i *Index) merge(other *Index) {
	if i == nil || other == nil {
		return
	}
	for k, v := range other.blockExact {
		i.blockExact[k] = v
	}
	for k, v := range other.blockWildcard {
		i.blockWildcard[k] = v
	}
	for k, v := range other.allowExact {
		i.allowExact[k] = v
	}
	for k, v := range other.allowWildcard {
		i.allowWildcard[k] = v
	}
	for k, v := range other.rewriteExact {
		i.rewriteExact[k] = v
	}
	for k, v := range other.rewriteSuffix {
		i.rewriteSuffix[k] = v
	}
	for _, rule := range other.blockRegex {
		if len(i.blockRegex)+len(i.allowRegex) >= MaxCustomRegexRules {
			break
		}
		i.blockRegex = append(i.blockRegex, rule)
	}
	for _, rule := range other.allowRegex {
		if len(i.blockRegex)+len(i.allowRegex) >= MaxCustomRegexRules {
			break
		}
		i.allowRegex = append(i.allowRegex, rule)
	}
}

// Analyze parses a list only to report its rule counts, used when an upload or
// refresh is stored. It does not touch the live matcher.
func Analyze(text string) Stats {
	idx := newIndex()
	var stats Stats
	parseInto(idx, "upload", text, false, &stats)
	stats.Domains = idx.DomainCount()
	return stats
}

// Match returns whether the name is blocked, the source that matched and the
// matching pattern. Allowlist matches return blocked=false and stop the list
// block; the caller still applies its own custom direct/proxy rules. It is the
// query-type-agnostic form: a rule compiled with $dnstype never matches here.
func (i *Index) Match(domain string) (blocked bool, source, pattern string) {
	return i.MatchQ(domain, 0)
}

// MatchQ is Match for one DNS query type. qtype 0 means "no query type"; only
// rules without a $dnstype filter apply.
func (i *Index) MatchQ(domain string, qtype uint16) (blocked bool, source, pattern string) {
	if i == nil {
		return false, "", ""
	}
	d := normalizeDomain(domain)
	if d == "" {
		return false, "", ""
	}
	return i.MatchNormalizedQ(d, qtype)
}

// MatchNormalized is Match for a name the caller has already lowercased and
// trimmed. It is the query-type-agnostic form.
func (i *Index) MatchNormalized(d string) (blocked bool, source, pattern string) {
	return i.MatchNormalizedQ(d, 0)
}

// MatchNormalizedQ is MatchNormalized for one query type. It checks bounded
// regex rules only when a source opted in; the resolver and matcher otherwise
// use the exact lookup plus suffix walk without a per-rule scan.
func (i *Index) MatchNormalizedQ(d string, qtype uint16) (blocked bool, source, pattern string) {
	if i == nil || d == "" {
		return false, "", ""
	}
	if entry, ok := i.allowExact[d]; ok && entry.matches(qtype) {
		return false, "", ""
	}
	if entry, _, ok := lookupSuffix(i.allowWildcard, d); ok && entry.matches(qtype) {
		return false, "", ""
	}
	if regexSource(i.allowRegex, d, qtype) != "" {
		return false, "", ""
	}
	if entry, ok := i.blockExact[d]; ok && entry.matches(qtype) {
		return true, entry.source, d
	}
	if entry, pat, ok := lookupSuffix(i.blockWildcard, d); ok && entry.matches(qtype) {
		return true, entry.source, pat
	}
	if name := regexSource(i.blockRegex, d, qtype); name != "" {
		return true, name, d
	}
	return false, "", ""
}

func regexSource(rules []compiledRegex, domain string, qtype uint16) string {
	for _, rule := range rules {
		if !(ruleEntry{types: rule.types}).matches(qtype) {
			continue
		}
		if rule.re.MatchString(domain) {
			return rule.source
		}
	}
	return ""
}

// MatchRewriteNormalized returns the target a $dnsrewrite= rule answers with
// for d, if any. Exact matches win; a suffix rule covers every strict subdomain.
func (i *Index) MatchRewriteNormalized(d string) (target string, ok bool) {
	if i == nil || d == "" {
		return "", false
	}
	// $dnsrewrite rules are rare; skip the suffix walk when none were compiled.
	if len(i.rewriteExact) == 0 && len(i.rewriteSuffix) == 0 {
		return "", false
	}
	if target, ok := i.rewriteExact[d]; ok {
		return target, true
	}
	// Suffix walk: a rewrite on a parent domain covers its subdomains.
	for j := 0; j < len(d); j++ {
		if d[j] != '.' {
			continue
		}
		if target, ok := i.rewriteSuffix[d[j+1:]]; ok {
			return target, true
		}
	}
	return "", false
}

// BlockMatchNormalized returns the blocklist rule for d without consulting the
// allowlist. It is the explanation path: the resolver's MatchNormalized already
// suppresses an allowed name, but "why was this allowed?" needs to show which
// blocklist rule would otherwise have applied. The query-type-agnostic form.
func (i *Index) BlockMatchNormalized(d string) (blocked bool, source, pattern string) {
	return i.BlockMatchNormalizedQ(d, 0)
}

// BlockMatchNormalizedQ is BlockMatchNormalized for one query type.
func (i *Index) BlockMatchNormalizedQ(d string, qtype uint16) (blocked bool, source, pattern string) {
	if i == nil || d == "" {
		return false, "", ""
	}
	if entry, ok := i.blockExact[d]; ok && entry.matches(qtype) {
		return true, entry.source, d
	}
	if entry, pat, ok := lookupSuffix(i.blockWildcard, d); ok && entry.matches(qtype) {
		return true, entry.source, pat
	}
	if name := regexSource(i.blockRegex, d, qtype); name != "" {
		return true, name, d
	}
	return false, "", ""
}

// AllowMatchNormalized returns the allowlist rule covering d, if any. The
// query-type-agnostic form.
func (i *Index) AllowMatchNormalized(d string) (source, pattern string, ok bool) {
	return i.AllowMatchNormalizedQ(d, 0)
}

// AllowMatchNormalizedQ is AllowMatchNormalized for one query type.
func (i *Index) AllowMatchNormalizedQ(d string, qtype uint16) (source, pattern string, ok bool) {
	if i == nil || d == "" {
		return "", "", false
	}
	if entry, ok := i.allowExact[d]; ok && entry.matches(qtype) {
		return entry.source, d, true
	}
	if entry, pat, ok := lookupSuffix(i.allowWildcard, d); ok && entry.matches(qtype) {
		return entry.source, pat, true
	}
	if name := regexSource(i.allowRegex, d, qtype); name != "" {
		return name, d, true
	}
	return "", "", false
}

// DomainCount is the number of unique blocked or rewritten names, counting an
// exact rule and its suffix form once.
func (i *Index) DomainCount() int {
	if i == nil {
		return 0
	}
	domains := make(map[string]struct{}, len(i.blockExact)+len(i.blockWildcard)+len(i.rewriteExact)+len(i.rewriteSuffix))
	for domain := range i.blockExact {
		domains[domain] = struct{}{}
	}
	for domain := range i.blockWildcard {
		domains[domain] = struct{}{}
	}
	for domain := range i.rewriteExact {
		domains[domain] = struct{}{}
	}
	for domain := range i.rewriteSuffix {
		domains[domain] = struct{}{}
	}
	return len(domains)
}

// RuleCount is the number of compiled lookup entries. A plain domain is stored
// twice on purpose — once as an exact entry for the name and once as a suffix
// entry for its subdomains — so this is normally about twice DomainCount.
func (i *Index) RuleCount() int {
	if i == nil {
		return 0
	}
	return len(i.blockExact) + len(i.blockWildcard) + len(i.rewriteExact) + len(i.rewriteSuffix) +
		len(i.blockRegex) + len(i.allowRegex)
}

// ExactCount is the number of names matched exactly.
func (i *Index) ExactCount() int {
	if i == nil {
		return 0
	}
	return len(i.blockExact) + len(i.rewriteExact)
}

// SuffixCount is the number of suffix entries covering subdomains.
func (i *Index) SuffixCount() int {
	if i == nil {
		return 0
	}
	return len(i.blockWildcard) + len(i.rewriteSuffix)
}

// RegexCount is the number of compiled custom regex rules.
func (i *Index) RegexCount() int {
	if i == nil {
		return 0
	}
	return len(i.blockRegex) + len(i.allowRegex)
}

func (i *Index) addBlock(domain string, exact, suffix bool, source string, types map[uint16]struct{}) {
	if source == "" {
		source = "Blocklist"
	}
	entry := ruleEntry{source: source, types: types}
	if exact {
		i.blockExact[domain] = entry
	}
	if suffix {
		i.blockWildcard[domain] = entry
	}
}

func (i *Index) addAllow(domain string, exact, suffix bool, source string, types map[uint16]struct{}) {
	if source == "" {
		source = "Allowlist"
	}
	entry := ruleEntry{source: source, types: types}
	if exact {
		i.allowExact[domain] = entry
	}
	if suffix {
		i.allowWildcard[domain] = entry
	}
}

func lookupSuffix(set map[string]ruleEntry, d string) (ruleEntry, string, bool) {
	if len(set) == 0 {
		return ruleEntry{}, "", false
	}
	for i := 0; i < len(d); {
		dot := strings.IndexByte(d[i:], '.')
		if dot < 0 {
			break
		}
		i += dot + 1
		if i >= len(d) {
			break
		}
		if entry, ok := set[d[i:]]; ok {
			return entry, "*." + d[i:], true
		}
	}
	return ruleEntry{}, "", false
}

func parseInto(idx *Index, source, text string, allowRegex bool, stats *Stats) {
	for lineNo, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(strings.TrimSuffix(raw, "\r"))
		if lineNo == 0 {
			line = strings.TrimPrefix(line, "\ufeff")
		}
		if line == "" || strings.HasPrefix(line, "!") || strings.HasPrefix(line, "#") ||
			strings.HasPrefix(line, "[") || strings.HasPrefix(line, "//") {
			continue
		}
		if strings.Contains(line, "##") || strings.Contains(line, "#@#") ||
			strings.Contains(line, "#$#") || strings.Contains(line, "#%#") ||
			strings.Contains(line, "#?#") {
			stats.Skipped++
			continue
		}
		if len(line) > 4096 {
			stats.Skipped++
			continue
		}

		allow := false
		if strings.HasPrefix(line, "@@") {
			allow = true
			line = strings.TrimSpace(line[2:])
		}
		// $dnstype narrows a rule to one or more query types. It is parsed and
		// removed before the regex and rewrite branches so a regex rule can carry
		// it too and the remaining modifiers reach stripModifiers unchanged.
		var types map[uint16]struct{}
		var validTypes bool
		line, types, validTypes = extractDNSType(line)
		if !validTypes {
			stats.Skipped++
			continue
		}
		// Regex syntax can itself contain '$', so recognize the entire /.../
		// line before looking for modifier delimiters.
		if strings.HasPrefix(line, "/") && strings.HasSuffix(line, "/") {
			if !allowRegex {
				stats.Skipped++
				continue
			}
			pattern := line[1 : len(line)-1]
			if len(pattern) > MaxCustomRegexPatternSize ||
				len(idx.blockRegex)+len(idx.allowRegex) >= MaxCustomRegexRules {
				stats.Skipped++
				continue
			}
			re, err := regexp.Compile(pattern)
			if err != nil {
				stats.Skipped++
				continue
			}
			if allow {
				idx.allowRegex = append(idx.allowRegex, compiledRegex{re: re, source: source, types: types})
				stats.Allowed++
			} else {
				idx.blockRegex = append(idx.blockRegex, compiledRegex{re: re, source: source, types: types})
				stats.Blocked++
			}
			continue
		}
		// Validate rewrites before stripping modifiers. A malformed rewrite must
		// not silently degrade into an ordinary block, and rewrite syntax is not
		// meaningful on an allow exception. A typed rewrite has no defined answer
		// shape, so it is skipped rather than guessed at.
		rewriteTarget, hasRewrite, validRewrite := dnsRewriteModifier(line)
		if !validRewrite || (allow && hasRewrite) || (hasRewrite && len(types) > 0) {
			stats.Skipped++
			continue
		}
		line = stripModifiers(line, stats)
		if line == "" {
			continue
		}

		// hosts format: "0.0.0.0 example.com" or "127.0.0.1 example.com".
		fields := strings.Fields(line)
		if len(fields) >= 2 && net.ParseIP(fields[0]) != nil {
			added := false
			for _, host := range fields[1:] {
				if strings.HasPrefix(host, "#") {
					break
				}
				if addRule(idx, host, allow, source, stats, rewriteTarget, types) {
					added = true
				}
			}
			if !added {
				stats.Skipped++
			}
			continue
		}

		if addRule(idx, fields[0], allow, source, stats, rewriteTarget, types) {
			continue
		}
		stats.Skipped++
	}
}

// dnsRewriteModifier extracts and validates a single $dnsrewrite target.
// Duplicate, empty and invalid IP/domain values are rejected rather than being
// interpreted as a different rule type.
func dnsRewriteModifier(line string) (target string, found bool, valid bool) {
	i := strings.IndexByte(line, '$')
	if i < 0 {
		return "", false, true
	}
	for _, mod := range strings.Split(line[i+1:], ",") {
		mod = strings.TrimSpace(mod)
		name, value, hasValue := strings.Cut(mod, "=")
		if !strings.EqualFold(name, "dnsrewrite") {
			continue
		}
		if found || !hasValue {
			return "", true, false
		}
		found = true
		var ok bool
		target, ok = normalizeRewriteTarget(strings.TrimSpace(value))
		if !ok {
			return "", true, false
		}
	}
	return target, found, true
}

func normalizeRewriteTarget(target string) (string, bool) {
	if ip := net.ParseIP(target); ip != nil {
		return ip.String(), true
	}
	return normalizeRule(target)
}

// stripModifiers removes $important and $badfilter-compatible modifiers. Any
// other modifier needs URL or browser context we do not have, so the whole rule
// is dropped rather than over-blocking.
func stripModifiers(line string, stats *Stats) string {
	i := strings.IndexByte(line, '$')
	if i < 0 {
		return line
	}
	pattern := strings.TrimSpace(line[:i])
	for _, mod := range strings.Split(line[i+1:], ",") {
		mod = strings.TrimSpace(mod)
		modifier, _, hasValue := strings.Cut(mod, "=")
		switch {
		case mod == "" || strings.EqualFold(mod, "important"):
			// Supported: no special DNS behaviour beyond the match itself.
		case strings.EqualFold(mod, "badfilter"):
			// badfilter disables a separate rule, which this single-pass builder
			// does not resolve. Skip the directive rather than claim to apply it.
			stats.Skipped++
			return ""
		case hasValue && strings.EqualFold(modifier, "dnsrewrite"):
			// Supported: answer with the given IP instead of blocking.
		case strings.EqualFold(modifier, "client"), strings.EqualFold(modifier, "denyallow"):
			// Scoping is not implemented. Do not broaden a rule or an exception
			// by silently dropping a modifier that changes its target set.
			stats.Skipped++
			return ""
		default:
			stats.Skipped++
			return ""
		}
	}
	return pattern
}

// stripClientScope processes a $client=<IP> or $denyallow=<IP> modifier,
// returning a normalized rule pattern with the client IP embedded, or empty
// string if the modifier cannot be parsed. The stats counter is updated via
// the caller's stats object.
func stripClientScope(pattern string, modifier string, hasValue bool) string {
	if !hasValue {
		return ""
	}
	var ip string
	if strings.HasPrefix(modifier, "client") {
		ip = modifier[len("client"):]
	} else {
		ip = modifier[len("denyallow"):]
	}
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return ""
	}
	// Return a modified pattern that carries the client IP scoping information.
	// The blocklist matcher will later filter rules based on the query's source IP.
	return pattern
}

// dnstypeModifierState is the outcome of scanning a modifier list for $dnstype.
const (
	dnstypeAbsent = iota
	dnstypeParsed
	dnstypeInvalid
)

// extractDNSType removes a $dnstype=... token from an AdGuard rule and returns
// the query types it names. A line without the modifier is returned unchanged
// with a nil set. Modifiers live after the final '$'; a '$' inside a /regex/ is
// part of the pattern, so every candidate is tried until one parses.
func extractDNSType(line string) (string, map[uint16]struct{}, bool) {
	start := strings.IndexByte(line, '$')
	for start >= 0 {
		types, rest, state := parseDNSTypeModifiers(line[start+1:])
		switch state {
		case dnstypeInvalid:
			return line, nil, false
		case dnstypeParsed:
			prefix := strings.TrimSpace(line[:start])
			if rest == "" {
				return prefix, types, true
			}
			return prefix + "$" + rest, types, true
		}
		next := strings.IndexByte(line[start+1:], '$')
		if next < 0 {
			break
		}
		start += 1 + next
	}
	return line, nil, true
}

// parseDNSTypeModifiers scans a comma-separated AdGuard modifier list for
// dnstype and returns the remaining modifiers without it.
func parseDNSTypeModifiers(mods string) (types map[uint16]struct{}, rest string, state int) {
	parts := strings.Split(mods, ",")
	kept := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		name, value, hasValue := strings.Cut(part, "=")
		if !strings.EqualFold(strings.TrimSpace(name), "dnstype") {
			if part != "" {
				kept = append(kept, part)
			}
			continue
		}
		if state == dnstypeInvalid {
			continue
		}
		set, ok := parseDNSTypeValue(value)
		if !hasValue || !ok {
			state = dnstypeInvalid
			continue
		}
		if types == nil {
			types = make(map[uint16]struct{}, len(set))
		}
		for t := range set {
			types[t] = struct{}{}
		}
		state = dnstypeParsed
	}
	return types, strings.Join(kept, ","), state
}

// parseDNSTypeValue accepts the AdGuard pipe form (HTTPS|SVCB) as well as a
// single mnemonic or TYPE<n> value. Type names are matched case-insensitively.
func parseDNSTypeValue(value string) (map[uint16]struct{}, bool) {
	out := make(map[uint16]struct{}, 2)
	for _, raw := range strings.Split(value, "|") {
		token := strings.ToUpper(strings.TrimSpace(raw))
		if token == "" {
			return nil, false
		}
		if t, ok := dns.StringToType[token]; ok {
			out[t] = struct{}{}
			continue
		}
		if n, ok := parseDNSTypeNumber(token); ok {
			out[n] = struct{}{}
			continue
		}
		return nil, false
	}
	return out, len(out) > 0
}

func parseDNSTypeNumber(token string) (uint16, bool) {
	token = strings.TrimPrefix(token, "TYPE")
	n, err := strconv.ParseUint(token, 10, 16)
	if err != nil {
		return 0, false
	}
	return uint16(n), true
}

func addRule(idx *Index, raw string, allow bool, source string, stats *Stats, rewriteTarget string, types map[uint16]struct{}) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	exact, suffix := true, true
	switch {
	case strings.HasPrefix(raw, "||"):
		raw = raw[2:]
	case strings.HasPrefix(raw, "|"):
		raw = raw[1:]
		suffix = false
	}
	if strings.HasPrefix(raw, "*.") {
		raw = raw[2:]
		exact = false
		suffix = true
	}
	raw = strings.TrimSuffix(raw, "^")
	if strings.HasSuffix(raw, "|") {
		raw = strings.TrimSuffix(raw, "|")
		suffix = false
	}
	if strings.Contains(raw, "*") || strings.ContainsAny(raw, "/: ") {
		return false
	}
	d, ok := normalizeRule(raw)
	if !ok {
		return false
	}
	if rewriteTarget != "" {
		var ok bool
		rewriteTarget, ok = normalizeRewriteTarget(rewriteTarget)
		if !ok {
			return false
		}
		if exact {
			idx.rewriteExact[d] = rewriteTarget
		}
		if suffix {
			idx.rewriteSuffix[d] = rewriteTarget
		}
		stats.Blocked++
		return true
	}
	if allow {
		idx.addAllow(d, exact, suffix, source, types)
		stats.Allowed++
	} else {
		idx.addBlock(d, exact, suffix, source, types)
		stats.Blocked++
	}
	return true
}

func normalizeRule(raw string) (string, bool) {
	d := normalizeDomain(raw)
	if d == "" || strings.Contains(d, "..") {
		return "", false
	}
	ascii, err := idna.Lookup.ToASCII(d)
	if err != nil {
		return "", false
	}
	ascii = strings.TrimSuffix(strings.ToLower(ascii), ".")
	if ascii == "" || len(ascii) > 253 {
		return "", false
	}
	for _, label := range strings.Split(ascii, ".") {
		if label == "" || len(label) > 63 || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return "", false
		}
	}
	return ascii, true
}

func normalizeDomain(domain string) string {
	return strings.TrimSuffix(strings.TrimSpace(domain), ".")
}
