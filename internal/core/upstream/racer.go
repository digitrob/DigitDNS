package upstream

import (
	"bytes"
	"cmp"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/miekg/dns"
	"github.com/quic-go/quic-go/http3"
)

type UpstreamPool struct {
	mu          sync.RWMutex
	upstreams   []string
	timeout     time.Duration
	udpClient   *dns.Client
	tcpClient   *dns.Client
	tlsClient   *dns.Client
	httpClient  *http.Client
	h3Client    *http.Client
	quicTLS     *tls.Config
	dnscrypt    *dnscryptTransport
	racing      bool
	ecsClientIP net.IP
	stats       map[string]*upstreamStatInternal
}

// Upstream schemes. A plain "host:port" (or bare host) is UDP, kept exactly as it
// was before encrypted transport existed so a stored config keeps working.
const (
	schemeUDP   = ""
	schemeTCP   = "tcp"
	schemeTLS   = "tls"
	schemeHTTPS = "https"
	schemeQUIC  = "quic"
	schemeH3    = "h3"
)

// dohDefaultPath is appended when a DoH (or DoH3) URL carries no path of its own.
// RFC 8484 does not fix an endpoint, but /dns-query is the de-facto convention
// every public resolver documents, so "https://dns.google" does the obvious thing.
const dohDefaultPath = "/dns-query"

// sdnsPrefix marks a DNS stamp (https://dnscrypt.info/stamps). A DNSCrypt stamp
// has no native scheme in this pool, so it is kept verbatim and spoken by the
// DNSCrypt client; the DoH, DoT and DoQ stamp types are translated to the
// matching native scheme so a pasted stamp and a hand-typed URL are one entry.
const sdnsPrefix = "sdns://"

// splitUpstreamScheme separates a leading "scheme://" from the rest of an
// upstream string. Only the first occurrence counts, and the scheme is
// lower-cased so "TLS://" and "tls://" are the same resolver.
//
// It looks for "://" rather than a bare ":" on purpose: an IPv6 literal such as
// "[2606:4700:4700::1111]:53" is full of colons and must not be mistaken for one.
func splitUpstreamScheme(raw string) (scheme, rest string) {
	if i := strings.Index(raw, "://"); i >= 0 {
		return strings.ToLower(strings.TrimSpace(raw[:i])), raw[i+3:]
	}
	return "", raw
}

// splitHostPortDefault splits "host:port", or takes a port-less host and gives
// it defPort. A bracketed IPv6 literal without a port ("[::1]") is unwrapped
// before net.JoinHostPort re-brackets it — otherwise the brackets would be
// doubled into a host that never existed.
func splitHostPortDefault(s, defPort string) (host, port string, ok bool) {
	if h, p, err := net.SplitHostPort(s); err == nil {
		host, port = h, p
	} else {
		host = s
		if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
			host = host[1 : len(host)-1]
		}
		port = defPort
	}
	host = strings.TrimSpace(host)
	if host == "" || port == "" {
		return "", "", false
	}
	if n, err := strconv.Atoi(port); err != nil || n <= 0 || n > 65535 {
		return "", "", false
	}
	return host, port, true
}

// normalizeUpstream canonicalises one operator-entered upstream and reports
// whether it is usable. The canonical string is the pool's identity for that
// resolver — it is the stats key, the value the dashboard reads back, and what a
// delete matches — so it must be stable across the spellings of the same server:
//
//	1.1.1.1            -> "1.1.1.1:53"                 (UDP, unchanged)
//	udp://1.1.1.1      -> "1.1.1.1:53"
//	tcp://dns.google   -> "tcp://dns.google:53"
//	tls://1.1.1.1      -> "tls://1.1.1.1:853"
//	https://dns.google -> "https://dns.google/dns-query"
//	quic://dns.adguard.com -> "quic://dns.adguard.com:853"
//	h3://dns.google    -> "h3://dns.google/dns-query"
//	sdns://...         -> "sdns://..." (DNSCrypt, kept verbatim) or the native
//	                      scheme for a DoH/DoT/DoQ stamp
func normalizeUpstream(raw string) (string, bool) {
	u := strings.TrimSpace(raw)
	if u == "" {
		return "", false
	}
	// A stamp is a single opaque "sdns://<base64>" token, not a "scheme://rest"
	// pair, so it must be decoded whole rather than split.
	if strings.HasPrefix(strings.ToLower(u), sdnsPrefix) {
		return normalizeStamp(u)
	}
	scheme, rest := splitUpstreamScheme(u)

	switch scheme {
	case schemeUDP, "udp":
		host, port, ok := splitHostPortDefault(rest, "53")
		if !ok || !validUpstreamHost(host) {
			return "", false
		}
		return net.JoinHostPort(host, port), true
	case schemeTCP:
		return normalizeHostPortScheme(schemeTCP, rest, "53")
	case schemeTLS:
		// DoT has its own IANA-assigned port; the TLS handshake still validates
		// the certificate against the host, so a plain hostname is the robust
		// form and an IP literal needs a cert with an IP SAN.
		return normalizeHostPortScheme(schemeTLS, rest, "853")
	case schemeQUIC, "doq":
		// RFC 9250 registers 853 for DoQ, the same port DoT uses over TCP.
		return normalizeHostPortScheme(schemeQUIC, rest, "853")
	case schemeHTTPS:
		return normalizeHTTPResolverURL(schemeHTTPS, rest)
	case schemeH3, "doh3":
		return normalizeHTTPResolverURL(schemeH3, rest)
	default:
		// "http://" (unauthenticated DoH) and anything else unrecognised is
		// rejected rather than silently downgraded to plaintext.
		return "", false
	}
}

