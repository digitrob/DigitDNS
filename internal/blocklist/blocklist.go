// Package blocklist compiles DNS blocklists into an immutable index.
//
// It deliberately supports only rules that can be answered from a domain name:
// hosts lines, domains-only lists, and Adblock/uBO network rules. Cosmetic rules,
// scriptlets, regex rules and modifiers that need the full URL are skipped and
// counted, never guessed at. The hot path is one exact lookup plus a suffix walk;
// there is no per-rule scan and no regex.
package blocklist

import (
	"net"
	"strings"

	"golang.org/x/net/idna"
)

// Source is one downloaded or uploaded list. Text is parsed at rebuild time, not
// on the DNS path.
type Source struct {
	Name    string
	Text    string
	Enabled bool
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
}

func newIndex() *Index {
	return &Index{
		blockExact:    make(map[string]string),
		blockWildcard: make(map[string]string),
		allowExact:    make(map[string]string),
		allowWildcard: make(map[string]string),
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
		parseInto(idx, src.Name, src.Text, &stats)
	}
	for _, domain := range allowlist {
		addRule(idx, domain, true, "Allowlist", &stats)
	}
	stats.Domains = idx.DomainCount()
	return idx, stats
}

// Analyze parses a list only to report its rule counts, used when an upload or
// refresh is stored. It does not touch the live matcher.
func Analyze(text string) Stats {
	idx := newIndex()
	var stats Stats
	parseInto(idx, "upload", text, &stats)
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
// trimmed. The resolver and matcher normalize once per query; this avoids a
// second scan of the same bytes when a blocklist is installed.
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
	if name, ok := i.blockExact[d]; ok {
		return true, name, d
	}
	if name, pat, ok := lookupSuffix(i.blockWildcard, d); ok {
		return true, name, pat
	}
	return false, "", ""
}

// DomainCount is the number of unique blocked names, counting an exact rule and
// its suffix form once.
func (i *Index) DomainCount() int {
	if i == nil {
		return 0
	}
	n := len(i.blockExact)
	for domain := range i.blockWildcard {
		if _, ok := i.blockExact[domain]; !ok {
			n++
		}
	}
	return n
}

// RuleCount is the number of blocked lookup entries. A plain domain is stored
// twice on purpose — once as an exact entry for the name and once as a suffix
// entry for its subdomains — so this is normally about twice DomainCount.
func (i *Index) RuleCount() int {
	if i == nil {
		return 0
	}
	return len(i.blockExact) + len(i.blockWildcard)
}

// ExactCount is the number of names matched exactly.
func (i *Index) ExactCount() int {
	if i == nil {
		return 0
	}
	return len(i.blockExact)
}

// SuffixCount is the number of suffix entries covering subdomains.
func (i *Index) SuffixCount() int {
	if i == nil {
		return 0
	}
	return len(i.blockWildcard)
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

func parseInto(idx *Index, source, text string, stats *Stats) {
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
		line = stripModifiers(line, stats)
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "/") && strings.HasSuffix(line, "/") {
			// Regex rules are intentionally not compiled in the hot path.
			stats.Skipped++
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
				if addRule(idx, host, allow, source, stats) {
					added = true
				}
			}
			if !added {
				stats.Skipped++
			}
			continue
		}

		if addRule(idx, fields[0], allow, source, stats) {
			continue
		}
		stats.Skipped++
	}
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
		switch mod {
		case "", "important":
			// Supported: no special DNS behaviour beyond the match itself.
		case "badfilter":
			// badfilter disables another rule; keeping the rule would be wrong.
			stats.Skipped++
			return ""
		default:
			stats.Skipped++
			return ""
		}
	}
	return pattern
}

func addRule(idx *Index, raw string, allow bool, source string, stats *Stats) bool {
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
