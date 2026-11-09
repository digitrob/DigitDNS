package dns

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"digitdns/internal/netutil"
	"github.com/miekg/dns"
	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
)

// alpnDoT is the ALPN identifier RFC 7858 registers for DNS-over-TLS.
const alpnDoT = "dot"

// How long an idle stream connection is kept, and how many may be open at once.
//
// miekg/dns closes an idle TCP connection after 8 seconds and after 128 queries.
// That is a fair default for port 53 and the wrong one for DoT, because what a
// DoT client is amortizing is a TLS handshake — two round trips, certificate
// verification, a key exchange — so a stub resolver that looks something up every
// ten seconds was paying for a fresh handshake on every name. On a phone over
// mobile data that is tens of milliseconds added to the query that starts a game.
// RFC 7766 §6.2.3 asks servers to hold idle connections open for as long as is
// reasonable; RFC 7858 §3.4 repeats it for DoT and points at RFC 7828 for telling
// the client the number, which handler.go now does. The asymmetry between the two
// timeouts is the cost being amortized: one round trip for TCP, a whole handshake
// for TLS.
//
// The 128-query ceiling is removed outright (-1). A household on Android Private
// DNS passes 128 queries in a couple of minutes, and forcing a reconnect there is
// exactly the cost this is meant to remove; what actually bounds the daemon is the
// connection cap, not a query counter. See netutil.LimitListener for what happens
// when a cap is reached. Plain TCP DNS is only used for retries and oversized
// answers so it needs the least headroom; DoT is where subscribers live.
const (
	tcpIdleTimeout = 30 * time.Second
	dotIdleTimeout = 60 * time.Second

	maxTCPConns   = 512
	maxDoTConns   = 1024
	maxHTTPSConns = 512
)

// The HTTPS listener carries the same handler as the plaintext one in
// internal/web, so its timeouts are deliberately identical — a dashboard that
// behaves differently on 8443 than on 8080 is a trap for whoever debugs it next.
const (
	httpsHeaderTimeout = 10 * time.Second
	httpsIdleTimeout   = 60 * time.Second
	shutdownGrace      = 3 * time.Second
)

// CertSource supplies the certificate the DoT/DoH handshakes present. The
// web server's hot-swap holder satisfies it, which is the point: the
// listeners hold the closure, so a Swap on the holder promotes a renewed
// pair into both live listeners without a rebind.
type CertSource interface {
	GetCertificate(*tls.ClientHelloInfo) (*tls.Certificate, error)
}

// TransportConfig carries the client-facing listener ports and the opt-in QUIC
// transports. It is a value, not a pointer, so an ApplyTransports call cannot
// retain a caller's struct that is mutated later.
type TransportConfig struct {
	Port           int
	DoTPort        int
	DoHPort        int
	DoQPort        int
	DoQEnabled     bool
	DoQAuth        string
	DoQTokenDomain string
	DoH3Enabled    bool
}

type Server struct {
	handler   *Handler
	tlsConfig *tls.Config
	webMux    http.Handler
	// dohHandler is the concrete DoH front-end behind webMux. It carries the
	// token list and the Alt-Svc advertisement, so the server keeps a reference
	// instead of reaching through the restricted mux wrapper.
	dohHandler  *DoHHandler
	udpServer   *dns.Server
	tcpServer   *dns.Server
	dotServer   *dns.Server
	dohServer   *http.Server
	doh3Server  *http3.Server
	doh3Conn    net.PacketConn
	doqListener *quic.Listener
	doqCtx      context.Context
	doqCancel   context.CancelFunc
	doqSlots    chan struct{}
	doqConns    map[*quic.Conn]struct{}
	doqMu       sync.Mutex
	doqTokens   []string
	doqTokensMu sync.RWMutex
	doqQueries  atomic.Uint64
	doqAuthFail atomic.Uint64
	bindHost    string
	port        int
	dotPort     int
	dohPort     int
	doqPort     int

	// transportCfg is the last applied policy. The DoQ fields are carried here
	// for the QUIC listener; DoH3 is applied by ApplyTransports.
	transportCfg TransportConfig

	// dotCertSource is the DoT/DoH listeners' dedicated certificate (the
	// custom DoH/DoT domain). nil means both ride tlsConfig — the panel pair.
	dotCertSource CertSource

	// tlsMu serializes the TLS-listener lifecycle: SetDOTCertSource and
	// RebindTLS are reachable from concurrent dashboard saves (and from the
	// daily renewal loop racing one), and an interleaved shutdown/start used
	// to race on dotServer/dohServer — a lost update could double-bind a port
	// or leave a listener down with the bind failure only logged. startDoT and
	// startHTTPS read dotCertSource, so they are called under it too.
	tlsMu sync.Mutex
}

