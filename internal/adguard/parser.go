// Package adguard parses AdGuard Home upstream configuration intoDigitDNS's
// domain-pattern vocabulary. It is deliberately read-only: applying anything to
// the rules store happens in the API layer, after the operator has seen a preview.
package adguard

import (
	"crypto/sha256"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"sort"
	"strings"
)

// MaxPatterns bounds a pasted or fetched source. A typo that turns a comment into
// a bracket group must not be able to enqueue an unbounded rules write.
const MaxPatterns = 10000

// ManagedBlockStart and ManagedBlockEnd bracket the only lines the sync engine
// may ever rewrite in an AdGuard config. They are kept as literals so the export
// and any future write-back agree byte for byte.
const (
	ManagedBlockStart = "# BEGIN DIGITDNS SYNC — managed"
	ManagedBlockEnd   = "# END DIGITDNS SYNC — managed"
)

// PatternKind records whether the leading "*." was present in the source. The
// distinction matters: DigitDNS's plain rules match the name and its subdomains,
// while "*.name" matches only strict subdomains.
type PatternKind string

const (
	KindExact    PatternKind = "exact"
	KindWildcard PatternKind = "wildcard"
)

// Pattern is one normalized AdGuard domain token. Value never carries the "*."
// prefix; Kind does. Group/Target are display provenance, not matching inputs.
type Pattern struct {
	Value        string      `json:"value"`
	Kind         PatternKind `json:"kind"`
	Group        string      `json:"group"`
	Target       string      `json:"target"`
	TargetIsSelf bool        `json:"targetIsSelf"`
	// Broad flags a domain that routes a large share of ordinary traffic. It is
	// metadata only: the operator still decides whether to import it.
	Broad bool `json:"broad"`
}

// String returns the form DigitDNS stores in its rule lists.
func (p Pattern) String() string {
	if p.Kind == KindWildcard {
		return "*." + p.Value
	}
	return p.Value
}

// Group is one active bracketed upstream rule from the source.
type Group struct {
	Label        string    `json:"label"`
	Target       string    `json:"target"`
	TargetIsSelf bool      `json:"targetIsSelf"`
	Patterns     []Pattern `json:"patterns"`
}

// ParseResult is everything the preview needs before anything is applied.
type ParseResult struct {
	Groups   []Group   `json:"groups"`
	Patterns []Pattern `json:"patterns"`
	Warnings []string  `json:"warnings"`
	Checksum string    `json:"checksum"`
}

