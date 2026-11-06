// Package dnssec validates DNS responses against the DNSSEC chain of trust.
//
// It is deliberately not a resolver: the caller supplies a Fetcher (normally
// backed by the same upstream pool that answered the client) and this package
// decides whether a response is Secure, Insecure or Bogus. That keeps the
// resolver's caching, ECS and transport policy in one place and leaves DNSSEC
// with a single job.
//
// The validator walks from the root trust anchors down through DS and DNSKEY
// records, then verifies the RRSIGs on the answer. Negative answers are checked
// with NSEC or NSEC3 proofs. A zone that is not signed (no DS at a delegation)
// is Insecure rather than Bogus: it can still be answered, just never with AD.
package dnssec

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"
)

// Status is the outcome of validating one response.
type Status uint8

const (
	// Insecure means the name is below an unsigned delegation, or the zone is
	// signed only with algorithms this build cannot check. The answer may be
	// served, but it must not claim AD.
	Insecure Status = iota
	// Secure means every relevant RRset and denial proof verified against the
	// root trust anchor.
	Secure
	// Bogus means the response claimed to be signed but the chain, a signature
	// or a denial proof did not hold. The caller must answer SERVFAIL.
	Bogus
)

func (s Status) String() string {
	switch s {
	case Secure:
		return "secure"
	case Bogus:
		return "bogus"
	default:
		return "insecure"
	}
}

// Result carries a validation outcome and, for Bogus, a short reason suitable
// for a log line or an Extended DNS Error.
type Result struct {
	Status Status
	Reason string
}

func secure() Result   { return Result{Status: Secure} }
func insecure() Result { return Result{Status: Insecure} }
func bogus(format string, args ...any) Result {
	return Result{Status: Bogus, Reason: fmt.Sprintf(format, args...)}
}

// Fetcher resolves one question. Implementations must set DO and must not let
// an upstream validating resolver filter the data: the validator needs the raw
// DNSKEY, DS, NSEC and NSEC3 records even when the domain is bogus.
type Fetcher interface {
	Fetch(ctx context.Context, name string, qtype uint16) (*dns.Msg, error)
}

// FetchFunc adapts a function to Fetcher.
type FetchFunc func(ctx context.Context, name string, qtype uint16) (*dns.Msg, error)

// Fetch implements Fetcher.
func (f FetchFunc) Fetch(ctx context.Context, name string, qtype uint16) (*dns.Msg, error) {
	return f(ctx, name, qtype)
}

const (
	// maxZoneCache bounds the memory the validator can pin. When it is reached
	// the whole map is dropped: zone keys are cheap to fetch again and an
	// eviction policy is not worth a data structure on this path.
	maxZoneCache = 4096
	// maxChainDepth stops a malicious or malformed chain from looping.
	maxChainDepth = 128
	// maxZoneKeyTTL caps how long a validated DNSKEY set is trusted. A DNSKEY
	// RRset can have a multi-day TTL; re-walking at least hourly keeps key
	// rollovers and revocations visible without a restart.
	maxZoneKeyTTL = time.Hour
	// maxNSEC3Iterations is a CPU guard. RFC 5155 permits 2500; real zones use
	// far fewer, and an attacker-supplied 65535 would turn one query into a
	// hash storm.
	maxNSEC3Iterations = 2500
)

type zoneEntry struct {
	keys    []*dns.DNSKEY
	expires time.Time
}

// Validator validates responses and caches the zone keys it has proven. It is
// safe for concurrent use.
type Validator struct {
	fetch   Fetcher
	anchors []*dns.DS
	now     func() time.Time

	mu       sync.RWMutex
	zones    map[string]*zoneEntry
	insecure map[string]time.Time
}

// New returns a validator using the IANA root trust anchors.
func New(fetch Fetcher) *Validator {
	return &Validator{
		fetch:    fetch,
		anchors:  defaultTrustAnchors(),
		now:      time.Now,
		zones:    make(map[string]*zoneEntry),
		insecure: make(map[string]time.Time),
	}
}

// ValidateTrustAnchors checks a list of DS or DNSKEY records without installing
// them. The settings UI uses it so a malformed anchor is refused at save time
// rather than discovered on the next restart.
func ValidateTrustAnchors(records []string) error {
	if len(records) == 0 {
		return nil
	}
	_, err := parseTrustAnchors(records)
	return err
}