// normalizeHostPortScheme canonicalises a "host[:port]" upstream for a scheme that
// addresses one host and port (TCP, DoT, DoQ), giving a port-less host defPort.
func normalizeHostPortScheme(scheme, rest, defPort string) (string, bool) {
	host, port, ok := splitHostPortDefault(rest, defPort)
	if !ok || !validUpstreamHost(host) {
		return "", false
	}
	return scheme + "://" + net.JoinHostPort(host, port), true
}

// normalizeHTTPResolverURL canonicalises the part after "https://" or "h3://".
// The port defaults to 443 and is dropped when it is the default, so
// "host/dns-query" and "host:443/dns-query" are one entry; the path defaults to
// /dns-query. DoH (RFC 8484) and DoH3 are the same service over a different
// transport, so they share this canonicalisation and differ only in scheme.
func normalizeHTTPResolverURL(scheme, rest string) (string, bool) {
	u, err := url.Parse(scheme + "://" + rest)
	if err != nil || u.User != nil || u.Fragment != "" {
		return "", false
	}
	host := u.Hostname()
	if host == "" || !validUpstreamHost(host) {
		return "", false
	}
	port := u.Port()
	if port != "" {
		if n, err := strconv.Atoi(port); err != nil || n <= 0 || n > 65535 {
			return "", false
		}
	}
	path := u.Path
	if path == "" || path == "/" {
		path = dohDefaultPath
	}
	// A port typed after the path ("…/dns-query:443") is not a port to the URL
	// parser: it becomes part of the path, every request 404s, and the entry
	// looks healthy in the list while never answering. Refuse it instead of
	// storing a resolver that cannot work. See the accepted spelling one line up
	// in the tests: "host:port/path".
	if pathHasPortSuffix(path) {
		return "", false
	}

	authority := host
	if strings.Contains(host, ":") {
		authority = "[" + host + "]" // IPv6 without a port still needs brackets
	}
	if port != "" && port != "443" {
		authority = net.JoinHostPort(host, port)
	}

	out := scheme + "://" + authority + path
	if u.RawQuery != "" {
		out += "?" + u.RawQuery
	}
	return out, true
}

