package dnssec

import (
	"context"

	"github.com/miekg/dns"
)

// validateDenial checks NXDOMAIN and NODATA proofs. The zone is the one that
// contains the denied name; the NSEC/NSEC3 records must be signed by that zone
// and must chain to the root, and the bitmap/cover checks must prove the
// negative answer.
func (v *Validator) validateDenial(ctx context.Context, q dns.Question, resp *dns.Msg, name string) Result {
	name = canonicalName(name)

	// The zone is decided by the name being denied, not by the SOA in the
	// response. Taking it from the SOA let a forged authority section point the
	// proof at an unsigned zone and turn a signed NXDOMAIN into an unauthenticated
	// (but accepted) one.
	_, zone, status, err := v.walk(ctx, name)
	if err != nil {
		return bogus("%v", err)
	}
	if status == Insecure {
		return insecure()
	}

	nsecs, nsec3s, result := v.validatedDenialRecords(ctx, resp, zone)
	if result.Status != Secure {
		return result
	}
	if len(nsecs) == 0 && len(nsec3s) == 0 {
		return bogus("negative answer for %s carries neither NSEC nor NSEC3", name)
	}

	if resp.Rcode == dns.RcodeNameError {
		return proveNoName(nsecs, nsec3s, name, zone)
	}
	return proveNoData(nsecs, nsec3s, name, zone, q.Qtype)
}

// denialWithKeys validates denial records against a known zone key set. It is
// used while walking the chain, where the signer is by definition the zone we
// just validated.
func (v *Validator) denialWithKeys(resp *dns.Msg, name string, qtype uint16, keys []*dns.DNSKEY, zone string) Result {
	name = canonicalName(name)
	zone = canonicalName(zone)

	nsecs, nsec3s, result := v.validatedDenialRecordsWithKeys(resp, keys, zone)
	if result.Status != Secure {
		return result
	}
	if len(nsecs) == 0 && len(nsec3s) == 0 {
		return bogus("no-DS answer for %s carries neither NSEC nor NSEC3", name)
	}
	if resp.Rcode == dns.RcodeNameError {
		return proveNoName(nsecs, nsec3s, name, zone)
	}
	return proveNoData(nsecs, nsec3s, name, zone, qtype)
}

func (v *Validator) validatedDenialRecords(ctx context.Context, resp *dns.Msg, zone string) ([]*dns.NSEC, []*dns.NSEC3, Result) {
	var nsecs []*dns.NSEC
	var nsec3s []*dns.NSEC3
	for _, set := range collectRRsets(resp.Ns) {
		if set.key.rrtype != dns.TypeNSEC && set.key.rrtype != dns.TypeNSEC3 {
			continue
		}
		// A proof is only authoritative for the zone that contains the denied
		// name. A valid NSEC from some other zone must not be accepted just
		// because its numeric interval happens to cover this name.
		if !signedBy(set.sigs, zone) {
			return nil, nil, bogus("denial RRset %s is not signed by %s", set.key.name, zone)
		}
		result := v.validateSet(ctx, set)
		if result.Status != Secure {
			return nil, nil, result
		}
		if set.key.rrtype == dns.TypeNSEC3 {
			if !inNSEC3Zone(set.key.name, zone) {
				return nil, nil, bogus("NSEC3 RRset %s is outside %s", set.key.name, zone)
			}
		} else if !dns.IsSubDomain(zone, set.key.name) {
			return nil, nil, bogus("NSEC RRset %s is outside %s", set.key.name, zone)
		}
		for _, rr := range set.rrs {
			switch t := rr.(type) {
			case *dns.NSEC:
				if !dns.IsSubDomain(zone, t.NextDomain) {
					return nil, nil, bogus("NSEC next name %s is outside %s", t.NextDomain, zone)
				}
				nsecs = append(nsecs, t)
			case *dns.NSEC3:
				if usableNSEC3(t) {
					nsec3s = append(nsec3s, t)
				}
			}
		}
	}
	return nsecs, nsec3s, secure()
}

