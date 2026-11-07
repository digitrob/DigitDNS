package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"digitdns/internal/core/proxy"
)

const (
	// handshakeTimeout bounds how long a client may take to say what it wants.
	// Without it a half-open connection holds a goroutine and a slot for free.
	handshakeTimeout = 10 * time.Second
	// idleTimeout closes a relay whose two directions have both gone quiet.
	idleTimeout      = 2 * time.Minute
	handshakeBufSize = 16 * 1024
)

// ErrNoDestination is returned when the opening bytes name no host — not TLS,
// not HTTP, or a truncated hello. Kept distinct from a refusal so the two drop
// reasons can be told apart in the log and in the counters.
var ErrNoDestination = errors.New("agent: no readable destination")

// Config is everything the agent needs to run. In the shipped binary this is
// built from the control-plane sync; on disk it is the cached copy, so a hub
// outage does not stop playback.
type Config struct {
	// Listen is a comma-separated list of bind addresses. The shipped default is
	// :80,:443 so the agent carries both plain HTTP and TLS.
	Listen string `json:"listen"`
	// Country is what this agent exits as. It is reported upstream and used to
	// label the status line; it never changes routing by itself, because the
	// host list is decided by the hub.
	Country string `json:"country"`
	// PublicIPv6 is the agent's globally routable IPv6, reported in the
	// heartbeat so the hub can answer proxied AAAA with it. Empty means the
	// agent has no usable IPv6; the hub then keeps that agent's AAAA sunk.
	PublicIPv6 string `json:"public_ipv6"`
	// Hosts and Clients are the allowlists. Both are computed upstream from the
	// operator's domain routes; the agent applies them and nothing else.
	Hosts   []string `json:"hosts"`
	Clients []string `json:"clients"`
	// HTTPSPort is the port dialled for a TLS connection; HTTPPort for a plain
	// request. An SNI proxy substitutes the address, not the service, so the
	// port is chosen from the protocol the client spoke.
	HTTPSPort int `json:"https_port"`
	HTTPPort  int `json:"http_port"`
	// AllowPrivate disables the private-address guard. Only for local testing:
	// with it on, a name that resolves to a loopback address is relayed, which
	// is exactly what an SSRF guard exists to prevent.
	AllowPrivate bool `json:"allow_private"`
	// IPv6Enabled is the effective hub policy. When false the agent binds only
	// IPv4 listeners, skips IPv6 detection and reports no IPv6 in its heartbeat.
	// The hub per-agent switch and the global Proxy IPv6 switch both feed this.
	IPv6Enabled bool `json:"ipv6_enabled"`
}

// Server is the agent relay.
type Server struct {
	cfg      Config
	allow    *Allowlist
	clients  *ClientList
	counters *Counters

	listeners []net.Listener
	// listenersMu guards rebinding the relay between IPv4-only and dual-stack
	// when the hub changes the effective IPv6 policy.
	listenersMu sync.Mutex
	ipv6Enabled bool
	// mu guards the two allowlists, which a sync may replace while connections
	// are in flight.
	mu      sync.RWMutex
	active  atomic.Int64
	total   atomic.Uint64
	refused atomic.Uint64
	nohost  atomic.Uint64
	// refusedIP counts requests whose destination was an IP literal. They can
	// never be relayed, and scanners produce a lot of them; keeping them apart
	// from the host allowlist makes the summary honest.
	refusedIP atomic.Uint64
	refusals  *refusalLog
}

// NewServer builds a relay from a config.
func NewServer(cfg Config) *Server {
	if cfg.HTTPSPort == 0 {
		cfg.HTTPSPort = 443
	}
	if cfg.HTTPPort == 0 {
		cfg.HTTPPort = 80
	}
	return &Server{
		cfg:      cfg,
		allow:    NewAllowlist(cfg.Hosts),
		clients:  NewClientList(cfg.Clients),
		counters: NewCounters(),
		refusals: newRefusalLog(time.Minute, time.Now),
	}
}

// Counters exposes the per-host byte counts for the status line and heartbeat.
func (s *Server) Counters() *Counters { return s.counters }

// Allowlist exposes the matcher so a status command can report its size.
func (s *Server) Allowlist() *Allowlist { return s.allow }

// ClientList exposes the client matcher.
func (s *Server) ClientList() *ClientList { return s.clients }

