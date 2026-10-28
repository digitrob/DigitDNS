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

// Strategy values accepted in a profile. Empty means inherit the pool's global
// racing setting.
const (
	StrategyFastest    = "fastest"
	StrategySequential = "sequential"
	// StrategyHedged starts with the fastest measured resolver and launches the
	// next after a short delay; the first usable answer wins.
	StrategyHedged = "hedged"
	// StrategyBestKnown uses the fastest measured resolver, then falls through
	// the remaining resolvers in latency order on failure.
	StrategyBestKnown = "best"
)

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
// meaning the pool's configured default).
func ValidStrategy(value string) bool {
	switch value {
	case "", StrategyFastest, StrategySequential, StrategyHedged, StrategyBestKnown:
		return true
	}
	return false
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