func parseTrustAnchors(records []string) ([]*dns.DS, error) {
	anchors := make([]*dns.DS, 0, len(records))
	for _, text := range records {
		rr, err := dns.NewRR(strings.TrimSpace(text))
		if err != nil {
			return nil, fmt.Errorf("trust anchor %q: %w", text, err)
		}
		switch t := rr.(type) {
		case *dns.DS:
			if canonicalName(t.Hdr.Name) != "." {
				return nil, fmt.Errorf("trust anchor %q is not for the root", t.Hdr.Name)
			}
			anchors = append(anchors, t)
		case *dns.DNSKEY:
			if canonicalName(t.Hdr.Name) != "." {
				return nil, fmt.Errorf("trust anchor %q is not for the root", t.Hdr.Name)
			}
			ds := t.ToDS(dns.SHA256)
			if ds == nil {
				return nil, fmt.Errorf("trust anchor %q could not be converted to a DS record", t.Hdr.Name)
			}
			anchors = append(anchors, ds)
		default:
			return nil, fmt.Errorf("trust anchor %q is neither DS nor DNSKEY", text)
		}
	}
	return anchors, nil
}

// SetTrustAnchors replaces the root trust anchors. Each record is either a DS
// or a DNSKEY in zone-file form, with the owner name "."; a DNSKEY is converted
// to a SHA-256 DS. Passing no records restores the built-in IANA anchors.
func (v *Validator) SetTrustAnchors(records []string) error {
	if len(records) == 0 {
		v.anchors = defaultTrustAnchors()
		return nil
	}
	anchors, err := parseTrustAnchors(records)
	if err != nil {
		return err
	}
	v.anchors = anchors
	return nil
}

// TrustAnchors returns the anchors in DS form, mainly for tests and logging.
func (v *Validator) TrustAnchors() []*dns.DS {
	out := make([]*dns.DS, len(v.anchors))
	copy(out, v.anchors)
	return out
}

// Validate checks resp against q. It never modifies resp.
func (v *Validator) Validate(ctx context.Context, q dns.Question, resp *dns.Msg) Result {
	if resp == nil {
		return bogus("nil response")
	}
	// RRSIG and ANY answers are special: an RRSIG RRset is not itself signed,
	// and ANY is refused before it reaches here. Treat them as unauthenticated
	// rather than manufacturing a bogus verdict.
	if q.Qtype == dns.TypeRRSIG || q.Qtype == dns.TypeANY {
		return insecure()
	}

	switch resp.Rcode {
	case dns.RcodeSuccess:
		if len(resp.Answer) == 0 {
			return v.validateDenial(ctx, q, resp, q.Name)
		}
		result := v.validateAnswer(ctx, q, resp)
		if result.Status == Bogus {
			return result
		}
		terminal, needDenial := terminalAndDenial(q, resp)
		if needDenial {
			denial := v.validateDenial(ctx, q, resp, terminal)
			if denial.Status == Bogus {
				return denial
			}
			if denial.Status == Insecure {
				return insecure()
			}
		}
		return result
	case dns.RcodeNameError:
		return v.validateDenial(ctx, q, resp, terminalName(q.Name, resp.Answer))
	default:
		// SERVFAIL and the other error rcodes are not authenticated data; the
		// caller passes them through as it always did.
		return insecure()
	}
}

// validateAnswer verifies every RRset in the answer section. A signed zone
// must sign every answer RRset; an unsigned zone may not be authenticated.
func (v *Validator) validateAnswer(ctx context.Context, q dns.Question, resp *dns.Msg) Result {
	sets := collectRRsets(resp.Answer)
	overall := Secure
	for _, set := range sets {
		if set.key.rrtype == dns.TypeOPT || set.key.rrtype == dns.TypeRRSIG {
			continue
		}
		// A DNAME synthesis produces an unsigned CNAME next to the signed
		// DNAME. RFC 6672 §2.3 says that CNAME is not signed, so skip it when
		// the answer already carries the DNAME that generated it.
		if set.key.rrtype == dns.TypeCNAME && !hasCoveredRRSIG(set) && hasType(resp.Answer, dns.TypeDNAME) {
			continue
		}
		status := v.validateSet(ctx, set)
		switch status.Status {
		case Bogus:
			return status
		case Insecure:
			overall = Insecure
		}
	}
	if len(sets) == 0 {
		return insecure()
	}
	return Result{Status: overall}
}

