package proxy

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"log"
	"net"
	"net/netip"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"digitdns/internal/database"
	"digitdns/internal/safego"
)

const (
	// handshakeBufSize holds the opening TLS record or HTTP request line. A
	// ClientHello carrying post-quantum key shares runs past 2 KiB, and it may
	// arrive split across TCP segments, so the buffer covers a full TLS record
	// and the read below loops until the record is complete.
	handshakeBufSize = 16 * 1024
	// 64 KiB halves the read/write syscalls per byte compared with 32 KiB. The
	// buffers are pooled, so this is two pages, not two allocations per relay.
	relayBufSize = 64 * 1024

	// maxConcurrentRelays caps how many relays run at once. Each one holds two
	// sockets, two goroutines and a relay buffer, and nothing in the protocol
	// stops a client from opening connections until the host runs out of file
	// descriptors.
	maxConcurrentRelays = 4096

	// handshakeTimeout bounds how long a connection may sit without producing a
	// parseable ClientHello or Host header.
	handshakeTimeout = 5 * time.Second

	// defaultIdleTimeout applies when settings.Timeout is unset, so a relay is
	// never left without a deadline.
	defaultIdleTimeout = 120 * time.Second

	// meterFlushBytes is how much one direction of a relay accumulates before it
	// reports usage. Accounting only at close would leave a long-lived stream
	// invisible until it ended, so a quota could be overshot by however much a
	// single connection managed to carry.
	meterFlushBytes = 1 << 20

	// unreadableLogSample is how often a connection that named no destination is
	// logged. One line per connection would let a port scanner — or a load
	// balancer's TCP health check, which connects and says nothing — fill the log,
	// so the event is sampled instead of dropped or reported in full.
	unreadableLogSample = 64
	// Malformed-probe penalty box. Eight bad handshakes in a minute is far
	// above anything a real client produces, and dropping that source for a
	// minute stops a scanner before it can park relay slots in handshake reads.
	malformedWindow    = time.Minute
	malformedThreshold = 8
	floodCooldown      = time.Minute
	floodMapLimit      = 10000
)

var (
	handshakeBufPool = sync.Pool{
		New: func() any {
			b := make([]byte, handshakeBufSize)
			return &b
		},
	}
	relayBufPool = sync.Pool{
		New: func() any {
			b := make([]byte, relayBufSize)
			return &b
		},
	}
)

type AccessValidator interface {
	IsIPAllowed(ip string) (*database.Client, bool)
	IsAllowAll() bool
}

// TrafficMeter accounts relayed bytes against a client and reports whether that
// client has spent its allowance. It is optional: NewServer asks the access
// validator for it by type assertion, so a validator that only knows about IP
// checks keeps compiling and simply meters nothing.
type TrafficMeter interface {
	AddTraffic(clientID string, n uint64)
	QuotaExceeded(c *database.Client) bool
}

// ChallengeResponder answers an ACME HTTP-01 challenge on the proxy's plain
// HTTP port. SetChallengeResponder wires it (nil on installs with no ACME);
// the narrowness is deliberate — it is the only thing this interface can do,
// so wiring it can never widen into a general "answer HTTP locally" backdoor.
//
// The host of the validation request is passed through: the responder (the
// ACME manager) is the authority on which domain an in-flight token is
// proving, because a domain saved from the dashboard changes the name mid-
// process while this proxy's own captured copy would go stale.
type ChallengeResponder interface {
	// ChallengeResponse returns the body for a challenge token presented to
	// the named host, or ok=false when no issuance is proving that pair.
	ChallengeResponse(host, token string) (string, bool)
}

// acmeChallengePrefix is the path prefix HTTP-01 validators request. Kept as a
// constant beside the code that compares it, not imported from a package the
// relay has no other reason to depend on.
const acmeChallengePrefix = "/.well-known/acme-challenge/"