func (v *Validator) validatedDenialRecordsWithKeys(resp *dns.Msg, keys []*dns.DNSKEY, zone string) ([]*dns.NSEC, []*dns.NSEC3, Result) {
	var nsecs []*dns.NSEC
	var nsec3s []*dns.NSEC3
	for _, set := range collectRRsets(resp.Ns) {
		switch set.key.rrtype {
		case dns.TypeNSEC, dns.TypeNSEC3:
			if len(set.sigs) == 0 {
				return nil, nil, bogus("denial RRset %s has no RRSIG", set.key.name)
			}
			if err := v.verifyRRSet(set.rrs, set.sigs, keys); err != nil {
				return nil, nil, bogus("denial RRset %s: %v", set.key.name, err)
			}
			if set.key.rrtype == dns.TypeNSEC3 {
				if !inNSEC3Zone(set.key.name, zone) {
					return nil, nil, bogus("NSEC3 RRset %s is outside %s", set.key.name, zone)
				}
			} else if !dns.IsSubDomain(zone, set.key.name) {
				return nil, nil, bogus("NSEC RRset %s is outside %s", set.key.name, zone)
			}
			for _, rr := range set.rrs {
				switch t := rr.(type) {
				case *dns.NSEC:
					if !dns.IsSubDomain(zone, t.NextDomain) {
						return nil, nil, bogus("NSEC next name %s is outside %s", t.NextDomain, zone)
					}
					nsecs = append(nsecs, t)
				case *dns.NSEC3:
					if usableNSEC3(t) {
						nsec3s = append(nsec3s, t)
					}
				}
			}
		}
	}
	return nsecs, nsec3s, secure()
}

func signedBy(sigs []*dns.RRSIG, zone string) bool {
	for _, sig := range sigs {
		if canonicalName(sig.SignerName) == canonicalName(zone) {
			return true
		}
	}
	return false
}

// inNSEC3Zone reports whether an NSEC3 owner is exactly one hash label below
// the zone apex.
func inNSEC3Zone(owner, zone string) bool {
	owner = canonicalName(owner)
	zone = canonicalName(zone)
	if !dns.IsSubDomain(zone, owner) {
		return false
	}
	return dns.CountLabel(owner) == dns.CountLabel(zone)+1
}

func proveNoName(nsecs []*dns.NSEC, nsec3s []*dns.NSEC3, name, zone string) Result {
	if nsecNoName(nsecs, name, zone) {
		return secure()
	}
	// Parameters are part of the proof: an NSEC3 chain with one salt or
	// iteration count must not be mixed with another.
	for _, group := range nsec3Groups(nsec3s) {
		if nsec3NoName(group, name, zone) {
			return secure()
		}
	}
	// An opt-out span authenticates that something below it may be unsigned,
	// but not that this particular name does not exist. Serving the NXDOMAIN
	// without AD is the conservative reading and is what an opt-out span is for.
	for _, group := range nsec3Groups(nsec3s) {
		if nsec3OptOutCovers(group, name, zone) {
			return insecure()
		}
	}
	return bogus("NXDOMAIN for %s has no valid denial proof", name)
}

func proveNoData(nsecs []*dns.NSEC, nsec3s []*dns.NSEC3, name, zone string, qtype uint16) Result {
	if nsecNoData(nsecs, name, zone, qtype) {
		return secure()
	}
	for _, group := range nsec3Groups(nsec3s) {
		if nsec3NoData(group, name, zone, qtype) {
			return secure()
		}
	}
	if qtype == dns.TypeDS {
		for _, group := range nsec3Groups(nsec3s) {
			if nsec3OptOutCovers(group, name, zone) {
				return insecure()
			}
		}
	}
	return bogus("NODATA for %s/%s has no valid denial proof", name, dns.TypeToString[qtype])
}