// Stats returns the connection counters.
func (s *Server) Stats() (active, total, refused, nohost int64) {
	return s.active.Load(), int64(s.total.Load()), int64(s.refused.Load()), int64(s.nohost.Load())
}

// RefusedIP returns the number of requests whose destination was a bare IP.
func (s *Server) RefusedIP() int64 { return int64(s.refusedIP.Load()) }

// Start binds the listen addresses and serves until Close.
func (s *Server) Start() error {
	s.listenersMu.Lock()
	defer s.listenersMu.Unlock()
	return s.bindListenersLocked(s.cfg.IPv6Enabled)
}

// SetIPv6Enabled applies the hub's effective IPv6 policy live. With it off the
// listeners are rebuilt from IPv4 addresses only, so the agent stops accepting
// IPv6 connections instead of merely hiding the address from DNS.
func (s *Server) SetIPv6Enabled(enabled bool) error {
	s.listenersMu.Lock()
	defer s.listenersMu.Unlock()
	if s.ipv6Enabled == enabled && len(s.listeners) > 0 {
		return nil
	}
	_ = s.closeListenersLocked()
	if err := s.bindListenersLocked(enabled); err != nil {
		return err
	}
	s.cfg.IPv6Enabled = enabled
	return nil
}

func (s *Server) bindListenersLocked(enabled bool) error {
	addrs := listenAddresses(s.cfg.Listen, enabled)
	if len(addrs) == 0 {
		return fmt.Errorf("agent: no listen address configured for IPv4-only mode")
	}
	network := "tcp"
	if !enabled {
		// A plain "tcp" bind of 0.0.0.0 can still use an IPv6 socket on some
		// platforms; tcp4 is what actually guarantees no IPv6 listener.
		network = "tcp4"
	}
	for _, addr := range addrs {
		ln, err := net.Listen(network, addr)
		if err != nil {
			_ = s.closeListenersLocked()
			return fmt.Errorf("agent: listen %s: %w", addr, err)
		}
		s.listeners = append(s.listeners, ln)
		go s.acceptLoop(ln)
	}
	s.ipv6Enabled = enabled
	return nil
}

// listenAddresses converts the configured bind list for the effective policy.
// In IPv4-only mode a wildcard (":80" or "[::]:80") becomes "0.0.0.0:80"; an
// explicit IPv6 literal is dropped because it cannot be bound as IPv4. In
// dual-stack mode the operator's list is used unchanged.
func listenAddresses(listen string, ipv6Enabled bool) []string {
	var out []string
	for _, raw := range strings.Split(listen, ",") {
		addr := strings.TrimSpace(raw)
		if addr == "" {
			continue
		}
		if ipv6Enabled {
			out = append(out, addr)
			continue
		}
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			continue
		}
		switch {
		case host == "" || host == "::" || host == "0.0.0.0":
			out = append(out, net.JoinHostPort("0.0.0.0", port))
		case strings.Contains(host, ":"):
			// Explicit IPv6 listener; no IPv4 equivalent.
			continue
		default:
			out = append(out, addr)
		}
	}
	return out
}

// Close stops every listener. Connections in flight finish on their own idle
// timeout rather than being cut, so a restart does not truncate a stream
// mid-play.
func (s *Server) Close() error {
	s.listenersMu.Lock()
	defer s.listenersMu.Unlock()
	return s.closeListenersLocked()
}

func (s *Server) closeListenersLocked() error {
	var first error
	for _, ln := range s.listeners {
		if err := ln.Close(); err != nil && first == nil {
			first = err
		}
	}
	s.listeners = nil
	return first
}

// refusalLog throttles the refusal lines that a public agent attracts. A
// scanner or a bare-IP probe can produce thousands of identical lines a minute;
// the first one is useful, the next thousand are not. Keyed by source+reason
// (and host where relevant), it emits one line per minute and reports how many
// were folded into it.
type refusalLog struct {
	mu       sync.Mutex
	entries  map[string]*refusalEntry
	interval time.Duration
	maxKeys  int
	now      func() time.Time
}

type refusalEntry struct {
	last       time.Time
	suppressed int
}

func newRefusalLog(interval time.Duration, now func() time.Time) *refusalLog {
	if interval <= 0 {
		interval = time.Minute
	}
	if now == nil {
		now = time.Now
	}
	return &refusalLog{
		entries:  map[string]*refusalEntry{},
		interval: interval,
		maxKeys:  2048,
		now:      now,
	}
}

