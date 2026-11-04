package aggregate

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"digitdns/internal/database"

	"github.com/miekg/dns"
)

// Bucket is one hourly or daily aggregate. Queries is fed by RecordTotal so it
// counts every query the resolver saw; the remaining fields are fed by
// RecordDetail from the query-log entry. That split matters: the live log
// samples rate-limit refusals, but the total card must still count every packet.
type Bucket struct {
	Queries uint64 `json:"queries"`
	Blocked uint64 `json:"blocked"`
	Proxied uint64 `json:"proxied"`
	Cached  uint64 `json:"cached"`
	Failed  uint64 `json:"failed"`

	ByAction       map[string]uint64 `json:"by_action,omitempty"`
	ByProtocol     map[string]uint64 `json:"by_protocol,omitempty"`
	ByUpstream     map[string]uint64 `json:"by_upstream,omitempty"`
	ByRelayCountry map[string]uint64 `json:"by_relay_country,omitempty"`
	ByQType        map[string]uint64 `json:"by_qtype,omitempty"`
	ByRcode        map[string]uint64 `json:"by_rcode,omitempty"`

	TopDomains        Counts `json:"top_domains"`
	TopBlockedDomains Counts `json:"top_blocked_domains"`
	TopClients        Counts `json:"top_clients"`

	// ClientHLL holds the HyperLogLog registers used for an unbounded distinct
	// client count. It is absent on buckets written before the field existed;
	// those fall back to the bounded top-client table and report approx=true.
	// Complete is false when the registers were seeded from a legacy bucket's
	// bounded client table, so a mixed range never claims HLL precision.
	ClientHLL         []byte `json:"client_hll,omitempty"`
	ClientHLLComplete bool   `json:"client_hll_complete,omitempty"`

	LatencySumMs float64 `json:"latency_sum_ms"`
	LatencyCount uint64  `json:"latency_count"`
}

func newBucket() *Bucket {
	return &Bucket{
		ByAction:          make(map[string]uint64),
		ByProtocol:        make(map[string]uint64),
		ByUpstream:        make(map[string]uint64),
		ByRelayCountry:    make(map[string]uint64),
		ByQType:           make(map[string]uint64),
		ByRcode:           make(map[string]uint64),
		ClientHLL:         newHLL(),
		ClientHLLComplete: true,
	}
}

func (b *Bucket) ensureMaps() {
	if b.ByAction == nil {
		b.ByAction = make(map[string]uint64)
	}
	if b.ByProtocol == nil {
		b.ByProtocol = make(map[string]uint64)
	}
	if b.ByUpstream == nil {
		b.ByUpstream = make(map[string]uint64)
	}
	if b.ByRelayCountry == nil {
		b.ByRelayCountry = make(map[string]uint64)
	}
	if b.ByQType == nil {
		b.ByQType = make(map[string]uint64)
	}
	if b.ByRcode == nil {
		b.ByRcode = make(map[string]uint64)
	}
}

func (b *Bucket) recordTotal() {
	b.Queries++
}

func (b *Bucket) record(item database.QueryLogItem, cfg Config, clientSalt []byte) {
	b.ensureMaps()

	action := strings.ToUpper(strings.TrimSpace(item.Action))
	if action == "" {
		action = "UNKNOWN"
	}
	b.ByAction[action]++

	blocked := action == "BLOCK"
	proxied := action == "PROXY" || action == "PROXY_IPV6_SINK"
	// Policy refusals are not resolver failures. A sinkhole is blocked, a
	// rate-limit/quota refusal is its own action, and only SERVFAIL/FORMERR-like
	// answers (including PROXY_FAILED) land in the failure counter.
	failed := strings.Contains(action, "FAILED") ||
		(failureRcode(item.Rcode) && action != "BLOCK" && action != "RATELIMIT" && action != "QUOTA")
	if blocked {
		b.Blocked++
	}
	if proxied {
		b.Proxied++
	}
	if item.Cached {
		b.Cached++
	}
	if failed {
		b.Failed++
	}

	proto := strings.ToUpper(strings.TrimSpace(item.Protocol))
	if proto == "" {
		proto = "UNKNOWN"
	}
	b.ByProtocol[proto]++

	if item.Upstream != "" {
		b.ByUpstream[item.Upstream]++
	}
	if proxied {
		country := strings.ToUpper(strings.TrimSpace(item.RelayCountry))
		if country == "" {
			country = "LOCAL"
		}
		b.ByRelayCountry[country]++
	}

	qtype := qtypeName(item.QType)
	b.ByQType[qtype]++

	rcode := rcodeName(item.Rcode)
	b.ByRcode[rcode]++

	domain := normalizeKey(item.Domain)
	b.TopDomains.add(domain, 1, cfg.TopDomains)
	if blocked {
		b.TopBlockedDomains.add(domain, 1, cfg.TopDomains)
	}

	rawIP := strings.TrimSpace(item.ClientIP)
	rawAccount := strings.TrimSpace(item.AccountName)
	client := rawClientKey(rawIP, rawAccount)
	// A per-client opt-out suppresses the identifier only. Totals, domains and
	// every other dimension stay accurate, but the excluded client never
	// reaches the top-client table or the distinct-client registers. Match both
	// the IP and the account name, because an operator may have entered either.
	if !cfg.excludesClient(client) && !cfg.excludesClient(rawIP) && !cfg.excludesClient(rawAccount) {
		key := anonymizeClient(client, clientSalt, cfg.AnonymizeClients)
		b.TopClients.add(key, 1, cfg.TopClients)
		if len(b.ClientHLL) != hllRegisters {
			b.ClientHLL = newHLL()
		}
		hllAdd(b.ClientHLL, key)
	}

	if item.LatencyMs > 0 {
		b.LatencySumMs += item.LatencyMs
	}
	b.LatencyCount++
}