func NewServer(
	h *Handler,
	tlsCfg *tls.Config,
	webMux http.Handler,
	doh *DoHHandler,
	bindHost string,
	cfg TransportConfig,
) *Server {
	if cfg.Port == 0 {
		cfg.Port = 53
	}
	if cfg.DoTPort == 0 {
		cfg.DoTPort = 853
	}
	if cfg.DoHPort == 0 {
		cfg.DoHPort = 8443
	}
	if cfg.DoQPort == 0 {
		cfg.DoQPort = 853
	}
	if cfg.DoQAuth == "" {
		cfg.DoQAuth = "ip"
	}
	if bindHost == "" {
		bindHost = "0.0.0.0"
	}

	return &Server{
		handler:      h,
		tlsConfig:    tlsCfg,
		webMux:       webMux,
		dohHandler:   doh,
		bindHost:     bindHost,
		port:         cfg.Port,
		dotPort:      cfg.DoTPort,
		dohPort:      cfg.DoHPort,
		doqPort:      cfg.DoQPort,
		transportCfg: cfg,
	}
}

func (s *Server) addr(port int) string {
	host := s.bindHost
	// Dual-stack on the unspecified wildcard: "0.0.0.0" binds IPv4 only, but a
	// host with AAAA records is reached (and ACME-validated, for the DoH port)
	// over IPv6. "::" accepts both families on every mainstream stack. An
	// operator who names an address keeps exactly what they named.
	if host == "" || host == "0.0.0.0" {
		host = "::"
	}
	return net.JoinHostPort(host, strconv.Itoa(port))
}

func idle(d time.Duration) func() time.Duration {
	return func() time.Duration { return d }
}

// Start binds every listener up front so that a failure (privileged port, port
// already in use, bad bind host) is returned to the caller instead of being
// swallowed inside a goroutine. Plain DNS on :53 is mandatory and produces an
// error; the optional DoT/DoH listeners only warn.
func (s *Server) Start() error {
	udpAddr := s.addr(s.port)
	pc, err := net.ListenPacket("udp", udpAddr)
	if err != nil {
		return fmt.Errorf("bind DNS UDP %s: %w", udpAddr, err)
	}
	s.udpServer = &dns.Server{PacketConn: pc, Handler: s.handler}
	// Capture the server in a local for the closure (v2.1.0 B-05 remediation):
	// the goroutine read the shared field, and the TCP-bind failure path below
	// nils it — a race whose loser dereferenced nil and took the whole process
	// down during startup error handling. The local never changes, so the
	// goroutine is immune to whatever happens to the field afterwards.
	udpSrv := s.udpServer
	go func() {
		log.Printf("[DNS] Starting UDP listener on %s", udpAddr)
		if err := udpSrv.ActivateAndServe(); err != nil {
			log.Printf("[DNS] UDP server stopped: %v", err)
		}
	}()

	tcpAddr := s.addr(s.port)
	tcpLn, err := net.Listen("tcp", tcpAddr)
	if err != nil {
		// The UDP goroutine holds its own reference, so shutting down through
		// the captured server is safe and the field is simply left pointing at
		// the (now shut-down) server instead of being nilled under it.
		_ = udpSrv.Shutdown()
		_ = pc.Close()
		return fmt.Errorf("bind DNS TCP %s: %w", tcpAddr, err)
	}
	s.tcpServer = &dns.Server{
		Listener:      netutil.LimitListener(tcpLn, maxTCPConns),
		Handler:       s.handler,
		IdleTimeout:   idle(tcpIdleTimeout),
		MaxTCPQueries: -1,
	}
	go func() {
		log.Printf("[DNS] Starting TCP listener on %s (idle %s)", tcpAddr, tcpIdleTimeout)
		if err := s.tcpServer.ActivateAndServe(); err != nil {
			log.Printf("[DNS] TCP server stopped: %v", err)
		}
	}()

	s.startDoT()
	s.startHTTPS()
	if err := s.startDoH3Locked(); err != nil {
		log.Printf("[DNS] Warning: %v", err)
	}
	if err := s.startDoQ(); err != nil {
		log.Printf("[DNS] Warning: %v", err)
	}
	return nil
}