type Server struct {
	settings     database.SNIProxySettings
	bindHost     string
	domain       string
	access       AccessValidator
	meter        TrafficMeter
	acme         ChallengeResponder
	acmeAnswered atomic.Uint64
	activeRelays atomic.Int64
	totalRelays  atomic.Uint64
	bytesSent    atomic.Uint64
	bytesRecv    atomic.Uint64
	// refused, unreadable and malformed are disjoint drop categories. refused
	// covers a destination this proxy read and declined (quota spent, relay table
	// full, target blocked or pointing back at this host). unreadable covers
	// connections whose opening bytes named no destination. malformed covers a
	// syntactically invalid hostname — almost always internet scanner noise, so it
	// must not make the Refused number climb.
	refused    atomic.Uint64
	unreadable atomic.Uint64
	// Malformed is kept separate on purpose: a public SNI port sees these
	// constantly, and counting them as refusals made a healthy proxy look broken.
	refusedMalformed atomic.Uint64
	refusedBlocked   atomic.Uint64
	refusedSelf      atomic.Uint64
	refusedQuota     atomic.Uint64
	refusedTableFull atomic.Uint64
	refusedResolve   atomic.Uint64
	refusedFlood     atomic.Uint64
	// floodMu guards flood, the per-source malformed-probe penalty box. It is
	// separate from mu, which guards listener lifecycle.
	floodMu        sync.Mutex
	flood          map[string]*sourceGuard
	floodLastSweep time.Time
	listeners      []net.Listener
	mu             sync.Mutex
	cancel         context.CancelFunc

	guard *targetGuard
	// slots is a counting semaphore: a relay takes one for its lifetime.
	slots chan struct{}

	// relayTarget resolves a vetted hostname to the hub-side egress that should
	// carry it. nil keeps the original direct dial, so installs without egress
	// configuration are unchanged.
	relayTarget  RelayTargetLookup
	egressDialer EgressDialer
	// engineEnabled is the live DNS policy engine state. When it reports false,
	// new relay connections are refused so turning the engine off stops
	// proxying immediately, not only after clients expire their DNS cache.
	engineEnabled func() bool
}

// RelayDialTarget is where the hub should dial for a vetted hostname. Kind
// "local" (or empty) keeps the direct dial; Kind "egress" routes the dial
// through the named SOCKS5 or WireGuard egress.
type RelayDialTarget struct {
	Kind       string
	EgressID   string
	EgressName string
	EgressKind string
	FailClosed bool
	Healthy    bool
}

// RelayTargetLookup resolves a hostname to a hub-side dial target.
type RelayTargetLookup func(hostname string) RelayDialTarget

// EgressDialer opens an origin connection through a configured egress. addrs
// are the addresses the guard already resolved and vetted, so the egress can
// use them in local-resolution mode without a second lookup.
type EgressDialer interface {
	Dial(ctx context.Context, id, host string, addrs []netip.Addr, port int) (net.Conn, error)
}

// SetRelayTarget installs the egress resolver. Call during wiring, before the
// listeners accept connections.
func (s *Server) SetRelayTarget(lookup RelayTargetLookup, dialer EgressDialer) {
	s.relayTarget = lookup
	s.egressDialer = dialer
}

// SetEngineEnabled installs the live policy-engine predicate. Called during
// wiring; nil keeps the historical always-on behaviour in test harnesses.
func (s *Server) SetEngineEnabled(fn func() bool) {
	s.engineEnabled = fn
}

// SetChallengeResponder attaches the ACME challenge source. Called once at
// wiring time, before listeners accept.
func (s *Server) SetChallengeResponder(r ChallengeResponder) {
	s.acme = r
}

func NewServer(
	settings database.SNIProxySettings,
	bindHost string,
	domain string,
	access AccessValidator,
) *Server {
	s := &Server{
		settings: settings,
		bindHost: bindHost,
		domain:   domain,
		access:   access,
		guard:    newTargetGuard(),
		slots:    make(chan struct{}, maxConcurrentRelays),
		flood:    make(map[string]*sourceGuard),
	}
	if m, ok := access.(TrafficMeter); ok {
		s.meter = m
	}
	return s
}

// sourceGuard is the per-source malformed-probe state. It is deliberately
// tiny: a scanner is identified by behaviour, not by an account.
type sourceGuard struct {
	malformed    int
	windowStart  time.Time
	blockedUntil time.Time
}

