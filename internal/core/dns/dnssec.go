package dns

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"digitdns/internal/core/cache"
	"digitdns/internal/core/dnssec"
	"digitdns/internal/core/upstream"
	"github.com/miekg/dns"
)

// dnssecValidationTimeout bounds the extra DNSKEY/DS lookups one client query
// may trigger. The normal upstream timeout still applies to each of them; this
// is the ceiling for the whole chain so a pathological zone cannot pin a
// request forever.
const dnssecValidationTimeout = 10 * time.Second

// ResponseValidator is the part of the DNSSEC validator the handler needs. The
// interface keeps tests able to inject a verdict without building a signed zone.
type ResponseValidator interface {
	Validate(ctx context.Context, q dns.Question, resp *dns.Msg) dnssec.Result
}

type validatorSlot struct{ v ResponseValidator }

func (h *Handler) currentValidator() ResponseValidator {
	if p := h.validator.Load(); p != nil {
		return p.v
	}
	return nil
}

// DNSSECEnabled reports whether validation is installed right now.
func (h *Handler) DNSSECEnabled() bool {
	return h.currentValidator() != nil
}

// EnableDNSSEC turns on chain validation. anchors are optional DS/DNSKEY
// records in zone-file form; an empty slice keeps the built-in IANA root
// anchors. It is safe to call on a running server: the query path swaps the
// validator pointer atomically, and the cache is flushed so answers validated
// under the previous policy cannot outlive it.
func (h *Handler) EnableDNSSEC(anchors []string) error {
	if h.upstreams == nil {
		return errors.New("dnssec: no upstream pool")
	}
	v := dnssec.New(poolFetcher{pool: h.upstreams})
	if len(anchors) > 0 {
		if err := v.SetTrustAnchors(anchors); err != nil {
			return err
		}
	}
	h.validator.Store(&validatorSlot{v: v})
	if h.cache != nil {
		h.cache.Flush()
	}
	return nil
}

// DisableDNSSEC turns validation off without a restart.
func (h *Handler) DisableDNSSEC() {
	h.validator.Store(nil)
	if h.cache != nil {
		h.cache.Flush()
	}
}

// SetDNSSEC applies a saved policy to the running resolver. The settings card
// uses this immediately after persisting, so the running badge changes without
// a service restart.
func (h *Handler) SetDNSSEC(enabled bool, anchors []string) error {
	if !enabled {
		h.DisableDNSSEC()
		return nil
	}
	return h.EnableDNSSEC(anchors)
}

// poolFetcher asks the configured pool for raw DNSSEC records. CD=1 is
// deliberate: an upstream that validates must still hand back the data (or the
// evidence of bogusness) rather than pre-empting this validator with SERVFAIL.
type poolFetcher struct {
	pool *upstream.UpstreamPool
}

func (f poolFetcher) Fetch(_ context.Context, name string, qtype uint16) (*dns.Msg, error) {
	q := new(dns.Msg)
	q.SetQuestion(dns.Fqdn(name), qtype)
	q.RecursionDesired = true
	q.CheckingDisabled = true
	q.SetEdns0(dns.DefaultMsgSize, true)

	domain := strings.TrimSuffix(strings.ToLower(dns.Fqdn(name)), ".")
	resp, _, _, err := f.pool.ExchangeFor(q, domain)
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, errors.New("empty upstream response")
	}
	if resp.Rcode != dns.RcodeSuccess && resp.Rcode != dns.RcodeNameError {
		return nil, fmt.Errorf("upstream returned %s", dns.RcodeToString[resp.Rcode])
	}
	return resp, nil
}

// validateUpstream applies the validator to an answer that came back from the
// pool. It clears any AD bit the upstream volunteered first: an AD this server
// did not produce is not evidence of anything.
//
// A Bogus result is returned to the caller as a SERVFAIL, with an Extended DNS
// Error when the client sent OPT so a diagnosing client can tell a broken zone
// from a broken resolver.
func (h *Handler) validateUpstream(q dns.Question, resp *dns.Msg) dnssec.Result {
	validator := h.currentValidator()
	if validator == nil || resp == nil {
		return dnssec.Result{Status: dnssec.Insecure}
	}
	resp.AuthenticatedData = false

	ctx, cancel := context.WithTimeout(context.Background(), dnssecValidationTimeout)
	defer cancel()
	result := validator.Validate(ctx, q, resp)
	if result.Status == dnssec.Secure {
		resp.AuthenticatedData = true
	}
	if result.Status == dnssec.Bogus {
		log.Printf("[DNSSEC] Bogus answer for %s/%s from upstream: %s", q.Name, dns.TypeToString[q.Qtype], result.Reason)
	}
	return result
}