// startDoT brings up the optional DNS-over-TLS listener. At boot a failure
// here is a warning, not an error — the resolver still answers on 53 — so
// Start discards the return; RebindTLS propagates it, because a rebind that
// leaves the listener down must not read as success.
func (s *Server) startDoT() error {
	if s.tlsConfig == nil {
		return nil
	}
	dotAddr := s.addr(s.dotPort)

	// Its own copy of the TLS config, and this is not tidiness. http.Server
	// appends "h2" and "http/1.1" to the NextProtos of whatever config it is
	// handed, in place, when ServeTLS starts. While both listeners shared one
	// config that mutation reached this one too, so DoT advertised HTTP/2 — and
	// Go's TLS stack aborts a handshake when the client offers ALPN and nothing
	// matches. A standards-compliant DoT client asking for "dot" was refused
	// outright, and only after the HTTPS dashboard came up, which is the kind of
	// failure that gets blamed on the client.
	dotCfg := s.tlsConfig.Clone()
	dotCfg.NextProtos = []string{alpnDoT}
	applyDOTCertSource(dotCfg, s.dotCertSource)

	dotLn, err := tls.Listen("tcp", dotAddr, dotCfg)
	if err != nil {
		log.Printf("[DNS] Warning: could not bind DoT on %s: %v", dotAddr, err)
		return fmt.Errorf("bind DoT %s: %w", dotAddr, err)
	}
	s.dotServer = &dns.Server{
		Listener:      netutil.LimitListener(dotLn, maxDoTConns),
		Net:           "tcp-tls",
		TLSConfig:     dotCfg,
		Handler:       s.handler.ServeWithProto("DoT"),
		IdleTimeout:   idle(dotIdleTimeout),
		MaxTCPQueries: -1,
	}
	go func() {
		log.Printf("[DNS] Starting DoT (DNS-over-TLS) on %s (idle %s, max %d conns)", dotAddr, dotIdleTimeout, maxDoTConns)
		if err := s.dotServer.ActivateAndServe(); err != nil {
			log.Printf("[DNS] DoT listener stopped: %v", err)
		}
	}()
	return nil
}

// applyDOTCertSource points a cloned listener config at the dedicated DoH/DoT
// certificate source. Clone copies the function value, not the certificate,
// so the live listener resolves the pair at handshake time — a Swap on the
// holder reaches every already-serving listener with no rebind.
func applyDOTCertSource(cfg *tls.Config, src CertSource) {
	if src == nil {
		return
	}
	cfg.GetCertificate = src.GetCertificate
	cfg.Certificates = nil
}

// SetDOTCertSource wires the dedicated DoH/DoT certificate source. Call
// before Start; to change it on a running server, rebind with RebindTLS.
// Passing nil detaches a dedicated source so the listeners fall back to the
// panel certificate — the clear-the-dot-domain gesture depends on that.
func (s *Server) SetDOTCertSource(src CertSource) {
	s.tlsMu.Lock()
	s.dotCertSource = src
	s.tlsMu.Unlock()
}