// sourceBlocked reports whether a source is in its penalty cooldown. It also
// expires the cooldown so the next connection is treated normally.
func (s *Server) sourceBlocked(ip string, now time.Time) bool {
	s.floodMu.Lock()
	defer s.floodMu.Unlock()
	g := s.flood[ip]
	if g == nil {
		return false
	}
	if now.Before(g.blockedUntil) {
		return true
	}
	if !g.blockedUntil.IsZero() {
		// The cooldown has expired: start the next window clean.
		g.blockedUntil = time.Time{}
		g.malformed = 0
		g.windowStart = now
	}
	return false
}

// noteMalformed records one malformed handshake from a source. The source is
// blocked after malformedThreshold within malformedWindow; a single scanner
// hitting a public port must not be able to consume the relay table by opening
// one slow handshake after another.
func (s *Server) noteMalformed(ip string, now time.Time) {
	s.floodMu.Lock()
	defer s.floodMu.Unlock()
	if s.flood == nil {
		s.flood = make(map[string]*sourceGuard)
	}
	g := s.flood[ip]
	if g == nil {
		g = &sourceGuard{windowStart: now}
		s.flood[ip] = g
	}
	if now.Sub(g.windowStart) > malformedWindow {
		g.malformed = 0
		g.windowStart = now
	}
	g.malformed++
	if g.malformed >= malformedThreshold {
		g.blockedUntil = now.Add(floodCooldown)
		g.malformed = 0
		log.Printf("[SNI Proxy] Blocking %s for %s after repeated malformed probes", ip, floodCooldown)
	}

	// Sweep rarely, not on every probe: expired entries would otherwise be a
	// slow memory leak on a public port. The hard cap protects against a
	// spoofed-source flood rotating through addresses.
	if now.Sub(s.floodLastSweep) < malformedWindow {
		return
	}
	s.floodLastSweep = now
	for key, entry := range s.flood {
		if now.Sub(entry.windowStart) > 2*malformedWindow && now.After(entry.blockedUntil) {
			delete(s.flood, key)
		}
	}
	for key := range s.flood {
		if len(s.flood) <= floodMapLimit {
			break
		}
		delete(s.flood, key)
	}
}

// idleTimeout is the per-direction inactivity deadline for a relay.
// settings.Timeout was parsed from the config and stored but never applied, so a
// client could open a relay, fall silent and hold two sockets open forever.
func (s *Server) idleTimeout() time.Duration {
	if s.settings.Timeout > 0 {
		return s.settings.Timeout
	}
	return defaultIdleTimeout
}

func (s *Server) Start() error {
	if !s.settings.Enabled {
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel

	host := s.bindHost
	if host == "" {
		host = "0.0.0.0"
	}

	ports := []struct {
		port  int
		isTLS bool
		name  string
	}{
		{s.settings.HTTPSPort, true, "HTTPS SNI Proxy"},
		{s.settings.HTTPPort, false, "HTTP Proxy"},
		{s.settings.GameChatTLSPort, true, "Chat TLS"},
		{s.settings.GameChatXMPPPort, false, "Chat XMPP"},
		{s.settings.RiotRTMPort, true, "Riot PVP.net RTM"},
		{s.settings.RiotPatcherPort, false, "Riot Patcher"},
	}

	for _, p := range ports {
		if p.port <= 0 {
			continue
		}
		addr := net.JoinHostPort(host, strconv.Itoa(p.port))
		// Dual-stack: an unspecified "0.0.0.0" binds IPv4 only, but a host
		// with AAAA records is validated over IPv6 by ACME (and reached over
		// IPv6 by every client whose resolver prefers it). The wildcard "::"
		// accepts both families on every mainstream stack, so an operator who
		// pointed both record types at this server — the normal v2.2 setup —
		// gets port 80/443 reachable on both. A host that explicitly names an
		// address family keeps exactly what it asked for.
		listenHost := host
		if host == "0.0.0.0" {
			listenHost = "::"
		}
		ln, err := net.Listen("tcp", net.JoinHostPort(listenHost, strconv.Itoa(p.port)))
		if err != nil {
			log.Printf("[SNI Proxy] Warning: Failed to bind %s on %s: %v", p.name, addr, err)
			continue
		}
		s.mu.Lock()
		s.listeners = append(s.listeners, ln)
		s.mu.Unlock()

		go s.acceptLoop(ctx, ln, p.isTLS, p.port)
	}

	return nil
}

func (s *Server) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, ln := range s.listeners {
		_ = ln.Close()
	}
	s.listeners = nil
}