// allow reports whether this key should be logged now, and how many identical
// lines were suppressed since the previous one.
func (r *refusalLog) allow(key string) (bool, int) {
	now := r.now()
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.entries) >= r.maxKeys {
		cutoff := now.Add(-10 * r.interval)
		for k, e := range r.entries {
			if e.last.Before(cutoff) {
				delete(r.entries, k)
			}
		}
		if len(r.entries) >= r.maxKeys {
			r.entries = map[string]*refusalEntry{}
		}
	}
	e := r.entries[key]
	if e == nil {
		r.entries[key] = &refusalEntry{last: now}
		return true, 0
	}
	if now.Sub(e.last) >= r.interval {
		suppressed := e.suppressed
		e.last = now
		e.suppressed = 0
		return true, suppressed
	}
	e.suppressed++
	return false, 0
}

func (s *Server) logRefusal(key, format string, args ...any) {
	ok, suppressed := s.refusals.allow(key)
	if !ok {
		return
	}
	if suppressed > 0 {
		args = append(args, suppressed)
		log.Printf(format+" (%d similar events suppressed)", args...)
		return
	}
	log.Printf(format, args...)
}

func (s *Server) acceptLoop(ln net.Listener) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		go s.handle(conn)
	}
}

func (s *Server) handle(clientConn net.Conn) {
	defer clientConn.Close()

	remoteIP, _, _ := net.SplitHostPort(clientConn.RemoteAddr().String())
	ip := net.ParseIP(remoteIP)

	// The client check comes first, before a byte is read or a slot is taken: a
	// stranger who found the address costs one ParseIP and a close.
	if !s.clientList().Allows(ip) {
		s.refused.Add(1)
		s.logRefusal("client|"+remoteIP, "[agent] refused %s: client not in the allowlist", remoteIP)
		return
	}

	buf := make([]byte, handshakeBufSize)
	n, err := readHandshake(clientConn, buf)
	if err != nil || n == 0 {
		s.logRefusal("handshake|"+remoteIP, "[agent] %s: no handshake (%d bytes, %v)", remoteIP, n, err)
		return
	}

	host, isTLS, err := destination(buf[:n])
	if err != nil {
		s.nohost.Add(1)
		s.logRefusal("nohost|"+remoteIP, "[agent] %s: no readable destination in %d bytes (%v)", remoteIP, n, err)
		return
	}

	// A bare IP is not a relayable destination: the agent has no hostname to
	// forward to. Scanners hit the public address this way constantly, so count
	// it separately and keep it out of the per-host refusal log.
	if net.ParseIP(strings.Trim(host, "[]")) != nil {
		s.refusedIP.Add(1)
		s.refused.Add(1)
		s.logRefusal("ip|"+remoteIP, "[agent] refused bare IP %s from %s: no destination name", host, remoteIP)
		return
	}

	// The allowlist is the security boundary. Everything the hub did not approve
	// is dropped here, so a stolen agent address buys nothing but refused
	// connections.
	if !s.hostList().Allows(host) {
		s.refused.Add(1)
		s.logRefusal("host|"+remoteIP+"|"+host, "[agent] refused %s from %s: not an approved host", host, remoteIP)
		return
	}

	targetConn, err := s.dial(host, isTLS)
	if err != nil {
		s.refused.Add(1)
		s.logRefusal("dial|"+remoteIP+"|"+host, "[agent] refused %s from %s: %v", host, remoteIP, err)
		return
	}
	defer targetConn.Close()

	s.active.Add(1)
	s.total.Add(1)
	defer s.active.Add(-1)

	if _, err := targetConn.Write(buf[:n]); err != nil {
		return
	}
	s.counters.addOut(host, uint64(n))

	// Count as bytes move rather than when the relay ends: a 90-minute stream
	// would otherwise report zero for 90 minutes, which is precisely the window
	// in which an operator wants to see the number move.
	var connIn, connOut atomic.Uint64
	up := &countingReader{r: clientConn, add: func(n uint64) {
		connIn.Add(n)
		s.counters.addIn(host, n)
	}}
	down := &countingReader{r: targetConn, add: func(n uint64) {
		connOut.Add(n)
		s.counters.addOut(host, n)
	}}

	started := time.Now()
	done := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(targetConn, up); done <- struct{}{} }()
	go func() { _, _ = io.Copy(clientConn, down); done <- struct{}{} }()

	select {
	case <-done:
	case <-time.After(idleTimeout):
	}
	_ = clientConn.Close()
	_ = targetConn.Close()
	<-done

	// One line per relayed connection. This is the only place the per-host split
	// is visible on the box itself, so it carries the numbers rather than making
	// an operator wait for the next heartbeat.
	log.Printf("[agent] %s from %s · %s · up %d B · down %d B",
		host, remoteIP, time.Since(started).Round(time.Millisecond), connIn.Load(), connOut.Load())
}

