package upstream

import (
	"bytes"
	"cmp"
	"context"
	"crypto/tls"
	"digitdns/internal/core/resolution"
	"digitdns/internal/safego"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand/v2"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
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
	// Policy: the global defaults and the per-domain overrides. Guarded by mu
	// like the list itself, so a settings save takes effect on the next query
	// without a restart.
	policy Policy
	// profileIndex is the compiled form of policy.Profiles. It is rebuilt by
	// SetPolicy and read on the query path, so the hot path never scans every
	// configured pattern.
	profileIndex *resolution.ProfileIndex

	// snap is the immutable query-path view of upstreams + policy + the compiled
	// profile index + the per-transport candidate lists. Settings saves replace
	// it wholesale, so a query never copies a slice or takes the settings lock.
	snap atomic.Pointer[poolSnapshot]

	// orderMu guards the cached latency order used by Hedged and Best Known. The
	// order changes only when a measurement does, so rebuilding it at most once
	// per second keeps those strategies from sorting the pool on every miss.
	orderMu    sync.Mutex
	orderBuilt time.Time
	orderFor   []string
	order      []string

	// openCircuits is a fast-path count: zero means no candidate can be skipped,
	// so the query path returns the snapshot list without touching any stat lock.
	openCircuits atomic.Int32

	// Connection reuse. DoT keeps a small idle pool per endpoint; DoQ keeps one
	// QUIC connection per endpoint and opens a stream per query.
	dotPoolsMu sync.Mutex
	dotPools   map[string]*dotConnPool
	doqMu      sync.Mutex
	doqConns   map[string]*doqConn
}

// poolSnapshot is the read-only query-path state. Every field is immutable once
// published; a settings save builds a new snapshot and swaps the pointer.
type poolSnapshot struct {
	upstreams    []string
	policy       Policy
	profileIndex *resolution.ProfileIndex
	racing       bool
	all          []string
	encrypted    []string
	regular      []string
}

// candidates returns the pre-filtered list for a transport. The returned slice
// is owned by the snapshot and must not be modified by the caller.
func (s *poolSnapshot) candidates(transport string) []string {
	switch resolution.NormalizeTransport(transport) {
	case resolution.TransportEncrypted:
		return s.encrypted
	case resolution.TransportRegular:
		return s.regular
	default:
		return s.all
	}
}