// pathHasPortSuffix reports whether a URL path ends in ":" followed by digits —
// the shape left behind when someone writes the port after the path.
func pathHasPortSuffix(path string) bool {
	i := strings.LastIndexByte(path, ':')
	if i < 0 || i == len(path)-1 {
		return false
	}
	for _, c := range path[i+1:] {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// NormalizeUpstream canonicalises a single operator-supplied upstream string,
// reporting false when it is not a usable resolver. It is exported so the
// dashboard's add handler validates a value exactly as the pool will store it.
func NormalizeUpstream(raw string) (string, bool) {
	return normalizeUpstream(raw)
}

type upstreamStatInternal struct {
	mu          sync.Mutex
	avgLatency  time.Duration
	hits        uint64
	errors      uint64
	lastUpdated time.Time
}

// normalizeUpstreams cleans a raw upstream list: trims blanks, canonicalises
// each entry (default ports, DoH default path, scheme), drops entries that are
// not a usable resolver, and de-duplicates. An upstream without a port silently
// fails every exchange, so normalizing here is what keeps a typo from taking the
// whole resolver offline.
func normalizeUpstreams(list []string) []string {
	out := make([]string, 0, len(list))
	seen := make(map[string]struct{}, len(list))
	for _, raw := range list {
		addr, ok := normalizeUpstream(raw)
		if !ok {
			continue
		}
		if _, dup := seen[addr]; dup {
			continue
		}
		seen[addr] = struct{}{}
		out = append(out, addr)
	}
	return out
}

// validUpstreamHost reports whether host is an IP literal or a plausible DNS
// name. Without this check net.JoinHostPort accepted anything that was not
// already host:port — "8.8.8.8:53:53" became "[8.8.8.8:53:53]:53", and
// "1.1.1.1 8.8.8.8" became a single bracketed host — so a config typo was stored
// as a real upstream that then failed every exchange. From the dashboard that
// looks like a dead resolver rather than a line the operator mistyped.
func validUpstreamHost(host string) bool {
	if net.ParseIP(host) != nil {
		return true
	}
	if host == "" || len(host) > 253 {
		return false
	}
	// SplitSeq rather than Split: this runs on every upstream the operator saves
	// and the labels are only inspected, never kept, so there is no reason to
	// allocate a slice for them.
	for label := range strings.SplitSeq(strings.TrimSuffix(host, "."), ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for i := 0; i < len(label); i++ {
			c := label[i]
			switch {
			case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '-', c == '_':
			default:
				return false
			}
		}
	}
	return true
}

// parseECSClientIP validates the operator's configured EDNS Client Subnet
// address. It is a single address for the whole resolver on purpose — see the
// note on attachECS — so getting it wrong is a silent, resolver-wide mistake, and
// the two ways to get it wrong both used to pass without a word:
//
//   - An unparseable value became "no ECS at all". The operator configured a
//     feature, the daemon disabled it, and nothing said so.
//   - A private, loopback or link-local address was sent verbatim. No upstream can
//     geolocate 192.168.1.10; RFC 7871 §5 says to treat such an address as
//     unroutable, so the option is either ignored (the good case) or answered from
//     the resolver's own location while the operator believes otherwise. Either
//     way it is inert configuration that looks active.
//
// Rejecting loudly and running without ECS is the honest outcome: the resolver
// still answers every query, and the log line says exactly what was discarded.
func parseECSClientIP(raw string) net.IP {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	ip := net.ParseIP(raw)
	if ip == nil {
		log.Printf("[Upstream] Ignoring ecs_client_ip %q: not an IP address. EDNS Client Subnet is disabled.", raw)
		return nil
	}
	if !ip.IsGlobalUnicast() || ip.IsPrivate() {
		log.Printf("[Upstream] Ignoring ecs_client_ip %s: not a public address, so no upstream can locate it. EDNS Client Subnet is disabled.", ip)
		return nil
	}

	// Log what is actually advertised, not what was configured: the address is
	// masked to /24 or /56 before it leaves, and an operator checking whether they
	// leaked a subscriber's exact address should be able to read the answer here.
	if v4 := ip.To4(); v4 != nil {
		log.Printf("[Upstream] EDNS Client Subnet advertising %s/24", v4.Mask(net.CIDRMask(24, 32)))
	} else {
		log.Printf("[Upstream] EDNS Client Subnet advertising %s/56", ip.Mask(net.CIDRMask(56, 128)))
	}
	return ip
}

func NewUpstreamPool(upstreams []string, timeout time.Duration, racing bool, ecsIP string) *UpstreamPool {
	if timeout <= 0 {
		timeout = 2500 * time.Millisecond
	}

	normalized := normalizeUpstreams(upstreams)

	pool := &UpstreamPool{
		upstreams:   normalized,
		timeout:     timeout,
		racing:      racing,
		ecsClientIP: parseECSClientIP(ecsIP),
		stats:       make(map[string]*upstreamStatInternal),
		udpClient: &dns.Client{
			Net:     "udp",
			Timeout: timeout,
		},
		tcpClient: &dns.Client{
			Net:     "tcp",
			Timeout: timeout,
		},
		// DoT verifies the upstream's certificate like any other TLS client; there
		// is deliberately no InsecureSkipVerify escape hatch. The host is taken
		// from the address by miekg/dns when TLSConfig.ServerName is empty.
		tlsClient: &dns.Client{
			Net:       "tcp-tls",
			Timeout:   timeout,
			TLSConfig: &tls.Config{MinVersion: tls.VersionTLS12},
		},
		// DoH posts a binary DNS message over HTTPS. Proxy is nil because an
		// upstream resolver is not a web fetch: honouring HTTP_PROXY here would
		// route every query through the operator's environment by accident, the
		// same reason the updater ignores it.
		httpClient: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				Proxy:              nil,
				TLSClientConfig:    &tls.Config{MinVersion: tls.VersionTLS12},
				ForceAttemptHTTP2:  true,
				MaxIdleConns:       4,
				IdleConnTimeout:    90 * time.Second,
				DisableCompression: true,
			},
		},
		// DoH3 posts the same RFC 8484 body over HTTP/3. The http3.Transport
		// negotiates the "h3" ALPN itself and verifies the certificate like the
		// HTTP/2 client above; there is no skip-verify escape hatch here either.
		h3Client: &http.Client{
			Timeout: timeout,
			Transport: &http3.Transport{
				TLSClientConfig:    &tls.Config{MinVersion: tls.VersionTLS12},
				DisableCompression: true,
			},
		},
		// DoQ dials a QUIC connection per query. NextProtos carries the RFC 9250
		// "doq" ALPN token; ServerName is filled in per dial from the upstream.
		quicTLS:  &tls.Config{MinVersion: tls.VersionTLS12, NextProtos: []string{doqALPN}},
		dnscrypt: newDNSCryptTransport(timeout),
	}

	for _, u := range normalized {
		pool.stats[u] = &upstreamStatInternal{
			avgLatency:  12 * time.Millisecond,
			lastUpdated: time.Now(),
		}
	}

	// No probe from here. Latency is pre-seeded above, so the pool is usable the
	// instant it is built, and the caller decides when to spend network I/O on
	// measuring it: the daemon fires BenchmarkAll once at startup, the dashboard
	// and the TUI fire it on demand. A probe launched from the constructor had no
	// handle to cancel or await, which made every test that reads these stats race
	// an unstoppable goroutine and put ~65 real DNS queries into a unit-test run.
	return pool
}