// nsecNoData proves that name exists but has no RRset of qtype.
func nsecNoData(nsecs []*dns.NSEC, name, zone string, qtype uint16) bool {
	name = canonicalName(name)
	for _, rr := range nsecs {
		if canonicalName(rr.Hdr.Name) != name {
			continue
		}
		if nsecTypePresent(rr.TypeBitMap, dns.TypeCNAME) && qtype != dns.TypeCNAME {
			continue
		}
		if !nsecTypePresent(rr.TypeBitMap, qtype) {
			return true
		}
	}

	// Wildcard NODATA: the wildcard exists and lacks qtype, and the next closer
	// name is covered so the wildcard is the one that matched.
	ce := closestEncloserNSEC(name, zone, nsecs, true)
	if ce == "" {
		return false
	}
	next := childTowards(ce, name)
	if next == "" || !nsecAnyCovers(nsecs, next) {
		return false
	}
	wildcard := "*." + ce
	for _, rr := range nsecs {
		if canonicalName(rr.Hdr.Name) == wildcard &&
			!nsecTypePresent(rr.TypeBitMap, qtype) &&
			!nsecTypePresent(rr.TypeBitMap, dns.TypeCNAME) {
			return true
		}
	}
	return false
}

// nsecNoName proves that name does not exist: an NSEC covers it and another
// NSEC covers the wildcard at the closest encloser.
func nsecNoName(nsecs []*dns.NSEC, name, zone string) bool {
	name = canonicalName(name)
	if !nsecAnyCovers(nsecs, name) {
		return false
	}
	ce := closestEncloserNSEC(name, zone, nsecs, false)
	if ce == "" {
		return false
	}
	return nsecAnyCovers(nsecs, "*."+ce)
}

// nsec3NoData proves NODATA with NSEC3, including wildcard NODATA.
func nsec3NoData(nsec3s []*dns.NSEC3, name, zone string, qtype uint16) bool {
	name = canonicalName(name)
	for _, rr := range nsec3s {
		if !rr.Match(name) {
			continue
		}
		if nsecTypePresent(rr.TypeBitMap, dns.TypeCNAME) && qtype != dns.TypeCNAME {
			continue
		}
		if !nsecTypePresent(rr.TypeBitMap, qtype) {
			return true
		}
	}

	ce := closestEncloserNSEC3(name, zone, nsec3s, true)
	if ce == "" {
		return false
	}
	next := childTowards(ce, name)
	cover := nsec3Cover(nsec3s, next)
	if next == "" || cover == nil || cover.Flags&1 != 0 {
		return false
	}
	wildcard := "*." + ce
	for _, rr := range nsec3s {
		if rr.Match(wildcard) &&
			!nsecTypePresent(rr.TypeBitMap, qtype) &&
			!nsecTypePresent(rr.TypeBitMap, dns.TypeCNAME) {
			return true
		}
	}
	return false
}

// nsec3NoName proves NXDOMAIN with NSEC3: the closest encloser exists, the next
// closer name is covered, and the wildcard at the closest encloser is covered.
func nsec3NoName(nsec3s []*dns.NSEC3, name, zone string) bool {
	name = canonicalName(name)
	ce := closestEncloserNSEC3(name, zone, nsec3s, false)
	if ce == "" {
		return false
	}
	next := childTowards(ce, name)
	cover := nsec3Cover(nsec3s, next)
	if next == "" || cover == nil || cover.Flags&1 != 0 {
		return false
	}
	return nsec3Cover(nsec3s, "*."+ce) != nil
}

// nsec3OptOutCovers reports an NSEC3 opt-out span covering name. It is only used
// to decide Insecure versus Bogus, never to prove Secure.
func nsec3OptOutCovers(nsec3s []*dns.NSEC3, name, zone string) bool {
	name = canonicalName(name)
	ce := closestEncloserNSEC3(name, zone, nsec3s, true)
	if ce == "" {
		return false
	}
	next := childTowards(ce, name)
	if next == "" {
		return false
	}
	rr := nsec3Cover(nsec3s, next)
	return rr != nil && rr.Flags&1 != 0
}