// Policy is the resolution policy installed by the settings layer.
type Policy struct {
	// Transport filters the pool: "any", "encrypted" (DoT/DoH/DoQ/DNSCrypt) or
	// "regular" (plain UDP/TCP). Empty means "any".
	Transport string
	// DefaultUpstream pins every name to one resolver unless a profile overrides
	// it. Empty means "use the pool".
	DefaultUpstream string
	// DefaultStrategy is "fastest" or "sequential"; empty keeps the pool's
	// constructed racing setting.
	DefaultStrategy string
	// Profiles are the per-domain overrides, keyed by domain or "*.domain".
	Profiles map[string]resolution.Profile
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

const (
	// latencyWindow is the number of successful live exchanges kept per upstream.
	// The card shows the median of this window: an EWMA is dragged up by a single
	// slow DNSSEC or TCP-fallback exchange and takes many fast samples to recover,
	// which made healthy resolvers look permanently slow.
	latencyWindow = 128
	// recentWindow is how many live outcomes the health dot considers. It is
	// deliberately short: "did this resolver work recently" is the question the dot
	// answers, not "has it ever failed since the process started".
	recentWindow = 64
)

type upstreamStatInternal struct {
	mu sync.Mutex
	// latencyRing holds the last latencyWindow successful live round trips. p50
	// is what the card shows; p95 is in the tooltip. Probes do not enter it, so
	// the live figure cannot be skewed by a startup benchmark on a bad network.
	latencyRing  [latencyWindow]time.Duration
	latencyNext  int
	latencyCount int
	samples      uint64
	lastMeasured time.Time

	hits            uint64
	errors          uint64 // live transport + policy, kept for the existing card
	transportErrors uint64
	policyErrors    uint64

	// Probe counters are separate from live traffic. Folding startup benchmarks
	// into hits/errors made an upstream that was down for one probe look broken
	// for the life of the process.
	probeHits        uint64
	probeErrors      uint64
	probePolicy      uint64
	lastProbeLatency time.Duration
	lastProbeOK      bool
	lastProbeAt      time.Time

	// recent is a ring of live outcomes: 1 is a transport failure, 0 is a usable
	// answer or a policy response (both prove reachability). The health dot reads
	// this instead of the lifetime error count.
	recent         [recentWindow]uint8
	recentNext     int
	recentLen      int
	recentFailures int

	// consecutiveFailures counts transport failures only. A REFUSED or SERVFAIL
	// proves the resolver is reachable; it must not trip the breaker.
	consecutiveFailures uint32
	lastError           string
	lastErrorAt         time.Time
	lastUpdated         time.Time
	// openUntil is UnixNano, or 0 while the circuit is closed. Atomic so the
	// query path can skip an open resolver without taking the per-stat mutex.
	openUntil atomic.Int64
	// probeUntil is a lease on the single half-open probe. It stops every
	// concurrent query from hitting a resolver at the moment its cooldown
	// expires; a canceled probe releases it when the lease runs out.
	probeUntil atomic.Int64
}

// observeLatency appends a successful live round trip to the rolling window.
func (st *upstreamStatInternal) observeLatency(rtt time.Duration) {
	if rtt <= 0 {
		return
	}
	st.latencyRing[st.latencyNext] = rtt
	st.latencyNext = (st.latencyNext + 1) % latencyWindow
	if st.latencyCount < latencyWindow {
		st.latencyCount++
	}
}

// observeRecent records one live outcome for the health dot.
func (st *upstreamStatInternal) observeRecent(failure bool) {
	var v uint8
	if failure {
		v = 1
	}
	if st.recentLen < recentWindow {
		st.recent[st.recentNext] = v
		st.recentNext = (st.recentNext + 1) % recentWindow
		st.recentLen++
		if failure {
			st.recentFailures++
		}
		return
	}
	if st.recent[st.recentNext] == 1 {
		st.recentFailures--
	}
	st.recent[st.recentNext] = v
	st.recentNext = (st.recentNext + 1) % recentWindow
	if failure {
		st.recentFailures++
	}
}

// latencyPercentiles returns the median, p95 and mean of the rolling window.
// A single allocation-free copy keeps the per-second stats read cheap.
func (st *upstreamStatInternal) latencyPercentiles() (p50, p95, avg time.Duration) {
	n := st.latencyCount
	if n == 0 {
		return 0, 0, 0
	}
	var vals [latencyWindow]time.Duration
	start := (st.latencyNext - n + latencyWindow) % latencyWindow
	for i := 0; i < n; i++ {
		vals[i] = st.latencyRing[(start+i)%latencyWindow]
	}
	slices.Sort(vals[:n])
	var sum time.Duration
	for _, v := range vals[:n] {
		sum += v
	}
	// Nearest-rank percentiles: ceil(p*n/100)-1, so a single slow outlier is
	// visible in p95 while p50 stays the typical exchange.
	rank := func(p int) int {
		idx := (p*n + 99) / 100
		if idx < 1 {
			idx = 1
		}
		return idx - 1
	}
	return vals[rank(50)], vals[rank(95)], sum / time.Duration(n)
}

// Circuit breaker policy. Five consecutive transport failures is enough to stop
// paying a timeout on every miss; thirty seconds is short enough that a resolver
// coming back is picked up without an operator action. Policy rcodes never count.
const (
	circuitFailureThreshold = 5
	circuitCooldown         = 30 * time.Second
	// circuitJitter spreads the recovery of several upstreams that failed
	// together, so a network blip does not make them all probe on the same tick.
	circuitJitter = 5 * time.Second
	// circuitProbeLease bounds a half-open probe that is canceled before it can
	// record an outcome. After this long another query may try.
	circuitProbeLease = 10 * time.Second
)

func circuitOpenFor() time.Duration {
	return circuitCooldown + rand.N(circuitJitter)
}

// statOutcome separates a resolver that answered from one that could not be
// reached. REFUSED/SERVFAIL are answers even when they are filtered responses.
type statOutcome uint8

const (
	statSuccess statOutcome = iota
	statPolicyRcode
	statTransportError
)

func classifyStat(resp *dns.Msg, err error) (statOutcome, string) {
	switch {
	case err != nil:
		return statTransportError, shortError(err)
	case resp == nil:
		return statTransportError, "no response"
	case resp.Truncated:
		return statTransportError, "truncated response"
	case usableRcode(resp):
		return statSuccess, ""
	default:
		if name, ok := dns.RcodeToString[resp.Rcode]; ok {
			return statPolicyRcode, name
		}
		return statPolicyRcode, fmt.Sprintf("rcode %d", resp.Rcode)
	}
}

func shortError(err error) string {
	msg := err.Error()
	const max = 160
	if len(msg) > max {
		return msg[:max] + "…"
	}
	return msg
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

	// TLS 1.3 resumption saves a round trip even when a connection has to be
	// re-established. The cache is shared by DoT, DoH and DoQ because the key is
	// the server name, not the transport.
	sessionCache := tls.NewLRUClientSessionCache(128)

	pool := &UpstreamPool{
		upstreams:   normalized,
		timeout:     timeout,
		racing:      racing,
		ecsClientIP: parseECSClientIP(ecsIP),
		stats:       make(map[string]*upstreamStatInternal),
		dotPools:    make(map[string]*dotConnPool),
		doqConns:    make(map[string]*doqConn),
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
			Net:     "tcp-tls",
			Timeout: timeout,
			TLSConfig: &tls.Config{
				MinVersion:         tls.VersionTLS12,
				ClientSessionCache: sessionCache,
			},
		},
		// DoH posts a binary DNS message over HTTPS. Proxy is nil because an
		// upstream resolver is not a web fetch: honouring HTTP_PROXY here would
		// route every query through the operator's environment by accident, the
		// same reason the updater ignores it.
		httpClient: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				Proxy:               nil,
				TLSClientConfig:     &tls.Config{MinVersion: tls.VersionTLS12, ClientSessionCache: sessionCache},
				ForceAttemptHTTP2:   true,
				MaxIdleConns:        32,
				MaxIdleConnsPerHost: 8,
				MaxConnsPerHost:     16,
				IdleConnTimeout:     90 * time.Second,
				DisableCompression:  true,
			},
		},
		// DoH3 posts the same RFC 8484 body over HTTP/3. The http3.Transport
		// negotiates the "h3" ALPN itself and verifies the certificate like the
		// HTTP/2 client above; there is no skip-verify escape hatch here either.
		h3Client: &http.Client{
			Timeout: timeout,
			Transport: &http3.Transport{
				TLSClientConfig:    &tls.Config{MinVersion: tls.VersionTLS12, ClientSessionCache: sessionCache},
				DisableCompression: true,
			},
		},
		// DoQ reuses one QUIC connection per endpoint and opens a stream per query.
		// NextProtos carries the RFC 9250 "doq" ALPN token; ServerName is filled in
		// per dial from the upstream.
		quicTLS:  &tls.Config{MinVersion: tls.VersionTLS12, NextProtos: []string{doqALPN}, ClientSessionCache: sessionCache},
		dnscrypt: newDNSCryptTransport(timeout),
	}

	for _, u := range normalized {
		pool.stats[u] = &upstreamStatInternal{lastUpdated: time.Now()}
	}
	pool.rebuildSnapshotLocked()

	// No probe from here. The pool is usable the instant it is built; the caller
	// decides when to spend network I/O on measuring it: the daemon fires
	// BenchmarkAll once at startup, the dashboard and the TUI fire it on demand.
	// A probe launched from the constructor had no handle to cancel or await,
	// which made every test that reads these stats race an unstoppable goroutine
	// and put ~65 real DNS queries into a unit-test run.
	//
	// Until something has actually answered, an upstream reports measured=false
	// and no latency. There is deliberately no seeded "12 ms": that number was
	// indistinguishable from a real measurement and dressed an untested resolver
	// in the green badge that means "fastest".
	return pool
}