func (p *UpstreamPool) recordStat(addr string, latency time.Duration, success bool) {
	p.mu.RLock()
	stat, ok := p.stats[addr]
	p.mu.RUnlock()

	if !ok {
		return
	}

	stat.mu.Lock()
	defer stat.mu.Unlock()

	if success {
		stat.hits++
		if stat.avgLatency == 0 {
			stat.avgLatency = latency
		} else {
			stat.avgLatency = (stat.avgLatency*7 + latency*3) / 10
		}
	} else {
		stat.errors++
	}
	stat.lastUpdated = time.Now()
}

// BenchmarkAll probes every upstream once and folds the round-trip time into its
// latency estimate. Only a resolver that actually answers the probe counts:
// scoring a refusal by its round-trip time alone ranked an instantly-refusing
// resolver as the fastest in the pool, which is exactly the one the dashboard
// should not be recommending — and the same goes for one that answers a question
// nobody asked.
func (p *UpstreamPool) BenchmarkAll() {
	m := new(dns.Msg)
	m.SetQuestion("cloudflare.com.", dns.TypeA)

	p.mu.RLock()
	servers := make([]string, len(p.upstreams))
	copy(servers, p.upstreams)
	timeout := p.timeout
	p.mu.RUnlock()

	var wg sync.WaitGroup
	for _, s := range servers {
		wg.Add(1)
		go func(addr string) {
			defer wg.Done()
			// Probe through the same per-scheme path a real query takes. Probing
			// everything over UDP:53 used to send a DoT/DoH upstream's probe to a
			// port that is not even listening, so it scored as broken.
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()
			start := time.Now()
			resp, _, err := p.queryOne(ctx, m, addr)
			dur := time.Since(start)
			p.recordStat(addr, dur, err == nil && usableRcode(resp))
		}(s)
	}
	wg.Wait()
}

func (p *UpstreamPool) Exchange(req *dns.Msg) (*dns.Msg, time.Duration, string, error) {
	if req == nil {
		return nil, 0, "", errors.New("nil dns request")
	}

	p.mu.RLock()
	upstreams := make([]string, len(p.upstreams))
	copy(upstreams, p.upstreams)
	racing := p.racing
	timeout := p.timeout
	ecs := p.ecsClientIP
	p.mu.RUnlock()

	if len(upstreams) == 0 {
		return nil, 0, "", errors.New("no upstream resolvers configured")
	}

	queryMsg := req
	if ecs != nil {
		queryMsg = req.Copy()
		p.attachECS(queryMsg, ecs)
	}

	if racing && len(upstreams) > 1 {
		return p.exchangeRacing(queryMsg, upstreams, timeout)
	}

	return p.exchangeSequential(queryMsg, upstreams, timeout)
}

