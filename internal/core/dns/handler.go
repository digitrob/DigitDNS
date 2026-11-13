package dns

import (
	"context"
	"fmt"
	"log"
	"math"
	"net"
	"strings"
	"sync/atomic"
	"time"

	"digitdns/internal/core/cache"
	"digitdns/internal/core/dnssec"
	"digitdns/internal/core/matcher"
	"digitdns/internal/core/upstream"
	"digitdns/internal/database"
	"digitdns/internal/netutil"
	"digitdns/internal/safego"
	"github.com/miekg/dns"
)

type AccessProvider interface {
	IsIPAllowed(ip string) (*database.Client, bool)
	IsAllowAll() bool
}

type TelemetrySink interface {
	PushQueryLog(item database.QueryLogItem)
	RecordQuery()
	// SSESubscribers reports how many live query-log streams are open.
	SSESubscribers() int64
}

// QuotaEnforcer reports whether a client has spent its traffic allowance. It is
// optional: the access provider is asked for it by type assertion, so a test
// double that only knows about IP checks keeps working and simply never has a
// quota applied.
type QuotaEnforcer interface {
	QuotaExceeded(c *database.Client) bool
}

// ClientQueryCounter is how the resolver tells the client service that an account
// just had a lookup answered. Optional like QuotaEnforcer, so a test double that
// only knows about IP checks keeps working.
type ClientQueryCounter interface {
	RecordClientQuery(clientID string)
}

// RelayTarget describes where a proxied domain should leave the hub. The zero
// kind is treated as "local", which keeps the hub relay as the default.
//
//   - "local": the hub's own public address is published.
//   - "agent": a public relay agent's address is published and the client
//     connects to that agent.
//   - "egress": the hub's public address is published; only the hub's outbound
//     dial goes through a SOCKS5 or WireGuard egress.
type RelayTarget struct {
	Kind string

	// Agent fields, populated for Kind == "agent".
	IPv4      net.IP
	IPv6      net.IP
	AgentID   string
	AgentName string
	Country   string

	// Egress fields, populated for Kind == "egress".
	EgressID   string
	EgressName string
	EgressKind string
	// FailClosed means the operator forbids a local fallback when the egress
	// is offline. Healthy is the last health probe result.
	FailClosed bool
	Healthy    bool
	// FallbackReason explains a local fallback or a fail-closed refusal.
	FallbackReason string
}

// RelayLookup resolves a proxied name to a relay target. It always returns a
// target; Kind "local" is the explicit no-assignment result.
type RelayLookup func(domain, clientIP string) RelayTarget

type Handler struct {
	access    AccessProvider
	cache     *cache.Cache
	matcher   *matcher.Matcher
	upstreams *upstream.UpstreamPool
	telemetry TelemetrySink
	// queryLog is the optional persistent query history. It is nil unless the
	// operator enabled it; Push never blocks.
	queryLog   func(database.QueryLogItem)
	publicIP   string
	publicIPv6 string
	// Parsed forms of the two addresses above. They are built when the operator
	// sets the address and reused for every self-name and proxied answer, so the
	// query path never re-parses the same literal.
	publicIPv4     net.IP
	publicIPv6Addr net.IP
	// relayLookup resolves a proxied name to a geo agent's public address. It is
	// nil on installs without agents, so the local relay path is unchanged.
	relayLookup RelayLookup
	// selfNames holds this server's own service hostnames — the panel domain,
	// the subscriber-portal domain, and the DoH/DoT host. Queries for these are
	// answered with the server's public address ahead of the access whitelist
	// (v2.6 self-service reachability), so a subscriber whose IP changed and has
	// fallen off the whitelist can still resolve the portal link to re-register.
	// Stored atomically: replaced when the operator changes a domain, read on the
	// query hot path.
	selfNames    atomic.Pointer[map[string]struct{}]
	totalQueries atomic.Uint64
	// engineDisabled bypasses every DNS policy rule when true. It is read on
	// the hot path and flipped live by the dashboard/Android engine switch.
	engineDisabled atomic.Bool

	quota        QuotaEnforcer
	queryCounter ClientQueryCounter

	// limiter is replaced only during startup, before any listener is accepting,
	// so the query path reads it without synchronisation.
	limiter *rateLimiter
	limited atomic.Uint64

	// proxyLeakProtection withholds HTTPS/SVCB answers for proxied names. It is
	// atomic because the dashboard applies the setting live.
	proxyLeakProtection atomic.Bool

	// proxyIPv6Enabled chooses whether proxied names may answer AAAA with the
	// hub's public IPv6. While false, AAAA stays sunk and the HTTPS/SVCB guard is
	// forced on even when proxyLeakProtection is off, so no IPv6 hint can leak.
	proxyIPv6Enabled atomic.Bool

	// blockPolicy supplies the operator's chosen block response. It is read only
	// for names that already matched a block rule, so a normal query never pays
	// for it. Nil keeps the historical null-IP answer.
	blockPolicy func() (mode string, ttl uint32)

	// validator is installed by EnableDNSSEC and may be replaced live by the
	// dashboard's DNSSEC card. A pointer keeps the query path lock-free while an
	// operator switches it on or off. While it is nil the
	// resolver behaves exactly as before: upstream AD is passed through and no
	// chain is walked.
	validator atomic.Pointer[validatorSlot]
}

func NewHandler(
	access AccessProvider,
	c *cache.Cache,
	m *matcher.Matcher,
	u *upstream.UpstreamPool,
	t TelemetrySink,
	publicIP string,
) *Handler {
	h := &Handler{
		access:    access,
		cache:     c,
		matcher:   m,
		upstreams: u,
		telemetry: t,
		limiter:   newRateLimiter(defaultRateLimitQPS),
	}
	h.SetPublicIP(publicIP)
	// Recommended default: proxied names must not advertise QUIC or origin
	// address hints. main and the settings record can turn it off explicitly.
	h.proxyLeakProtection.Store(true)
	// IPv6 is opt-in: an upgraded install keeps forcing proxied clients onto the
	// IPv4 relay until an operator supplies and enables a public IPv6.
	h.proxyIPv6Enabled.Store(false)
	if q, ok := access.(QuotaEnforcer); ok {
		h.quota = q
	}
	if q, ok := access.(ClientQueryCounter); ok {
		h.queryCounter = q
	}
	// Let the cache renew its own entries through the pool. A name anyone is
	// actually using is then refreshed in the background before it expires,
	// instead of costing whoever asks next a full upstream round trip.
	if c != nil && u != nil {
		c.SetRefresherWithOptions(h.refreshUpstream)
	}
	return h
}

