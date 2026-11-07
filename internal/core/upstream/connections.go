package upstream

import (
	"context"
	"crypto/tls"
	"sync"
	"time"

	"github.com/miekg/dns"
	"github.com/quic-go/quic-go"
)

// Connection reuse constants. DoT connections are cheap to keep but expensive to
// establish (a full TLS handshake), so a small idle pool per upstream removes the
// handshake from every cache miss. DoQ is multiplexed: one QUIC connection per
// upstream carries every stream, so the handshake is paid once per idle window.
const (
	dotIdleTimeout         = 30 * time.Second
	dotMaxIdlePerUpstream  = 4
	dotMaxConnsPerUpstream = 16
	doqIdleTimeout         = 60 * time.Second
)

type dotConn struct {
	conn     *dns.Conn
	lastUsed time.Time
}

// dotConnPool is a bounded pool of persistent DoT connections for one endpoint.
// A connection carries one exchange at a time; the semaphore bounds the number
// of live connections so a query flood cannot open an unbounded number of TLS
// sessions.
type dotConnPool struct {
	mu   sync.Mutex
	idle []*dotConn
	sem  chan struct{}
}

func newDoTConnPool() *dotConnPool {
	return &dotConnPool{sem: make(chan struct{}, dotMaxConnsPerUpstream)}
}

func (p *dotConnPool) acquire(ctx context.Context, dial func(context.Context) (*dns.Conn, error)) (*dns.Conn, bool, error) {
	now := time.Now()
	p.mu.Lock()
	for len(p.idle) > 0 {
		c := p.idle[len(p.idle)-1]
		p.idle = p.idle[:len(p.idle)-1]
		if now.Sub(c.lastUsed) <= dotIdleTimeout {
			p.mu.Unlock()
			return c.conn, true, nil
		}
		_ = c.conn.Close()
		<-p.sem
	}
	p.mu.Unlock()

	select {
	case p.sem <- struct{}{}:
	case <-ctx.Done():
		return nil, false, ctx.Err()
	}
	conn, err := dial(ctx)
	if err != nil {
		<-p.sem
		return nil, false, err
	}
	return conn, false, nil
}

func (p *dotConnPool) release(conn *dns.Conn, healthy bool) {
	if conn == nil {
		return
	}
	if !healthy {
		_ = conn.Close()
		<-p.sem
		return
	}
	p.mu.Lock()
	if len(p.idle) < dotMaxIdlePerUpstream {
		p.idle = append(p.idle, &dotConn{conn: conn, lastUsed: time.Now()})
		p.mu.Unlock()
		return
	}
	p.mu.Unlock()
	_ = conn.Close()
	<-p.sem
}

func (p *dotConnPool) close() {
	p.mu.Lock()
	idle := p.idle
	p.idle = nil
	p.mu.Unlock()
	for _, c := range idle {
		_ = c.conn.Close()
	}
}

// dotPoolFor returns the per-endpoint DoT pool, creating it on first use.
func (p *UpstreamPool) dotPoolFor(endpoint string) *dotConnPool {
	p.dotPoolsMu.Lock()
	defer p.dotPoolsMu.Unlock()
	pool := p.dotPools[endpoint]
	if pool == nil {
		pool = newDoTConnPool()
		p.dotPools[endpoint] = pool
	}
	return pool
}

// exchangeDoT sends one query over a persistent TLS connection. A pooled
// connection that the server closed while idle is retried once on a fresh
// connection; that retry is what makes reuse invisible to the caller.
func (p *UpstreamPool) exchangeDoT(ctx context.Context, q *dns.Msg, endpoint string) (*dns.Msg, time.Duration, error) {
	pool := p.dotPoolFor(endpoint)
	resp, rtt, reused, err := p.exchangeOnDoT(ctx, pool, q, endpoint)
	if err != nil && reused && ctx.Err() == nil {
		resp, rtt, _, err = p.exchangeOnDoT(ctx, pool, q, endpoint)
	}
	return resp, rtt, err
}

func (p *UpstreamPool) exchangeOnDoT(ctx context.Context, pool *dotConnPool, q *dns.Msg, endpoint string) (*dns.Msg, time.Duration, bool, error) {
	conn, reused, err := pool.acquire(ctx, func(ctx context.Context) (*dns.Conn, error) {
		return p.tlsClient.DialContext(ctx, endpoint)
	})
	if err != nil {
		return nil, 0, false, err
	}

	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(p.timeout)
	}
	_ = conn.SetDeadline(deadline)

	start := time.Now()
	if err := conn.WriteMsg(q); err != nil {
		pool.release(conn, false)
		return nil, time.Since(start), reused, err
	}
	resp, err := conn.ReadMsg()
	if err != nil {
		pool.release(conn, false)
		return nil, time.Since(start), reused, err
	}
	if resp.Id != q.Id {
		pool.release(conn, false)
		return nil, time.Since(start), reused, dns.ErrId
	}
	_ = conn.SetDeadline(time.Time{})
	pool.release(conn, true)
	return resp, time.Since(start), reused, nil
}

type doqConn struct {
	conn     *quic.Conn
	lastUsed time.Time
}

// doqConnection returns the shared QUIC connection for an endpoint, dialing one
// when none is live. quic.Conn is safe for concurrent streams, so one connection
// carries every query.
func (p *UpstreamPool) doqConnection(ctx context.Context, endpoint string, tlsConf *tls.Config) (*quic.Conn, error) {
	p.doqMu.Lock()
	if c := p.doqConns[endpoint]; c != nil {
		if c.conn.Context().Err() == nil && time.Since(c.lastUsed) <= doqIdleTimeout {
			c.lastUsed = time.Now()
			p.doqMu.Unlock()
			return c.conn, nil
		}
		_ = c.conn.CloseWithError(0, "")
		delete(p.doqConns, endpoint)
	}
	p.doqMu.Unlock()

	conn, err := quic.DialAddr(ctx, endpoint, tlsConf, &quic.Config{
		HandshakeIdleTimeout: p.timeout,
		MaxIdleTimeout:       doqIdleTimeout,
	})
	if err != nil {
		return nil, err
	}

	p.doqMu.Lock()
	if existing := p.doqConns[endpoint]; existing != nil && existing.conn.Context().Err() == nil {
		p.doqMu.Unlock()
		_ = conn.CloseWithError(0, "")
		return existing.conn, nil
	}
	p.doqConns[endpoint] = &doqConn{conn: conn, lastUsed: time.Now()}
	p.doqMu.Unlock()
	return conn, nil
}

func (p *UpstreamPool) dropDoQ(endpoint string, conn *quic.Conn) {
	p.doqMu.Lock()
	if c := p.doqConns[endpoint]; c != nil && c.conn == conn {
		delete(p.doqConns, endpoint)
	}
	p.doqMu.Unlock()
	_ = conn.CloseWithError(0, "")
}

// closeConnectionPool drops every persistent connection for an upstream that was
// removed from the pool. It runs after the settings snapshot is published, so no
// new query can take a connection for this endpoint.
func (p *UpstreamPool) closeConnectionPool(endpoint string) {
	p.dotPoolsMu.Lock()
	dot := p.dotPools[endpoint]
	delete(p.dotPools, endpoint)
	p.dotPoolsMu.Unlock()
	if dot != nil {
		dot.close()
	}

	p.doqMu.Lock()
	if c := p.doqConns[endpoint]; c != nil {
		delete(p.doqConns, endpoint)
		_ = c.conn.CloseWithError(0, "")
	}
	p.doqMu.Unlock()
}