// usableRcode reports whether an answer is one the client can be given as final.
//
// Only NOERROR and NXDOMAIN are: NXDOMAIN is an authoritative "this name does
// not exist" and must never be retried elsewhere. SERVFAIL, REFUSED, NOTIMP and
// FORMERR are that particular resolver declining, not an answer — and on a
// filtered network they are the normal reply for exactly the domains this
// resolver exists to reach. Treating them as answers meant the fastest upstream
// won the race with a REFUSED while a working upstream was still in flight, so a
// game launcher got a hard failure that a plain public resolver would have
// answered.
func usableRcode(resp *dns.Msg) bool {
	if resp == nil {
		return false
	}
	// A TC=1 answer is a POINTER, not an answer (v2.1.0 B-01 remediation): the
	// payload is deliberately clipped and the client is told to re-ask over
	// TCP. Treating it as usable let a partial beat a complete answer from a
	// slower upstream in racing mode, land in the cache for a full TTL, and —
	// worst of all — be served with TC=1 over TCP/DoT/DoH, which RFC 7766
	// §4.2.2 forbids on a connection transport. The UDP retry in exchange()
	// already re-asks over TCP; if that retry also fails, the honest result is
	// a failure, and "clipped beats nothing" no longer poisons the cache.
	if resp.Truncated {
		return false
	}
	switch resp.Rcode {
	case dns.RcodeSuccess, dns.RcodeNameError:
		return true
	default:
		return false
	}
}

// ErrQuestionMismatch is an upstream reply that answers something other than what
// was asked. It is a failure, not an answer: the caller must try another upstream.
var ErrQuestionMismatch = errors.New("upstream answered a different question")

// answersTheQuestion reports whether a reply belongs to req.
//
// miekg/dns correlates a reply to a query by transaction ID and nothing else, and
// usableRcode looks only at the rcode, so without this check an answer for some
// other name is accepted — and the handler then stores it in the cache under the
// name the client asked for, where every later client is served it too. That is
// cache poisoning, and over plaintext UDP the ID plus the source port is the whole
// of what an off-path attacker has to guess. Every production resolver (BIND,
// Unbound, dnsmasq) makes this check; dropping the reply costs one lost race,
// because the caller treats a mismatch like any other upstream failure and the
// next upstream answers.
func answersTheQuestion(req, resp *dns.Msg) bool {
	if req == nil || resp == nil {
		return false
	}
	// Anything other than exactly one question on both sides is uncorrelatable: this
	// resolver only ever forwards a single-question query, so a reply carrying none
	// or several is not one of ours to match.
	if len(req.Question) != 1 || len(resp.Question) != 1 {
		return false
	}
	asked, answered := req.Question[0], resp.Question[0]

	// Case-insensitively, per RFC 4343. A byte comparison would be wrong even absent
	// an attacker: an upstream may echo the name in any case, and 0x20-encoding — the
	// anti-spoofing trick of randomising that case on purpose — depends on it.
	return answered.Qtype == asked.Qtype &&
		answered.Qclass == asked.Qclass &&
		strings.EqualFold(answered.Name, asked.Name)
}

// questionSummary names a message's question for a log line, including the case
// where there is nothing to name.
func questionSummary(m *dns.Msg) string {
	if m == nil {
		return "no reply"
	}
	if len(m.Question) == 0 {
		return "no question"
	}
	q := m.Question[0]
	return q.Name + " " + dns.Type(q.Qtype).String()
}