// SetProxyLeakProtection chooses whether proxied names withhold HTTPS/SVCB
// answers. It is safe to call while serving: the query path reads the value
// atomically, and the dashboard applies the saved policy live.
func (h *Handler) SetProxyLeakProtection(enabled bool) {
	h.proxyLeakProtection.Store(enabled)
}

// SetBlockPolicy wires the filter service's response mode into the block path.
func (h *Handler) SetBlockPolicy(fn func() (mode string, ttl uint32)) {
	h.blockPolicy = fn
}

// refreshUpstream re-resolves one question for the cache's background refresh.
//
// It builds a fresh query rather than replaying a client's: the stored question
// and its EDNS/DO/CD policy are all the cache knows, and a refresh must not carry
// some earlier client's transaction ID or unrelated EDNS0 options into the pool.
// DO and CD are the exception: a DO=1 entry must be refreshed with DO=1 or the
// replacement would lose the signatures the entry promised, and a CD=1 entry
// must stay checking-disabled. The pool attaches its own ECS option from the
// configured public address, so a refresh is scoped exactly as a client-driven
// query is.
func (h *Handler) refreshUpstream(q dns.Question, opts cache.QueryOptions) (*dns.Msg, error) {
	m := new(dns.Msg)
	m.SetQuestion(q.Name, q.Qtype)
	m.Question[0].Qclass = q.Qclass
	m.RecursionDesired = true
	m.CheckingDisabled = opts.CD

	validator := h.currentValidator()
	validating := validator != nil && !opts.CD
	if opts.EDNS || validating {
		m.SetEdns0(dns.DefaultMsgSize, opts.DO || validating)
	}

	domain := strings.TrimSuffix(strings.ToLower(dns.Fqdn(q.Name)), ".")
	resp, _, _, err := h.upstreams.ExchangeFor(m, domain)
	if err != nil || resp == nil {
		return resp, err
	}
	if validator == nil {
		return resp, nil
	}

	resp.AuthenticatedData = false
	if !validating {
		return resp, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), dnssecValidationTimeout)
	defer cancel()
	result := validator.Validate(ctx, q, resp)
	switch result.Status {
	case dnssec.Secure:
		resp.AuthenticatedData = true
	case dnssec.Bogus:
		// A refresh that could not validate must not replace a good entry with
		// unauthenticated data. The stale answer keeps being served until the
		// zone recovers or the grace window closes.
		log.Printf("[DNSSEC] Bogus refresh for %s/%s: %s", q.Name, dns.TypeToString[q.Qtype], result.Reason)
		return nil, fmt.Errorf("DNSSEC bogus: %s", result.Reason)
	}
	return resp, nil
}

func cacheOptions(r *dns.Msg) cache.QueryOptions {
	opt := r.IsEdns0()
	return cache.QueryOptions{
		EDNS: opt != nil,
		DO:   opt != nil && opt.Do(),
		CD:   r.CheckingDisabled,
	}
}

// SetRateLimit replaces the per-source query limit. A value of zero or less turns
// limiting off entirely. Call it during startup only.
func (h *Handler) SetRateLimit(qps int) {
	h.limiter = newRateLimiter(qps)
}

// RateLimited is the number of queries dropped or refused for exceeding a
// source's query rate since start.
func (h *Handler) RateLimited() uint64 {
	return h.limited.Load()
}

// RateLimitQPS is the per-source limit in effect, or 0 when limiting is off.
// The dropped count alone is not interpretable — "1,204 dropped" reads as an
// attack to an operator who set the limit to 20 by hand and as a
// misconfiguration to one who left it at the default — so the dashboard is given
// both numbers.
func (h *Handler) RateLimitQPS() int {
	if h.limiter == nil {
		return 0
	}
	return int(h.limiter.qps)
}

// SetPublicIP installs the address proxied A answers point at. Only a usable
// IPv4 is accepted (v2.1.0 B-13 remediation): an IPv6 literal or a typo made
// dns.NewRR fail per query, and the discarded error turned every proxied A
// answer into an empty NOERROR logged as a successful "PROXY" — clients failed
// silently and the log said everything was fine. The daemon cannot run without
// an operator fixing the value, but it can refuse to pretend.
func (h *Handler) SetPublicIP(ip string) {
	if ip != "" {
		if parsed := net.ParseIP(strings.TrimSpace(ip)); parsed == nil || parsed.To4() == nil {
			log.Printf("[DNS] Warning: public IP %q is not a usable IPv4 address; proxied A records will answer SERVFAIL until it is fixed in Settings or -public-ip", ip)
			h.publicIP = ""
			h.publicIPv4 = nil
			return
		}
	}
	h.publicIP = strings.TrimSpace(ip)
	h.publicIPv4 = net.ParseIP(h.publicIP).To4()
}

// SetPublicIPv6 installs the address proxied AAAA answers point at. Only a
// globally routable IPv6 literal is accepted; anything else clears it, and the
// resolver keeps sinking AAAA on proxied names so an IPv4 client is forced onto
// the A record. Set it only when the server actually has a reachable IPv6 on
// which the SNI proxy listens, or IPv6 clients would be handed an address that
// answers nothing. The address can be stored while ProxyIPv6Enabled is false;
// SetProxyIPv6Enabled controls whether it is actually published.
func (h *Handler) SetPublicIPv6(ip string) {
	ip = strings.TrimSpace(ip)
	if ip == "" {
		h.publicIPv6 = ""
		h.publicIPv6Addr = nil
		return
	}
	parsed, ok := netutil.ParsePublicIPv6(ip)
	if !ok {
		log.Printf("[DNS] Warning: public IPv6 %q is not a usable global IPv6 address; proxied AAAA stays sunk (IPv4 forced)", ip)
		h.publicIPv6 = ""
		h.publicIPv6Addr = nil
		return
	}
	h.publicIPv6 = parsed.String()
	h.publicIPv6Addr = parsed
}