func (s *Server) acceptLoop(ctx context.Context, ln net.Listener, isTLS bool, listenPort int) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return
			default:
			}
			// A closed listener never recovers, so the old unconditional retry
			// spun this goroutine at 20 Hz for the life of the process whenever
			// Stop() raced ahead of the context check above.
			if errors.Is(err, net.ErrClosed) {
				return
			}
			time.Sleep(50 * time.Millisecond)
			continue
		}

		go s.handleConnection(ctx, conn, isTLS, listenPort)
	}
}

// readHandshake fills buf until the opening message is complete: a whole TLS
// record for TLS, or the end of the request headers for plain HTTP. A single
// Read was assumed to deliver the entire ClientHello, so a hello split across
// TCP segments — routine once post-quantum key shares push it past one segment —
// failed to parse and the connection was silently dropped.
func readHandshake(conn net.Conn, buf []byte, isTLS bool) (int, error) {
	_ = conn.SetReadDeadline(time.Now().Add(handshakeTimeout))
	defer func() { _ = conn.SetReadDeadline(time.Time{}) }()

	total := 0
	for total < len(buf) {
		n, err := conn.Read(buf[total:])
		total += n
		if total > 0 && handshakeComplete(buf[:total], isTLS) {
			return total, nil
		}
		if err != nil {
			if total > 0 {
				return total, nil
			}
			return 0, err
		}
	}
	return total, nil
}