// queryOne sends one query to one upstream, retrying over TCP when the answer
// comes back truncated so large RRsets (many A records, DNSSEC) are not silently
// clipped. A failed TCP retry leaves the truncated UDP answer in place — clipped
// beats nothing. Whatever comes back has to answer the question that was asked.
func (p *UpstreamPool) queryOne(ctx context.Context, req *dns.Msg, addr string) (*dns.Msg, time.Duration, error) {
	// Never forward the client's chosen transaction ID upstream. miekg/dns
	// correlates a reply to its query by ID alone, so a resolver that echoes the
	// client's ID upstream hands an off-path spoofer half the entropy RFC 5452
	// relies on: the client picks the ID, leaving only the source port to guess,
	// and a whitelisted subscriber who chose the ID can then blind-spray forged
	// replies to poison the shared cache. Query under a fresh random ID per
	// exchange and restore the client's ID on the reply; answersTheQuestion is
	// the second fence, not the only one.
	q := req.Copy()
	clientID := req.Id

	scheme, endpoint := splitUpstreamScheme(addr)

	var (
		resp *dns.Msg
		rtt  time.Duration
		err  error
	)
	switch {
	case strings.HasPrefix(addr, sdnsPrefix):
		// DNSCrypt encrypts the whole DNS message, ID included, so a fresh random
		// ID still carries entropy and there is no transport-level correlation to
		// replace it.
		q.Id = dns.Id()
		resp, rtt, err = p.dnscrypt.exchange(addr, q)
	case scheme == schemeQUIC:
		// RFC 9250 §4.2.1: a DoQ query MUST use transaction ID 0. Correlation is
		// by the QUIC stream, so a nonzero ID is a protocol violation.
		q.Id = 0
		resp, rtt, err = p.exchangeDoQ(ctx, q, endpoint)
	case scheme == schemeH3:
		// RFC 8484 §4.1 over HTTP/3: transaction ID 0, correlation by the request.
		// net/http only speaks the "https" scheme, so the canonical "h3://" URL is
		// retargeted at the HTTP/3 client as "https://" — the ALPN is what selects
		// HTTP/3, not the URL scheme.
		q.Id = 0
		resp, rtt, err = p.exchangeDoH(ctx, p.h3Client, q, schemeHTTPS+"://"+endpoint)
	case scheme == schemeHTTPS:
		// RFC 8484 §4.1: a DoH query MUST use transaction ID 0. Correlation is
		// by the HTTP request, so the ID carries no entropy and every client's
		// query would otherwise be a fingerprintable value.
		q.Id = 0
		resp, rtt, err = p.exchangeDoH(ctx, p.httpClient, q, addr)
	case scheme == schemeTLS:
		// The client's ID rule above still holds: query under a fresh ID.
		q.Id = dns.Id()
		resp, rtt, err = p.tlsClient.ExchangeContext(ctx, q, endpoint)
	case scheme == schemeTCP:
		q.Id = dns.Id()
		resp, rtt, err = p.tcpClient.ExchangeContext(ctx, q, endpoint)
	default:
		q.Id = dns.Id()
		resp, rtt, err = p.udpClient.ExchangeContext(ctx, q, endpoint)
		if err == nil && resp != nil && resp.Truncated {
			if tcpResp, tcpRTT, tcpErr := p.tcpClient.ExchangeContext(ctx, q, endpoint); tcpErr == nil && tcpResp != nil {
				resp, rtt, err = tcpResp, tcpRTT, nil
			}
		}
	}
	if err == nil && !answersTheQuestion(q, resp) {
		// Returned as an error rather than as a fallback so that neither exchange path
		// can hand this message to the client or the cache.
		return nil, rtt, fmt.Errorf("%w: asked %s of %s, got %s",
			ErrQuestionMismatch, questionSummary(q), addr, questionSummary(resp))
	}
	if resp != nil {
		// Restore the client's ID so the handler can hand this reply — or a cache
		// entry derived from it — back to the client that asked.
		resp.Id = clientID
	}
	return resp, rtt, err
}