func (p *UpstreamPool) recordStat(addr string, rtt time.Duration, outcome statOutcome, reason string, probe bool) {
	p.mu.RLock()
	stat, ok := p.stats[addr]
	p.mu.RUnlock()

	if !ok {
		return
	}

	stat.mu.Lock()
	defer stat.mu.Unlock()

	now := time.Now()
	stat.lastUpdated = now
	// Any outcome releases the half-open lease: the probe has finished one way
	// or the other.
	stat.probeUntil.Store(0)
	switch outcome {
	case statSuccess:
		if probe {
			// A benchmark proves the resolver works and gives the dashboard a
			// starting latency, but it is not client traffic: it must not inflate
			// hits, the rolling live median or the recent-health ring.
			stat.probeHits++
			stat.lastProbeLatency = rtt
			stat.lastProbeOK = true
			stat.lastProbeAt = now
			break
		}
		stat.hits++
		stat.samples++
		stat.observeLatency(rtt)
		stat.observeRecent(false)
		stat.lastMeasured = now
		stat.consecutiveFailures = 0
		if stat.openUntil.Swap(0) != 0 {
			p.openCircuits.Add(-1)
			log.Printf("[Upstream] Circuit closed for %s after a successful probe", addr)
		}
	case statPolicyRcode:
		// The resolver answered. A filtered REFUSED is worth showing, but it is
		// not a reason to stop asking this resolver anything at all.
		if probe {
			stat.probePolicy++
			stat.lastProbeOK = false
			stat.lastProbeAt = now
			break
		}
		stat.policyErrors++
		// Reachable, so it counts as a good outcome for the health dot even
		// though the rcode is not a usable answer.
		stat.observeRecent(false)
		stat.consecutiveFailures = 0
		if stat.openUntil.Swap(0) != 0 {
			p.openCircuits.Add(-1)
			log.Printf("[Upstream] Circuit closed for %s after a policy response", addr)
		}
		stat.lastError = reason
		stat.lastErrorAt = now
	case statTransportError:
		if probe {
			// A failed startup probe says the network was bad at that moment; it
			// must not open the breaker or colour the live card for its lifetime.
			stat.probeErrors++
			stat.lastProbeOK = false
			stat.lastProbeAt = now
			break
		}
		stat.transportErrors++
		stat.observeRecent(true)
		stat.consecutiveFailures++
		stat.lastError = reason
		stat.lastErrorAt = now
		if stat.consecutiveFailures >= circuitFailureThreshold {
			oldUntil := stat.openUntil.Load()
			wasOpen := oldUntil != 0
			halfOpen := wasOpen && now.UnixNano() >= oldUntil
			stat.openUntil.Store(now.Add(circuitOpenFor()).UnixNano())
			switch {
			case !wasOpen:
				p.openCircuits.Add(1)
				log.Printf("[Upstream] Circuit open for %s after %d consecutive failures: %s",
					addr, stat.consecutiveFailures, reason)
			case halfOpen:
				log.Printf("[Upstream] Circuit re-opened for %s after a failed probe: %s", addr, reason)
			}
		}
	}
	stat.errors = stat.transportErrors + stat.policyErrors
}

// upstreamAvailable reports whether an upstream is outside its breaker cooldown.
// It reads only the atomic openUntil, so the query path can test every candidate
// without taking any per-stat mutex.
func (p *UpstreamPool) upstreamAvailable(addr string, now time.Time) bool {
	p.mu.RLock()
	stat := p.stats[addr]
	p.mu.RUnlock()
	if stat == nil {
		return true
	}
	until := stat.openUntil.Load()
	if until == 0 {
		return true
	}
	nowNs := now.UnixNano()
	if nowNs < until {
		return false
	}
	// The cooldown has expired: allow exactly one half-open probe. A query that
	// claims the lease is the only one that will reach the resolver; the rest
	// keep skipping it until the probe records an outcome. A probe canceled by a
	// race winner is released when its lease expires.
	if lease := stat.probeUntil.Load(); lease != 0 {
		if nowNs < lease {
			return false
		}
		stat.probeUntil.CompareAndSwap(lease, 0)
	}
	return stat.probeUntil.CompareAndSwap(0, now.Add(circuitProbeLease).UnixNano())
}

// availableCandidates drops resolvers whose breaker is open. If every candidate
// is open, the original list is returned: a resolver that recovered must get a
// chance to close its own circuit, and returning an empty pool would turn one
// temporary outage into SERVFAIL for every client.
func (p *UpstreamPool) availableCandidates(list []string) []string {
	// Common case: every breaker is closed. Return the immutable snapshot list
	// without a single stat lock.
	if p.openCircuits.Load() == 0 {
		return list
	}
	now := time.Now()
	out := make([]string, 0, len(list))
	for _, u := range list {
		if p.upstreamAvailable(u, now) {
			out = append(out, u)
		}
	}
	if len(out) == 0 || len(out) == len(list) {
		return list
	}
	return out
}

// BenchmarkAll probes every upstream once and folds the round-trip time into its
// latency estimate. Only a resolver that actually answers the probe counts:
// scoring a refusal by its round-trip time alone ranked an instantly-refusing
// resolver as the fastest in the pool, which is exactly the one the dashboard
// should not be recommending — and the same goes for one that answers a question
// nobody asked.
func (p *UpstreamPool) BenchmarkAll() {
	p.mu.RLock()
	servers := make([]string, len(p.upstreams))
	copy(servers, p.upstreams)
	p.mu.RUnlock()

	var wg sync.WaitGroup
	for _, s := range servers {
		wg.Add(1)
		go func(addr string) {
			defer wg.Done()
			p.BenchmarkUpstream(addr)
		}(s)
	}
	wg.Wait()
}

// benchmarkSamples is the number of warm measurements a benchmark takes. The
// first query is a discarded warm-up: DoT/DoH/DoQ/DoH3 include a TCP/TLS/QUIC/
// HTTP handshake, and reporting that cold round trip as the resolver's latency
// made fresh benchmarks look much slower than the same resolver on a re-run.
const benchmarkSamples = 3

