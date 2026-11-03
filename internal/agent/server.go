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
}

// Server is the agent relay.
type Server struct {
	cfg      Config
	allow    *Allowlist
	clients  *ClientList
	counters *Counters

	listeners []net.Listener
	// mu guards the two allowlists, which a sync may replace while connections
	// are in flight.
	mu      sync.RWMutex
	active  atomic.Int64
	total   atomic.Uint64
	refused atomic.Uint64
	nohost  atomic.Uint64
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

// Start binds the listen address and serves until Close.
func (s *Server) Start() error {
	for _, addr := range strings.Split(s.cfg.Listen, ",") {
		addr = strings.TrimSpace(addr)
		if addr == "" {
			continue
		}
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			_ = s.Close()
			return fmt.Errorf("agent: listen %s: %w", addr, err)
		}
		s.listeners = append(s.listeners, ln)
		go s.acceptLoop(ln)
	}
	if len(s.listeners) == 0 {
		return fmt.Errorf("agent: no listen address configured")
	}
	return nil
}

// Close stops every listener. Connections in flight finish on their own idle
// timeout rather than being cut, so a restart does not truncate a stream
// mid-play.
func (s *Server) Close() error {
	var first error
	for _, ln := range s.listeners {
		if err := ln.Close(); err != nil && first == nil {
			first = err
		}
	}
	return first
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
		log.Printf("[agent] refused %s: client not in the allowlist", remoteIP)
		return
	}

	buf := make([]byte, handshakeBufSize)
	n, err := readHandshake(clientConn, buf)
	if err != nil || n == 0 {
		log.Printf("[agent] %s: no handshake (%d bytes, %v)", remoteIP, n, err)
		return
	}

	host, isTLS, err := destination(buf[:n])
	if err != nil {
		s.nohost.Add(1)
		log.Printf("[agent] %s: no readable destination in %d bytes (%v)", remoteIP, n, err)
		return
	}

	// The allowlist is the security boundary. Everything the hub did not approve
	// is dropped here, so a stolen agent address buys nothing but refused
	// connections.
	if !s.hostList().Allows(host) {
		s.refused.Add(1)
		log.Printf("[agent] refused %s from %s: not an approved host", host, remoteIP)
		return
	}

	targetConn, err := s.dial(host, isTLS)
	if err != nil {
		s.refused.Add(1)
		log.Printf("[agent] refused %s from %s: %v", host, remoteIP, err)
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