// exchangeDoH sends one DNS query over DNS-over-HTTPS (RFC 8484) and returns the
// decoded reply. The binary message is POSTed as application/dns-message; a
// non-200, an unreadable body, or a body that will not unpack is an upstream
// failure like any other. The caller has already set q.Id = 0. client is the
// HTTP/2 client for "https://" or the HTTP/3 client for "h3://".
func (p *UpstreamPool) exchangeDoH(ctx context.Context, client *http.Client, q *dns.Msg, rawURL string) (*dns.Msg, time.Duration, error) {
	packed, err := q.Pack()
	if err != nil {
		return nil, 0, fmt.Errorf("pack DoH query for %s: %w", rawURL, err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, rawURL, bytes.NewReader(packed))
	if err != nil {
		return nil, 0, fmt.Errorf("build DoH request for %s: %w", rawURL, err)
	}
	httpReq.Header.Set("Content-Type", "application/dns-message")
	httpReq.Header.Set("Accept", "application/dns-message")

	start := time.Now()
	httpResp, err := client.Do(httpReq)
	if err != nil {
		return nil, time.Since(start), err
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		return nil, time.Since(start), fmt.Errorf("DoH server %s returned HTTP %d", rawURL, httpResp.StatusCode)
	}

	// A DNS message cannot exceed 64 KiB, so cap the read: a hostile or broken
	// endpoint must not be able to make the resolver allocate without bound.
	body, err := io.ReadAll(io.LimitReader(httpResp.Body, dns.MaxMsgSize))
	if err != nil {
		return nil, time.Since(start), fmt.Errorf("read DoH response from %s: %w", rawURL, err)
	}

	out := new(dns.Msg)
	if err := out.Unpack(body); err != nil {
		return nil, time.Since(start), fmt.Errorf("unpack DoH response from %s: %w", rawURL, err)
	}
	return out, time.Since(start), nil
}

func (p *UpstreamPool) exchangeRacing(req *dns.Msg, upstreams []string, timeout time.Duration) (*dns.Msg, time.Duration, string, error) {
	type result struct {
		resp     *dns.Msg
		duration time.Duration
		upstream string
		err      error
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	resChan := make(chan result, len(upstreams))

	for _, u := range upstreams {
		go func(addr string) {
			start := time.Now()
			resp, rtt, err := p.queryOne(ctx, req, addr)
			dur := time.Since(start)

			// A query abandoned because another upstream already answered is not
			// an upstream failure. Recording it added an error to every healthy
			// resolver on every raced query, so the dashboard's error column
			// counted how often each resolver *lost the race* — the fastest
			// upstream looked the most broken the more it was used. A deadline
			// that expired is still counted: that one is the upstream's fault.
			//
			// v2.1.0 B-12 remediation: the classification tests the ERROR, not
			// the shared context's current state. A genuine failure at t=1ms
			// raced a winner at t=5ms used to reach this line after cancel()
			// and read ctx.Err()==Canceled, silently dropping a real failure
			// from the dashboard. errors.Is(err, ...) answers per-goroutine.
			if err == nil || !errors.Is(err, context.Canceled) {
				p.recordStat(addr, dur, err == nil && usableRcode(resp))
			}

			select {
			case resChan <- result{resp: resp, duration: rtt, upstream: addr, err: err}:
			case <-ctx.Done():
			}
		}(u)
	}

	var (
		lastErr  error
		fallback *result
	)
collect:
	for range upstreams {
		select {
		case res := <-resChan:
			if res.err == nil && usableRcode(res.resp) {
				cancel() // immediately cancel other upstreams to free CPU
				return res.resp, res.duration, res.upstream, nil
			}
			if res.err != nil {
				lastErr = res.err
				continue
			}
			// A real DNS message, just not a usable one. Hold the first as a
			// fallback and keep waiting: if no upstream does better, the client
			// should still get the upstream's own verdict rather than a
			// fabricated SERVFAIL.
			if fallback == nil && res.resp != nil {
				held := res
				fallback = &held
			}
		case <-ctx.Done():
			break collect
		}
	}

	if fallback != nil {
		return fallback.resp, fallback.duration, fallback.upstream, nil
	}
	if lastErr != nil {
		return nil, 0, "", lastErr
	}
	if ctx.Err() != nil {
		return nil, 0, "", errors.New("upstream query timed out")
	}
	return nil, 0, "", errors.New("all upstream servers failed to respond")
}

// exchangeSequential tries each upstream in order. The whole walk shares one
// deadline (matching the racing path) so a long upstream list cannot stall a
// client for len(upstreams) × timeout. The first upstream always gets an
// attempt; later ones are skipped once the budget is spent.
func (p *UpstreamPool) exchangeSequential(req *dns.Msg, upstreams []string, timeout time.Duration) (*dns.Msg, time.Duration, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var (
		lastErr      error
		fallback     *dns.Msg
		fallbackRTT  time.Duration
		fallbackAddr string
	)
	for i, u := range upstreams {
		if i > 0 && ctx.Err() != nil {
			break
		}
		start := time.Now()
		resp, rtt, err := p.queryOne(ctx, req, u)
		ok := err == nil && usableRcode(resp)
		p.recordStat(u, time.Since(start), ok)
		if ok {
			return resp, rtt, u, nil
		}
		// A SERVFAIL or REFUSED used to end the walk here, so a single filtered
		// resolver at the head of the list meant the ones behind it were never
		// tried at all.
		if err != nil {
			lastErr = err
			continue
		}
		if fallback == nil && resp != nil {
			fallback, fallbackRTT, fallbackAddr = resp, rtt, u
		}
	}
	if fallback != nil {
		return fallback, fallbackRTT, fallbackAddr, nil
	}
	if lastErr == nil {
		lastErr = errors.New("all upstream servers failed to respond")
	}
	return nil, 0, "", lastErr
}

// UpstreamStat is an exported, read-only snapshot of a single upstream server health.
type UpstreamStat struct {
	Address   string  `json:"address"`
	LatencyNs int64   `json:"latency"` // nanoseconds (frontend divides by 1e6)
	LatencyMs float64 `json:"latency_ms"`
	Hits      uint64  `json:"hits"`
	Errors    uint64  `json:"errors"`
}

// GetUpstreamStats returns a snapshot of latency/health stats for all upstreams, sorted by latency.
func (p *UpstreamPool) GetUpstreamStats() []UpstreamStat {
	type snapshot struct {
		latency time.Duration
		hits    uint64
		errors  uint64
	}

	p.mu.RLock()
	addrs := make([]string, len(p.upstreams))
	copy(addrs, p.upstreams)
	stats := make(map[string]snapshot, len(p.stats))
	for addr, st := range p.stats {
		st.mu.Lock()
		stats[addr] = snapshot{latency: st.avgLatency, hits: st.hits, errors: st.errors}
		st.mu.Unlock()
	}
	p.mu.RUnlock()

	out := make([]UpstreamStat, 0, len(addrs))
	for _, addr := range addrs {
		s := UpstreamStat{Address: addr, LatencyNs: int64(12 * time.Millisecond)}
		if raw, ok := stats[addr]; ok {
			s.LatencyNs = int64(raw.latency)
			s.Hits = raw.hits
			s.Errors = raw.errors
		}
		s.LatencyMs = float64(s.LatencyNs) / 1e6
		out = append(out, s)
	}

	slices.SortFunc(out, func(a, b UpstreamStat) int {
		if a.LatencyNs != b.LatencyNs {
			return cmp.Compare(a.LatencyNs, b.LatencyNs)
		}
		// Equal latencies are the normal state right after a restart — every
		// upstream starts at the same seeded 12ms — and an unstable sort made the
		// dashboard's resolver list reshuffle itself on every refresh.
		return strings.Compare(a.Address, b.Address)
	})
	return out
}

// SetUpstreams atomically replaces the upstream server list (thread-safe, live reload).
func (p *UpstreamPool) SetUpstreams(list []string) {
	normalized := normalizeUpstreams(list)

	p.mu.Lock()
	defer p.mu.Unlock()

	p.upstreams = normalized
	for _, u := range p.upstreams {
		if _, ok := p.stats[u]; !ok {
			p.stats[u] = &upstreamStatInternal{
				avgLatency:  12 * time.Millisecond,
				lastUpdated: time.Now(),
			}
		}
	}
	// Drop stats for removed upstreams
	keep := make(map[string]struct{}, len(p.upstreams))
	for _, u := range p.upstreams {
		keep[u] = struct{}{}
	}
	for addr := range p.stats {
		if _, ok := keep[addr]; !ok {
			delete(p.stats, addr)
		}
	}
}

// GetUpstreams returns a copy of the active upstream list.
func (p *UpstreamPool) GetUpstreams() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make([]string, len(p.upstreams))
	copy(out, p.upstreams)
	return out
}

// attachECS adds an EDNS0 Client Subnet option scoped to the configured
// netmask. The address is masked first: sending all four octets with a /24
// source netmask would leak the exact client host to every upstream.
//
// One subnet for the whole resolver, chosen by the operator, rather than each
// client's own — and that is a decision, not a shortcut. The cache is keyed by
// (type, class, name) and nothing else, so a per-client subnet would have every
// CDN answer fetched for one subscriber's city served to every other subscriber
// from then on: worse routing than sending no ECS at all, and silently wrong.
// Doing it properly means an RFC 7871 subnet cache — an entry per (name, scope)
// with the upstream's returned SourceScope deciding what may be shared — which
// multiplies entry count by the number of distinct client networks and cuts the
// hit rate this resolver's latency depends on. For a VPS serving one region, one
// operator-chosen subnet gets the same CDN placement at none of that cost.
func (p *UpstreamPool) attachECS(msg *dns.Msg, clientIP net.IP) {
	var ecs *dns.EDNS0_SUBNET
	if ipv4 := clientIP.To4(); ipv4 != nil {
		const bits = 24
		ecs = &dns.EDNS0_SUBNET{
			Code:          dns.EDNS0SUBNET,
			Address:       ipv4.Mask(net.CIDRMask(bits, 32)),
			Family:        1,
			SourceNetmask: bits,
			SourceScope:   0,
		}
	} else if ipv6 := clientIP.To16(); ipv6 != nil {
		const bits = 56
		ecs = &dns.EDNS0_SUBNET{
			Code:          dns.EDNS0SUBNET,
			Address:       ipv6.Mask(net.CIDRMask(bits, 128)),
			Family:        2,
			SourceNetmask: bits,
			SourceScope:   0,
		}
	}
	if ecs == nil {
		return
	}

	opt := msg.IsEdns0()
	if opt == nil {
		msg.SetEdns0(4096, false)
		opt = msg.IsEdns0()
		if opt == nil {
			return
		}
	}
	// Replace any subnet option already present instead of appending a second one.
	for i, o := range opt.Option {
		if o.Option() == dns.EDNS0SUBNET {
			opt.Option[i] = ecs
			return
		}
	}
	opt.Option = append(opt.Option, ecs)
}