// BenchmarkUpstream probes one resolver through the same per-scheme path a live
// query takes. It is used after an add so a newly configured resolver has a
// latency on the next stats poll instead of staying "not measured" until the
// next manual/startup benchmark.
func (p *UpstreamPool) BenchmarkUpstream(addr string) {
	if p == nil {
		return
	}
	defer safego.Recover("upstream.benchmark")
	canonical, ok := NormalizeUpstream(addr)
	if !ok {
		return
	}

	p.mu.RLock()
	_, exists := p.stats[canonical]
	timeout := p.timeout
	p.mu.RUnlock()
	if !exists {
		// It was removed while the add response was being written; recording a
		// stat for it would resurrect a resolver that is no longer configured.
		return
	}

	m := new(dns.Msg)
	m.SetQuestion("cloudflare.com.", dns.TypeA)

	// Warm-up. Its result is deliberately not recorded; it only establishes the
	// connection/caches that the measured probes should reuse.
	p.benchmarkQuery(canonical, m, timeout)

	samples := make([]time.Duration, 0, benchmarkSamples)
	failures := 0
	failureRTT := time.Duration(0)
	failureOutcome := statTransportError
	failureReason := "benchmark produced no usable answer"

	for range benchmarkSamples {
		rtt, outcome, reason := p.benchmarkQuery(canonical, m, timeout)
		if outcome == statSuccess {
			samples = append(samples, rtt)
		} else {
			failures++
			failureRTT, failureOutcome, failureReason = rtt, outcome, reason
		}
	}

	// Require a majority of the measured probes to answer. A single lucky packet
	// must not hide a resolver that failed two of three attempts, and the last
	// attempt being the lucky one must not turn the whole benchmark green.
	if len(samples) < 2 {
		p.recordStat(canonical, failureRTT, failureOutcome, failureReason, true)
		return
	}
	if failures > 0 {
		// Keep the partial-failure evidence in the probe counters, but the
		// majority answer decides the probe verdict and latency.
		p.recordStat(canonical, failureRTT, failureOutcome, failureReason, true)
	}
	p.recordStat(canonical, medianDuration(samples), statSuccess, "", true)
}

// benchmarkQuery sends one probe through the same per-scheme path a live query
// takes, with its own timeout so a failed warm-up cannot consume the budget for
// the measured samples.
func (p *UpstreamPool) benchmarkQuery(addr string, m *dns.Msg, timeout time.Duration) (time.Duration, statOutcome, string) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	resp, rtt, err := p.safeQueryOne(ctx, m, addr)
	outcome, reason := classifyStat(resp, err)
	return rtt, outcome, reason
}

func medianDuration(values []time.Duration) time.Duration {
	if len(values) == 0 {
		return 0
	}
	slices.Sort(values)
	middle := len(values) / 2
	if len(values)%2 == 1 {
		return values[middle]
	}
	return (values[middle-1] + values[middle]) / 2
}

func (p *UpstreamPool) Exchange(req *dns.Msg) (*dns.Msg, time.Duration, string, error) {
	return p.ExchangeFor(req, "")
}

// DegradedUpstreams reports how many configured upstreams the health monitor
// currently considers degraded. It shares upstreamHealthIssue with the monitor,
// so the Prometheus gauge and the log warning cannot disagree about what
// "degraded" means.
func (p *UpstreamPool) DegradedUpstreams() int {
	if p == nil {
		return 0
	}
	degraded := 0
	for _, s := range p.GetUpstreamStats() {
		if _, bad := upstreamHealthIssue(s); bad {
			degraded++
		}
	}
	return degraded
}

// Health monitoring. The startup benchmark gives every upstream a latency and a
// one-off probe verdict; without a slow repeat the dashboard's probe column goes
// stale and a resolver that quietly starts failing is only visible in the logs
// of the queries that happened to pick it. These constants keep the repeat cheap
// (one probe per upstream every five minutes) and make "degraded" mean either an
// open breaker, a failed latest probe, or a recent live error rate that is too
// high to be ordinary noise.
const (
	upstreamHealthInterval   = 5 * time.Minute
	upstreamHealthMinSamples = 20
	upstreamHealthErrorRate  = 0.10
)

// RunHealthMonitor re-probes every upstream on a slow timer and logs one warning
// per degraded upstream plus one recovery line. It returns a stop function; the
// daemon defers it so the goroutine cannot outlive the pool.
func (p *UpstreamPool) RunHealthMonitor(interval time.Duration) (stop func()) {
	if interval <= 0 {
		interval = upstreamHealthInterval
	}
	done := make(chan struct{})
	var once sync.Once
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		alerted := make(map[string]bool)
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				p.BenchmarkAll()
				p.reportUpstreamHealth(alerted)
			}
		}
	}()
	return func() { once.Do(func() { close(done) }) }
}

// reportUpstreamHealth logs state transitions, not every tick: an operator gets
// one line when an upstream starts failing and one when it recovers. alerted is
// owned by the monitor goroutine.
func (p *UpstreamPool) reportUpstreamHealth(alerted map[string]bool) {
	for _, s := range p.GetUpstreamStats() {
		reason, degraded := upstreamHealthIssue(s)
		switch {
		case degraded && !alerted[s.Address]:
			alerted[s.Address] = true
			log.Printf("[Upstream] Health warning: %s — %s (last error: %s)", s.Address, reason, s.LastError)
		case !degraded && alerted[s.Address]:
			delete(alerted, s.Address)
			log.Printf("[Upstream] Health recovered: %s (p50 %.1fms)", s.Address, s.LatencyP50Ms)
		}
	}
}

// upstreamHealthIssue is the single definition of "degraded" used by the monitor
// and its tests. A probe that has never run is not a verdict, so an empty
// LastProbeAt cannot mark a fresh upstream unhealthy.
func upstreamHealthIssue(s UpstreamStat) (string, bool) {
	switch {
	case s.CircuitOpen:
		return "circuit open", true
	case s.LastProbeAt != "" && !s.LastProbeOK:
		return "latest probe failed", true
	case s.RecentSamples >= upstreamHealthMinSamples && s.RecentErrorRate >= upstreamHealthErrorRate:
		return fmt.Sprintf("recent error rate %.0f%% over %d queries", s.RecentErrorRate*100, s.RecentSamples), true
	default:
		return "", false
	}
}

