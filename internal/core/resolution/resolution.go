// Package resolution holds the per-domain upstream policy shared by the DNS
// handler, the racer and the settings model.
//
// A profile answers one question for one name: which resolvers may answer it,
// how they are raced, and what happens when they cannot. The zero value means
// "no opinion" — inherit the global defaults — so a stored map only carries the
// domains an operator actually configured.
package resolution

import (
	"net/url"
	"strings"
)

// Strategy values accepted by the resolution settings. Empty means inherit the
// pool's global racing setting. Adaptive is a global strategy in this release;
// per-domain profiles cannot carry it.
const (
	// StrategyFastest is the pre-5.5.7 race-every-resolver strategy. It is
	// retired from the UI and normalised to Hedged, but remains accepted so an
	// older client can still save a settings record.
	StrategyFastest    = "fastest"
	StrategySequential = "sequential"
	// StrategyHedged starts with the fastest measured resolver and launches the
	// next after a short delay; the first usable answer wins.
	StrategyHedged = "hedged"
	// StrategyBestKnown uses the fastest measured resolver, then falls through
	// the remaining resolvers in latency order on failure.
	StrategyBestKnown = "best"
	// StrategyAdaptive benchmarks the pool on a schedule and keeps using the
	// fastest healthy resolver until the next run. Unlike Hedged/Best Known it
	// is a stable, periodically re-evaluated choice rather than a per-query
	// latency sort.
	StrategyAdaptive = "adaptive"
)

// Adaptive interval choices, in minutes. They are deliberately a small fixed
// set: an adaptive run is a real benchmark, so a free-form interval would let
// an operator turn it into a query amplifier.
const (
	DefaultAdaptiveIntervalMinutes = 60
	MinAdaptiveIntervalMinutes     = 5
	MaxAdaptiveIntervalMinutes     = 1440
)

// AdaptiveIntervalChoices is the ordered set the panel and the app offer.
var AdaptiveIntervalChoices = []int{5, 15, 60, 360, 1440}

// ValidAdaptiveInterval reports whether minutes is one of the offered choices.
func ValidAdaptiveInterval(minutes int) bool {
	for _, choice := range AdaptiveIntervalChoices {
		if minutes == choice {
			return true
		}
	}
	return false
}

// NormalizeAdaptiveInterval maps a stored/requested value onto an offered
// choice. Zero (the zero value of a field added to an old record) and anything
// unknown become the one-hour default rather than a surprising value.
func NormalizeAdaptiveInterval(minutes int) int {
	if ValidAdaptiveInterval(minutes) {
		return minutes
	}
	return DefaultAdaptiveIntervalMinutes
}

// Fallback values accepted in a profile. Empty means FallbackPool.
const (
	FallbackPool = "pool"
	FallbackFail = "fail"
)

// Transport values accepted globally and per profile. The empty string means
// "inherit" in a profile and "any" globally.
const (
	TransportAny       = "any"
	TransportEncrypted = "encrypted"
	TransportRegular   = "regular"
)

// Per-upstream protocols. The global policy is a set of these, so an install
// can allow, say, plain DNS and DoH while excluding DoT and DoQ. The order is
// the one the UI presents and the one NormalizeProtocols returns.
const (
	ProtocolPlain    = "plain"
	ProtocolDoT      = "dot"
	ProtocolDoH      = "doh"
	ProtocolDoH3     = "doh3"
	ProtocolDoQ      = "doq"
	ProtocolDNSCrypt = "dnscrypt"
)

// AllProtocols is the canonical order used by the settings view and the chips.
var AllProtocols = []string{
	ProtocolPlain,
	ProtocolDoT,
	ProtocolDoH,
	ProtocolDoH3,
	ProtocolDoQ,
	ProtocolDNSCrypt,
}

// ValidProtocol reports whether value names an upstream protocol.
func ValidProtocol(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case ProtocolPlain, ProtocolDoT, ProtocolDoH, ProtocolDoH3, ProtocolDoQ, ProtocolDNSCrypt:
		return true
	}
	return false
}

