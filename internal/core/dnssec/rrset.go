package dnssec

import (
	"github.com/miekg/dns"
)

type rrsetKey struct {
	name   string
	rrtype uint16
	class  uint16
}

type rrset struct {
	key  rrsetKey
	rrs  []dns.RR
	sigs []*dns.RRSIG
}

// collectRRsets groups a section by owner/type/class and attaches the matching
// RRSIGs. RRSIGs without an RRset are ignored: they are response metadata, not
// authenticated data.
func collectRRsets(section []dns.RR) []rrset {
	index := make(map[rrsetKey]int)
	sets := make([]rrset, 0, len(section))
	for _, rr := range section {
		if rr == nil || rr.Header().Rrtype == dns.TypeRRSIG {
			continue
		}
		h := rr.Header()
		key := rrsetKey{name: canonicalName(h.Name), rrtype: h.Rrtype, class: h.Class}
		i, ok := index[key]
		if !ok {
			i = len(sets)
			index[key] = i
			sets = append(sets, rrset{key: key})
		}
		sets[i].rrs = append(sets[i].rrs, rr)
	}
	for _, rr := range section {
		sig, ok := rr.(*dns.RRSIG)
		if !ok {
			continue
		}
		key := rrsetKey{name: canonicalName(sig.Hdr.Name), rrtype: sig.TypeCovered, class: sig.Hdr.Class}
		if i, ok := index[key]; ok {
			sets[i].sigs = append(sets[i].sigs, sig)
		}
	}
	return sets
}

// extractSet pulls one owner/type RRset and its signatures out of a section.
func extractSet(section []dns.RR, name string, rrtype uint16) ([]dns.RR, []*dns.RRSIG) {
	name = canonicalName(name)
	var rrs []dns.RR
	var sigs []*dns.RRSIG
	for _, rr := range section {
		h := rr.Header()
		if canonicalName(h.Name) != name {
			continue
		}
		if rrtype == dns.TypeRRSIG {
			continue
		}
		if h.Rrtype == rrtype {
			rrs = append(rrs, rr)
		}
		if sig, ok := rr.(*dns.RRSIG); ok && sig.TypeCovered == rrtype {
			sigs = append(sigs, sig)
		}
	}
	return rrs, sigs
}

func hasCoveredRRSIG(set rrset) bool {
	return len(set.sigs) > 0
}

// terminalName follows a CNAME chain in the answer and returns the final owner.
func terminalName(start string, answer []dns.RR) string {
	name := canonicalName(start)
	for depth := 0; depth < 16; depth++ {
		next := ""
		for _, rr := range answer {
			cname, ok := rr.(*dns.CNAME)
			if !ok {
				continue
			}
			if canonicalName(cname.Hdr.Name) == name {
				next = canonicalName(cname.Target)
				break
			}
		}
		if next == "" || next == name {
			return name
		}
		name = next
	}
	return name
}

// terminalAndDenial reports whether a negative proof is still missing after the
// answer section has been validated. A CNAME chain that ends in NODATA or
// NXDOMAIN needs a denial for the final target, not for the original name.
func terminalAndDenial(q dns.Question, resp *dns.Msg) (string, bool) {
	if resp.Rcode == dns.RcodeNameError {
		return terminalName(q.Name, resp.Answer), true
	}
	terminal := terminalName(q.Name, resp.Answer)
	// A DNAME answer is positive even though the synthesized CNAME may be the
	// only other record with the queried owner.
	if hasType(resp.Answer, dns.TypeDNAME) {
		return "", false
	}
	for _, rr := range resp.Answer {
		h := rr.Header()
		if canonicalName(h.Name) != terminal {
			continue
		}
		if h.Rrtype == q.Qtype {
			return "", false
		}
		if q.Qtype == dns.TypeCNAME && h.Rrtype == dns.TypeCNAME {
			return "", false
		}
	}
	return terminal, true
}

// nsecTypePresent reports whether an NSEC/NSEC3 bitmap contains a type.
func nsecTypePresent(bitmap []uint16, rrtype uint16) bool {
	for _, t := range bitmap {
		if t == rrtype {
			return true
		}
	}
	return false
}