// handleConnection relays one accepted connection. listenPort is the port this
// connection arrived on, and is also the port the relay dials on the far side: an
// SNI proxy substitutes the address, not the service, so a client that connected
// to 443 is talking to the real host's 443. That is why a name whose traffic runs
// on any other port cannot be proxied by this daemon at all.
func (s *Server) handleConnection(ctx context.Context, clientConn net.Conn, isTLS bool, listenPort int) {
	defer clientConn.Close()
	defer safego.Recover("sni.handleConnection")

	clientIP, _, _ := net.SplitHostPort(clientConn.RemoteAddr().String())
	// A source that has produced repeated malformed handshakes is dropped before
	// it can consume a handshake read or a relay slot. This is a penalty box,
	// not a rate limit: a source that behaves normally never enters it.
	if s.sourceBlocked(clientIP, time.Now()) {
		s.refused.Add(1)
		s.refusedFlood.Add(1)
		return
	}

	tuneTCP(clientConn)

	// ACME HTTP-01 challenge serving (v2.2.0), before the access gate.
	//
	// The CA's validator is not a subscriber and never can be — it connects to
	// port 80 from wherever the CA validates from, moments after an issuance
	// was requested from the dashboard. Everything else this proxy does for a
	// connection still requires a registered, in-quota source; this is the one
	// deliberately carved-out path, and it is as narrow as the protocol allows:
	// plain HTTP, on the configured HTTP port, GET, a Host matching the
	// configured domain, and a token the ACME manager is actively proving (the
	// mapping exists only for the seconds a validation is in flight). A scanner
	// hitting the path gets the same relay-or-drop it always got, because no
	// token matches.
	//
	// The handshake is read here once; on a miss the bytes are handed to the
	// relay path below through earlyHandshake, so nothing is read twice.
	var earlyHandshake []byte
	var earlyN int
	if !isTLS && s.acme != nil && listenPort == s.settings.HTTPPort {
		bufPtr := handshakeBufPool.Get().(*[]byte)
		buf := *bufPtr
		n, err := readHandshake(clientConn, buf, isTLS)
		if err != nil || n == 0 {
			handshakeBufPool.Put(bufPtr)
			return
		}
		if s.maybeServeACMEChallenge(clientConn, buf[:n]) {
			handshakeBufPool.Put(bufPtr)
			return
		}
		earlyHandshake = buf[:n]
		earlyN = n
		defer func() { handshakeBufPool.Put(bufPtr) }()
	}

	// The engine switch is authoritative for new relays. DNS already stops
	// advertising proxied names, but a client with a cached answer could still
	// arrive here; refusing closes that window immediately.
	if s.engineEnabled != nil && !s.engineEnabled() {
		s.refused.Add(1)
		return
	}

	// Access control. The client is resolved even in allow_all mode — where
	// IsIPAllowed reports true for everyone — because a recognised account still
	// has to be metered and quota-checked. An unknown source in that mode gets a
	// nil client and is relayed unmetered, which is what allow_all means.
	var client *database.Client
	if s.access != nil {
		c, allowed := s.access.IsIPAllowed(clientIP)
		if !allowed {
			return
		}
		client = c
	}

	// A client that has spent its allowance is dropped before any work is done on
	// its behalf. Refusing rather than disabling the account keeps the block
	// reversible: raising the limit or resetting the counter restores service
	// without an operator re-enabling anything by hand.
	if client != nil && s.meter != nil && s.meter.QuotaExceeded(client) {
		s.refused.Add(1)
		s.refusedQuota.Add(1)
		return
	}

	// Take a relay slot before doing any work on behalf of the client, so an
	// overloaded proxy sheds load at accept time instead of after paying for a
	// handshake parse and a DNS lookup.
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	default:
		s.refused.Add(1)
		s.refusedTableFull.Add(1)
		return
	}

	bufPtr := handshakeBufPool.Get().(*[]byte)
	defer handshakeBufPool.Put(bufPtr)
	buf := *bufPtr

	var n int
	if earlyHandshake != nil {
		// The ACME pre-check already read the handshake; reuse those exact
		// bytes rather than reading (and consuming) a second time.
		n = earlyN
		copy(buf, earlyHandshake)
		earlyHandshake = nil
	} else {
		var err error
		n, err = readHandshake(clientConn, buf, isTLS)
		if err != nil || n == 0 {
			return
		}
	}

	var hostname string
	var err error
	if isTLS {
		hostname, err = ExtractSNI(buf[:n])
	} else {
		hostname, err = ExtractHTTPHost(buf[:n])
	}
	if err != nil || hostname == "" {
		// This used to return in complete silence: no counter, no log line. That
		// silence is how a rule can advertise a proxy path for a service whose
		// protocol this relay cannot read and have nobody find out — DNS answered,
		// TCP connected, and the client sits waiting for a peer that will never
		// speak. cm.steampowered.com was exactly that case (see
		// matcher/realtime.go), and it took a code audit to notice rather than a log.
		//
		// Counted here and *not* also in s.refused, so the two are disjoint and can
		// be added: every other drop site is a decision this proxy made about a
		// destination it knew, whereas this one never learned a destination at all.
		// Folding it into refused would leave an operator unable to tell "the guards
		// are working" from "a name is sending clients somewhere I cannot carry them".
		if s.unreadable.Add(1)%unreadableLogSample == 1 {
			log.Printf("[SNI Proxy] Port %d: %s sent no readable destination (%s, opens with %s); "+
				"a name resolving here whose traffic is not %s cannot be relayed",
				listenPort, clientIP, describeProto(isTLS), openingBytes(buf[:n]), describeProto(isTLS))
		}
		s.noteMalformed(clientIP, time.Now())
		return
	}

	// Resolve and vet before dialling, then dial the vetted address rather than
	// the name. The old code string-matched the hostname and handed the name to
	// the dialer, so any name that resolved to a private address — DNS rebinding,
	// or simply "internal.corp.example" — turned the relay into an SSRF pivot.
	dialCtx, cancel := context.WithTimeout(ctx, handshakeTimeout)
	_, addrs, err := s.guard.resolve(dialCtx, hostname)
	cancel()
	if err != nil {
		switch {
		case errors.Is(err, ErrTargetMalformed):
			// Scanner noise, not a relay decision. Keep it visible in the detail
			// counter but do not make the Refused total climb.
			s.refusedMalformed.Add(1)
			s.noteMalformed(clientIP, time.Now())
		case errors.Is(err, ErrTargetBlocked):
			s.refused.Add(1)
			s.refusedBlocked.Add(1)
		case errors.Is(err, ErrTargetSelf):
			s.refused.Add(1)
			s.refusedSelf.Add(1)
		default:
			s.refused.Add(1)
			s.refusedResolve.Add(1)
			log.Printf("[SNI Proxy] Refused relay from %s: %v", clientIP, err)
		}
		return
	}

	targetConn, err := s.dialTarget(ctx, hostname, addrs, listenPort, clientIP)
	if err != nil {
		return
	}
	defer targetConn.Close()
	tuneTCP(targetConn)

	s.activeRelays.Add(1)
	s.totalRelays.Add(1)
	defer s.activeRelays.Add(-1)

	idle := s.idleTimeout()
	_ = targetConn.SetWriteDeadline(time.Now().Add(idle))
	if isTLS && s.settings.EnableFragmentation && s.settings.FragmentSize > 0 {
		err = SendFragmented(targetConn, buf[:n], s.settings.FragmentSize, s.settings.FragmentDelayMs)
	} else {
		_, err = targetConn.Write(buf[:n])
	}
	_ = targetConn.SetWriteDeadline(time.Time{})
	if err != nil {
		return
	}
	// The handshake is the first thing the client spent, so it is counted like
	// any other relayed byte rather than being absorbed silently.
	s.bytesSent.Add(uint64(n))

	var report func(uint64)
	if client != nil && s.meter != nil {
		report = func(delta uint64) {
			s.meter.AddTraffic(client.ID, delta)

			// Enforce mid-relay as well. The check at connection time cannot
			// bound a single long-lived stream, so without this one connection
			// could carry an unlimited amount past the limit. The account is
			// re-resolved instead of reusing the pointer captured above: a flush
			// replaces the entries in the access table, and the captured copy
			// would keep reporting the total as it stood when the relay opened.
			cur := client
			if fresh, ok := s.access.IsIPAllowed(clientIP); ok && fresh != nil {
				cur = fresh
			}
			if s.meter.QuotaExceeded(cur) {
				// Closing both sockets makes the next read and write in each
				// direction fail, so both copies unwind on their own.
				_ = clientConn.Close()
				_ = targetConn.Close()
			}
		}
		report(uint64(n))
	}

	s.relay(clientConn, targetConn, idle, report)
}