// SetProxyIPv6Enabled chooses whether proxied names may publish the server's
// public IPv6. It is safe to call while serving: the query path reads the value
// atomically, and the dashboard applies the setting live. When false, AAAA is
// sunk and the HTTPS/SVCB sink is forced regardless of ProxyLeakProtection.
func (h *Handler) SetProxyIPv6Enabled(enabled bool) {
	h.proxyIPv6Enabled.Store(enabled)
}

// SetRelayLookup installs the geo-agent resolver. Call during startup only; the
// lookup itself is safe for concurrent use.
// SetQueryLogSink installs the optional persistent query history.
func (h *Handler) SetQueryLogSink(fn func(database.QueryLogItem)) {
	h.queryLog = fn
}

func (h *Handler) SetRelayLookup(fn RelayLookup) {
	h.relayLookup = fn
}

// SetEngineEnabled turns the DNS policy engine on or off. Off means every
// query is resolved directly: custom records, block/proxy/direct rules,
// service and agent assignments and ad blocking are all skipped. Access
// control and quota remain in force because they protect the resolver itself.
func (h *Handler) SetEngineEnabled(enabled bool) {
	h.engineDisabled.Store(!enabled)
}

// EngineEnabled reports the live engine state.
func (h *Handler) EngineEnabled() bool {
	return !h.engineDisabled.Load()
}

// SetSelfDomains installs this server's own service hostnames (panel domain,
// subscriber-portal domain, DoH/DoT host). A query for one of these is answered
// with the server's public address ahead of the access whitelist, so a
// subscriber whose source IP changed can still resolve the portal link they need
// to re-register (v2.6). Blank entries are dropped and names are lower-cased and
// stripped of a trailing dot so the hot-path lookup is a plain map hit.
func (h *Handler) SetSelfDomains(domains []string) {
	set := make(map[string]struct{}, len(domains))
	for _, d := range domains {
		d = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(d)), ".")
		if d != "" {
			set[d] = struct{}{}
		}
	}
	h.selfNames.Store(&set)
}

// isSelfName reports whether domain (lower-case, no trailing dot) is one of this
// server's own service hostnames.
func (h *Handler) isSelfName(domain string) bool {
	set := h.selfNames.Load()
	if set == nil {
		return false
	}
	_, ok := (*set)[domain]
	return ok
}

// TotalQueries is the number of questions this handler has processed since start,
// across every transport (UDP, TCP, DoT, DoH).
func (h *Handler) TotalQueries() uint64 {
	return h.totalQueries.Load()
}

// Proxy answer TTLs. The hub's own egress is stable, so it can be cached for a
// minute. A geo agent is a remote box that can disappear: a shorter TTL lets a
// client move to a healthy egress without waiting out the two-minute liveness
// window plus a full minute of resolver cache.
const (
	localProxyAnswerTTL uint32 = 60
	agentProxyAnswerTTL uint32 = 30
)

func (h *Handler) ServeDNS(w dns.ResponseWriter, r *dns.Msg) {
	h.serve(w, r, "")
}

// ServeWithProto returns a handler that tags every query with an explicit
// protocol label. The DoT listener runs over plain TCP as far as the
// ResponseWriter is concerned, so without this its traffic is indistinguishable
// from port-53 TCP in the query log.
func (h *Handler) ServeWithProto(proto string) dns.Handler {
	return dns.HandlerFunc(func(w dns.ResponseWriter, r *dns.Msg) {
		h.serve(w, r, proto)
	})
}

func (h *Handler) serve(w dns.ResponseWriter, r *dns.Msg, proto string) {
	// One malformed message must cost one SERVFAIL, not the whole resolver.
	// net/http recovers per request; the DNS server does not, and a panic in
	// this goroutine terminates the process.
	defer func() {
		if rec := recover(); rec != nil {
			safego.Handle("dns.ServeDNS", rec)
			if w == nil || r == nil {
				return
			}
			// Best effort: a writer that panics on the failure reply must not
			// turn the containment into a second panic.
			func() {
				defer func() { _ = recover() }()
				fail := new(dns.Msg)
				fail.SetRcode(r, dns.RcodeServerFailure)
				_ = w.WriteMsg(fail)
			}()
		}
	}()
	clientIP, _, _ := net.SplitHostPort(w.RemoteAddr().String())
	if clientIP == "" {
		clientIP = w.RemoteAddr().String()
	}
	if proto == "" {
		proto = w.RemoteAddr().Network()
	}
	proto = normalizeProto(proto)

	resp := h.ProcessQuery(r, clientIP, proto, w.RemoteAddr().String())
	if resp == nil {
		return
	}
	attachTCPKeepalive(resp, r, proto)
	// Datagram replies must fit what the client said it can receive, otherwise
	// the write fails outright or the answer is fragmented and dropped.
	if proto == "UDP" {
		resp.Truncate(udpSize(r))
	}
	_ = w.WriteMsg(resp)
}

// attachTCPKeepalive answers RFC 7828's edns-tcp-keepalive option, which is how a
// server tells a connection-oriented client how long it may keep the connection
// idle. Without the option the client can only guess, and guessing badly is
// expensive in both directions: guess too long and it writes a query into a socket
// the server has already closed, then waits out a timeout before retrying; guess
// too short and it throws away a connection — and for DoT a whole TLS handshake —
// that would have been reused.
//
// Sent only when the query asked for it, and never over UDP: RFC 7828 §3.3
// forbids it there, where there is no connection to hold open and the option would
// be extra bytes in a reply that can be aimed at a spoofed source.
func attachTCPKeepalive(resp, req *dns.Msg, proto string) {
	if resp == nil || req == nil {
		return
	}
	var timeout time.Duration
	switch proto {
	case "TCP":
		timeout = tcpIdleTimeout
	case "DoT":
		timeout = dotIdleTimeout
	default:
		return
	}

	reqOpt := req.IsEdns0()
	if reqOpt == nil || findOption(reqOpt, dns.EDNS0TCPKEEPALIVE) == nil {
		return
	}

	opt := resp.IsEdns0()
	if opt == nil {
		// A reply may only carry an OPT record when the query did (RFC 6891 §6.1.1),
		// and this one did — that is where the option came from. An answer served
		// from cache can still arrive without one.
		resp.SetEdns0(dns.DefaultMsgSize, reqOpt.Do())
		if opt = resp.IsEdns0(); opt == nil {
			return
		}
	}

	ka := &dns.EDNS0_TCP_KEEPALIVE{Code: dns.EDNS0TCPKEEPALIVE, Timeout: keepaliveUnits(timeout)}
	for i, o := range opt.Option {
		if o.Option() == dns.EDNS0TCPKEEPALIVE {
			opt.Option[i] = ka
			return
		}
	}
	opt.Option = append(opt.Option, ka)
}