// validateSet verifies one RRset with any matching RRSIG. It returns Insecure
// when the signer has no chain to the root, and Bogus when a secure signer's
// signatures are missing or fail.
func (v *Validator) validateSet(ctx context.Context, set rrset) Result {
	if len(set.sigs) == 0 {
		_, _, status, err := v.walk(ctx, set.key.name)
		if err != nil {
			return bogus("%v", err)
		}
		if status == Insecure {
			return insecure()
		}
		return bogus("no RRSIG for %s/%s", set.key.name, dns.TypeToString[set.key.rrtype])
	}

	sawInsecure := false
	for _, sig := range set.sigs {
		if !v.sigTimeValid(sig) {
			continue
		}
		keys, _, status, err := v.walk(ctx, sig.SignerName)
		if err != nil {
			return bogus("%v", err)
		}
		if status == Insecure {
			sawInsecure = true
			continue
		}
		for _, key := range keys {
			if err := sig.Verify(key, set.rrs); err == nil {
				return secure()
			}
		}
	}
	if sawInsecure {
		return insecure()
	}
	return bogus("RRSIG did not verify for %s/%s", set.key.name, dns.TypeToString[set.key.rrtype])
}

// walk resolves the chain of trust from the root anchors to target and returns
// the validated keys of the deepest signed zone at or above target.
func (v *Validator) walk(ctx context.Context, target string) ([]*dns.DNSKEY, string, Status, error) {
	target = canonicalName(target)
	if target != "." && !dns.IsSubDomain(".", target) {
		return nil, "", Bogus, fmt.Errorf("invalid name %q", target)
	}

	root, err := v.rootKeys(ctx)
	if err != nil {
		return nil, "", Bogus, err
	}
	if root == nil {
		return nil, "", Insecure, nil
	}
	if target == "." {
		return root, ".", Secure, nil
	}
	if v.cachedInsecure(target) {
		return nil, "", Insecure, nil
	}

	zone := "."
	keys := root
	for depth := 0; depth < maxChainDepth; depth++ {
		next := childTowards(zone, target)
		if next == "" {
			return keys, zone, Secure, nil
		}

		// A zone whose DNSKEY set is already proven needs no second DS walk.
		// This is the common case: the signer of an answer is the zone itself,
		// and its ancestors were cached by the previous query.
		if entry := v.cachedZone(next); entry != nil {
			zone, keys = next, entry.keys
			continue
		}

		dsResp, err := v.fetch.Fetch(ctx, next, dns.TypeDS)
		if err != nil || dsResp == nil {
			return nil, "", Bogus, fmt.Errorf("fetch DS %s: %w", next, err)
		}
		dsSet, dsSigs := extractSet(dsResp.Answer, next, dns.TypeDS)
		if len(dsSet) > 0 {
			if err := v.verifyRRSet(dsSet, dsSigs, keys); err != nil {
				return nil, "", Bogus, fmt.Errorf("DS %s: %w", next, err)
			}
			child, status, err := v.childKeys(ctx, next, dsSet, keys)
			if err != nil {
				return nil, "", Bogus, err
			}
			if status == Insecure {
				v.rememberInsecure(next, dsResp)
				return nil, "", Insecure, nil
			}
			zone, keys = next, child
			continue
		}

		// No DS at this name. The parent must prove that absence; without the
		// proof an attacker could strip the DS and turn a signed zone insecure.
		result := v.denialWithKeys(dsResp, next, dns.TypeDS, keys, zone)
		if result.Status == Bogus {
			return nil, "", Bogus, fmt.Errorf("no-DS proof for %s: %s", next, result.Reason)
		}
		if result.Status == Insecure {
			v.rememberInsecure(next, dsResp)
			return nil, "", Insecure, nil
		}

		// A name can be unsigned but still a delegation. In that case everything
		// below it is insecure; otherwise target lives inside the current zone.
		nsResp, err := v.fetch.Fetch(ctx, next, dns.TypeNS)
		if err != nil || nsResp == nil {
			return nil, "", Bogus, fmt.Errorf("fetch NS %s: %w", next, err)
		}
		if hasType(nsResp.Answer, dns.TypeNS) {
			v.rememberInsecure(next, nsResp)
			return nil, "", Insecure, nil
		}
		return keys, zone, Secure, nil
	}
	return nil, "", Bogus, errors.New("DNSSEC chain too deep")
}

