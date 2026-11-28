package proxy

import (
	"context"
	"encoding/binary"
	"errors"
	"log"
	"net"
	"net/netip"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// QUIC fast-fail guard.
//
// The relay speaks TCP only: it reads a TLS ClientHello or an HTTP request and
// dials the origin. A proxied name answers with this server's address, so a
// client that learned HTTP/3 from an HTTPS/SVCB record (or from an Alt-Svc cache
// predating that record) sends QUIC over UDP to the same address. Nothing is
// listening there, so the client waits for its own handshake timeout — seconds
// of dead air — before retrying over TCP.
//
// A QUIC server that does not support the client's version answers its Initial
// with a Version Negotiation packet. Answering with a version list containing
// only a reserved (greased) version tells the client that no usable version
// exists, so it abandons QUIC immediately and falls back to TCP. That is a
// protocol-correct response for a server with no QUIC support, and it turns a
// multi-second stall into one round trip.
//
// Only datagrams of at least 1200 bytes are answered: a valid QUIC Initial is
// padded to that size, and responding only to Initial-sized datagrams keeps the
// guard from being used as a reflector for small spoofed packets. The reply is
// smaller than the trigger, so the amplification factor is below one.

const (
	// quicMinInitialPacket is the smallest valid client Initial datagram
	// (RFC 9000 §14.1). Anything shorter cannot be a real QUIC handshake.
	quicMinInitialPacket = 1200

	// quicGreasedVersion is a reserved version (0x?a?a?a?a). Clients MUST NOT
	// support a reserved version, so a Version Negotiation packet that lists only
	// this one has no acceptable version and the client aborts QUIC.
	quicGreasedVersion uint32 = 0x1a2a3a4a

	// Per-source and global response budgets. A spoofed-source flood must not
	// turn the guard into a packet amplifier or a CPU sink.
	quicRejectPerSourceRate = 64
	quicRejectGlobalRate    = 8192
	quicRejectSourceTTL     = 10 * time.Second
	quicRejectMaxSources    = 65536
	quicRejectSweepEvery    = 10 * time.Second
)

// quicRejector owns the UDP socket bound alongside the TCP relay. One goroutine
// reads it; the counters are atomic so the stats service can read them at any
// time.
type quicRejector struct {
	conn *net.UDPConn

	rejected atomic.Uint64
	ignored  atomic.Uint64
	limited  atomic.Uint64

	mu        sync.Mutex
	sources   map[netip.Addr]*quicSource
	lastSweep time.Time
	global    float64
	globalAt  time.Time
}

type quicSource struct {
	tokens float64
	last   time.Time
}

func newQUICRejector(conn *net.UDPConn) *quicRejector {
	now := time.Now()
	return &quicRejector{
		conn:      conn,
		sources:   make(map[netip.Addr]*quicSource),
		lastSweep: now,
		global:    quicRejectGlobalRate,
		globalAt:  now,
	}
}

// run reads datagrams until the context ends or the socket is closed by Stop.
func (q *quicRejector) run(ctx context.Context) {
	buf := make([]byte, 64*1024)
	for {
		// The deadline makes context cancellation observable without closing the
		// socket from a second goroutine. A timeout is the normal idle path.
		if err := q.conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
			return
		}
		n, addr, err := q.conn.ReadFromUDPAddrPort(buf)
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				select {
				case <-ctx.Done():
					return
				default:
				}
			}
			// A transient read error (ICMP surfaced on the socket, a bad
			// datagram) must not stop the guard for the life of the process.
			continue
		}
		q.handle(buf[:n], addr)
	}
}

func (q *quicRejector) handle(pkt []byte, addr netip.AddrPort) {
	if len(pkt) < quicMinInitialPacket {
		q.ignored.Add(1)
		return
	}
	dcid, scid, ok := parseQUICLongHeader(pkt)
	if !ok {
		q.ignored.Add(1)
		return
	}
	if !q.allow(addr.Addr()) {
		q.limited.Add(1)
		return
	}
	q.rejected.Add(1)
	_, _ = q.conn.WriteToUDPAddrPort(buildVersionNegotiation(dcid, scid), addr)
}

// allow applies the per-source and global token buckets.
func (q *quicRejector) allow(ip netip.Addr) bool {
	now := time.Now()
	q.mu.Lock()
	defer q.mu.Unlock()

	elapsed := now.Sub(q.globalAt).Seconds()
	q.global = min(quicRejectGlobalRate, q.global+elapsed*quicRejectGlobalRate)
	q.globalAt = now
	if q.global < 1 {
		return false
	}

	g := q.sources[ip]
	if g == nil {
		if len(q.sources) >= quicRejectMaxSources {
			q.sweepLocked(now, true)
			if len(q.sources) >= quicRejectMaxSources {
				return false
			}
		}
		g = &quicSource{tokens: quicRejectPerSourceRate, last: now}
		q.sources[ip] = g
	}
	refill := now.Sub(g.last).Seconds()
	g.tokens = min(quicRejectPerSourceRate, g.tokens+refill*quicRejectPerSourceRate)
	g.last = now
	if g.tokens < 1 {
		return false
	}
	g.tokens--
	q.global--

	if now.Sub(q.lastSweep) >= quicRejectSweepEvery {
		q.sweepLocked(now, false)
	}
	return true
}

