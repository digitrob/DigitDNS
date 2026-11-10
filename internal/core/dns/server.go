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
	dnscryptlib "github.com/ameshkov/dnscrypt/v2"
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
	// maxDoH3Conns matches DoQ's cap. HTTP/3 multiplexes every request from a
	// client onto one connection, so this is far more headroom than a resolver
	// needs; it exists so a handshake flood that slips past the per-source
	// limiter cannot pin one goroutine per connection.
	maxDoH3Conns = 1024
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
	// DoT is enabled unless DoTDisabled is set, matching the database field's
	// backward-compatible default. DoT tokens use the same <token>.<domain>
	// shape as DoQ; mTLS applies to DoT, DoH and DoQ.
	DoTDisabled    bool
	DoTAuth        string
	DoTTokenDomain string
	MTLSEnabled    bool
	MTLSCAPEM      string
	// DNSCrypt provider keys live in the separate dnscrypt setting and are
	// installed with SetDNSCryptConfig before ApplyTransports.
	DNSCryptEnabled bool
	DNSCryptPort    int
}

type Server struct {
	handler   *Handler
	tlsConfig *tls.Config
	webMux    http.Handler
	// dohHandler is the concrete DoH front-end behind webMux. It carries the
	// token list and the Alt-Svc advertisement, so the server keeps a reference
	// instead of reaching through the restricted mux wrapper.
	dohHandler *DoHHandler
	udpServer  *dns.Server
	tcpServer  *dns.Server
	dotServer  *dns.Server
	dohServer  *http.Server
	doh3Server *http3.Server
	doh3Conn   net.PacketConn
	// doh3Ready points at the current listener's readiness flag. A pointer per
	// listen, not one shared bool: when an old Serve goroutine exits after a
	// restart, it must clear only its own flag, never the new listener's. The
	// field itself is guarded by tlsMu, and the flag it points at is atomic so
	// TransportStatus can read it under the same lock.
	doh3Ready atomic.Pointer[atomic.Bool]
	// doh3Conns is the live HTTP/3 connection set. A map, not a counter, so a
	// connection context that ends after a listener restart can only delete its
	// own entry and never drive a new listener's gauge negative.
	doh3ConnsMu    sync.Mutex
	doh3Conns      map[*quic.Conn]struct{}
	doh3ConnsTotal atomic.Uint64
	// doh3HandshakeRejects counts connections refused by the per-source limiter
	// or the connection cap. It is a counter because the interesting signal is a
	// rate, not the current queue depth.
	doh3HandshakeRejects atomic.Uint64
	// listenerErrMu guards the last asynchronous listener failure. The save
	// response cannot see a Serve goroutine that dies later, so TransportStatus
	// carries this alongside the booleans and the settings card keeps showing it.
	listenerErrMu sync.Mutex
	doh3Err       string
	doqErr        string
	doqListener   *quic.Listener
	doqCtx        context.Context
	doqCancel     context.CancelFunc
	doqSlots      chan struct{}
	doqConns      map[*quic.Conn]struct{}
	doqMu         sync.Mutex
	doqTokens     []string
	doqTokensMu   sync.RWMutex
	doqQueries    atomic.Uint64
	doqAuthFail   atomic.Uint64
	doqConnsTotal atomic.Uint64
	// doqHandshakeRejects covers both the pre-handshake limiter and the
	// established-connection cap that closes a conn with EXCESSIVE_LOAD.
	doqHandshakeRejects atomic.Uint64
	doqProtoErrors      atomic.Uint64
	// DoT SNI-token list and DNSCrypt listener state. The DNSCrypt keys are
	// installed by SetDNSCryptConfig; the listener itself is lifecycle-managed
	// under tlsMu like the other encrypted transports.
	dotTokens         []string
	dotTokensMu       sync.RWMutex
	dnscryptServer    *dnscryptlib.Server
	dnscryptUDP       *net.UDPConn
	dnscryptTCP       net.Listener
	dnscryptRC        *dnscryptlib.ResolverConfig
	dnscryptCert      *dnscryptlib.Cert
	dnscryptQueries   atomic.Uint64
	dnscryptErrors    atomic.Uint64
	dnscryptListening atomic.Bool
	dnscryptErr       string
	// quicRejectLogAt rate-limits the "handshake refused" log line. A flood is
	// exactly when it would otherwise write once per packet.
	quicRejectLogAt atomic.Int64
	bindHost        string
	port            int
	dotPort         int
	dohPort         int
	doqPort         int

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
	if cfg.DNSCryptPort == 0 {
		cfg.DNSCryptPort = defaultDNSCryptPort
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
	if err := s.startDNSCryptLocked(); err != nil {
		log.Printf("[DNS] Warning: %v", err)
	}
	return nil
}

// startDoT brings up the optional DNS-over-TLS listener. At boot a failure
// here is a warning, not an error — the resolver still answers on 53 — so
// Start discards the return; RebindTLS propagates it, because a rebind that
// leaves the listener down must not read as success.
func (s *Server) startDoT() error {
	if s.transportCfg.DoTDisabled {
		return nil
	}
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
	if err := s.applyMTLS(dotCfg, true); err != nil {
		log.Printf("[DNS] Warning: DoT mTLS: %v", err)
		return fmt.Errorf("configure DoT mTLS: %w", err)
	}

	auth := strings.ToLower(strings.TrimSpace(s.transportCfg.DoTAuth))
	if auth == "" {
		auth = "ip"
	}
	switch auth {
	case "ip":
	case "sni-token":
		if strings.TrimSpace(s.transportCfg.DoTTokenDomain) == "" {
			return errors.New("DoT SNI-token authentication requires a token domain")
		}
		if len(s.dotTokenList()) == 0 {
			return errors.New("DoT SNI-token authentication requires at least one token")
		}
	default:
		return fmt.Errorf("invalid DoT authentication mode %q", auth)
	}

	dotLn, err := tls.Listen("tcp", dotAddr, dotCfg)
	if err != nil {
		log.Printf("[DNS] Warning: could not bind DoT on %s: %v", dotAddr, err)
		return fmt.Errorf("bind DoT %s: %w", dotAddr, err)
	}
	var listener net.Listener = dotLn
	if auth == "sni-token" {
		listener = &sniTokenListener{Listener: dotLn, check: s.dotSNIAllowed}
	}
	dotSrv := &dns.Server{
		Listener:      netutil.LimitListener(listener, maxDoTConns),
		Net:           "tcp-tls",
		TLSConfig:     dotCfg,
		Handler:       s.handler.ServeWithProto("DoT"),
		IdleTimeout:   idle(dotIdleTimeout),
		MaxTCPQueries: -1,
	}
	s.dotServer = dotSrv
	// Capture the server in a local: a rebind nils s.dotServer while this
	// goroutine is still starting, and reading the field here would panic on a
	// nil dereference (the same hazard the port-53 listeners document above).
	go func() {
		log.Printf("[DNS] Starting DoT (DNS-over-TLS) on %s (idle %s, max %d conns)", dotAddr, dotIdleTimeout, maxDoTConns)
		if err := dotSrv.ActivateAndServe(); err != nil {
			// A deliberate rebind closes the listener; only a listener that
			// died while still installed is news.
			s.tlsMu.Lock()
			current := s.dotServer == dotSrv
			s.tlsMu.Unlock()
			if current {
				log.Printf("[DNS] DoT listener stopped: %v", err)
			}
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
	return s.rebindTLSListenersLocked()
}

// rebindTLSListenersLocked restarts DoT, DoH, DoH3 and DoQ from the current
// transportCfg. The caller holds tlsMu. DNSCrypt is independent of TLS and is
// left alone.
func (s *Server) rebindTLSListenersLocked() error {
	ctx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
	defer cancel()
	if dotSrv := s.dotServer; dotSrv != nil {
		s.dotServer = nil
		// Close the listening socket before waiting for the serve loop: the
		// shutdown channel is closed just before serveTCP's deferred listener
		// close, so waiting alone can hand the rebind a port that is still in
		// use for a few microseconds.
		if ln := dotSrv.Listener; ln != nil {
			_ = ln.Close()
		}
		_ = dotSrv.ShutdownContext(ctx)
	}
	if dohSrv := s.dohServer; dohSrv != nil {
		s.dohServer = nil
		_ = dohSrv.Close()
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
	if err := s.applyMTLS(dohCfg, false); err != nil {
		log.Printf("[DNS] Warning: DoH mTLS: %v", err)
		return fmt.Errorf("configure DoH mTLS: %w", err)
	}
	if s.dohHandler != nil {
		s.dohHandler.SetMTLSRequire(s.transportCfg.MTLSEnabled)
	}

	dohSrv := &http.Server{
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
	s.dohServer = dohSrv
	go func() {
		log.Printf("[DNS] Starting DoH & public portal on https://%s/dns-query", dohAddr)
		if err := dohSrv.ServeTLS(netutil.LimitListener(dohLn, maxHTTPSConns), "", ""); err != nil && err != http.ErrServerClosed {
			log.Printf("[DNS] HTTPS/DoH TLS server stopped: %v", err)
		}
	}()
	return nil
}

// startDoH3Locked brings up the HTTP/3 face of the DoH endpoint. It is opt-in,
// so an operator who never enables it pays for no UDP socket and no QUIC
// state. The caller must hold tlsMu or be Start before any listener serves.
//
// It does not return until the Serve goroutine has been entered. That is the
// readiness guarantee the settings card needs: a bind failure is synchronous
// and returned, while a failure after the bind (or a Serve that exits later) is
// recorded in doh3Err and reflected by TransportStatus instead of leaving a
// non-nil server that looks healthy forever.
func (s *Server) startDoH3Locked() error {
	if !s.transportCfg.DoH3Enabled || s.doh3Server != nil {
		return nil
	}
	if s.tlsConfig == nil || s.webMux == nil {
		err := errors.New("DoH3 requires a TLS certificate and the DoH handler")
		s.setDoH3Error(err.Error())
		return err
	}
	addr := s.addr(s.dohPort)
	pc, err := net.ListenPacket("udp", addr)
	if err != nil {
		err = fmt.Errorf("bind DoH3 %s: %w", addr, err)
		s.setDoH3Error(err.Error())
		return err
	}

	h3Cfg := s.tlsConfig.Clone()
	h3Cfg.NextProtos = []string{"h3"}
	applyDOTCertSource(h3Cfg, s.dotCertSource)
	if err := s.applyMTLS(h3Cfg, false); err != nil {
		err = fmt.Errorf("configure DoH3 mTLS: %w", err)
		s.setDoH3Error(err.Error())
		return err
	}
	if s.dohHandler != nil {
		s.dohHandler.SetMTLSRequire(s.transportCfg.MTLSEnabled)
	}

	limiter := newQUICHandshakeLimiter()
	s.doh3ConnsMu.Lock()
	s.doh3Conns = make(map[*quic.Conn]struct{})
	s.doh3ConnsMu.Unlock()

	quicCfg := &quic.Config{
		HandshakeIdleTimeout: 5 * time.Second,
		MaxIdleTimeout:       httpsIdleTimeout,
		MaxIncomingStreams:   256,
		// HTTP/3 needs the unidirectional streams for its control and QPACK
		// channels — quic-go opens them itself. Zero here would break the
		// protocol, so cap them rather than disabling them.
		MaxIncomingUniStreams:          8,
		Allow0RTT:                      false,
		InitialStreamReceiveWindow:     64 << 10,
		MaxStreamReceiveWindow:         1 << 20,
		InitialConnectionReceiveWindow: 256 << 10,
		MaxConnectionReceiveWindow:     4 << 20,
	}
	// The hook runs before the connection is allocated and TLS starts, so a
	// source over its handshake budget costs a map lookup and one refusal packet.
	quicCfg.GetConfigForClient = func(info *quic.ClientInfo) (*quic.Config, error) {
		if limiter.Allow(info.RemoteAddr, time.Now()) {
			return quicCfg, nil
		}
		s.doh3HandshakeRejects.Add(1)
		s.logHandshakeReject("DoH3", info.RemoteAddr)
		return nil, errQUICHandshakeLimited
	}

	srv := &http3.Server{
		Handler:    s.webMux,
		TLSConfig:  h3Cfg,
		QUICConfig: quicCfg,
		ConnContext: func(ctx context.Context, conn *quic.Conn) context.Context {
			if !s.trackDoH3Conn(conn) {
				return ctx
			}
			go func() {
				<-ctx.Done()
				s.doh3ConnsMu.Lock()
				delete(s.doh3Conns, conn)
				s.doh3ConnsMu.Unlock()
			}()
			return ctx
		},
	}
	s.doh3Server = srv
	s.doh3Conn = pc
	ready := new(atomic.Bool)
	s.doh3Ready.Store(ready)
	s.clearDoH3Error()
	if s.dohHandler != nil {
		s.dohHandler.SetAltSvc(fmt.Sprintf(`h3=":%d"; ma=86400`, s.dohPort))
	}

	started := make(chan struct{})
	go func() {
		log.Printf("[DNS] Starting DoH3 (HTTP/3) on udp://%s/dns-query", addr)
		// Serve returns only when the listener closes or setup fails; everything
		// before this point (bind, TLS clone, QUIC config) is synchronous and
		// already succeeded. Marking ready here closes the window in which the
		// save response would report "enabled, not listening" for a live server.
		ready.Store(true)
		close(started)
		err := srv.Serve(pc)
		ready.Store(false)
		if err == nil || errors.Is(err, http.ErrServerClosed) {
			return
		}
		log.Printf("[DNS] DoH3 listener stopped: %v", err)

		// Serve is gone; drop the dead fields so ApplyTransports can retry and
		// TransportStatus cannot keep reporting a listener that no longer exists.
		// The lock also makes a restart race harmless: if srv is no longer the
		// current listener, a newer one owns the fields and this one only closes
		// its own socket.
		s.tlsMu.Lock()
		if s.doh3Server == srv {
			s.setDoH3Error("DoH3 listener stopped: " + err.Error())
			s.doh3Server = nil
			s.doh3Conn = nil
			s.doh3Ready.Store(nil)
			s.doh3ConnsMu.Lock()
			s.doh3Conns = nil
			s.doh3ConnsMu.Unlock()
			if s.dohHandler != nil {
				s.dohHandler.SetAltSvc("")
			}
		}
		s.tlsMu.Unlock()
		_ = pc.Close()
	}()
	<-started
	return nil
}

// doh3IsReady reports whether the current HTTP/3 Serve loop is live. The caller
// must hold tlsMu (or be Start before listeners exist).
func (s *Server) doh3IsReady() bool {
	ready := s.doh3Ready.Load()
	return ready != nil && ready.Load()
}

// trackDoH3Conn admits an established HTTP/3 connection under the global cap.
// The handshake limiter already bounds new handshakes per source; this bounds
// how many live connections one process will hold.
func (s *Server) trackDoH3Conn(conn *quic.Conn) bool {
	if conn == nil {
		return false
	}
	s.doh3ConnsMu.Lock()
	defer s.doh3ConnsMu.Unlock()
	if s.doh3Conns == nil {
		// A stop landed between Serve starting and the first connection; the
		// server is being torn down, so refuse rather than resurrect the set.
		s.doh3HandshakeRejects.Add(1)
		_ = conn.CloseWithError(quic.ApplicationErrorCode(doqExcessiveLoad), "server stopping")
		return false
	}
	if len(s.doh3Conns) >= maxDoH3Conns {
		s.doh3HandshakeRejects.Add(1)
		_ = conn.CloseWithError(quic.ApplicationErrorCode(doqExcessiveLoad), "too many connections")
		return false
	}
	s.doh3Conns[conn] = struct{}{}
	s.doh3ConnsTotal.Add(1)
	return true
}

func (s *Server) doh3ConnectionCount() int {
	s.doh3ConnsMu.Lock()
	defer s.doh3ConnsMu.Unlock()
	return len(s.doh3Conns)
}

// errQUICHandshakeLimited is returned from GetConfigForClient when a source is
// over its handshake budget. quic-go turns it into a refused connection before
// any TLS work, and the text never reaches a client.
var errQUICHandshakeLimited = errors.New("QUIC handshake rate limit reached")

// logHandshakeReject writes at most one line per ten seconds for both QUIC
// transports. The counter is the real signal; this exists so an operator
// tailing the log can tell a limiter refusal from a network fault.
func (s *Server) logHandshakeReject(transport string, remote net.Addr) {
	now := time.Now().Unix()
	last := s.quicRejectLogAt.Load()
	if now-last < 10 {
		return
	}
	if !s.quicRejectLogAt.CompareAndSwap(last, now) {
		return
	}
	log.Printf("[DNS] %s handshake from %s refused: per-source rate limit", transport, sourceHost(remote))
}

// stopDoH3Locked tears down the HTTP/3 listener and stops advertising it. The
// caller must hold tlsMu. Shutdown gets the same grace as the other listeners,
// then Close severs whatever is left so a long-lived request cannot hold the
// daemon open.
func (s *Server) stopDoH3Locked() {
	s.doh3Ready.Store(nil)
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
	s.doh3ConnsMu.Lock()
	s.doh3Conns = nil
	s.doh3ConnsMu.Unlock()
	s.clearDoH3Error()
}

// ApplyTransports applies a live DoH3/DoQ change. The caller gets a synchronous
// bind error; anything that fails after the listener is up is reported through
// TransportStatus and the persistent last_error on the settings card.
//
// TransportStatus reports whether the optional QUIC listeners are actually
// serving, plus the reason a listener that was enabled is not. The settings card
// uses this rather than the saved toggles so a bind failure or a Serve goroutine
// that died later cannot present itself as a running endpoint.
func (s *Server) TransportStatus() (doh3 bool, doq bool, listenerErr string) {
	s.tlsMu.Lock()
	doh3 = s.doh3IsReady()
	doq = s.doqListener != nil
	s.tlsMu.Unlock()

	s.listenerErrMu.Lock()
	switch {
	case s.doh3Err != "" && s.doqErr != "":
		listenerErr = s.doh3Err + "; " + s.doqErr
	case s.doh3Err != "":
		listenerErr = s.doh3Err
	default:
		listenerErr = s.doqErr
	}
	s.listenerErrMu.Unlock()
	return doh3, doq, listenerErr
}

func (s *Server) setDoH3Error(msg string) {
	s.listenerErrMu.Lock()
	s.doh3Err = msg
	s.listenerErrMu.Unlock()
}

func (s *Server) clearDoH3Error() {
	s.setDoH3Error("")
}

func (s *Server) setDoQError(msg string) {
	s.listenerErrMu.Lock()
	s.doqErr = msg
	s.listenerErrMu.Unlock()
}

func (s *Server) clearDoQError() {
	s.setDoQError("")
}

// TransportMetrics is the small set of QUIC listener counters the Prometheus
// endpoint exposes. Query and protocol aggregates already live in the stats
// service; these are the transport-specific ones it cannot see.
//
// Connection gauges answer "is anyone actually using the QUIC listener?", the
// totals separate churn from steady state, and the reject counters make a
// handshake flood or a too-small connection cap visible instead of looking like
// ordinary traffic.
type TransportMetrics struct {
	DoH3Requests         uint64
	DoH3Connections      int64
	DoH3ConnectionsTotal uint64
	DoH3HandshakeRejects uint64

	DoQQueries          uint64
	DoQConnections      int64
	DoQConnectionsTotal uint64
	DoQAuthFailures     uint64
	DoQProtocolErrors   uint64
	DoQHandshakeRejects uint64

	DNSCryptQueries uint64
	DNSCryptErrors  uint64
}

func (s *Server) TransportMetrics() TransportMetrics {
	var m TransportMetrics
	if s.dohHandler != nil {
		m.DoH3Requests = s.dohHandler.DoH3Requests()
	}
	m.DoH3Connections = int64(s.doh3ConnectionCount())
	m.DoH3ConnectionsTotal = s.doh3ConnsTotal.Load()
	m.DoH3HandshakeRejects = s.doh3HandshakeRejects.Load()

	m.DoQQueries = s.doqQueries.Load()
	m.DoQConnections = int64(s.doqConnectionCount())
	m.DoQConnectionsTotal = s.doqConnsTotal.Load()
	m.DoQAuthFailures = s.doqAuthFail.Load()
	m.DoQProtocolErrors = s.doqProtoErrors.Load()
	m.DoQHandshakeRejects = s.doqHandshakeRejects.Load()
	m.DNSCryptQueries = s.dnscryptQueries.Load()
	m.DNSCryptErrors = s.dnscryptErrors.Load()
	return m
}

func (s *Server) doqConnectionCount() int {
	s.doqMu.Lock()
	defer s.doqMu.Unlock()
	return len(s.doqConns)
}

func (s *Server) ApplyTransports(cfg TransportConfig) error {
	if cfg.DoHPort == 0 {
		cfg.DoHPort = 8443
	}
	if cfg.DoQPort == 0 {
		cfg.DoQPort = 853
	}
	if cfg.DNSCryptPort == 0 {
		cfg.DNSCryptPort = defaultDNSCryptPort
	}
	auth := strings.ToLower(strings.TrimSpace(cfg.DoQAuth))
	if auth == "" {
		auth = "ip"
	}
	if auth != "ip" && auth != "sni-token" {
		return fmt.Errorf("invalid DoQ authentication mode %q", auth)
	}
	cfg.DoQAuth = auth
	dotAuth := strings.ToLower(strings.TrimSpace(cfg.DoTAuth))
	if dotAuth == "" {
		dotAuth = "ip"
	}
	if dotAuth != "ip" && dotAuth != "sni-token" {
		return fmt.Errorf("invalid DoT authentication mode %q", dotAuth)
	}
	cfg.DoTAuth = dotAuth
	cfg.DoTTokenDomain = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(cfg.DoTTokenDomain), "."))

	s.tlsMu.Lock()
	defer s.tlsMu.Unlock()

	// A disabled transport is stopped here even when no listener exists: that is
	// also where a boot-time bind error is cleared, so the card does not keep
	// showing a failure for an endpoint the operator has just turned off.
	if !cfg.DoH3Enabled || (s.doh3Server != nil && (s.dohPort != cfg.DoHPort || !s.doh3IsReady())) {
		s.stopDoH3Locked()
	}
	if !cfg.DoQEnabled || (s.doqListener != nil && (s.doqPort != cfg.DoQPort || s.transportCfg.DoQAuth != auth)) {
		s.stopDoQ()
	}
	if !cfg.DNSCryptEnabled || (s.dnscryptServer != nil && s.transportCfg.DNSCryptPort != cfg.DNSCryptPort) {
		s.stopDNSCryptLocked()
	}

	prevDoTAuth := strings.ToLower(strings.TrimSpace(s.transportCfg.DoTAuth))
	if prevDoTAuth == "" {
		prevDoTAuth = "ip"
	}
	// A TLS listener rebind tears down every encrypted connection, so only a
	// real change triggers it: the DoT gate's mode/domain and the mTLS gate.
	// DoT's port change is only meaningful when a port was supplied.
	dotPortChanged := cfg.DoTPort != 0 && s.dotPort != cfg.DoTPort
	needTLSRebind := prevDoTAuth != cfg.DoTAuth ||
		s.transportCfg.DoTDisabled != cfg.DoTDisabled ||
		s.transportCfg.DoTTokenDomain != cfg.DoTTokenDomain ||
		s.transportCfg.MTLSEnabled != cfg.MTLSEnabled ||
		s.transportCfg.MTLSCAPEM != cfg.MTLSCAPEM ||
		dotPortChanged
	s.transportCfg = cfg
	if cfg.DoTPort != 0 {
		s.dotPort = cfg.DoTPort
	}
	s.dohPort = cfg.DoHPort
	s.doqPort = cfg.DoQPort
	if needTLSRebind {
		if err := s.rebindTLSListenersLocked(); err != nil {
			return err
		}
	}
	if cfg.DoH3Enabled && !s.doh3IsReady() {
		if err := s.startDoH3Locked(); err != nil {
			return err
		}
	}
	if cfg.DoQEnabled && s.doqListener == nil {
		if err := s.startDoQ(); err != nil {
			return err
		}
	}
	if cfg.DNSCryptEnabled && s.dnscryptServer == nil {
		if err := s.startDNSCryptLocked(); err != nil {
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
	s.stopDNSCryptLocked()
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