func closestEncloserNSEC(name, zone string, nsecs []*dns.NSEC, includeName bool) string {
	zone = canonicalName(zone)
	start := canonicalName(name)
	if !includeName {
		start = parentName(start)
	}
	for candidate := start; candidate != ""; candidate = parentName(candidate) {
		if !dns.IsSubDomain(zone, candidate) {
			return ""
		}
		if candidate == zone {
			return candidate
		}
		for _, rr := range nsecs {
			owner := canonicalName(rr.Hdr.Name)
			if owner == candidate || (owner != candidate && dns.IsSubDomain(candidate, owner)) {
				return candidate
			}
		}
		if candidate == "." {
			return ""
		}
	}
	return ""
}

func closestEncloserNSEC3(name, zone string, nsec3s []*dns.NSEC3, includeName bool) string {
	zone = canonicalName(zone)
	start := canonicalName(name)
	if !includeName {
		start = parentName(start)
	}
	for candidate := start; candidate != ""; candidate = parentName(candidate) {
		if !dns.IsSubDomain(zone, candidate) {
			return ""
		}
		for _, rr := range nsec3s {
			if rr.Match(candidate) {
				return candidate
			}
		}
		if candidate == zone {
			return ""
		}
		if candidate == "." {
			return ""
		}
	}
	return ""
}

func nsecAnyCovers(nsecs []*dns.NSEC, name string) bool {
	for _, rr := range nsecs {
		if nsecCovers(rr, name) {
			return true
		}
	}
	return false
}

func nsec3Cover(nsec3s []*dns.NSEC3, name string) *dns.NSEC3 {
	for _, rr := range nsec3s {
		if rr.Cover(name) {
			return rr
		}
	}
	return nil
}

func nsecCovers(rr *dns.NSEC, name string) bool {
	owner := canonicalName(rr.Hdr.Name)
	next := canonicalName(rr.NextDomain)
	n := canonicalName(name)
	if n == owner {
		return false
	}
	if owner == next {
		return true
	}
	if canonicalLess(owner, next) {
		return canonicalLess(owner, n) && canonicalLess(n, next)
	}
	// The last NSEC in a zone points back at the apex; the interval wraps.
	return canonicalLess(owner, n) || canonicalLess(n, next)
}

// canonicalLess implements the canonical DNS name order of RFC 4034 §6.1:
// labels are compared from the right, byte-wise and case-insensitively, and an
// ancestor sorts before its descendants.
func canonicalLess(a, b string) bool {
	la := dns.SplitDomainName(canonicalName(a))
	lb := dns.SplitDomainName(canonicalName(b))
	i, j := len(la)-1, len(lb)-1
	for i >= 0 && j >= 0 {
		if la[i] != lb[j] {
			return la[i] < lb[j]
		}
		i--
		j--
	}
	return len(la) < len(lb)
}

func usableNSEC3(rr *dns.NSEC3) bool {
	return rr.Hash == dns.SHA1 && rr.Iterations <= maxNSEC3Iterations
}

type nsec3Params struct {
	hash       uint8
	iterations uint16
	salt       string
}

// nsec3Groups splits a proof into chains that share hash parameters. Records
// from different chains cannot be combined to satisfy one proof.
func nsec3Groups(rrs []*dns.NSEC3) [][]*dns.NSEC3 {
	index := make(map[nsec3Params]int)
	var groups [][]*dns.NSEC3
	for _, rr := range rrs {
		if !usableNSEC3(rr) {
			continue
		}
		key := nsec3Params{hash: rr.Hash, iterations: rr.Iterations, salt: rr.Salt}
		i, ok := index[key]
		if !ok {
			i = len(groups)
			index[key] = i
			groups = append(groups, nil)
		}
		groups[i] = append(groups[i], rr)
	}
	return groups
}