// dialTarget chooses between the direct dial and a configured hub-side egress.
// A fail-closed egress never falls back to the direct path: the operator asked
// for this name to leave through it, and a silent local dial would leak the
// hub's address. A fail-open egress falls back after a dial error.
func (s *Server) dialTarget(ctx context.Context, hostname string, addrs []netip.Addr, port int, clientIP string) (net.Conn, error) {
	target := RelayDialTarget{Kind: "local"}
	if s.relayTarget != nil {
		target = s.relayTarget(hostname)
	}
	if target.Kind != "egress" || s.egressDialer == nil {
		return s.dialVetted(ctx, addrs, port)
	}
	if target.FailClosed && !target.Healthy {
		s.refused.Add(1)
		log.Printf("[SNI Proxy] Refused relay from %s for %s: egress %q is offline and fail-closed", clientIP, hostname, target.EgressName)
		return nil, errors.New("proxy: fail-closed egress is offline")
	}
	conn, err := s.egressDialer.Dial(ctx, target.EgressID, hostname, addrs, port)
	if err == nil {
		return conn, nil
	}
	if target.FailClosed {
		s.refused.Add(1)
		log.Printf("[SNI Proxy] Refused relay from %s for %s: egress %q failed: %v", clientIP, hostname, target.EgressName, err)
		return nil, err
	}
	log.Printf("[SNI Proxy] Egress %q failed for %s, falling back to direct: %v", target.EgressName, hostname, err)
	return s.dialVetted(ctx, addrs, port)
}

// dialVetted connects to the first reachable vetted address. Passing an address
// rather than a hostname closes the window in which a second lookup — the one
// the dialer would perform — could return an address the guard never saw.
func (s *Server) dialVetted(ctx context.Context, addrs []netip.Addr, port int) (net.Conn, error) {
	dialer := &net.Dialer{Timeout: handshakeTimeout, KeepAlive: 30 * time.Second}
	var lastErr error
	for _, addr := range addrs {
		conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(addr.String(), strconv.Itoa(port)))
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = errors.New("proxy: no target addresses to dial")
	}
	return nil, lastErr
}