// dial connects to host on the configured port, refusing private destinations
// unless the operator turned the guard off. The name is resolved here rather
// than handed to the dialer, so a name that points at a private address (DNS
// rebinding, or simply an internal hostname in the list) cannot turn the agent
// into an SSRF pivot.
func (s *Server) dial(host string, isTLS bool) (net.Conn, error) {
	ctx := &net.Dialer{Timeout: 10 * time.Second}
	port := s.cfg.HTTPPort
	if isTLS {
		port = s.cfg.HTTPSPort
	}

	if s.cfg.AllowPrivate {
		return ctx.Dial("tcp", net.JoinHostPort(host, fmt.Sprint(port)))
	}

	// A context is required here: LookupIPAddr dereferences it, so a nil one
	// panics the whole process on the first relay rather than failing the
	// connection.
	lookupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	addrs, err := net.DefaultResolver.LookupIPAddr(lookupCtx, host)
	if err != nil {
		return nil, fmt.Errorf("resolve %s: %w", host, err)
	}
	for _, addr := range addrs {
		if isPrivate(addr.IP) {
			return nil, fmt.Errorf("%s resolves to a private address", host)
		}
	}
	var lastErr error
	for _, addr := range addrs {
		conn, err := ctx.Dial("tcp", net.JoinHostPort(addr.IP.String(), fmt.Sprint(port)))
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = errors.New("no addresses")
	}
	return nil, lastErr
}

func isPrivate(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsUnspecified()
}

// destination extracts the host from the opening bytes and reports whether they
// were a TLS ClientHello, which decides how the bytes are forwarded on.
func destination(head []byte) (host string, isTLS bool, err error) {
	if len(head) > 0 && head[0] == 0x16 {
		host, err = proxy.ExtractSNI(head)
		return strings.TrimSpace(host), true, err
	}
	host, err = proxy.ExtractHTTPHost(head)
	return strings.TrimSpace(host), false, err
}

func readHandshake(conn net.Conn, buf []byte) (int, error) {
	_ = conn.SetReadDeadline(time.Now().Add(handshakeTimeout))
	defer func() { _ = conn.SetReadDeadline(time.Time{}) }()

	total := 0
	for total < len(buf) {
		n, err := conn.Read(buf[total:])
		total += n
		if total > 0 && complete(buf[:total]) {
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

// complete reports whether the buffered bytes already contain a whole
// ClientHello or request head. Without it, readHandshake would block until the
// buffer filled, which for a short request never happens.
func complete(b []byte) bool {
	if len(b) == 0 {
		return false
	}
	if b[0] == 0x16 {
		// TLS record: 5-byte header carrying the payload length.
		if len(b) < 5 {
			return false
		}
		need := 5 + int(b[3])<<8 + int(b[4])
		return len(b) >= need
	}
	return strings.Contains(string(b), "\r\n\r\n")
}

type countingReader struct {
	r   io.Reader
	add func(uint64)
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	if n > 0 {
		c.add(uint64(n))
	}
	return n, err
}

// UpdateAllowlists swaps the host and client allowlists in place. A resync must
// not drop connections that are already streaming, so the server keeps running
// and only the matchers are replaced. The fields are guarded because a resync
// happens on the hub's schedule, not between connections.
func (s *Server) UpdateAllowlists(hosts, clients []string) {
	allow := NewAllowlist(hosts)
	clientList := NewClientList(clients)

	s.mu.Lock()
	s.allow = allow
	s.clients = clientList
	s.mu.Unlock()
}

func (s *Server) hostList() *Allowlist {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.allow
}

func (s *Server) clientList() *ClientList {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.clients
}