// sweepLocked drops idle sources. force bounds the map even when every tracked
// source is recent: only the map-size path passes true.
func (q *quicRejector) sweepLocked(now time.Time, force bool) {
	q.lastSweep = now
	for ip, g := range q.sources {
		if now.Sub(g.last) > quicRejectSourceTTL {
			delete(q.sources, ip)
		}
	}
	if !force {
		return
	}
	for ip := range q.sources {
		if len(q.sources) < quicRejectMaxSources {
			break
		}
		delete(q.sources, ip)
	}
}

func (q *quicRejector) stats() (rejected, ignored, limited uint64) {
	if q == nil {
		return 0, 0, 0
	}
	return q.rejected.Load(), q.ignored.Load(), q.limited.Load()
}

// parseQUICLongHeader extracts the connection IDs from a QUIC long-header
// packet. It returns ok=false for short-header packets, Version Negotiation
// packets (version 0) and truncated input. The packet type bits differ between
// QUIC versions, so this deliberately checks only the invariant long-header
// shape: any non-zero version with well-formed connection IDs is an attempt to
// start or continue a connection this relay cannot serve.
func parseQUICLongHeader(pkt []byte) (dcid, scid []byte, ok bool) {
	const maxCIDLen = 20
	if len(pkt) < 7 || pkt[0]&0x80 == 0 {
		return nil, nil, false
	}
	if binary.BigEndian.Uint32(pkt[1:5]) == 0 {
		return nil, nil, false
	}
	dlen := int(pkt[5])
	if dlen > maxCIDLen || len(pkt) < 6+dlen+1 {
		return nil, nil, false
	}
	dcid = pkt[6 : 6+dlen]
	slenPos := 6 + dlen
	slen := int(pkt[slenPos])
	if slen > maxCIDLen || len(pkt) < slenPos+1+slen {
		return nil, nil, false
	}
	scid = pkt[slenPos+1 : slenPos+1+slen]
	return dcid, scid, true
}

// buildVersionNegotiation builds the reply. The server swaps the client's
// connection IDs: the packet's destination is the client's source ID and its
// source is the client's destination ID (RFC 9000 §17.2.1).
func buildVersionNegotiation(clientDCID, clientSCID []byte) []byte {
	out := make([]byte, 0, 1+4+1+len(clientSCID)+1+len(clientDCID)+4)
	// Long header form with the fixed bit set. The remaining bits are ignored
	// by clients in a Version Negotiation packet.
	out = append(out, 0xc0)
	out = binary.BigEndian.AppendUint32(out, 0) // Version Negotiation
	out = append(out, byte(len(clientSCID)))
	out = append(out, clientSCID...)
	out = append(out, byte(len(clientDCID)))
	out = append(out, clientDCID...)
	out = binary.BigEndian.AppendUint32(out, quicGreasedVersion)
	return out
}

// startQUICRejector binds the UDP companion socket for the HTTPS relay port.
// A bind failure is logged and otherwise ignored: the relay still works, QUIC
// clients simply keep their old, slower timeout behaviour.
func (s *Server) startQUICRejector(ctx context.Context) {
	if !s.settings.QUICRejectEnabled() {
		return
	}
	port := s.settings.HTTPSPort
	if port <= 0 {
		return
	}
	host := s.bindHost
	if host == "" {
		host = "0.0.0.0"
	}

	conn, err := listenQUICRejector(host, port)
	if err != nil {
		log.Printf("[SNI Proxy] Warning: QUIC fast-fail guard could not bind UDP %d: %v (QUIC clients will wait for their own timeout)", port, err)
		return
	}

	rejector := newQUICRejector(conn)
	s.mu.Lock()
	s.quic = rejector
	s.quicConn = conn
	s.mu.Unlock()

	log.Printf("[SNI Proxy] QUIC fast-fail guard listening on UDP %d", port)
	go rejector.run(ctx)
}

// listenQUICRejector binds the wildcard exactly as the TCP listeners do: the
// unspecified "0.0.0.0" becomes "::" so a dual-stack host answers QUIC on both
// families, while an explicitly named address keeps its own family. A host with
// IPv6 disabled falls back to IPv4.
func listenQUICRejector(host string, port int) (*net.UDPConn, error) {
	listenHost := host
	if host == "0.0.0.0" {
		listenHost = "::"
	}
	addr, err := net.ResolveUDPAddr("udp", net.JoinHostPort(listenHost, strconv.Itoa(port)))
	if err == nil {
		conn, listenErr := net.ListenUDP("udp", addr)
		if listenErr == nil {
			return conn, nil
		}
		err = listenErr
	}
	// The dual-stack attempt failed (IPv6 disabled, a conflicting socket on
	// "::"). A wildcard IPv4 bind still fixes QUIC for the common case.
	if host == "0.0.0.0" {
		addr, resolveIPv4Err := net.ResolveUDPAddr("udp", net.JoinHostPort("0.0.0.0", strconv.Itoa(port)))
		if resolveIPv4Err != nil {
			return nil, resolveIPv4Err
		}
		return net.ListenUDP("udp", addr)
	}
	return nil, err
}