// ValidProtocols reports whether a non-empty protocol set contains only known
// names. The global policy must keep at least one protocol enabled; an empty
// set would leave the pool with no candidates.
func ValidProtocols(values []string) bool {
	if len(values) == 0 {
		return false
	}
	for _, value := range values {
		if !ValidProtocol(value) {
			return false
		}
	}
	return true
}

// NormalizeProtocols deduplicates a protocol set and returns it in canonical
// order. Empty/invalid input becomes all protocols, which is the pre-chips
// behaviour and the safe default for an old record.
func NormalizeProtocols(values []string) []string {
	seen := make(map[string]bool, len(values))
	for _, value := range values {
		if ValidProtocol(value) {
			seen[strings.ToLower(strings.TrimSpace(value))] = true
		}
	}
	if len(seen) == 0 {
		return append([]string(nil), AllProtocols...)
	}
	out := make([]string, 0, len(seen))
	for _, protocol := range AllProtocols {
		if seen[protocol] {
			out = append(out, protocol)
		}
	}
	return out
}

// ProtocolsForTransport maps the legacy any/encrypted/regular choice onto the
// equivalent protocol set. It is the migration path for records saved before
// the transport chips existed.
func ProtocolsForTransport(transport string) []string {
	switch NormalizeTransport(transport) {
	case TransportEncrypted:
		return []string{ProtocolDoT, ProtocolDoH, ProtocolDoH3, ProtocolDoQ, ProtocolDNSCrypt}
	case TransportRegular:
		return []string{ProtocolPlain}
	default:
		return append([]string(nil), AllProtocols...)
	}
}

// LegacyTransportFor collapses a protocol set onto the single value older
// clients and profile rules understand. A mixed set has no legacy spelling, so
// it reads as "any" while the protocol set remains authoritative.
func LegacyTransportFor(protocols []string) string {
	normalized := NormalizeProtocols(protocols)
	plain, encrypted := false, false
	for _, protocol := range normalized {
		if protocol == ProtocolPlain {
			plain = true
		} else {
			encrypted = true
		}
	}
	switch {
	case plain && !encrypted:
		return TransportRegular
	case encrypted && !plain:
		return TransportEncrypted
	default:
		return TransportAny
	}
}

// Protocol classifies one canonical upstream address. Anything that is not one
// of the encrypted schemes is plain UDP/TCP, matching NormalizeUpstream's
// fallback for a bare host or tcp://.
func Protocol(upstream string) string {
	lower := strings.ToLower(strings.TrimSpace(upstream))
	switch {
	case strings.HasPrefix(lower, "tls://"):
		return ProtocolDoT
	case strings.HasPrefix(lower, "https://"):
		return ProtocolDoH
	case strings.HasPrefix(lower, "h3://"):
		return ProtocolDoH3
	case strings.HasPrefix(lower, "quic://"):
		return ProtocolDoQ
	case strings.HasPrefix(lower, "sdns://"):
		return ProtocolDNSCrypt
	default:
		return ProtocolPlain
	}
}

// NormalizeTransport maps a stored value onto one of the three choices. An
// unknown value falls back to "any" so a hand-edited config can never leave the
// pool with no candidates at all.
func NormalizeTransport(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case TransportEncrypted:
		return TransportEncrypted
	case TransportRegular:
		return TransportRegular
	default:
		return TransportAny
	}
}

// ValidTransport reports whether value is an accepted transport (or empty,
// meaning inherit globally).
func ValidTransport(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", TransportAny, TransportEncrypted, TransportRegular:
		return true
	}
	return false
}

// Profile is the resolution rule attached to one domain pattern.
type Profile struct {
	// Upstream pins resolution to one configured upstream (canonical form). Empty
	// means "use the eligible pool".
	Upstream string `json:"upstream,omitempty"`
	// Strategy overrides the global racing choice for this domain.
	Strategy string `json:"strategy,omitempty"`
	// Transport overrides the global transport choice (any|encrypted|regular).
	// Empty inherits it.
	Transport string `json:"transport,omitempty"`
	// EncryptedOnly is the pre-transport form this field replaces. It is read so
	// a profile saved by 3.4.11 keeps its meaning; every save rewrites it as
	// Transport and clears the pointer.
	EncryptedOnly *bool `json:"encrypted_only,omitempty"`
	// Fallback says what to do when the pinned/filtered candidates fail: use the
	// ordinary pool, or answer SERVFAIL. Empty means pool.
	Fallback string `json:"fallback,omitempty"`
}