// RebindTLS restarts the DoT and DoH listeners so a changed dotCertSource
// (set <-> nil) reaches them. Port 53 and every relay are untouched. Needed
// only for the mode flip: renewals of the same name hot-swap through the
// source's closure and never rebind.
//
// The returned error covers the listeners that could not come back up — a
// bind failure is logged by the starter but must not read as success to the
// caller, which has just told the operator "the listeners now serve X".
func (s *Server) RebindTLS() error {
	s.tlsMu.Lock()
	defer s.tlsMu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
	defer cancel()
	if s.dotServer != nil {
		_ = s.dotServer.ShutdownContext(ctx)
		s.dotServer = nil
	}
	if s.dohServer != nil {
		_ = s.dohServer.Close()
		s.dohServer = nil
	}
	s.stopDoH3Locked()
	s.stopDoQ()
	var errs []error
	if err := s.startDoT(); err != nil {
		errs = append(errs, err)
	}
	if err := s.startHTTPS(); err != nil {
		errs = append(errs, err)
	}
	if s.transportCfg.DoH3Enabled {
		if err := s.startDoH3Locked(); err != nil {
			errs = append(errs, err)
		}
	}
	if s.transportCfg.DoQEnabled {
		if err := s.startDoQ(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// startHTTPS brings up the TLS dashboard, which is also where the DoH endpoint
// lives. Optional in the same way DoT is; the error return follows the same
// rule as startDoT's — Start discards it, RebindTLS propagates it.
func (s *Server) startHTTPS() error {
	if s.tlsConfig == nil || s.webMux == nil {
		return nil
	}
	dohAddr := s.addr(s.dohPort)

	dohLn, err := net.Listen("tcp", dohAddr)
	if err != nil {
		log.Printf("[DNS] Warning: could not bind HTTPS/DoH on %s: %v", dohAddr, err)
		return fmt.Errorf("bind HTTPS/DoH %s: %w", dohAddr, err)
	}

	dohCfg := s.tlsConfig.Clone()
	dohCfg.NextProtos = []string{"h2", "http/1.1"}
	applyDOTCertSource(dohCfg, s.dotCertSource)

	s.dohServer = &http.Server{
		Handler:           s.webMux,
		TLSConfig:         dohCfg,
		ReadHeaderTimeout: httpsHeaderTimeout,
		// WriteTimeout must stay 0 here for the same reason it does on the
		// plaintext server: the SSE query stream holds a response open for as
		// long as the dashboard is watching, and a write deadline would sever it
		// every minute.
		WriteTimeout: 0,
		IdleTimeout:  httpsIdleTimeout,
	}
	go func() {
		log.Printf("[DNS] Starting DoH & public portal on https://%s/dns-query", dohAddr)
		if err := s.dohServer.ServeTLS(netutil.LimitListener(dohLn, maxHTTPSConns), "", ""); err != nil && err != http.ErrServerClosed {
			log.Printf("[DNS] HTTPS/DoH TLS server stopped: %v", err)
		}
	}()
	return nil
}

// startDoH3Locked brings up the HTTP/3 face of the DoH endpoint. It is opt-in,
// so an operator who never enables it pays for no UDP socket and no QUIC
// state. The caller must hold tlsMu or be Start before any listener serves.
func (s *Server) startDoH3Locked() error {
	if !s.transportCfg.DoH3Enabled || s.doh3Server != nil {
		return nil
	}
	if s.tlsConfig == nil || s.webMux == nil {
		return errors.New("DoH3 requires a TLS certificate and the DoH handler")
	}
	addr := s.addr(s.dohPort)
	pc, err := net.ListenPacket("udp", addr)
	if err != nil {
		return fmt.Errorf("bind DoH3 %s: %w", addr, err)
	}

	h3Cfg := s.tlsConfig.Clone()
	h3Cfg.NextProtos = []string{"h3"}
	applyDOTCertSource(h3Cfg, s.dotCertSource)

	srv := &http3.Server{
		Handler:   s.webMux,
		TLSConfig: h3Cfg,
		QUICConfig: &quic.Config{
			MaxIdleTimeout:        httpsIdleTimeout,
			MaxIncomingStreams:    256,
			MaxIncomingUniStreams: 0,
			Allow0RTT:             false,
		},
	}
	s.doh3Server = srv
	s.doh3Conn = pc
	if s.dohHandler != nil {
		s.dohHandler.SetAltSvc(fmt.Sprintf(`h3=":%d"; ma=86400`, s.dohPort))
	}
	go func() {
		log.Printf("[DNS] Starting DoH3 (HTTP/3) on udp://%s/dns-query", addr)
		if err := srv.Serve(pc); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("[DNS] DoH3 listener stopped: %v", err)
		}
	}()
	return nil
}

// stopDoH3Locked tears down the HTTP/3 listener and stops advertising it. The
// caller must hold tlsMu. Shutdown gets the same grace as the other listeners,
// then Close severs whatever is left so a long-lived request cannot hold the
// daemon open.
func (s *Server) stopDoH3Locked() {
	if s.dohHandler != nil {
		s.dohHandler.SetAltSvc("")
	}
	if s.doh3Server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
		_ = s.doh3Server.Shutdown(ctx)
		// Shutdown gets the grace window; closing the packet connection below
		// tears down anything still open without waiting for a client that may
		// never finish.
		cancel()
		s.doh3Server = nil
	}
	if s.doh3Conn != nil {
		_ = s.doh3Conn.Close()
		s.doh3Conn = nil
	}
}

// ApplyTransports applies a live DoH3 change. DoQ is stored in the config for
// the QUIC listener phase but is not started here yet; enabling it before that
// phase is a clear error rather than a silent no-op.
// TransportStatus reports whether the optional QUIC listeners are actually
// serving. The settings card uses this rather than the saved toggles so a bind
// failure cannot present itself as a running endpoint.
func (s *Server) TransportStatus() (doh3 bool, doq bool) {
	s.tlsMu.Lock()
	defer s.tlsMu.Unlock()
	return s.doh3Server != nil, s.doqListener != nil
}

// TransportMetrics is the small set of QUIC listener counters the Prometheus
// endpoint exposes. Query and protocol aggregates already live in the stats
// service; these are the transport-specific ones it cannot see.
type TransportMetrics struct {
	DoH3Requests   uint64
	DoQQueries     uint64
	DoQAuthFailure uint64
}

func (s *Server) TransportMetrics() TransportMetrics {
	var m TransportMetrics
	if s.dohHandler != nil {
		m.DoH3Requests = s.dohHandler.DoH3Requests()
	}
	m.DoQQueries = s.doqQueries.Load()
	m.DoQAuthFailure = s.doqAuthFail.Load()
	return m
}

func (s *Server) ApplyTransports(cfg TransportConfig) error {
	if cfg.DoHPort == 0 {
		cfg.DoHPort = 8443
	}
	if cfg.DoQPort == 0 {
		cfg.DoQPort = 853
	}
	auth := strings.ToLower(strings.TrimSpace(cfg.DoQAuth))
	if auth == "" {
		auth = "ip"
	}
	if auth != "ip" && auth != "sni-token" {
		return fmt.Errorf("invalid DoQ authentication mode %q", auth)
	}
	cfg.DoQAuth = auth

	s.tlsMu.Lock()
	defer s.tlsMu.Unlock()

	if s.doh3Server != nil && (!cfg.DoH3Enabled || s.dohPort != cfg.DoHPort) {
		s.stopDoH3Locked()
	}
	if s.doqListener != nil && (!cfg.DoQEnabled || s.doqPort != cfg.DoQPort || s.transportCfg.DoQAuth != auth) {
		s.stopDoQ()
	}

	s.transportCfg = cfg
	s.dohPort = cfg.DoHPort
	s.doqPort = cfg.DoQPort
	if cfg.DoH3Enabled && s.doh3Server == nil {
		if err := s.startDoH3Locked(); err != nil {
			return err
		}
	}
	if cfg.DoQEnabled && s.doqListener == nil {
		if err := s.startDoQ(); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) Stop() {
	ctx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
	defer cancel()

	for _, srv := range []*dns.Server{s.udpServer, s.tcpServer} {
		if srv != nil {
			_ = srv.ShutdownContext(ctx)
		}
	}
	// The TLS listeners' fields are mutated under tlsMu (RebindTLS); shutdown
	// reads them under the same lock so a concurrent rebind cannot leave a
	// freshly rebound listener that nothing ever closes.
	s.tlsMu.Lock()
	dotServer, dohServer := s.dotServer, s.dohServer
	s.dotServer, s.dohServer = nil, nil
	s.stopDoH3Locked()
	s.stopDoQ()
	s.tlsMu.Unlock()
	if dotServer != nil {
		_ = dotServer.ShutdownContext(ctx)
	}
	if dohServer != nil {
		// Shutdown drains in-flight requests, which the SSE stream never
		// finishes, so the deadline is expected to fire; Close then severs what
		// is left instead of leaving it to the process exit.
		if err := dohServer.Shutdown(ctx); err != nil {
			_ = dohServer.Close()
		}
	}
}