// ensureDNSSECRequest makes an upstream query capable of returning DNSSEC
// records even when the client did not ask for them. The client's own flags are
// restored on the response before it is served.
func ensureDNSSECRequest(m *dns.Msg) {
	if opt := m.IsEdns0(); opt != nil {
		opt.SetDo(true)
		if opt.UDPSize() < dns.DefaultMsgSize {
			opt.SetUDPSize(dns.DefaultMsgSize)
		}
		return
	}
	m.SetEdns0(dns.DefaultMsgSize, true)
}

// capDNSSECLifetime lowers the served TTLs to the nearest RRSIG expiration.
// The cache applies the same ceiling when it stores the answer; doing it here
// too keeps the first client from handing downstream caches a TTL that outlives
// the signature.
func capDNSSECLifetime(resp *dns.Msg) {
	if resp == nil {
		return
	}
	now := uint32(time.Now().Unix())
	var remaining uint32
	for _, section := range [][]dns.RR{resp.Answer, resp.Ns, resp.Extra} {
		for _, rr := range section {
			sig, ok := rr.(*dns.RRSIG)
			if !ok {
				continue
			}
			left := sig.Expiration - now
			if int32(left) <= 0 {
				continue
			}
			if remaining == 0 || left < remaining {
				remaining = left
			}
		}
	}
	if remaining == 0 {
		return
	}
	for _, section := range [][]dns.RR{resp.Answer, resp.Ns, resp.Extra} {
		for _, rr := range section {
			h := rr.Header()
			if h.Rrtype == dns.TypeOPT {
				continue
			}
			if h.Ttl > remaining {
				h.Ttl = remaining
			}
		}
	}
}

// normalizeForClient removes DNSSEC material a client did not ask for. This
// matters because validation forces DO=1 upstream even for a plain query.
func normalizeForClient(resp *dns.Msg, opts cache.QueryOptions, qtype uint16) {
	if resp == nil {
		return
	}
	if !opts.EDNS {
		resp.Extra = removeOPTRecords(resp.Extra)
	} else if opt := resp.IsEdns0(); opt != nil && !opts.DO {
		opt.SetDo(false)
	}
	if opts.DO {
		return
	}
	resp.Answer = stripDNSSECRecords(resp.Answer, qtype)
	resp.Ns = stripDNSSECRecords(resp.Ns, qtype)
	resp.Extra = stripDNSSECRecords(resp.Extra, qtype)
}

func stripDNSSECRecords(rrs []dns.RR, qtype uint16) []dns.RR {
	if len(rrs) == 0 {
		return rrs
	}
	out := rrs[:0]
	for _, rr := range rrs {
		t := rr.Header().Rrtype
		switch {
		case t == dns.TypeRRSIG && qtype != dns.TypeRRSIG:
			continue
		case t == dns.TypeNSEC && qtype != dns.TypeNSEC:
			continue
		case t == dns.TypeNSEC3 && qtype != dns.TypeNSEC3:
			continue
		case t == dns.TypeOPT:
			// OPT is handled by the EDNS branch above.
		}
		out = append(out, rr)
	}
	return out
}

func removeOPTRecords(rrs []dns.RR) []dns.RR {
	if len(rrs) == 0 {
		return rrs
	}
	out := rrs[:0]
	for _, rr := range rrs {
		if rr.Header().Rrtype != dns.TypeOPT {
			out = append(out, rr)
		}
	}
	return out
}

// attachBogusEDE adds Extended DNS Error 6 (DNSSEC Bogus) to a SERVFAIL sent to
// a client that used EDNS. A reply never introduces OPT to a plain query.
func attachBogusEDE(m, req *dns.Msg) {
	opt := req.IsEdns0()
	if opt == nil {
		return
	}
	m.SetEdns0(opt.UDPSize(), opt.Do())
	if mopt := m.IsEdns0(); mopt != nil {
		mopt.Option = append(mopt.Option, &dns.EDNS0_EDE{
			InfoCode:  dns.ExtendedErrorCodeDNSBogus,
			ExtraText: "DNSSEC validation failed",
		})
	}
}