// attachProhibitedEDE adds the RFC 8914 "Prohibited" Extended DNS Error to an
// access-refusal reply, but only when the query itself carried an OPT record —
// RFC 6891 §6.1.1 forbids a reply from introducing OPT the client never offered,
// and a plain client sees nothing but the REFUSED it always saw. The option
// merge mirrors attachTCPKeepalive: a reply that already carries OPT (the
// keepalive path above may have run first on connection transports) gets the
// option merged in rather than a second OPT appended.
func attachProhibitedEDE(resp, req *dns.Msg) {
	if resp == nil || req == nil {
		return
	}
	reqOpt := req.IsEdns0()
	if reqOpt == nil {
		return
	}
	opt := resp.IsEdns0()
	if opt == nil {
		resp.SetEdns0(dns.DefaultMsgSize, reqOpt.Do())
		if opt = resp.IsEdns0(); opt == nil {
			return
		}
	}
	for _, o := range opt.Option {
		if o.Option() == dns.EDNS0EDE {
			return // one EDE per reply; the first cause stands
		}
	}
	opt.Option = append(opt.Option, &dns.EDNS0_EDE{
		InfoCode:  dns.ExtendedErrorCodeProhibited,
		ExtraText: "This address is not registered with this DigitDNS server.",
	})
}

func findOption(opt *dns.OPT, code uint16) dns.EDNS0 {
	for _, o := range opt.Option {
		if o.Option() == code {
			return o
		}
	}
	return nil
}

// keepaliveUnits converts an idle timeout to the unit RFC 7828 carries it in,
// hundreds of milliseconds. Never zero: a server sending zero is telling the
// client to close the connection at once, which is the opposite of the message.
func keepaliveUnits(d time.Duration) uint16 {
	units := d / (100 * time.Millisecond)
	if units < 1 {
		return 1
	}
	return uint16(min(units, math.MaxUint16))
}

// makeARecord builds an A answer without going through the textual RR parser.
// The numeric paths (proxy, blocked, custom, self-name) used to format and parse
// a DNS record per query; the typed struct is the same wire answer with no
// allocation for the presentation form and no parser on the hot path.
func makeARecord(name string, ttl uint32, ip net.IP) *dns.A {
	v4 := ip.To4()
	if v4 == nil {
		return nil
	}
	return &dns.A{
		Hdr: dns.RR_Header{Name: name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: ttl},
		A:   v4,
	}
}

// makeAAAARecord is the AAAA twin. A v4-mapped address is refused rather than
// widened: the caller has already decided this is the AAAA path, and answering
// it with an IPv4 address would be the wrong record.
func makeAAAARecord(name string, ttl uint32, ip net.IP) *dns.AAAA {
	if ip.To4() != nil {
		return nil
	}
	v6 := ip.To16()
	if v6 == nil {
		return nil
	}
	return &dns.AAAA{
		Hdr:  dns.RR_Header{Name: name, Rrtype: dns.TypeAAAA, Class: dns.ClassINET, Ttl: ttl},
		AAAA: v6,
	}
}

func makeCNAMERecord(name string, ttl uint32, target string) *dns.CNAME {
	target = strings.TrimSpace(target)
	if target == "" {
		return nil
	}
	target = dns.Fqdn(target)
	if target == "." {
		return nil
	}
	if _, ok := dns.IsDomainName(target); !ok {
		return nil
	}
	return &dns.CNAME{
		Hdr:    dns.RR_Header{Name: name, Rrtype: dns.TypeCNAME, Class: dns.ClassINET, Ttl: ttl},
		Target: target,
	}
}

func makeTXTRecord(name string, ttl uint32, text string) *dns.TXT {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	return &dns.TXT{
		Hdr: dns.RR_Header{Name: name, Rrtype: dns.TypeTXT, Class: dns.ClassINET, Ttl: ttl},
		Txt: []string{text},
	}
}

// udpSize is the largest UDP payload this client will accept: its EDNS0
// advertised buffer, or the RFC 1035 512-byte floor when the query carries no
// OPT record. Clamped to 4096 so an absurd advertisement cannot be used to
// generate fragmented traffic.
func udpSize(r *dns.Msg) int {
	size := dns.MinMsgSize
	if opt := r.IsEdns0(); opt != nil {
		if adv := int(opt.UDPSize()); adv > size {
			size = adv
		}
	}
	return min(size, 4096)
}

// questionName is the queried name for logging, without its trailing dot. A
// message with no question at all still has to be loggable: the rate limiter runs
// before the shape of the query is validated.
func questionName(r *dns.Msg) string {
	if len(r.Question) == 0 {
		return ""
	}
	return strings.TrimSuffix(strings.ToLower(r.Question[0].Name), ".")
}

func firstQuestionType(r *dns.Msg) uint16 {
	if r == nil || len(r.Question) == 0 {
		return 0
	}
	return r.Question[0].Qtype
}

func normalizeProto(p string) string {
	p = strings.TrimSpace(p)
	switch {
	case strings.EqualFold(p, "udp"), strings.EqualFold(p, "udp4"), strings.EqualFold(p, "udp6"):
		return "UDP"
	case strings.EqualFold(p, "tcp"), strings.EqualFold(p, "tcp4"), strings.EqualFold(p, "tcp6"):
		return "TCP"
	case strings.EqualFold(p, "dot"), strings.EqualFold(p, "tcp-tls"):
		return "DoT"
	case strings.EqualFold(p, "doh"), strings.EqualFold(p, "https"):
		return "DoH"
	}
	return p
}

