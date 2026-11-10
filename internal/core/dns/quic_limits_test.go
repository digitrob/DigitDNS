package dns

import (
	"fmt"
	"net"
	"testing"
	"time"
)

func udpSource(port int) net.Addr {
	return &net.UDPAddr{IP: net.ParseIP("203.0.113.9"), Port: port}
}

func udpSourceAt(ip string, port int) net.Addr {
	return &net.UDPAddr{IP: net.ParseIP(ip), Port: port}
}

func TestQUICHandshakeLimiterBurstAndRefill(t *testing.T) {
	l := newQUICHandshakeLimiter()
	now := time.Unix(1_700_000_000, 0)
	src := udpSource(40000)

	for i := 0; i < quicHandshakeBurst; i++ {
		if !l.Allow(src, now) {
			t.Fatalf("handshake %d refused inside the burst", i+1)
		}
	}
	if l.Allow(src, now) {
		t.Fatal("handshake beyond the burst was allowed")
	}

	// Sustained rate is quicHandshakePerSecond, so half a second buys one token.
	now = now.Add(500 * time.Millisecond)
	if !l.Allow(src, now) {
		t.Fatal("a refilled token was not allowed")
	}
	if l.Allow(src, now) {
		t.Fatal("a second handshake was allowed from a single refilled token")
	}
}

func TestQUICHandshakeLimiterIsPerSource(t *testing.T) {
	l := newQUICHandshakeLimiter()
	now := time.Unix(1_700_000_000, 0)

	for i := 0; i < quicHandshakeBurst; i++ {
		l.Allow(udpSource(40000), now)
	}
	if l.Allow(udpSource(40000), now) {
		t.Fatal("exhausted source was allowed")
	}
	if !l.Allow(udpSourceAt("203.0.113.10", 40000), now) {
		t.Fatal("a different source shared the exhausted bucket")
	}
	// Ports are not part of the identity: a host that reconnects from a new
	// ephemeral port is still the same source.
	if l.Allow(udpSource(40001), now) {
		t.Fatal("a new source port bypassed the exhausted IP bucket")
	}
}

func TestQUICHandshakeLimiterSweepsIdleSources(t *testing.T) {
	l := newQUICHandshakeLimiter()
	now := time.Unix(1_700_000_000, 0)
	l.Allow(udpSourceAt("198.51.100.7", 40000), now)

	// Move past both the sweep interval and the bucket lifetime with a new
	// source, which triggers the sweep.
	later := now.Add(quicHandshakeBucketLife + time.Minute)
	l.Allow(udpSourceAt("198.51.100.8", 40000), later)

	l.mu.Lock()
	_, stale := l.buckets["198.51.100.7"]
	l.mu.Unlock()
	if stale {
		t.Fatal("an idle source bucket was not swept")
	}
}

func TestQUICHandshakeLimiterBoundsTheMap(t *testing.T) {
	l := newQUICHandshakeLimiter()
	l.maxSources = 2
	now := time.Unix(1_700_000_000, 0)
	for i := 0; i < 2; i++ {
		addr := &net.UDPAddr{IP: net.ParseIP(fmt.Sprintf("198.51.100.%d", i+1)), Port: 40000}
		if !l.Allow(addr, now) {
			t.Fatalf("source %d refused below the cap", i)
		}
	}
	overflow := &net.UDPAddr{IP: net.ParseIP("198.51.100.99"), Port: 40000}
	if l.Allow(overflow, now) {
		t.Fatal("a new source was admitted after the map cap")
	}
	// An existing source still works: the cap only refuses unknown addresses.
	existing := &net.UDPAddr{IP: net.ParseIP("198.51.100.1"), Port: 40000}
	if !l.Allow(existing, now) {
		t.Fatal("the limiter stopped serving an existing bucket at the cap")
	}
}