// relay shuttles bytes in both directions until either side finishes or falls
// idle. Each direction half-closes its peer on completion, so a server that
// replies and hangs up does not leave the opposite copy blocked on a socket that
// will never produce another byte. report, when non-nil, is called from both
// directions with each chunk of accounted traffic and must be safe for concurrent
// use.
func (s *Server) relay(clientConn, targetConn net.Conn, idle time.Duration, report func(uint64)) {
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		// Half-close even when the copy panics: the peer must see EOF rather
		// than a connection that is only closed when the other direction ends.
		defer closeWrite(targetConn)
		defer safego.Recover("sni.relay.send")
		// A metered relay needs the userspace buffer; the unmetered splice path
		// does not, so do not reserve 64 KiB per direction for nothing.
		var buf []byte
		if report != nil {
			b := relayBufPool.Get().(*[]byte)
			defer relayBufPool.Put(b)
			buf = *b
		}
		s.bytesSent.Add(copyIdle(targetConn, clientConn, buf, idle, report))
	}()

	go func() {
		defer wg.Done()
		defer closeWrite(clientConn)
		defer safego.Recover("sni.relay.recv")
		var buf []byte
		if report != nil {
			b := relayBufPool.Get().(*[]byte)
			defer relayBufPool.Put(b)
			buf = *b
		}
		s.bytesRecv.Add(copyIdle(clientConn, targetConn, buf, idle, report))
	}()

	wg.Wait()
}

// copyIdle is io.CopyBuffer with an inactivity deadline refreshed per chunk. It
// reports progress to report every meterFlushBytes so that accounting for a
// long-lived connection does not wait for the connection to end.
//
// When there is no meter to update, both ends are plain TCP and the kernel can
// move the bytes with splice instead of copying them through this process. That
// path is chosen only for unmetered relays: splice bypasses the per-chunk Read
// hook the meter needs. It still honours the idle timeout — a ReadFrom call with
// no data within idle returns a timeout, and only a call that also moved bytes
// resets the window.
func copyIdle(dst, src net.Conn, buf []byte, idle time.Duration, report func(uint64)) uint64 {
	if report == nil {
		if dt, ok := dst.(*net.TCPConn); ok {
			if st, ok := src.(*net.TCPConn); ok {
				return copySpliceIdle(dt, st, idle)
			}
		}
	}
	var total, unreported uint64
	// The caller may pass no buffer on the unmetered path. If the splice fast
	// path did not apply (one side is not a plain TCP socket), take one from the
	// pool rather than reading into a nil slice, which would spin.
	if len(buf) == 0 {
		b := relayBufPool.Get().(*[]byte)
		defer relayBufPool.Put(b)
		buf = *b
	}
	// Whatever is still unreported when the copy ends has to be handed over, on
	// every exit path.
	defer func() {
		if report != nil && unreported > 0 {
			report(unreported)
		}
	}()

	for {
		if idle > 0 {
			_ = src.SetReadDeadline(time.Now().Add(idle))
		}
		n, rerr := src.Read(buf)
		if n > 0 {
			if idle > 0 {
				_ = dst.SetWriteDeadline(time.Now().Add(idle))
			}
			written, werr := dst.Write(buf[:n])
			if written > 0 {
				total += uint64(written)
				unreported += uint64(written)
				if report != nil && unreported >= meterFlushBytes {
					report(unreported)
					unreported = 0
				}
			}
			if werr != nil {
				return total
			}
		}
		if rerr != nil {
			return total
		}
	}
}

// copySpliceIdle is the zero-copy relay path. TCPConn.ReadFrom asks Linux to
// splice the socket buffers, so a bulk transfer does not pass through userspace
// twice. The idle deadline is still refreshed per ReadFrom call: the call only
// returns after an idle timeout, and a call that moved bytes is treated as
// activity and retried.
func copySpliceIdle(dst, src *net.TCPConn, idle time.Duration) uint64 {
	var total uint64
	for {
		if idle > 0 {
			_ = src.SetReadDeadline(time.Now().Add(idle))
			// Bound a receiver that has stopped reading. The relay still closes
			// both sockets when this returns, so a stuck peer cannot park a slot.
			_ = dst.SetWriteDeadline(time.Now().Add(4 * idle))
		}
		n, err := dst.ReadFrom(src)
		if n > 0 {
			total += uint64(n)
		}
		if err == nil {
			return total
		}
		if ne, ok := err.(net.Error); ok && ne.Timeout() && n > 0 {
			continue
		}
		return total
	}
}