func (h *Handler) ProcessQuery(r *dns.Msg, clientIP string, protocol ...string) *dns.Msg {
	start := time.Now()
	h.totalQueries.Add(1)
	if h.telemetry != nil {
		h.telemetry.RecordQuery()
	}

	proto := "UDP"
	if len(protocol) > 0 && protocol[0] != "" {
		proto = normalizeProto(protocol[0])
	}

	// peerIP is the immediate transport peer, NOT the header-resolved client
	// address. The rate limiter's loopback exemption keys on it (v2.1.0 fix for
	// the finding that a spoofable clientIP could claim the exemption), while
	// the bucket itself still keys on the effective client address.
	peerIP := clientIP
	if len(protocol) > 1 && protocol[1] != "" {
		peerIP = protocol[1]
	}

	// Rate limit before any parsing or lookup, so a flood costs one hash and one
	// mutex. A UDP query over the limit is dropped without a reply: the source
	// address of a flood is typically forged, and a "you are being limited"
	// answer is still a packet aimed at whoever the flood names. Connection-
	// oriented transports have a peer that completed a handshake, and the DoH
	// bridge turns a nil response into HTTP 500, so those are refused explicitly.
	if !h.limiter.allow(clientIP, peerIP) {
		n := h.limited.Add(1)
		// One log line per drop would give a flood a query-log write and a
		// fan-out to every SSE dashboard subscriber for each packet it sends —
		// the amplification this check exists to remove. Sampling keeps the
		// offending source visible at bounded cost.
		if n%rateLimitLogSample == 1 {
			h.logQuery(start, clientIP, "Public", proto, questionName(r), "Rate limit", "RATELIMIT", false, "", "", firstQuestionType(r), dns.RcodeRefused)
		}
		if proto == "UDP" {
			return nil
		}
		m := new(dns.Msg)
		m.SetRcode(r, dns.RcodeRefused)
		return m
	}

	// Exactly one question is the only shape ever deployed; anything else is
	// malformed and is a favourite probe for amplification reflectors.
	if len(r.Question) != 1 {
		m := new(dns.Msg)
		m.SetRcode(r, dns.RcodeFormatError)
		return m
	}
	if r.Opcode != dns.OpcodeQuery {
		m := new(dns.Msg)
		m.SetRcode(r, dns.RcodeNotImplemented)
		return m
	}

	q := r.Question[0]
	qName := strings.ToLower(q.Name)
	domain := strings.TrimSuffix(qName, ".")

	// Self-service reachability (v2.6): answer this server's OWN names — panel,
	// subscriber portal, DoH/DoT host — with its public address BEFORE the access
	// gate. A subscriber whose IP changed has fallen off the whitelist, so every
	// name they ask is REFUSED, including the portal link they need to re-register.
	// Answering only the server's own names, only A/AAAA, only its own address,
	// still behind the rate limiter, keeps that one door open without turning the
	// resolver into an open one.
	if (q.Qtype == dns.TypeA || q.Qtype == dns.TypeAAAA) && h.isSelfName(domain) {
		m := new(dns.Msg)
		m.SetReply(r)
		if q.Qtype == dns.TypeA && h.publicIPv4 != nil {
			if rr := makeARecord(q.Name, 60, h.publicIPv4); rr != nil {
				m.Answer = append(m.Answer, rr)
			}
		} else if q.Qtype == dns.TypeAAAA && h.publicIPv6Addr != nil && h.proxyIPv6Enabled.Load() {
			if rr := makeAAAARecord(q.Name, 60, h.publicIPv6Addr); rr != nil {
				m.Answer = append(m.Answer, rr)
			}
		}
		// The other address family (or a missing public address) is a deliberate
		// empty NOERROR: the name exists here, that family just has no address.
		h.logQuery(start, clientIP, "Self", proto, domain, "Self-service", "SELF", false, "", "", q.Qtype, m.Rcode)
		return m
	}

	// 1. Validate Access Control
	var accountName = "Public"
	var activeClient *database.Client
	if h.access != nil {
		client, allowed := h.access.IsIPAllowed(clientIP)
		if allowed && client != nil {
			activeClient = client
			accountName = client.Name
			// Counted before quota and rule handling: the account asked, and a
			// blocked or quota-refused lookup is still a query it made.
			if h.queryCounter != nil {
				h.queryCounter.RecordClientQuery(client.ID)
			}
		} else if !h.access.IsAllowAll() {
			m := new(dns.Msg)
			m.SetRcode(r, dns.RcodeRefused)
			// RFC 8914 Extended DNS Error, so a client that sent OPT learns WHY the
			// answer is REFUSED instead of retrying or blaming its resolver: the
			// address is not registered with this server. Sent only to queries that
			// carried an OPT record (a reply may never introduce one the query did
			// not), and only here — every other REFUSED below has a different cause
			// a "Prohibited" label would misstate.
			attachProhibitedEDE(m, r)
			return m
		}
	}

	// An account that has spent its traffic allowance is refused, not disabled:
	// disabling would need an operator to re-enable it by hand after a top-up,
	// while a refusal clears itself the moment the limit is raised or the counter
	// is reset. An unidentified "Public" source has no allowance to exceed, so
	// this only ever applies to a client the access provider recognised.
	if activeClient != nil && h.quota != nil && h.quota.QuotaExceeded(activeClient) {
		m := new(dns.Msg)
		m.SetRcode(r, dns.RcodeRefused)
		h.logQuery(start, clientIP, accountName, proto, q.Name, "Traffic quota exceeded", "QUOTA", false, "", "", q.Qtype, dns.RcodeRefused)
		return m
	}

	// 2. RFC 8482: never serve ANY. Dumping every RRset for a name is the
	// classic DNS amplification primitive, so refuse before any lookup.
	if q.Qtype == dns.TypeANY {
		m := new(dns.Msg)
		m.SetRcode(r, dns.RcodeRefused)
		h.logQuery(start, clientIP, accountName, proto, domain, "ANY refused", "BLOCK", false, "", "", q.Qtype, dns.RcodeRefused)
		return m
	}

	engineOff := h.engineDisabled.Load()

	// 3. Custom record overrides outrank every other rule. Both address
	if !engineOff {
		if customVal, ok := h.matcher.GetCustomRecord(domain); ok {
			customVal = strings.TrimSpace(customVal)
			ip := net.ParseIP(customVal)
			if ip != nil && (q.Qtype == dns.TypeA || q.Qtype == dns.TypeAAAA) {
				m := new(dns.Msg)
				m.SetReply(r)
				isV4 := ip.To4() != nil
				if isV4 && q.Qtype == dns.TypeA {
					if rr := makeARecord(q.Name, 60, ip); rr != nil {
						m.Answer = append(m.Answer, rr)
					}
				} else if !isV4 && q.Qtype == dns.TypeAAAA {
					if rr := makeAAAARecord(q.Name, 60, ip); rr != nil {
						m.Answer = append(m.Answer, rr)
					}
				}
				// The other family deliberately gets an empty NOERROR.
				h.logQuery(start, clientIP, accountName, proto, domain, "Custom Record", "CUSTOM", false, "", "", q.Qtype, m.Rcode)
				return m
			}
			// An IP rewrite queried for a non-matching type (CNAME, TXT, MX, ...)
			// returns an empty NOERROR rather than falling through to upstream.
			if ip != nil {
				m := new(dns.Msg)
				m.SetReply(r)
				h.logQuery(start, clientIP, accountName, proto, domain, "Custom Record", "CUSTOM", false, "", "", q.Qtype, m.Rcode)
				return m
			}
			// Special response codes (AdGuard $dnsrewrite=NXDOMAIN/REFUSED/SERVFAIL).
			upper := strings.ToUpper(customVal)
			switch upper {
			case "NXDOMAIN":
				m := new(dns.Msg)
				m.SetRcode(r, dns.RcodeNameError)
				h.logQuery(start, clientIP, accountName, proto, domain, "Custom Record", "CUSTOM", false, "", "", q.Qtype, m.Rcode)
				return m
			case "REFUSED":
				m := new(dns.Msg)
				m.SetRcode(r, dns.RcodeRefused)
				h.logQuery(start, clientIP, accountName, proto, domain, "Custom Record", "CUSTOM", false, "", "", q.Qtype, m.Rcode)
				return m
			case "SERVFAIL":
				m := new(dns.Msg)
				m.SetRcode(r, dns.RcodeServerFailure)
				h.logQuery(start, clientIP, accountName, proto, domain, "Custom Record", "CUSTOM", false, "", "", q.Qtype, m.Rcode)
				return m
			}
			// TXT rewrite: $dnsrewrite=TXT "text".
			if strings.HasPrefix(upper, "TXT ") {
				text := strings.Trim(strings.TrimSpace(customVal[4:]), "\"")
				if q.Qtype == dns.TypeTXT || q.Qtype == dns.TypeA || q.Qtype == dns.TypeAAAA {
					m := new(dns.Msg)
					m.SetReply(r)
					if rr := makeTXTRecord(q.Name, 60, text); rr != nil {
						m.Answer = append(m.Answer, rr)
					}
					h.logQuery(start, clientIP, accountName, proto, domain, "Custom Record", "CUSTOM", false, "", "", q.Qtype, m.Rcode)
					return m
				}
			}
			// A non-IP value is treated as a CNAME target ($dnsrewrite=host).
			if ip == nil && (q.Qtype == dns.TypeA || q.Qtype == dns.TypeAAAA || q.Qtype == dns.TypeCNAME) {
				m := new(dns.Msg)
				m.SetReply(r)
				if rr := makeCNAMERecord(q.Name, 60, customVal); rr != nil {
					m.Answer = append(m.Answer, rr)
				}
				h.logQuery(start, clientIP, accountName, proto, domain, "Custom Record", "CUSTOM", false, "", "", q.Qtype, m.Rcode)
				return m
			}
		}
	}

	// 4. Rule matcher. Every client is matched against the same admin-wide
	// rules: the per-client policy lists a stored account may still carry are
	// inert in this build.
	action, ruleName, matchedPattern := matcher.ActionDirect, "Engine off", ""
	if !engineOff {
		action, ruleName, matchedPattern = h.matcher.MatchDetailLower(domain)
	}

	switch action {
	case matcher.ActionBlock:
		m := new(dns.Msg)
		m.SetReply(r)
		mode, ttl := "null_ip", uint32(10)
		if h.blockPolicy != nil {
			mode, ttl = h.blockPolicy()
		}
		switch mode {
		case "nxdomain":
			m.Rcode = dns.RcodeNameError
		case "refused":
			m.Rcode = dns.RcodeRefused
		default:
			// Keep the historical null-IP shape: A gets 0.0.0.0, AAAA stays an
			// empty NOERROR. Existing clients and the guard tests rely on it.
			if q.Qtype == dns.TypeA {
				m.Answer = append(m.Answer, makeARecord(q.Name, ttl, net.IPv4zero))
			}
		}
		h.logQuery(start, clientIP, accountName, proto, domain, ruleName, "BLOCK", false, "", matchedPattern, q.Qtype, m.Rcode)
		return m

	case matcher.ActionProxy:
		// A domain assigned to a relay agent exits from that agent's public
		// address. The local relay is the fallback when no agent is assigned or
		// the assigned one is offline, so an agent outage cannot silently leak the
		// origin's real address. Egress targets are different: the client still
		// reaches the hub, so the hub's own address is published and only the
		// outbound dial changes. A fail-closed egress refuses to fall back.
		target := RelayTarget{Kind: "local", IPv4: h.publicIPv4, IPv6: h.publicIPv6Addr}
		if h.relayLookup != nil {
			target = h.relayLookup(domain, clientIP)
			if target.Kind == "" {
				target.Kind = "local"
			}
		}
		targetIPv4, targetIPv6 := h.publicIPv4, h.publicIPv6Addr
		if target.Kind == "agent" {
			targetIPv4, targetIPv6 = target.IPv4, target.IPv6
		}
		// The IPv6 switch is a hard gate: with it off the address is ignored even
		// though it may still be stored, so a later enable does not need the
		// operator to re-enter it. The same gate is applied after the relay
		// lookup, because an agent may publish an IPv6 of its own.
		ipv6Allowed := h.proxyIPv6Enabled.Load()
		if !ipv6Allowed {
			targetIPv6 = nil
		}
		// Fail closed before answering anything, including the AAAA and HTTPS
		// sinks: an offline egress must produce an error, never a hub address that
		// would let the client bypass the operator's routing policy.
		if target.Kind == "egress" && target.FailClosed && !target.Healthy {
			m := new(dns.Msg)
			m.SetReply(r)
			h.logQueryRelay(start, clientIP, accountName, proto, domain, ruleName, "PROXY_FAILED", false, "", matchedPattern, target, q.Qtype, dns.RcodeServerFailure)
			m.SetRcode(r, dns.RcodeServerFailure)
			return m
		}

		if q.Qtype == dns.TypeA {
			if len(targetIPv4) == 0 {
				m := new(dns.Msg)
				m.SetReply(r)
				// An agent with no IPv4 must not fall through to the cache or an
				// upstream answer: that would expose the real host address.
				h.logQueryRelay(start, clientIP, accountName, proto, domain, ruleName, "PROXY_FAILED", false, "", matchedPattern, target, q.Qtype, dns.RcodeServerFailure)
				m.SetRcode(r, dns.RcodeServerFailure)
				return m
			}
			m := new(dns.Msg)
			m.SetReply(r)
			ttl := localProxyAnswerTTL
			if target.Kind == "agent" {
				ttl = agentProxyAnswerTTL
			}
			// The relay router publishes already-parsed addresses, so the query
			// path neither re-parses the same literal nor allocates for it.
			rr := makeARecord(q.Name, ttl, targetIPv4)
			if rr == nil {
				// Unreachable while SetPublicIP validates and the relay router
				// only publishes parsed addresses, but a distinct action here
				// means a future regression can never masquerade as a successful
				// proxy answer again (v2.1.0 B-13 remediation).
				log.Printf("[DNS] Could not build proxied A record for %s from %s", q.Name, targetIPv4)
				h.logQueryRelay(start, clientIP, accountName, proto, domain, ruleName, "PROXY_FAILED", false, "", matchedPattern, target, q.Qtype, dns.RcodeServerFailure)
				m.SetRcode(r, dns.RcodeServerFailure)
				return m
			}
			m.Answer = append(m.Answer, rr)
			h.logQueryRelay(start, clientIP, accountName, proto, domain, ruleName, "PROXY", false, "", matchedPattern, target, q.Qtype, m.Rcode)
			return m
		}
		if q.Qtype == dns.TypeAAAA {
			// A proxied name's AAAA: if the egress has a reachable IPv6, answer it
			// so an IPv6 client reaches the SNI proxy over v6. Otherwise keep the
			// leak guard — an empty NOERROR — so an IPv4 client is forced onto the
			// proxied A record instead of escaping to the real host over v6.
			m := new(dns.Msg)
			m.SetReply(r)
			if len(targetIPv6) != 0 {
				ttl := localProxyAnswerTTL
				if target.Kind == "agent" {
					ttl = agentProxyAnswerTTL
				}
				if rr := makeAAAARecord(q.Name, ttl, targetIPv6); rr != nil {
					m.Answer = append(m.Answer, rr)
					h.logQueryRelay(start, clientIP, accountName, proto, domain, ruleName, "PROXY", false, "", matchedPattern, target, q.Qtype, m.Rcode)
					return m
				}
			}
			if target.Kind == "agent" || target.Kind == "egress" || target.FallbackReason != "" {
				h.logQueryRelay(start, clientIP, accountName, proto, domain, ruleName, "PROXY_IPV6_SINK", false, "", matchedPattern, target, q.Qtype, m.Rcode)
			} else {
				h.logQuery(start, clientIP, accountName, proto, domain, ruleName, "PROXY_IPV6_SINK", false, "", matchedPattern, q.Qtype, m.Rcode)
			}
			return m
		}
		// With proxy IPv6 off the HTTPS/SVCB sink is unconditional: an upstream
		// record would otherwise carry ipv6hint (and h3) and reopen the IPv6
		// bypass even if the operator had disabled the proxy leak guard. When
		// IPv6 is on, the guard remains the operator's explicit policy.
		if (q.Qtype == dns.TypeHTTPS || q.Qtype == dns.TypeSVCB) &&
			(h.proxyLeakProtection.Load() || !h.proxyIPv6Enabled.Load()) {
			// HTTPS/SVCB can carry two things that bypass an SNI relay:
			// an h3 ALPN entry (QUIC/UDP 443) and ipv4hint/ipv6hint with the
			// origin's real addresses. Letting the upstream answer through would
			// leak both, even when A/AAAA are proxied. An empty NOERROR makes the
			// client fall back to the proxied A/AAAA answer and TCP TLS.
			m := new(dns.Msg)
			m.SetReply(r)
			m.AuthenticatedData = false
			if target.Kind == "agent" || target.Kind == "egress" || target.FallbackReason != "" {
				h.logQueryRelay(start, clientIP, accountName, proto, domain, ruleName, "PROXY_HTTPS_SINK", false, "", matchedPattern, target, q.Qtype, m.Rcode)
			} else {
				h.logQuery(start, clientIP, accountName, proto, domain, ruleName, "PROXY_HTTPS_SINK", false, "", matchedPattern, q.Qtype, m.Rcode)
			}
			return m
		}
	}

	// 5. Cache, consulted only on the DIRECT path. When the lookup ran before
	// the matcher, a client whose policy resolved to DIRECT populated an entry
	// that was then served to clients whose policy said PROXY or BLOCK,
	// silently bypassing their rules.
	//
	// Lookup, not Get: an entry that has just expired is still served, with a short
	// TTL, while it is refreshed behind the scenes, and an entry approaching expiry
	// is refreshed early. Either way nobody waits on an upstream for a name the
	// resolver already knows.
	cacheOpts := cacheOptions(r)
	if h.cache != nil {
		if cachedResp, stale := h.cache.LookupWithOptionsLower(q, qName, cacheOpts); cachedResp != nil {
			cachedResp.Id = r.Id
			cachedResp.Question = r.Question
			cachedResp.RecursionDesired = r.RecursionDesired
			rule, act := "Cached", "CACHED"
			if stale {
				// Logged apart from a live hit: the client's experience is
				// identical, but an operator seeing a lot of these is looking at an
				// upstream that has stopped answering, not at a healthy cache.
				rule, act = "Cached (stale, refreshing)", "STALE"
			}
			h.logQuery(start, clientIP, accountName, proto, domain, rule, act, true, "cache", "", q.Qtype, cachedResp.Rcode)
			return cachedResp
		}
	}

	// 6. Query Upstream Resolver Pool (Fastest Racing)
	//
	// With validation on, ask upstream for DNSSEC records even when the client
	// did not. A DO=0 client still gets the protection; the signatures are
	// stripped again after validation.
	validator := h.currentValidator()
	validating := validator != nil && !cacheOpts.CD
	upstreamQuery := r
	if validating {
		upstreamQuery = r.Copy()
		ensureDNSSECRequest(upstreamQuery)
	}
	resp, _, upstream, err := h.upstreams.ExchangeFor(upstreamQuery, domain)
	if err != nil {
		log.Printf("[DNS] Upstream query error for %s: %v", domain, err)
		m := new(dns.Msg)
		m.SetRcode(r, dns.RcodeServerFailure)
		h.logQuery(start, clientIP, accountName, proto, domain, ruleName, "SERVFAIL", false, "", matchedPattern, q.Qtype, dns.RcodeServerFailure)
		return m
	}

	// DNSSEC: clear any AD the upstream volunteered, then set it only when the
	// chain of trust and the answer's signatures verify here. A truncated
	// answer cannot be validated; it is left for the transport retry path.
	if validator != nil && resp != nil {
		if !validating || resp.Truncated {
			resp.AuthenticatedData = false
		} else if result := h.validateUpstream(q, resp); result.Status == dnssec.Bogus {
			m := new(dns.Msg)
			m.SetRcode(r, dns.RcodeServerFailure)
			attachBogusEDE(m, r)
			h.logQuery(start, clientIP, accountName, proto, domain, "DNSSEC Bogus", "BOGUS", false, "", matchedPattern, q.Qtype, dns.RcodeServerFailure)
			return m
		}
	}

	// Cache successful and authoritative-negative answers. NXDOMAIN was
	// excluded here, so every miss for a non-existent name re-queried upstream.
	// TC=1 messages are never cached (v2.1.0 B-01 remediation): a truncated
	// reply is an instruction to retry over TCP, not an answer — caching it
	// served the same partial to every client for a full TTL, and a TC=1 over
	// a connection transport is a protocol violation. The racer now refuses to
	// hand one back as a winner; this gate is the second fence.
	if h.cache != nil && resp != nil && !resp.Truncated && (resp.Rcode == dns.RcodeSuccess || resp.Rcode == dns.RcodeNameError) {
		h.cache.PutWithOptions(q, resp, cacheOpts)
		// The cached copy is clamped to the configured TTL window; clamp the
		// answer this client gets too, so the first requester is not the one
		// client told to hold the record for longer than the cache will.
		h.cache.ClampTTLs(resp)
	}

	// Validation forced DO=1 upstream. Put the client's own EDNS/DO view back
	// before the answer leaves, including removing the OPT record entirely from
	// a reply to a client that never sent one.
	if validator != nil {
		capDNSSECLifetime(resp)
		normalizeForClient(resp, cacheOpts, q.Qtype)
	}

	// A stream client (TCP/DoT/DoH) must never receive TC=1 (RFC 7766 §4.2.2):
	// the whole point of a connection transport is that the answer fits. If a
	// truncated answer somehow survived the racer, answer SERVFAIL instead of
	// sending clients into a retry loop over a connection that cannot carry
	// the "please retry" flag usefully.
	if proto != "UDP" && resp != nil && resp.Truncated {
		log.Printf("[DNS] upstream returned TC=1 on %s for %s; answering SERVFAIL", proto, domain)
		m := new(dns.Msg)
		m.SetRcode(r, dns.RcodeServerFailure)
		return m
	}

	h.logQuery(start, clientIP, accountName, proto, domain, ruleName, action.String(), false, upstream, matchedPattern, q.Qtype, resp.Rcode)
	return resp
}