// ExchangeFor resolves req under the profile configured for domain (empty means
// no domain — the global policy only). It is the only entry point the DNS handler
// uses; Exchange stays as the profile-less form for benchmark and test callers.
//
// Selection order:
//  1. The configured pool, filtered to the transport the profile chose, or the
//     global one when the profile inherits it.
//  2. A pinned upstream, when it is in that eligible set. A pin narrows the
//     policy; it never overrides the transport, so a pin the transport excludes
//     is ignored.
//
// A profile that produces no candidates, or whose candidates all fail, falls
// back to the ordinary pool. The fallback never bypasses a global transport
// setting: it is the globally eligible pool, not the raw list. The retired
// per-profile "fail" choice is read as pool by Normalized.
func (p *UpstreamPool) ExchangeFor(req *dns.Msg, domain string) (*dns.Msg, time.Duration, string, error) {
	if req == nil {
		return nil, 0, "", errors.New("nil dns request")
	}

	snap := p.snap.Load()
	if snap == nil {
		return nil, 0, "", errors.New("upstream pool is not initialized")
	}
	upstreams := snap.upstreams
	racing := snap.racing
	timeout := p.timeout
	ecs := p.ecsClientIP
	policy := snap.policy
	profileIndex := snap.profileIndex

	if len(upstreams) == 0 {
		return nil, 0, "", errors.New("no upstream resolvers configured")
	}

	queryMsg := req
	if ecs != nil {
		queryMsg = req.Copy()
		p.attachECS(queryMsg, ecs)
	}

	profile, hasProfile := profileIndex.Match(domain)
	effectiveTransport := policy.Transport
	if hasProfile {
		effectiveTransport = profile.TransportFor(policy.Transport)
	}

	// The fallback pool is the globally eligible one, never the raw list and
	// never the pinned default — "fall back" means the ordinary pool. It never
	// bypasses the global transport either: an encrypted-only install stays
	// encrypted-only even when a profile's own candidates fail.
	fallbackCandidates := p.availableCandidates(snap.candidates(policy.Transport))

	candidates := p.availableCandidates(snap.candidates(effectiveTransport))
	strategy := poolStrategy(racing)
	switch policy.DefaultStrategy {
	case resolution.StrategyFastest, resolution.StrategySequential,
		resolution.StrategyHedged, resolution.StrategyBestKnown:
		strategy = policy.DefaultStrategy
	}

	// A profile pin outranks the global default; the global default outranks the
	// pool. The transport is the policy and a pin only narrows it: a pin the
	// transport excludes is ignored rather than used, so "encrypted only" can
	// never be bypassed by a stale or hand-written pin. The UI disables such a
	// choice, and the card warns when an existing pin is being ignored.
	pinned := ""
	if hasProfile && profile.Upstream != "" {
		pinned = profile.Upstream
	} else if policy.DefaultUpstream != "" {
		pinned = policy.DefaultUpstream
	}
	candidates = pinCandidate(candidates, pinned)
	if hasProfile {
		switch profile.Strategy {
		case resolution.StrategyFastest, resolution.StrategySequential,
			resolution.StrategyHedged, resolution.StrategyBestKnown:
			strategy = profile.Strategy
		}
	}

	var (
		lastResp     *dns.Msg
		lastDur      time.Duration
		lastUpstream string
		lastErr      error
	)
	if len(candidates) > 0 {
		resp, dur, upstream, err := p.exchangeWith(queryMsg, candidates, strategy, timeout)
		if err == nil && usableRcode(resp) {
			return resp, dur, upstream, nil
		}
		// A REFUSED, SERVFAIL or truncated reply is not a resolution: it is
		// exactly the case the fallback exists for. Keep the upstream's verdict
		// so it can still be returned when there is no distinct pool to try.
		lastResp, lastDur, lastUpstream = resp, dur, upstream
		if err != nil {
			lastErr = err
		} else if resp != nil {
			lastErr = errors.New("upstream returned " + dns.RcodeToString[resp.Rcode])
		} else {
			lastErr = errors.New("upstream returned no response")
		}
	}

	// Every resolution path now falls back to the pool: a global default pin, a
	// per-domain profile, or a profile transport that produced nothing. The old
	// per-domain "fail instead of fall back" choice is retired, so trying the
	// default route is the only fallback behaviour.
	distinctFallback := len(fallbackCandidates) > 0 && !sameUpstreams(candidates, fallbackCandidates)
	if !distinctFallback {
		if lastResp != nil {
			// The pool itself answered (or refused) as a unit; hand its verdict
			// back rather than inventing a SERVFAIL.
			return lastResp, lastDur, lastUpstream, nil
		}
		if len(candidates) == 0 {
			return nil, 0, "", errors.New("no upstream matches the resolution profile for " + domain)
		}
		return nil, 0, "", lastErr
	}

	resp, dur, upstream, err := p.exchangeWith(queryMsg, fallbackCandidates, poolStrategy(racing), timeout)
	if err == nil {
		return resp, dur, upstream, nil
	}
	return nil, 0, "", err
}

// poolStrategy is the strategy represented by the legacy racing flag. Hedged is
// the default when racing is enabled: it keeps the latency benefit of asking the
// second resolver when the first is slow, without the N-way query amplification
// and N goroutines that Fastest spends on every cache miss.
func poolStrategy(racing bool) string {
	if racing {
		return resolution.StrategyHedged
	}
	return resolution.StrategySequential
}

// exchangeWith runs one attempt list under the requested strategy.
func (p *UpstreamPool) exchangeWith(req *dns.Msg, upstreams []string, strategy string, timeout time.Duration) (*dns.Msg, time.Duration, string, error) {
	if len(upstreams) == 0 {
		return nil, 0, "", errors.New("no upstream resolvers configured")
	}
	switch strategy {
	case resolution.StrategySequential:
		return p.exchangeSequential(req, upstreams, timeout)
	case resolution.StrategyHedged:
		if len(upstreams) > 1 {
			return p.exchangeHedged(req, upstreams, timeout)
		}
		return p.exchangeSequential(req, upstreams, timeout)
	case resolution.StrategyBestKnown:
		return p.exchangeBestKnown(req, upstreams, timeout)
	default:
		if len(upstreams) > 1 {
			return p.exchangeRacing(req, upstreams, timeout)
		}
		return p.exchangeSequential(req, upstreams, timeout)
	}
}

// hedgeStartDelay is how long the fastest measured resolver gets before Hedged
// starts the next one. It is intentionally a constant: the strategy is a
// per-query trade-off, not another setting to tune on every install.
const hedgeStartDelay = 50 * time.Millisecond