// rootKeys validates the root DNSKEY RRset against the configured anchors.
func (v *Validator) rootKeys(ctx context.Context) ([]*dns.DNSKEY, error) {
	if entry := v.cachedZone("."); entry != nil {
		return entry.keys, nil
	}
	if len(v.anchors) == 0 {
		return nil, nil
	}

	resp, err := v.fetch.Fetch(ctx, ".", dns.TypeDNSKEY)
	if err != nil || resp == nil {
		return nil, fmt.Errorf("fetch root DNSKEY: %w", err)
	}
	rrs, sigs := extractSet(resp.Answer, ".", dns.TypeDNSKEY)
	if len(rrs) == 0 {
		return nil, errors.New("root DNSKEY answer is empty")
	}
	if !v.verifyWithAnyAnchor(rrs, sigs) {
		return nil, errors.New("root DNSKEY did not verify against any trust anchor")
	}
	keys := dnskeys(rrs)
	v.putZone(".", keys, rrsetExpiry(rrs, sigs, v.now()))
	return keys, nil
}

// childKeys verifies a child DNSKEY RRset using the DS set the parent signed.
func (v *Validator) childKeys(ctx context.Context, zone string, dsRRs []dns.RR, parentKeys []*dns.DNSKEY) ([]*dns.DNSKEY, Status, error) {
	usable := usableDS(dsRRs)
	if len(usable) == 0 {
		// RFC 4035 §5.2: no supported algorithm or digest means the zone is
		// treated as unsigned, not as a validation failure.
		return nil, Insecure, nil
	}

	resp, err := v.fetch.Fetch(ctx, zone, dns.TypeDNSKEY)
	if err != nil || resp == nil {
		return nil, Bogus, fmt.Errorf("fetch DNSKEY %s: %w", zone, err)
	}
	rrs, sigs := extractSet(resp.Answer, zone, dns.TypeDNSKEY)
	if len(rrs) == 0 {
		return nil, Bogus, fmt.Errorf("DNSKEY %s is empty", zone)
	}
	keys := dnskeys(rrs)

	matched := false
	for _, sig := range sigs {
		if !v.sigTimeValid(sig) {
			continue
		}
		for _, key := range keys {
			if !matchesAnyDS(key, usable) {
				continue
			}
			matched = true
			if err := sig.Verify(key, rrs); err == nil {
				v.putZone(zone, keys, rrsetExpiry(rrs, sigs, v.now()))
				return keys, Secure, nil
			}
		}
	}
	if !matched {
		return nil, Bogus, fmt.Errorf("DNSKEY %s has no key matching the parent DS", zone)
	}
	return nil, Bogus, fmt.Errorf("DNSKEY %s did not verify with any DS-matched key", zone)
}

// verifyRRSet checks that at least one signature over rrs verifies with one of
// keys.
func (v *Validator) verifyRRSet(rrs []dns.RR, sigs []*dns.RRSIG, keys []*dns.DNSKEY) error {
	for _, sig := range sigs {
		if !v.sigTimeValid(sig) {
			continue
		}
		for _, key := range keys {
			if err := sig.Verify(key, rrs); err == nil {
				return nil
			}
		}
	}
	return errors.New("no valid RRSIG")
}

func (v *Validator) verifyWithAnyAnchor(rrs []dns.RR, sigs []*dns.RRSIG) bool {
	for _, sig := range sigs {
		if !v.sigTimeValid(sig) {
			continue
		}
		for _, key := range dnskeys(rrs) {
			for _, anchor := range v.anchors {
				if !matchesDS(key, anchor) {
					continue
				}
				if err := sig.Verify(key, rrs); err == nil {
					return true
				}
			}
		}
	}
	return false
}

// sigTimeValid applies RFC 4034 §3.1.5's serial-number comparison rather than a
// plain Unix timestamp comparison, so a signature that wraps past 2038 still
// validates.
func (v *Validator) sigTimeValid(sig *dns.RRSIG) bool {
	now := uint32(v.now().Unix())
	return int32(now-sig.Inception) >= 0 && int32(sig.Expiration-now) >= 0
}

func (v *Validator) cachedZone(zone string) *zoneEntry {
	v.mu.RLock()
	defer v.mu.RUnlock()
	entry, ok := v.zones[zone]
	if !ok || v.now().After(entry.expires) {
		return nil
	}
	return entry
}