func (h *Handler) logQuery(start time.Time, ip, account, proto, domain, rule, action string, cached bool, upstreamAddr, matchedPattern string, qtype uint16, rcode int) {
	// Quick win: skip building the full QueryLogItem when nobody is watching.
	// The TelemetrySink already gates SSE fan-out on subscriber count, but it still
	// does the aggregator.RecordDetail, histogram observe, and ring buffer write.
	// If no SSE subscribers and no persistent query log, skip the entire call.
	if (h.telemetry == nil || h.telemetry.SSESubscribers() == 0) && h.queryLog == nil {
		return
	}

	item := database.QueryLogItem{
		Timestamp:        start,
		ClientIP:         ip,
		AccountName:      account,
		Protocol:         proto,
		Domain:           domain,
		RuleMatched:      rule,
		Action:           action,
		LatencyMs:        float64(time.Since(start).Microseconds()) / 1000.0,
		Cached:           cached,
		QType:            qtype,
		Rcode:            rcode,
		Upstream:         upstreamAddr,
		UpstreamProtocol: upstream.ProtocolOf(upstreamAddr),
		MatchedPattern:   matchedPattern,
	}
	if h.telemetry != nil {
		h.telemetry.PushQueryLog(item)
	}
	if h.queryLog != nil {
		h.queryLog(item)
	}
}