// Active reports whether the profile changes anything at all.
func (p Profile) Active() bool {
	return p.Upstream != "" || p.Strategy != "" || p.Transport != "" || p.EncryptedOnly != nil
}

// TransportFor returns the transport this profile resolves to, given the global
// default. A profile value wins; the legacy encrypted_only flag is next; the
// global choice is the fallback.
func (p Profile) TransportFor(global string) string {
	if p.Transport != "" {
		return NormalizeTransport(p.Transport)
	}
	if p.EncryptedOnly != nil {
		if *p.EncryptedOnly {
			return TransportEncrypted
		}
		return TransportAny
	}
	return NormalizeTransport(global)
}

// ValidStrategy reports whether value is an accepted racing strategy (or empty,
// meaning the pool's configured default). StrategyFastest is accepted for
// backwards compatibility but NormalizeStrategy turns it into Hedged.
func ValidStrategy(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", StrategyFastest, StrategySequential, StrategyHedged, StrategyBestKnown, StrategyAdaptive:
		return true
	}
	return false
}

// NormalizeStrategy turns a stored/requested strategy into the strategy the
// current release actually runs. Empty is the out-of-the-box default and now
// means Adaptive (fastest); Fastest is the retired race-every-resolver mode and
// aliases Hedged.
func NormalizeStrategy(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case StrategyAdaptive:
		return StrategyAdaptive
	case StrategySequential:
		return StrategySequential
	case StrategyHedged:
		return StrategyHedged
	case StrategyBestKnown:
		return StrategyBestKnown
	case StrategyFastest:
		return StrategyHedged
	case "":
		return StrategyAdaptive
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

// NormalizeProfileStrategy normalises a per-domain override without turning an
// empty value into an inherited default: empty still means "inherit global".
// Fastest is retired here too and becomes Hedged.
func NormalizeProfileStrategy(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "":
		return ""
	case StrategyFastest:
		return StrategyHedged
	case StrategyAdaptive:
		// Adaptive is a global strategy in this release. A profile that asks
		// for it inherits the global default instead of carrying a second
		// scheduler.
		return ""
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

// Normalized rewrites the legacy encrypted_only flag as the equivalent
// transport so responses and stored settings carry one representation.
func (p Profile) Normalized() Profile {
	if p.Transport == "" && p.EncryptedOnly != nil {
		if *p.EncryptedOnly {
			p.Transport = TransportEncrypted
		} else {
			p.Transport = TransportAny
		}
	}
	p.EncryptedOnly = nil
	// Every profile now falls back to the pool. The old per-profile "fail"
	// choice is retired; clearing the field also migrates records saved by
	// earlier releases.
	p.Fallback = ""
	return p
}

// Encrypted reports whether an upstream is carried over an encrypted transport:
// DoT (tls://), DoH (https://), DoQ (quic://), DoH3 (h3://) or DNSCrypt
// (sdns://). Plain UDP/TCP upstreams — bare host:port or tcp:// — are not.
func Encrypted(upstream string) bool {
	lower := strings.ToLower(strings.TrimSpace(upstream))
	for _, prefix := range []string{"tls://", "https://", "quic://", "h3://", "sdns://"} {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return false
}

// Domain returns the host a profile key applies to, without a leading "*." and
// without a trailing dot.
func Domain(key string) string {
	d := strings.TrimSpace(strings.ToLower(key))
	d = strings.TrimSuffix(d, ".")
	d = strings.TrimPrefix(d, "*.")
	return d
}

// Matches reports whether the pattern covers the domain. A plain key is exact:
// "foo.com" covers foo.com alone. A wildcard key covers strict subdomains only:
// "*.foo.com" covers api.foo.com but not foo.com. Exactness is the least
// surprising reading for a per-domain resolver choice; a bare domain silently
// changing where its subdomains resolve would be a trap.
func Matches(pattern, domain string) bool {
	d := Domain(domain)
	if d == "" {
		return false
	}
	wild := strings.HasPrefix(strings.TrimSpace(pattern), "*.")
	base := Domain(pattern)
	if base == "" {
		return false
	}
	if !wild {
		return d == base
	}
	return strings.HasSuffix(d, "."+base)
}

// Match finds the most specific profile for a domain: an exact key wins, then
// the longest suffix (a longer parent is more specific than a shorter one).
func Match(profiles map[string]Profile, domain string) (Profile, bool) {
	if len(profiles) == 0 {
		return Profile{}, false
	}
	d := Domain(domain)
	if d == "" {
		return Profile{}, false
	}
	// Exact (non-wildcard) match first.
	if p, ok := lookup(profiles, d); ok {
		return p, true
	}
	bestLen := -1
	var best Profile
	for pattern, p := range profiles {
		base := Domain(pattern)
		if base == "" || !Matches(pattern, d) {
			continue
		}
		if len(base) > bestLen {
			bestLen = len(base)
			best = p
		}
	}
	if bestLen >= 0 {
		return best, true
	}
	return Profile{}, false
}

func lookup(profiles map[string]Profile, domain string) (Profile, bool) {
	for pattern, p := range profiles {
		// A wildcard never matches its own apex; "*." means strict subdomains.
		if strings.HasPrefix(strings.TrimSpace(pattern), "*.") {
			continue
		}
		if Domain(pattern) == domain {
			return p, true
		}
	}
	return Profile{}, false
}

// ProfileIndex is the compiled form of a profile map. Compiling once when the
// policy is installed turns the resolver's per-query lookup from a scan over
// every configured pattern into an exact map hit plus a label walk, which is the
// difference between O(profiles) and O(labels) on every query.
type ProfileIndex struct {
	exact    map[string]Profile
	wildcard map[string]Profile
}

// CompileProfiles builds an immutable lookup index. The returned pointer must not
// be mutated; callers replace it wholesale when the policy changes.
func CompileProfiles(profiles map[string]Profile) *ProfileIndex {
	idx := &ProfileIndex{}
	if len(profiles) == 0 {
		return idx
	}
	idx.exact = make(map[string]Profile, len(profiles))
	idx.wildcard = make(map[string]Profile)
	for pattern, p := range profiles {
		trimmed := strings.TrimSpace(pattern)
		wild := strings.HasPrefix(trimmed, "*.")
		base := Domain(trimmed)
		if base == "" {
			continue
		}
		if wild {
			idx.wildcard[base] = p
		} else {
			idx.exact[base] = p
		}
	}
	return idx
}

// Match finds the most specific profile for domain. An exact key wins, then the
// longest wildcard suffix. A nil index behaves as "no profiles", which keeps
// pools built before a policy is installed and test doubles safe.
func (idx *ProfileIndex) Match(domain string) (Profile, bool) {
	if idx == nil {
		return Profile{}, false
	}
	d := Domain(domain)
	if d == "" {
		return Profile{}, false
	}
	if p, ok := idx.exact[d]; ok {
		return p, true
	}
	if len(idx.wildcard) == 0 {
		return Profile{}, false
	}
	// Walk parents from most specific to least. The first wildcard hit is the
	// longest suffix, so it is the profile the old scan selected.
	for i := 0; i < len(d); {
		dot := strings.IndexByte(d[i:], '.')
		if dot < 0 {
			break
		}
		i += dot + 1
		if i >= len(d) {
			break
		}
		if p, ok := idx.wildcard[d[i:]]; ok {
			return p, true
		}
	}
	return Profile{}, false
}

// ValidUpstreamKey reports whether a non-empty pinned upstream parses as one of
// the resolver URL forms. It is deliberately permissive: the pool's normaliser
// is the authority, and this only rejects obvious junk before it is stored.
func ValidUpstreamKey(upstream string) bool {
	u := strings.TrimSpace(upstream)
	if u == "" {
		return true
	}
	if strings.Contains(u, "://") {
		parsed, err := url.Parse(u)
		return err == nil && parsed.Host != ""
	}
	// host:port or a bare host is accepted; the upstream normaliser decides.
	return !strings.ContainsAny(u, " \t\n")
}
