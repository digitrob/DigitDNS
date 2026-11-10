package dns

import (
	"context"
	"crypto/subtle"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"strings"
	"time"

	"digitdns/internal/safego"
	"github.com/miekg/dns"
	"github.com/quic-go/quic-go"
)

// RFC 9250: DoQ uses the "doq" ALPN, one query per bidirectional stream, a
// two-octet length prefix, and transaction ID 0. These error codes are the
// application/stream codes the RFC defines for the cases this server can hit.
const (
	alpnDoQ           = "doq"
	doqMaxMessageSize = 4096
	doqIdleTimeout    = 60 * time.Second
	doqStreamTimeout  = 5 * time.Second
	maxDoQConns       = 1024
	maxDoQStreams     = 128
	doqInternalError  = 0x1
	doqProtocolError  = 0x2
	doqExcessiveLoad  = 0x4
)

// startDoQ brings up the RFC 9250 listener. The caller holds tlsMu or is Start
// before any listener serves. DoQ is opt-in; a disabled server pays for nothing.
func (s *Server) startDoQ() error {
	if !s.transportCfg.DoQEnabled || s.doqListener != nil {
		return nil
	}
	if s.tlsConfig == nil {
		err := errors.New("DoQ requires a TLS certificate")
		s.setDoQError(err.Error())
		return err
	}
	auth := strings.ToLower(strings.TrimSpace(s.transportCfg.DoQAuth))
	if auth == "" {
		auth = "ip"
	}
	switch auth {
	case "ip":
	case "sni-token":
		if strings.TrimSpace(s.transportCfg.DoQTokenDomain) == "" {
			err := errors.New("DoQ SNI-token authentication requires a token domain")
			s.setDoQError(err.Error())
			return err
		}
		if len(s.doqTokenList()) == 0 {
			err := errors.New("DoQ SNI-token authentication requires at least one token")
			s.setDoQError(err.Error())
			return err
		}
	default:
		err := fmt.Errorf("invalid DoQ authentication mode %q", auth)
		s.setDoQError(err.Error())
		return err
	}

	doqCfg := s.tlsConfig.Clone()
	doqCfg.NextProtos = []string{alpnDoQ}
	applyDOTCertSource(doqCfg, s.dotCertSource)

	limiter := newQUICHandshakeLimiter()
	quicCfg := &quic.Config{
		HandshakeIdleTimeout:           5 * time.Second,
		MaxIdleTimeout:                 doqIdleTimeout,
		MaxIncomingStreams:             maxDoQStreams,
		MaxIncomingUniStreams:          0,
		Allow0RTT:                      false,
		InitialStreamReceiveWindow:     16 << 10,
		MaxStreamReceiveWindow:         64 << 10,
		InitialConnectionReceiveWindow: 64 << 10,
		MaxConnectionReceiveWindow:     256 << 10,
	}
	// DoQ connections are cheap after the handshake but the handshake is not.
	// Refusing before TLS is what makes a flood cost one lookup instead of a key
	// exchange, and the client sees a normal QUIC connection refusal.
	quicCfg.GetConfigForClient = func(info *quic.ClientInfo) (*quic.Config, error) {
		if limiter.Allow(info.RemoteAddr, time.Now()) {
			return quicCfg, nil
		}
		s.doqHandshakeRejects.Add(1)
		s.logHandshakeReject("DoQ", info.RemoteAddr)
		return nil, errQUICHandshakeLimited
	}

	addr := s.addr(s.doqPort)
	ln, err := quic.ListenAddr(addr, doqCfg, quicCfg)
	if err != nil {
		err = fmt.Errorf("bind DoQ %s: %w", addr, err)
		s.setDoQError(err.Error())
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	slots := make(chan struct{}, maxDoQConns)
	s.doqListener = ln
	s.doqCtx = ctx
	s.doqCancel = cancel
	s.doqSlots = slots
	s.doqConns = make(map[*quic.Conn]struct{})
	s.clearDoQError()
	go s.serveDoQ(ctx, ln, slots)
	log.Printf("[DNS] Starting DoQ (DNS-over-QUIC) on udp://%s", addr)
	return nil
}

// SetDoQTokens installs the SNI-label token list. The API saves tokens before
// applying the transport settings, so a startDoQ in sni-token mode sees the
// list it is about to enforce. Tokens are lower-cased and de-duplicated because
// they are matched against the leftmost DNS label of the SNI.
func (s *Server) SetDoQTokens(tokens []string) {
	seen := make(map[string]bool, len(tokens))
	out := make([]string, 0, len(tokens))
	for _, entry := range tokens {
		token := strings.ToLower(strings.TrimSpace(strings.TrimSuffix(entry, ".")))
		if token == "" || seen[token] {
			continue
		}
		seen[token] = true
		out = append(out, token)
	}
	s.doqTokensMu.Lock()
	s.doqTokens = out
	s.doqTokensMu.Unlock()
}

func (s *Server) doqTokenList() []string {
	s.doqTokensMu.RLock()
	defer s.doqTokensMu.RUnlock()
	return append([]string(nil), s.doqTokens...)
}

// doqSNIAllowed checks the SNI shape <token>.<doq_token_domain> against the
// configured token list. The token is never logged; only the rejection and the
// source address are.
func (s *Server) doqSNIAllowed(conn *quic.Conn) bool {
	base := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(s.transportCfg.DoQTokenDomain), "."))
	sni := strings.ToLower(strings.TrimSuffix(conn.ConnectionState().TLS.ServerName, "."))
	if base == "" || sni == "" {
		return false
	}
	suffix := "." + base
	if !strings.HasSuffix(sni, suffix) {
		return false
	}
	token := strings.TrimSuffix(sni, suffix)
	if token == "" || strings.Contains(token, ".") {
		return false
	}
	for _, allowed := range s.doqTokenList() {
		if subtle.ConstantTimeCompare([]byte(token), []byte(allowed)) == 1 {
			return true
		}
	}
	return false
}