var (
	domainRE = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]*[a-z0-9])?)*$`)
	broadRE  = regexp.MustCompile(`(^|\.)(google\.com|googleapis\.com|googleusercontent\.com|gstatic\.com)$`)
)

var broadDomains = map[string]bool{
	"www.google.com":         true,
	"play.googleapis.com":    true,
	"android.googleapis.com": true,
	"clients4.google.com":    true,
	"clients6.google.com":    true,
}

// Parse reads active AdGuard upstream rules. Every bracketed group is included,
// whatever its target: the operator decides what a non-DigitDNS target means.
func Parse(text string, selfHosts []string) ParseResult {
	result := ParseResult{
		Groups:   make([]Group, 0),
		Patterns: make([]Pattern, 0),
		Warnings: make([]string, 0),
		Checksum: Checksum(text),
	}
	self := selfHostSet(selfHosts)
	seen := make(map[string]struct{})
	broadSeen := make(map[string]struct{})
	foreignSeen := make(map[string]struct{})
	label := ""

	for _, rawLine := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(strings.TrimPrefix(rawLine, "\ufeff"))
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") {
			label = commentLabel(line, label)
			continue
		}
		if !strings.HasPrefix(line, "[") {
			continue
		}
		close := strings.Index(line, "]")
		if close < 0 {
			result.Warnings = append(result.Warnings, "skipped a bracketed rule with no closing ']'")
			continue
		}
		target := strings.Fields(strings.TrimSpace(line[close+1:]))
		if len(target) == 0 {
			result.Warnings = append(result.Warnings, "skipped a bracketed rule with no upstream target")
			continue
		}
		upstream := target[0]
		group := Group{
			Label:        label,
			Target:       upstream,
			TargetIsSelf: IsSelfTarget(upstream, self),
			Patterns:     make([]Pattern, 0),
		}
		if !group.TargetIsSelf {
			host := TargetHost(upstream)
			if _, warned := foreignSeen[host]; host != "" && !warned {
				foreignSeen[host] = struct{}{}
				result.Warnings = append(result.Warnings,
					fmt.Sprintf("upstream %s is not this DigitDNS server; its domains can be imported, but AdGuard will keep routing them there", host))
			}
		}
		for _, token := range strings.Split(line[1:close], "/") {
			pattern, ok := NormalizeToken(token)
			if !ok {
				trimmed := strings.TrimSpace(token)
				if trimmed != "" && strings.HasPrefix(trimmed, "~/") {
					result.Warnings = append(result.Warnings, "skipped regex pattern "+trimmed+"; regex rules are not imported in v1")
				} else if trimmed != "" {
					result.Warnings = append(result.Warnings, "skipped unsupported pattern "+trimmed)
				}
				continue
			}
			pattern.Group = label
			pattern.Target = upstream
			pattern.TargetIsSelf = group.TargetIsSelf
			key := pattern.String()
			if _, duplicate := seen[key]; duplicate {
				continue
			}
			if len(result.Patterns) >= MaxPatterns {
				result.Warnings = append(result.Warnings, fmt.Sprintf("stopped after %d patterns (source cap)", MaxPatterns))
				break
			}
			seen[key] = struct{}{}
			result.Patterns = append(result.Patterns, pattern)
			group.Patterns = append(group.Patterns, pattern)
			if IsBroad(pattern.Value) {
				if _, warned := broadSeen[key]; !warned {
					broadSeen[key] = struct{}{}
					result.Warnings = append(result.Warnings,
						fmt.Sprintf("broad domain %s routes a large share of ordinary traffic", key))
				}
			}
		}
		if len(group.Patterns) > 0 {
			result.Groups = append(result.Groups, group)
		}
		if len(result.Patterns) >= MaxPatterns {
			break
		}
	}
	return result
}

// Checksum identifies the exact source text a preview was built from.
func Checksum(text string) string {
	sum := sha256.Sum256([]byte(text))
	return fmt.Sprintf("%x", sum[:])
}

// NormalizeToken turns one AdGuard bracket token into a DigitDNS rule pattern.
func NormalizeToken(raw string) (Pattern, bool) {
	token := strings.TrimSpace(raw)
	token = strings.TrimPrefix(token, "||")
	token = strings.TrimSuffix(token, "^")
	token = strings.ToLower(strings.TrimSuffix(token, "."))
	if token == "" {
		return Pattern{}, false
	}
	kind := KindExact
	if strings.HasPrefix(token, "*.") {
		kind = KindWildcard
		token = token[2:]
	}
	if !domainRE.MatchString(token) || strings.Contains(token, "*") {
		return Pattern{}, false
	}
	return Pattern{Value: token, Kind: kind, Broad: IsBroad(token)}, true
}

// Covers reports whether `pattern` already covers `candidate` under DigitDNS's
// matcher: a plain name covers itself and all subdomains; a wildcard covers only
// strict subdomains.
func Covers(pattern, candidate Pattern) bool {
	if pattern.Value == "" || candidate.Value == "" {
		return false
	}
	if pattern.Kind == KindExact {
		return pattern.Value == candidate.Value ||
			strings.HasSuffix(candidate.Value, "."+pattern.Value)
	}
	return strings.HasSuffix(candidate.Value, "."+pattern.Value)
}

// Overlaps is symmetric coverage, used to collapse a wildcard and its parent into
// one "already covered" row rather than proposing the same domain twice.
func Overlaps(a, b Pattern) bool { return Covers(a, b) || Covers(b, a) }

// CoveredByAny returns the first pattern in `patterns` that covers `candidate`.
func CoveredByAny(candidate Pattern, patterns []Pattern) (Pattern, bool) {
	for _, existing := range patterns {
		if Covers(existing, candidate) {
			return existing, true
		}
	}
	return Pattern{}, false
}

// GenerateManagedBlock renders the DigitDNS-only patterns as an AdGuard upstream
// rule. Patterns are split across lines so a large list stays reviewable.
func GenerateManagedBlock(patterns []Pattern, upstream string) string {
	if len(patterns) == 0 || strings.TrimSpace(upstream) == "" {
		return ""
	}
	keys := make([]string, 0, len(patterns))
	seen := make(map[string]struct{}, len(patterns))
	for _, p := range patterns {
		key := p.String()
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var out strings.Builder
	out.WriteString(ManagedBlockStart)
	out.WriteByte('\n')
	for start := 0; start < len(keys); start += 100 {
		end := start + 100
		if end > len(keys) {
			end = len(keys)
		}
		out.WriteString("[/" + strings.Join(keys[start:end], "/") + "]" + upstream + "\n")
	}
	out.WriteString(ManagedBlockEnd)
	return out.String()
}

func selfHostSet(hosts []string) map[string]struct{} {
	set := make(map[string]struct{}, len(hosts))
	for _, host := range hosts {
		h := TargetHost(host)
		if h != "" {
			set[h] = struct{}{}
		}
	}
	return set
}

// IsSelfTarget reports whether an upstream points at one of DigitDNS's own
// hosts. Scheme and port are ignored: an AdGuard quic:// marker still identifies
// this server even when DigitDNS does not terminate DoQ itself.
func IsSelfTarget(target string, self map[string]struct{}) bool {
	host := TargetHost(target)
	_, ok := self[host]
	return ok
}

// TargetHost extracts a comparable hostname from any AdGuard upstream spelling.
func TargetHost(target string) string {
	value := strings.TrimSpace(target)
	if value == "" {
		return ""
	}
	if parsed, err := url.Parse(value); err == nil && parsed.Hostname() != "" {
		return normalizeHost(parsed.Hostname())
	}
	if host, _, err := net.SplitHostPort(value); err == nil {
		return normalizeHost(host)
	}
	return normalizeHost(strings.TrimRight(strings.Fields(value)[0], "/"))
}

func normalizeHost(host string) string {
	host = strings.Trim(strings.ToLower(strings.TrimSpace(host)), "[]")
	return strings.TrimSuffix(host, ".")
}

func commentLabel(line, previous string) string {
	text := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "#"))
	text = strings.Trim(text, " =-/")
	if text == "" || strings.Contains(text, "//") || strings.Contains(text, "/") {
		return previous
	}
	if strings.Contains(text, "DIGITDNS SYNC") {
		return previous
	}
	if len(text) > 120 {
		return previous
	}
	return text
}

// IsBroad reports whether a normalized domain is known to carry a large share of
// ordinary traffic. Callers use it as a review flag, never as an automatic veto.
func IsBroad(value string) bool {
	if broadDomains[value] {
		return true
	}
	return broadRE.MatchString(value)
}
