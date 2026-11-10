package dns

import (
	"net"
	"sync"
	"time"
)

// New QUIC connections are the expensive part of both encrypted transports: a
// handshake costs a key exchange and certificate work before the request rate
// limiter ever sees a query. A flood of Initial packets therefore reaches the
// CPU even when every eventual query would be refused. This limiter is consulted
// from quic.Config.GetConfigForClient, which quic-go calls before it allocates
// the connection and runs TLS, so a rejected source costs one map lookup.
//
// The limits are per source address and deliberately generous: a phone or a
// browser opens a handful of connections, and a household behind one NAT shares
// the bucket. The point is to stop a single host from opening thousands of
// handshakes per second, not to police normal reconnects.
const (
	quicHandshakeBurst      = 60
	quicHandshakePerSecond  = 2.0
	quicHandshakeBucketLife = 10 * time.Minute
	quicHandshakeSweepEvery = time.Minute
	quicHandshakeMaxSources = 1 << 16
)

type quicHandshakeBucket struct {
	tokens float64
	last   time.Time
}

// quicHandshakeLimiter is a per-source token bucket. Its map is bounded so a
// spoofed-source flood cannot turn the defence into an unbounded allocation:
// once the map is full, addresses it has never seen are refused until a sweep
// evicts the idle ones.
type quicHandshakeLimiter struct {
	mu         sync.Mutex
	buckets    map[string]*quicHandshakeBucket
	burst      float64
	perSecond  float64
	lastSweep  time.Time
	maxSources int
}

func newQUICHandshakeLimiter() *quicHandshakeLimiter {
	return &quicHandshakeLimiter{
		buckets:    make(map[string]*quicHandshakeBucket),
		burst:      quicHandshakeBurst,
		perSecond:  quicHandshakePerSecond,
		maxSources: quicHandshakeMaxSources,
	}
}

// Allow consumes one handshake token for the source. now is passed in so tests
// can drive the clock without sleeping.
func (l *quicHandshakeLimiter) Allow(remote net.Addr, now time.Time) bool {
	if l == nil || remote == nil {
		return true
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.lastSweep.IsZero() || now.Sub(l.lastSweep) >= quicHandshakeSweepEvery {
		l.sweepLocked(now)
	}

	source := sourceHost(remote)
	bucket := l.buckets[source]
	if bucket == nil {
		if len(l.buckets) >= l.maxSources {
			return false
		}
		bucket = &quicHandshakeBucket{tokens: l.burst, last: now}
		l.buckets[source] = bucket
	} else {
		elapsed := now.Sub(bucket.last).Seconds()
		if elapsed > 0 {
			bucket.tokens += elapsed * l.perSecond
			if bucket.tokens > l.burst {
				bucket.tokens = l.burst
			}
		}
		bucket.last = now
	}

	if bucket.tokens < 1 {
		return false
	}
	bucket.tokens--
	return true
}

func (l *quicHandshakeLimiter) sweepLocked(now time.Time) {
	l.lastSweep = now
	for source, bucket := range l.buckets {
		if now.Sub(bucket.last) > quicHandshakeBucketLife {
			delete(l.buckets, source)
		}
	}
}

// sourceHost reduces a peer address to its IP. QUIC always carries a UDP
// address, but the fallback keeps a malformed or unusual net.Addr from
// collapsing every source into one bucket.
func sourceHost(remote net.Addr) string {
	if host, _, err := net.SplitHostPort(remote.String()); err == nil {
		return host
	}
	return remote.String()
}