func closeWrite(conn net.Conn) {
	if tc, ok := conn.(*net.TCPConn); ok {
		_ = tc.CloseWrite()
	}
}

// describeProto names what a listener expects to read, for the log line above.
func describeProto(isTLS bool) string {
	if isTLS {
		return "TLS with SNI"
	}
	return "HTTP with a Host header"
}

// openingBytes renders the first few bytes of a connection that named no
// destination. It is the one piece of evidence that separates "a scanner
// connected" from "a real service is speaking a protocol this relay cannot read":
// a TLS handshake opens 16 03 01, an HTTP request opens with a method, and
// anything else is a binary protocol that has no destination to extract. Hex, and
// four bytes of it, so nothing from the wire reaches the log as text.
func openingBytes(buf []byte) string {
	if len(buf) == 0 {
		return "no bytes"
	}
	return hex.EncodeToString(buf[:min(len(buf), 4)])
}

func tuneTCP(conn net.Conn) {
	if tc, ok := conn.(*net.TCPConn); ok {
		_ = tc.SetNoDelay(true)
		_ = tc.SetKeepAlive(true)
		_ = tc.SetKeepAlivePeriod(30 * time.Second)
	}
}

// handshakeComplete reports whether buf already holds everything the parsers
// need, so readHandshake can stop instead of blocking for the full deadline.
func handshakeComplete(buf []byte, isTLS bool) bool {
	if isTLS {
		if len(buf) < 5 {
			return false
		}
		// TLS record header: type, version(2), length(2).
		recordLen := int(buf[3])<<8 | int(buf[4])
		return len(buf) >= 5+recordLen
	}
	return bytes.Contains(buf, []byte("\r\n\r\n")) || bytes.Contains(buf, []byte("\n\n"))
}

// GetStats reports active relays, the lifetime relay count and bytes relayed in
// each direction.
func (s *Server) GetStats() (int64, uint64, uint64, uint64) {
	return s.activeRelays.Load(),
		s.totalRelays.Load(),
		s.bytesSent.Load(),
		s.bytesRecv.Load()
}

// GuardStats reports the two actionable ways a connection reaches this proxy and
// leaves again without ever becoming a relay, neither of which appears in
// GetStats. They are disjoint and exclude malformed scanner probes:
//
//   - refused: this proxy read a destination and declined it — the client's quota
//     was spent, the relay table was already at maxConcurrentRelays, or the target
//     was blocked or resolved back to this host. Mostly evidence that the guards
//     are working.
//   - unreadable: the opening bytes named no destination, so there was nothing to
//     dial. A port scanner looks exactly like this, so a handful on a public IP is
//     background noise. A count that climbs with real traffic is not: it means a
//     name is being answered with this server's address and the client then speaks
//     something this relay cannot read a destination out of — traffic that is UDP,
//     or TCP on a port this relay does not accept on, or TCP that is neither TLS
//     nor HTTP. The client cannot diagnose it either, because the substitution
//     happened in DNS.
//
// Malformed hostnames are reported separately by GuardStatsDetail. They are
// scanner noise and must not make Refused climb.
//
// Both are lifetime totals since start.
func (s *Server) GuardStats() (refused, unreadable uint64) {
	return s.refused.Load(), s.unreadable.Load()
}

// GuardStatsDetail is GuardStats plus the refusal reasons. Blocked, Self, Quota,
// TableFull, ResolveFailed and Flood sum to Refused. Malformed is separate
// scanner noise and is deliberately not part of Refused.
type GuardStatsDetail struct {
	Refused       uint64
	Unreadable    uint64
	Malformed     uint64
	Blocked       uint64
	Self          uint64
	Quota         uint64
	TableFull     uint64
	ResolveFailed uint64
	Flood         uint64
}

func (s *Server) GuardStatsDetail() GuardStatsDetail {
	return GuardStatsDetail{
		Refused:       s.refused.Load(),
		Unreadable:    s.unreadable.Load(),
		Malformed:     s.refusedMalformed.Load(),
		Blocked:       s.refusedBlocked.Load(),
		Self:          s.refusedSelf.Load(),
		Quota:         s.refusedQuota.Load(),
		TableFull:     s.refusedTableFull.Load(),
		ResolveFailed: s.refusedResolve.Load(),
		Flood:         s.refusedFlood.Load(),
	}
}