func (v *Validator) putZone(zone string, keys []*dns.DNSKEY, expires time.Time) {
	if expires.After(v.now().Add(maxZoneKeyTTL)) {
		expires = v.now().Add(maxZoneKeyTTL)
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if len(v.zones) >= maxZoneCache {
		v.zones = make(map[string]*zoneEntry)
	}
	v.zones[zone] = &zoneEntry{keys: keys, expires: expires}
}

func (v *Validator) cachedInsecure(name string) bool {
	v.mu.RLock()
	defer v.mu.RUnlock()
	now := v.now()
	for zone, expires := range v.insecure {
		if now.Before(expires) && dns.IsSubDomain(zone, name) {
			return true
		}
	}
	return false
}

func (v *Validator) rememberInsecure(zone string, resp *dns.Msg) {
	ttl := uint32(300)
	if resp != nil {
		if t := minTTL(resp.Ns); t > 0 && t < ttl {
			ttl = t
		}
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if len(v.insecure) >= maxZoneCache {
		v.insecure = make(map[string]time.Time)
	}
	v.insecure[zone] = v.now().Add(time.Duration(ttl) * time.Second)
}

func rrsetExpiry(rrs []dns.RR, sigs []*dns.RRSIG, now time.Time) time.Time {
	expires := now.Add(time.Duration(minTTL(rrs)) * time.Second)
	for _, sig := range sigs {
		if e := time.Unix(int64(sig.Expiration), 0); e.Before(expires) {
			expires = e
		}
	}
	return expires
}

func minTTL(rrs []dns.RR) uint32 {
	var out uint32
	for _, rr := range rrs {
		ttl := rr.Header().Ttl
		if ttl == 0 {
			continue
		}
		if out == 0 || ttl < out {
			out = ttl
		}
	}
	return out
}

func dnskeys(rrs []dns.RR) []*dns.DNSKEY {
	out := make([]*dns.DNSKEY, 0, len(rrs))
	for _, rr := range rrs {
		if key, ok := rr.(*dns.DNSKEY); ok {
			out = append(out, key)
		}
	}
	return out
}

func usableDS(rrs []dns.RR) []*dns.DS {
	out := make([]*dns.DS, 0, len(rrs))
	for _, rr := range rrs {
		ds, ok := rr.(*dns.DS)
		if !ok || !supportedAlgorithm(ds.Algorithm) || !supportedDigest(ds.DigestType) {
			continue
		}
		out = append(out, ds)
	}
	return out
}

func matchesAnyDS(key *dns.DNSKEY, dss []*dns.DS) bool {
	for _, ds := range dss {
		if matchesDS(key, ds) {
			return true
		}
	}
	return false
}

func matchesDS(key *dns.DNSKEY, ds *dns.DS) bool {
	if key.KeyTag() != ds.KeyTag || key.Algorithm != ds.Algorithm {
		return false
	}
	computed := key.ToDS(ds.DigestType)
	if computed == nil {
		return false
	}
	return strings.EqualFold(computed.Digest, ds.Digest)
}

func supportedAlgorithm(alg uint8) bool {
	switch alg {
	case dns.RSASHA1, dns.RSASHA1NSEC3SHA1, dns.RSASHA256, dns.RSASHA512,
		dns.ECDSAP256SHA256, dns.ECDSAP384SHA384, dns.ED25519:
		return true
	}
	return false
}

func supportedDigest(digest uint8) bool {
	switch digest {
	case dns.SHA1, dns.SHA256, dns.SHA384:
		return true
	}
	return false
}

// childTowards returns the next label of target below zone, or "" when zone is
// already target (or target is not below zone).
func childTowards(zone, target string) string {
	zone = canonicalName(zone)
	target = canonicalName(target)
	if zone == target {
		return ""
	}
	if !dns.IsSubDomain(zone, target) {
		return ""
	}
	zl := dns.CountLabel(zone)
	tl := dns.CountLabel(target)
	if tl <= zl {
		return ""
	}
	labels := dns.SplitDomainName(target)
	return strings.Join(labels[tl-zl-1:], ".") + "."
}

// parentName returns the immediate parent of name, or "" at the root.
func parentName(name string) string {
	name = canonicalName(name)
	if name == "." {
		return ""
	}
	labels := dns.SplitDomainName(name)
	if len(labels) <= 1 {
		return "."
	}
	return strings.Join(labels[1:], ".") + "."
}

func canonicalName(name string) string {
	if name == "" {
		return "."
	}
	return strings.ToLower(dns.Fqdn(name))
}

// hasType reports whether any record in rrs has the given type.
func hasType(rrs []dns.RR, rrtype uint16) bool {
	for _, rr := range rrs {
		if rr.Header().Rrtype == rrtype {
			return true
		}
	}
	return false
}