// orderedByLatency returns a copy of upstreams with the fastest measured
// resolver first. Untimed resolvers keep their configured order after the known
// ones. The live stats are the same numbers the resolver card shows.
func (p *UpstreamPool) orderedByLatency(upstreams []string) []string {
	if len(upstreams) <= 1 {
		return upstreams
	}

	p.orderMu.Lock()
	defer p.orderMu.Unlock()
	if !p.orderBuilt.IsZero() && time.Since(p.orderBuilt) < time.Second && sameUpstreams(p.orderFor, upstreams) {
		return p.order
	}

	stats := p.GetUpstreamStats()
	latency := make(map[string]time.Duration, len(stats))
	for _, s := range stats {
		latency[s.Address] = time.Duration(s.LatencyNs)
	}
	out := slices.Clone(upstreams)
	slices.SortStableFunc(out, func(a, b string) int {
		la, lb := latency[a], latency[b]
		switch {
		case la == 0 && lb == 0:
			return 0
		case la == 0:
			return 1
		case lb == 0:
			return -1
		default:
			return cmp.Compare(la, lb)
		}
	})
	p.orderFor = slices.Clone(upstreams)
	p.order = out
	p.orderBuilt = time.Now()
	return out
}

// exchangeBestKnown sends to the fastest measured resolver only, falling through
// the rest in latency order when it fails. It costs one query at a time and
// accepts a full timeout on the first resolver, which is the deliberate trade
// against Hedged's early second query.
func (p *UpstreamPool) exchangeBestKnown(req *dns.Msg, upstreams []string, timeout time.Duration) (*dns.Msg, time.Duration, string, error) {
	return p.exchangeSequential(req, p.orderedByLatency(upstreams), timeout)
}