// merge folds an hourly bucket into a daily bucket (or a recovered bucket into
// the one loaded at startup). Totals and maps are additive; top-N tables merge
// through their bounded add path so the daily table is still capped.
func (b *Bucket) merge(other *Bucket, cfg Config) {
	if other == nil {
		return
	}
	b.Queries += other.Queries
	b.Blocked += other.Blocked
	b.Proxied += other.Proxied
	b.Cached += other.Cached
	b.Failed += other.Failed
	b.LatencySumMs += other.LatencySumMs
	b.LatencyCount += other.LatencyCount

	b.ensureMaps()
	mergeMap(b.ByAction, other.ByAction)
	mergeMap(b.ByProtocol, other.ByProtocol)
	mergeMap(b.ByUpstream, other.ByUpstream)
	mergeMap(b.ByRelayCountry, other.ByRelayCountry)
	mergeMap(b.ByQType, other.ByQType)
	mergeMap(b.ByRcode, other.ByRcode)

	b.TopDomains.merge(other.TopDomains, cfg.TopDomains)
	b.TopBlockedDomains.merge(other.TopBlockedDomains, cfg.TopDomains)
	b.TopClients.merge(other.TopClients, cfg.TopClients)
	// Prefer the registers when both sides have them. A legacy hourly bucket
	// cannot be merged register-wise, so seed from its bounded client keys;
	// that undercounts only if the old bucket already clipped at TopClients.
	if other.ClientHLLComplete && len(other.ClientHLL) == hllRegisters {
		if len(b.ClientHLL) != hllRegisters {
			b.ClientHLL = newHLL()
			b.ClientHLLComplete = false
		}
		hllMerge(b.ClientHLL, other.ClientHLL)
	} else if len(other.TopClients.Items) > 0 {
		if len(b.ClientHLL) != hllRegisters {
			b.ClientHLL = newHLL()
		}
		for key := range other.TopClients.Items {
			hllAdd(b.ClientHLL, key)
		}
		b.ClientHLLComplete = false
	}
}

func mergeMap(dst, src map[string]uint64) {
	for k, n := range src {
		dst[k] += n
	}
}

func (b *Bucket) compact(cfg Config) {
	b.ensureMaps()
	b.TopDomains.compact(cfg.TopDomains)
	b.TopBlockedDomains.compact(cfg.TopDomains)
	b.TopClients.compact(cfg.TopClients)
}

func (b *Bucket) clone() *Bucket {
	if b == nil {
		return nil
	}
	out := &Bucket{
		Queries:           b.Queries,
		Blocked:           b.Blocked,
		Proxied:           b.Proxied,
		Cached:            b.Cached,
		Failed:            b.Failed,
		LatencySumMs:      b.LatencySumMs,
		LatencyCount:      b.LatencyCount,
		TopDomains:        b.TopDomains.clone(),
		TopClients:        b.TopClients.clone(),
		TopBlockedDomains: b.TopBlockedDomains.clone(),
		ClientHLL:         append([]byte(nil), b.ClientHLL...),
		ClientHLLComplete: b.ClientHLLComplete,
	}
	out.ByAction = cloneMap(b.ByAction)
	out.ByProtocol = cloneMap(b.ByProtocol)
	out.ByUpstream = cloneMap(b.ByUpstream)
	out.ByRelayCountry = cloneMap(b.ByRelayCountry)
	out.ByQType = cloneMap(b.ByQType)
	out.ByRcode = cloneMap(b.ByRcode)
	return out
}

func cloneMap(src map[string]uint64) map[string]uint64 {
	if len(src) == 0 {
		return make(map[string]uint64)
	}
	out := make(map[string]uint64, len(src))
	for k, n := range src {
		out[k] = n
	}
	return out
}

func rawClientKey(ip, account string) string {
	key := strings.TrimSpace(ip)
	if key == "" {
		key = strings.TrimSpace(account)
	}
	if key == "" {
		return "unknown"
	}
	return key
}

func anonymizeClient(key string, salt []byte, anonymize bool) string {
	if !anonymize {
		return key
	}
	mac := hmac.New(sha256.New, salt)
	_, _ = mac.Write([]byte(key))
	return "anon:" + hex.EncodeToString(mac.Sum(nil))[:16]
}

func normalizeKey(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	s = strings.TrimSuffix(s, ".")
	if s == "" {
		return "unknown"
	}
	return s
}

func qtypeName(qtype uint16) string {
	if qtype == 0 {
		return "UNKNOWN"
	}
	if name, ok := dns.TypeToString[qtype]; ok {
		return name
	}
	return fmt.Sprintf("TYPE%d", qtype)
}

func rcodeName(rcode int) string {
	if rcode == 0 {
		return "NOERROR"
	}
	if name, ok := dns.RcodeToString[rcode]; ok {
		return name
	}
	return fmt.Sprintf("RCODE%d", rcode)
}

// failureRcode treats NXDOMAIN as a valid negative answer, not a failure. The
// cards separate "blocked" from "failed" on purpose: a sinkhole is policy, a
// SERVFAIL/REFUSED is a resolver problem.
func failureRcode(rcode int) bool {
	switch rcode {
	case dns.RcodeSuccess, dns.RcodeNameError:
		return false
	default:
		return true
	}
}