// stopDoQ closes the listener and every accepted connection. It is safe to call
// when DoQ was never enabled. The caller holds tlsMu.
func (s *Server) stopDoQ() {
	if s.doqCancel != nil {
		s.doqCancel()
		s.doqCancel = nil
	}
	if s.doqListener != nil {
		_ = s.doqListener.Close()
		s.doqListener = nil
	}
	s.doqMu.Lock()
	conns := s.doqConns
	s.doqConns = nil
	s.doqMu.Unlock()
	for conn := range conns {
		_ = conn.CloseWithError(quic.ApplicationErrorCode(doqExcessiveLoad), "server stopping")
	}
	s.doqCtx = nil
	s.doqSlots = nil
	s.clearDoQError()
}

func (s *Server) serveDoQ(ctx context.Context, ln *quic.Listener, slots chan struct{}) {
	for {
		conn, err := ln.Accept(ctx)
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, quic.ErrServerClosed) {
				return
			}
			log.Printf("[DNS] DoQ accept failed: %v", err)
			continue
		}
		select {
		case slots <- struct{}{}:
		default:
			s.doqHandshakeRejects.Add(1)
			_ = conn.CloseWithError(quic.ApplicationErrorCode(doqExcessiveLoad), "too many connections")
			continue
		}

		s.doqMu.Lock()
		if s.doqConns == nil {
			s.doqMu.Unlock()
			<-slots
			s.doqHandshakeRejects.Add(1)
			_ = conn.CloseWithError(quic.ApplicationErrorCode(doqExcessiveLoad), "server stopping")
			continue
		}
		s.doqConns[conn] = struct{}{}
		s.doqConnsTotal.Add(1)
		s.doqMu.Unlock()

		go func() {
			defer func() {
				<-slots
				s.doqMu.Lock()
				delete(s.doqConns, conn)
				s.doqMu.Unlock()
				_ = conn.CloseWithError(0, "")
			}()
			s.handleDoQConn(ctx, conn)
		}()
	}
}

func (s *Server) handleDoQConn(ctx context.Context, conn *quic.Conn) {
	clientIP, _, err := net.SplitHostPort(conn.RemoteAddr().String())
	if err != nil {
		clientIP = conn.RemoteAddr().String()
	}
	if strings.EqualFold(strings.TrimSpace(s.transportCfg.DoQAuth), "sni-token") && !s.doqSNIAllowed(conn) {
		s.doqAuthFail.Add(1)
		log.Printf("[DNS] DoQ rejected connection from %s: invalid SNI token", clientIP)
		_ = conn.CloseWithError(quic.ApplicationErrorCode(doqProtocolError), "invalid DoQ token")
		return
	}
	for {
		stream, err := conn.AcceptStream(ctx)
		if err != nil {
			return
		}
		go func() {
			defer safego.Recover("doq.handleStream")
			s.handleDoQStream(conn, stream, clientIP)
		}()
	}
}

func (s *Server) handleDoQStream(conn *quic.Conn, stream *quic.Stream, clientIP string) {
	_ = stream.SetReadDeadline(time.Now().Add(doqStreamTimeout))

	var lenBuf [2]byte
	if _, err := io.ReadFull(stream, lenBuf[:]); err != nil {
		s.doqProtoErrors.Add(1)
		closeDoQStream(stream, doqProtocolError)
		return
	}
	size := int(binary.BigEndian.Uint16(lenBuf[:]))
	if size == 0 || size > doqMaxMessageSize {
		s.doqProtoErrors.Add(1)
		closeDoQStream(stream, doqProtocolError)
		return
	}
	body := make([]byte, size)
	if _, err := io.ReadFull(stream, body); err != nil {
		s.doqProtoErrors.Add(1)
		closeDoQStream(stream, doqProtocolError)
		return
	}

	var query dns.Msg
	if err := query.Unpack(body); err != nil {
		s.doqProtoErrors.Add(1)
		closeDoQStream(stream, doqProtocolError)
		return
	}
	// RFC 9250 §4.2.1: DoQ queries carry transaction ID 0. A nonzero ID is a
	// protocol error, not something to silently rewrite.
	if query.Id != 0 {
		s.doqProtoErrors.Add(1)
		closeDoQStream(stream, doqProtocolError)
		return
	}
	s.doqQueries.Add(1)

	resp := s.handler.ProcessQuery(&query, clientIP, "DoQ", clientIP)
	if resp == nil {
		closeDoQStream(stream, doqInternalError)
		return
	}
	resp.Id = 0
	packed, err := resp.Pack()
	if err != nil || len(packed) > 0xffff {
		closeDoQStream(stream, doqInternalError)
		return
	}

	binary.BigEndian.PutUint16(lenBuf[:], uint16(len(packed)))
	_ = stream.SetWriteDeadline(time.Now().Add(doqStreamTimeout))
	if _, err := stream.Write(lenBuf[:]); err != nil {
		return
	}
	if _, err := stream.Write(packed); err != nil {
		return
	}
	_ = stream.Close()
}

func closeDoQStream(stream *quic.Stream, code uint64) {
	stream.CancelRead(quic.StreamErrorCode(code))
	stream.CancelWrite(quic.StreamErrorCode(code))
	_ = stream.Close()
}