// exchangeHedged starts the fastest measured resolver, then starts the next one
// every hedgeStartDelay until a usable answer arrives. A hard failure moves on
// immediately. This gets most of the latency benefit of racing while sending
// exactly one query when the first resolver is fast.
func (p *UpstreamPool) exchangeHedged(req *dns.Msg, upstreams []string, timeout time.Duration) (*dns.Msg, time.Duration, string, error) {
	if len(upstreams) <= 1 {
		return p.exchangeSequential(req, upstreams, timeout)
	}

	order := p.orderedByLatency(upstreams)
	type result struct {
		resp     *dns.Msg
		duration time.Duration
		upstream string
		err      error
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	resChan := make(chan result, len(order))

	start := func(addr string) {
		go func() {
			defer safego.Recover("upstream.hedged")
			resp, rtt, err := p.safeQueryOne(ctx, req, addr)
			// A canceled loser is not a resolver failure; a real error is.
			if err == nil || !errors.Is(err, context.Canceled) {
				outcome, reason := classifyStat(resp, err)
				// Record the transport round trip, not the goroutine's wall time:
				// scheduling and race bookkeeping are not upstream latency.
				p.recordStat(addr, rtt, outcome, reason, false)
			}
			select {
			case resChan <- result{resp: resp, duration: rtt, upstream: addr, err: err}:
			case <-ctx.Done():
			}
		}()
	}

	started, inFlight := 0, 0
	launch := func() {
		if started >= len(order) {
			return
		}
		start(order[started])
		started++
		inFlight++
	}

	launch()
	timer := time.NewTimer(hedgeStartDelay)
	defer timer.Stop()
	resetTimer := func() {
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer.Reset(hedgeStartDelay)
	}

	var (
		lastErr  error
		fallback *result
	)
collect:
	for {
		if inFlight == 0 && started == len(order) {
			break
		}
		select {
		case res := <-resChan:
			inFlight--
			if res.err == nil && usableRcode(res.resp) {
				cancel()
				return res.resp, res.duration, res.upstream, nil
			}
			if res.err != nil {
				lastErr = res.err
			} else if fallback == nil && res.resp != nil {
				held := res
				fallback = &held
			}
			// A failed resolver should not make the client wait for the hedge
			// timer before the next one starts.
			if started < len(order) {
				launch()
				resetTimer()
			}
		case <-timer.C:
			if started < len(order) {
				launch()
				timer.Reset(hedgeStartDelay)
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

// pinCandidate narrows the eligible list to the pinned resolver. A pin the
// transport excluded — or one that does not normalise — leaves the eligible pool
// untouched, because the transport is the policy and the pin only narrows it.
func pinCandidate(eligible []string, pinned string) []string {
	if pinned == "" {
		return eligible
	}
	canonical, ok := normalizeUpstream(pinned)
	if !ok || !slices.Contains(eligible, canonical) {
		return eligible
	}
	return []string{canonical}
}

// ProtocolOf returns the transport a canonical resolver address uses. A bare
// host:port is plain UDP (the historical default), tcp:// is TCP, and the
// encrypted schemes map to the names the live log shows. "cache" and empty
// answers have no resolution transport and return "".
func ProtocolOf(address string) string {
	a := strings.ToLower(strings.TrimSpace(address))
	switch {
	case a == "" || a == "cache":
		return ""
	case strings.HasPrefix(a, "tcp://"):
		return "TCP"
	case strings.HasPrefix(a, "tls://"):
		return "DoT"
	case strings.HasPrefix(a, "https://"):
		return "DoH"
	case strings.HasPrefix(a, "quic://"), strings.HasPrefix(a, "doq://"):
		return "DoQ"
	case strings.HasPrefix(a, "h3://"):
		return "DoH3"
	case strings.HasPrefix(a, "sdns://"):
		return "DNSCrypt"
	default:
		return "UDP"
	}
}

// eligibleUpstreams applies a transport filter to a candidate list.
func eligibleUpstreams(list []string, transport string) []string {
	switch resolution.NormalizeTransport(transport) {
	case resolution.TransportEncrypted:
		out := make([]string, 0, len(list))
		for _, u := range list {
			if resolution.Encrypted(u) {
				out = append(out, u)
			}
		}
		return out
	case resolution.TransportRegular:
		out := make([]string, 0, len(list))
		for _, u := range list {
			if !resolution.Encrypted(u) {
				out = append(out, u)
			}
		}
		return out
	default:
		return list
	}
}

func sameUpstreams(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
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
		resp, rtt, err = p.exchangeDoT(ctx, q, endpoint)
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
// safeQueryOne is queryOne behind a panic boundary. The racing and hedged paths
// run each exchange in its own goroutine; without this, a panic anywhere in a
// transport or in message parsing would terminate the daemon instead of failing
// one query. A panic is reported as a transport error so the caller records it
// and tries the next candidate.
func (p *UpstreamPool) safeQueryOne(ctx context.Context, req *dns.Msg, addr string) (resp *dns.Msg, rtt time.Duration, err error) {
	defer func() {
		if rec := recover(); rec != nil {
			safego.Handle("upstream.query", rec)
			resp = nil
			rtt = 0
			err = fmt.Errorf("upstream %s panicked: %v", addr, rec)
		}
	}()
	return p.queryOne(ctx, req, addr)
}

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
			defer safego.Recover("upstream.racing")
			resp, rtt, err := p.safeQueryOne(ctx, req, addr)

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
				outcome, reason := classifyStat(resp, err)
				// The transport RTT, not goroutine wall time.
				p.recordStat(addr, rtt, outcome, reason, false)
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
		resp, rtt, err := p.safeQueryOne(ctx, req, u)
		outcome, reason := classifyStat(resp, err)
		ok := outcome == statSuccess
		p.recordStat(u, rtt, outcome, reason, false)
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
//
// LatencyMs is the median of the last latencyWindow successful live exchanges
// (or, before any live traffic, the last successful probe). It is 0 and
// Measured is false when nothing has answered; clients must not render a
// fabricated value in that case.
type UpstreamStat struct {
	Address        string  `json:"address"`
	LatencyNs      int64   `json:"latency"` // nanoseconds (frontend divides by 1e6)
	LatencyMs      float64 `json:"latency_ms"`
	LatencyP50Ms   float64 `json:"latency_p50_ms"`
	LatencyP95Ms   float64 `json:"latency_p95_ms"`
	LatencyAvgMs   float64 `json:"latency_avg_ms"`
	Measured       bool    `json:"measured"`
	Samples        uint64  `json:"samples"`
	LastMeasuredAt string  `json:"last_measured_at,omitempty"`
	// LatencyIsProbe is true while the displayed number comes from the warm
	// benchmark median because there are not yet enough live samples to trust a
	// live p50. One slow live exchange must not dominate the card.
	LatencyIsProbe      bool    `json:"latency_is_probe,omitempty"`
	Hits                uint64  `json:"hits"`
	Errors              uint64  `json:"errors"`
	TransportErrors     uint64  `json:"transport_errors"`
	PolicyErrors        uint64  `json:"policy_errors"`
	RecentSamples       int     `json:"recent_samples"`
	RecentErrorRate     float64 `json:"recent_error_rate"`
	ConsecutiveFailures uint32  `json:"consecutive_failures"`
	CircuitOpen         bool    `json:"circuit_open"`
	// CircuitState is closed, open, or half-open. half-open means the cooldown
	// has expired and exactly one probe is allowed; it is not a healthy state
	// yet, but it is no longer being skipped.
	CircuitState string `json:"circuit_state"`
	LastError    string `json:"last_error,omitempty"`
	LastErrorAt  string `json:"last_error_at,omitempty"`
	// Probe counters are the startup/explicit benchmark, kept apart from live
	// traffic so a single bad probe does not make the card look broken.
	ProbeHits   uint64  `json:"probe_hits"`
	ProbeErrors uint64  `json:"probe_errors"`
	ProbePolicy uint64  `json:"probe_policy"`
	LastProbeMs float64 `json:"last_probe_ms"`
	LastProbeOK bool    `json:"last_probe_ok"`
	LastProbeAt string  `json:"last_probe_at,omitempty"`
}

// minLiveLatencySamples is the number of successful live exchanges needed
// before the card trusts a live p50 over the warm benchmark median. A single
// slow query (TCP fallback, DNSSEC retry, one congested packet) otherwise made a
// healthy resolver look broken until enough traffic arrived to move the median.
const minLiveLatencySamples = 3

// GetUpstreamStats returns a snapshot of latency/health stats for all upstreams, sorted by latency.
func (p *UpstreamPool) GetUpstreamStats() []UpstreamStat {
	type snapshot struct {
		latency             time.Duration
		latencyP95          time.Duration
		latencyAvg          time.Duration
		samples             uint64
		measuredAt          time.Time
		hits                uint64
		errors              uint64
		transportErrors     uint64
		policyErrors        uint64
		recentLen           int
		recentFailures      int
		consecutiveFailures uint32
		lastError           string
		lastErrorAt         time.Time
		openUntil           int64
		probeHits           uint64
		probeErrors         uint64
		probePolicy         uint64
		lastProbeLatency    time.Duration
		lastProbeOK         bool
		lastProbeAt         time.Time
	}

	p.mu.RLock()
	addrs := make([]string, len(p.upstreams))
	copy(addrs, p.upstreams)
	stats := make(map[string]snapshot, len(p.stats))
	for addr, st := range p.stats {
		st.mu.Lock()
		p50, p95, avg := st.latencyPercentiles()
		stats[addr] = snapshot{
			latency:             p50,
			latencyP95:          p95,
			latencyAvg:          avg,
			samples:             st.samples,
			measuredAt:          st.lastMeasured,
			hits:                st.hits,
			errors:              st.errors,
			transportErrors:     st.transportErrors,
			policyErrors:        st.policyErrors,
			recentLen:           st.recentLen,
			recentFailures:      st.recentFailures,
			consecutiveFailures: st.consecutiveFailures,
			lastError:           st.lastError,
			lastErrorAt:         st.lastErrorAt,
			openUntil:           st.openUntil.Load(),
			probeHits:           st.probeHits,
			probeErrors:         st.probeErrors,
			probePolicy:         st.probePolicy,
			lastProbeLatency:    st.lastProbeLatency,
			lastProbeOK:         st.lastProbeOK,
			lastProbeAt:         st.lastProbeAt,
		}
		st.mu.Unlock()
	}
	p.mu.RUnlock()

	now := time.Now()
	out := make([]UpstreamStat, 0, len(addrs))
	for _, addr := range addrs {
		// No seeded latency. An upstream that has never answered is honestly
		// unmeasured; the dashboard renders "—", not a fake 12 ms.
		s := UpstreamStat{Address: addr, CircuitState: "closed"}
		if raw, ok := stats[addr]; ok {
			s.Hits = raw.hits
			s.Errors = raw.errors
			s.TransportErrors = raw.transportErrors
			s.PolicyErrors = raw.policyErrors
			s.ConsecutiveFailures = raw.consecutiveFailures
			s.CircuitOpen = raw.openUntil != 0 && now.UnixNano() < raw.openUntil
			switch {
			case raw.openUntil == 0:
				s.CircuitState = "closed"
			case now.UnixNano() < raw.openUntil:
				s.CircuitState = "open"
			default:
				s.CircuitState = "half-open"
			}
			s.LastError = raw.lastError
			if !raw.lastErrorAt.IsZero() {
				s.LastErrorAt = raw.lastErrorAt.UTC().Format(time.RFC3339)
			}
			s.Samples = raw.samples
			switch {
			case raw.samples >= minLiveLatencySamples:
				s.Measured = true
				s.LatencyNs = int64(raw.latency)
				s.LatencyP50Ms = float64(raw.latency) / 1e6
				s.LatencyP95Ms = float64(raw.latencyP95) / 1e6
				s.LatencyAvgMs = float64(raw.latencyAvg) / 1e6
				if !raw.measuredAt.IsZero() {
					s.LastMeasuredAt = raw.measuredAt.UTC().Format(time.RFC3339)
				}
			case raw.lastProbeOK && raw.lastProbeLatency > 0:
				// Not enough live exchanges yet (or none): show the warm
				// benchmark median. The UI marks it as a probe value.
				s.Measured = true
				s.LatencyIsProbe = true
				s.LatencyNs = int64(raw.lastProbeLatency)
				s.LatencyP50Ms = float64(raw.lastProbeLatency) / 1e6
				s.LatencyP95Ms = s.LatencyP50Ms
				s.LatencyAvgMs = s.LatencyP50Ms
			case raw.samples > 0:
				// No probe reference is available (older server / probe never
				// succeeded), so fall back to the live median we do have.
				s.Measured = true
				s.LatencyNs = int64(raw.latency)
				s.LatencyP50Ms = float64(raw.latency) / 1e6
				s.LatencyP95Ms = float64(raw.latencyP95) / 1e6
				s.LatencyAvgMs = float64(raw.latencyAvg) / 1e6
				if !raw.measuredAt.IsZero() {
					s.LastMeasuredAt = raw.measuredAt.UTC().Format(time.RFC3339)
				}
			}
			s.RecentSamples = raw.recentLen
			if raw.recentLen > 0 {
				s.RecentErrorRate = float64(raw.recentFailures) / float64(raw.recentLen)
			}
			s.ProbeHits = raw.probeHits
			s.ProbeErrors = raw.probeErrors
			s.ProbePolicy = raw.probePolicy
			s.LastProbeOK = raw.lastProbeOK
			if raw.lastProbeLatency > 0 {
				s.LastProbeMs = float64(raw.lastProbeLatency) / 1e6
			}
			if !raw.lastProbeAt.IsZero() {
				s.LastProbeAt = raw.lastProbeAt.UTC().Format(time.RFC3339)
			}
		}
		s.LatencyMs = float64(s.LatencyNs) / 1e6
		out = append(out, s)
	}

	slices.SortFunc(out, func(a, b UpstreamStat) int {
		if a.LatencyNs != b.LatencyNs {
			return cmp.Compare(a.LatencyNs, b.LatencyNs)
		}
		// Equal latencies are the normal state before the first measurement —
		// every unmeasured upstream reports 0 — and an unstable sort made the
		// dashboard's resolver list reshuffle itself on every refresh.
		return strings.Compare(a.Address, b.Address)
	})
	return out
}

// SetPolicy installs the global defaults and the per-domain profiles. It copies
// the map, so callers may keep and mutate their own after the call.
func (p *UpstreamPool) SetPolicy(policy Policy) {
	copied := make(map[string]resolution.Profile, len(policy.Profiles))
	for k, v := range policy.Profiles {
		copied[k] = v
	}
	policy.Profiles = copied
	index := resolution.CompileProfiles(copied)
	p.mu.Lock()
	p.policy = policy
	p.profileIndex = index
	p.rebuildSnapshotLocked()
	p.mu.Unlock()
}

// rebuildSnapshotLocked publishes a new immutable query-path snapshot. Callers
// hold p.mu.
func (p *UpstreamPool) rebuildSnapshotLocked() {
	snap := &poolSnapshot{
		upstreams:    append([]string(nil), p.upstreams...),
		policy:       p.policy,
		profileIndex: p.profileIndex,
		racing:       p.racing,
	}
	snap.all = snap.upstreams
	for _, u := range snap.upstreams {
		if resolution.Encrypted(u) {
			snap.encrypted = append(snap.encrypted, u)
		} else {
			snap.regular = append(snap.regular, u)
		}
	}
	if snap.encrypted == nil {
		snap.encrypted = []string{}
	}
	if snap.regular == nil {
		snap.regular = []string{}
	}
	p.snap.Store(snap)
}

// SetUpstreams atomically replaces the upstream server list (thread-safe, live reload).
func (p *UpstreamPool) SetUpstreams(list []string) {
	normalized := normalizeUpstreams(list)

	p.mu.Lock()
	removed := make([]string, 0, len(p.upstreams))
	keep := make(map[string]struct{}, len(normalized))
	for _, u := range normalized {
		keep[u] = struct{}{}
	}
	for _, u := range p.upstreams {
		if _, ok := keep[u]; !ok {
			removed = append(removed, u)
		}
	}

	p.upstreams = normalized
	for _, u := range p.upstreams {
		if _, ok := p.stats[u]; !ok {
			p.stats[u] = &upstreamStatInternal{lastUpdated: time.Now()}
		}
	}
	// Drop stats for removed upstreams, keeping the open-circuit count honest.
	for addr, st := range p.stats {
		if _, ok := keep[addr]; !ok {
			if st.openUntil.Swap(0) != 0 {
				p.openCircuits.Add(-1)
			}
			delete(p.stats, addr)
		}
	}
	p.rebuildSnapshotLocked()
	p.mu.Unlock()

	for _, u := range removed {
		p.closeConnectionPool(u)
	}
}

// GetUpstreams returns a copy of the active upstream list.
func (p *UpstreamPool) GetUpstreams() []string {
	snap := p.snap.Load()
	if snap == nil {
		return nil
	}
	out := make([]string, len(snap.upstreams))
	copy(out, snap.upstreams)
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