// logQueryRelay is logQuery with the relay target that answered the proxied
// name. The local relay path keeps using logQuery, so its payload is unchanged.
func (h *Handler) logQueryRelay(start time.Time, ip, account, proto, domain, rule, action string, cached bool, upstreamAddr, matchedPattern string, target RelayTarget, qtype uint16, rcode int) {
	// Same gating as logQuery: skip item construction when nobody is watching.
	if (h.telemetry == nil || h.telemetry.SSESubscribers() == 0) && h.queryLog == nil {
		return
	}

	item := database.QueryLogItem{
		Timestamp:           start,
		ClientIP:            ip,
		AccountName:         account,
		Protocol:            proto,
		Domain:              domain,
		RuleMatched:         rule,
		Action:              action,
		LatencyMs:           float64(time.Since(start).Microseconds()) / 1000.0,
		Cached:              cached,
		QType:               qtype,
		Rcode:               rcode,
		Upstream:            upstreamAddr,
		UpstreamProtocol:    upstream.ProtocolOf(upstreamAddr),
		MatchedPattern:      matchedPattern,
		RelayAgentID:        target.AgentID,
		RelayAgentName:      target.AgentName,
		RelayCountry:        target.Country,
		RelayEgressID:       target.EgressID,
		RelayEgressName:     target.EgressName,
		RelayEgressKind:     target.EgressKind,
		RelayFallbackReason: target.FallbackReason,
	}
	if h.telemetry != nil {
		h.telemetry.PushQueryLog(item)
	}
	if h.queryLog != nil {
		h.queryLog(item)
	}
}
