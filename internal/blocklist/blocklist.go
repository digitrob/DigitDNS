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
	"strings"

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
	blockExact    map[string]string
	blockWildcard map[string]string
	allowExact    map[string]string
	allowWildcard map[string]string
	// rewrites holds $dnsrewrite targets (IP addresses or CNAME names): exact
	// keys and suffix (parent) keys.
	rewriteExact  map[string]string
	rewriteSuffix map[string]string
	// regex rules, only populated when the source opts in (custom rules).
	blockRegex []compiledRegex
	allowRegex []compiledRegex
}

// compiledRegex is one compiled AdGuard regex rule with its source label.
type compiledRegex struct {
	re     *regexp.Regexp
	source string
}

const (
	maxCustomRegexRules       = 64
	maxCustomRegexPatternSize = 512
)

func newIndex() *Index {
	return &Index{
		blockExact:    make(map[string]string),
		blockWildcard: make(map[string]string),
		allowExact:    make(map[string]string),
		allowWildcard: make(map[string]string),
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
		addRule(idx, domain, true, "Allowlist", &stats, "")
	}
	stats.Domains = idx.DomainCount()
	return idx, stats
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
// block; the caller still applies its own custom direct/proxy rules.
func (i *Index) Match(domain string) (blocked bool, source, pattern string) {
	if i == nil {
		return false, "", ""
	}
	d := normalizeDomain(domain)
	if d == "" {
		return false, "", ""
	}
	return i.MatchNormalized(d)
}

// MatchNormalized is Match for a name the caller has already lowercased and
// trimmed. It checks bounded regex rules only when a source opted in; the
// resolver and matcher otherwise use the exact lookup plus suffix walk without
// a per-rule scan.
func (i *Index) MatchNormalized(d string) (blocked bool, source, pattern string) {
	if i == nil || d == "" {
		return false, "", ""
	}
	if _, ok := i.allowExact[d]; ok {
		return false, "", ""
	}
	if _, _, ok := lookupSuffix(i.allowWildcard, d); ok {
		return false, "", ""
	}
	if regexSource(i.allowRegex, d) != "" {
		return false, "", ""
	}
	if name, ok := i.blockExact[d]; ok {
		return true, name, d
	}
	if name, pat, ok := lookupSuffix(i.blockWildcard, d); ok {
		return true, name, pat
	}
	if name := regexSource(i.blockRegex, d); name != "" {
		return true, name, d
	}
	return false, "", ""
}

func regexSource(rules []compiledRegex, domain string) string {
	for _, rule := range rules {
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
// blocklist rule would otherwise have applied.
func (i *Index) BlockMatchNormalized(d string) (blocked bool, source, pattern string) {
	if i == nil || d == "" {
		return false, "", ""
	}
	if name, ok := i.blockExact[d]; ok {
		return true, name, d
	}
	if name, pat, ok := lookupSuffix(i.blockWildcard, d); ok {
		return true, name, pat
	}
	if name := regexSource(i.blockRegex, d); name != "" {
		return true, name, d
	}
	return false, "", ""
}

// AllowMatchNormalized returns the allowlist rule covering d, if any.
func (i *Index) AllowMatchNormalized(d string) (source, pattern string, ok bool) {
	if i == nil || d == "" {
		return "", "", false
	}
	if name, ok := i.allowExact[d]; ok {
		return name, d, true
	}
	if name, pat, ok := lookupSuffix(i.allowWildcard, d); ok {
		return name, pat, true
	}
	if name := regexSource(i.allowRegex, d); name != "" {
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

func (i *Index) addBlock(domain string, exact, suffix bool, source string) {
	if source == "" {
		source = "Blocklist"
	}
	if exact {
		i.blockExact[domain] = source
	}
	if suffix {
		i.blockWildcard[domain] = source
	}
}

func (i *Index) addAllow(domain string, exact, suffix bool, source string) {
	if source == "" {
		source = "Allowlist"
	}
	if exact {
		i.allowExact[domain] = source
	}
	if suffix {
		i.allowWildcard[domain] = source
	}
}

func lookupSuffix(set map[string]string, d string) (string, string, bool) {
	if len(set) == 0 {
		return "", "", false
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
		if name, ok := set[d[i:]]; ok {
			return name, "*." + d[i:], true
		}
	}
	return "", "", false
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
		// Regex syntax can itself contain '$', so recognize the entire /.../
		// line before looking for modifier delimiters.
		if strings.HasPrefix(line, "/") && strings.HasSuffix(line, "/") {
			if !allowRegex {
				stats.Skipped++
				continue
			}
			pattern := line[1 : len(line)-1]
			if len(pattern) > maxCustomRegexPatternSize ||
				len(idx.blockRegex)+len(idx.allowRegex) >= maxCustomRegexRules {
				stats.Skipped++
				continue
			}
			re, err := regexp.Compile(pattern)
			if err != nil {
				stats.Skipped++
				continue
			}
			if allow {
				idx.allowRegex = append(idx.allowRegex, compiledRegex{re: re, source: source})
				stats.Allowed++
			} else {
				idx.blockRegex = append(idx.blockRegex, compiledRegex{re: re, source: source})
				stats.Blocked++
			}
			continue
		}
		// Validate rewrites before stripping modifiers. A malformed rewrite must
		// not silently degrade into an ordinary block, and rewrite syntax is not
		// meaningful on an allow exception.
		rewriteTarget, hasRewrite, validRewrite := dnsRewriteModifier(line)
		if !validRewrite || (allow && hasRewrite) {
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
				if addRule(idx, host, allow, source, stats, rewriteTarget) {
					added = true
				}
			}
			if !added {
				stats.Skipped++
			}
			continue
		}

		if addRule(idx, fields[0], allow, source, stats, rewriteTarget) {
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

func addRule(idx *Index, raw string, allow bool, source string, stats *Stats, rewriteTarget string) bool {
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
		idx.addAllow(d, exact, suffix, source)
		stats.Allowed++
	} else {
		idx.addBlock(d, exact, suffix, source)
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
