package upstream

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"testing"
	"time"

	"github.com/miekg/dns"
	"github.com/quic-go/quic-go/http3"
)

// The bootstrap resolver must use the configured servers, not the OS resolver.
func TestBootstrapResolverUsesConfiguredBootstrap(t *testing.T) {
	fake := newTestResolver(t, always(answer{rcode: dns.RcodeSuccess, ip: "203.0.113.240"}), nil)
	resolver := newBootstrapResolver([]string{fake.addr})

	ips, err := resolver.LookupHost(context.Background(), "encrypted-upstream.test.")
	if err != nil {
		t.Fatalf("bootstrap lookup: %v", err)
	}
	if len(ips) == 0 || ips[0] != "203.0.113.240" {
		t.Fatalf("bootstrap lookup returned %v, want 203.0.113.240", ips)
	}
	if fake.udpHits.Load() == 0 {
		t.Fatal("configured bootstrap server received no query")
	}
}

// Keep-warm must send a query to encrypted upstreams but reuse the connection
// it already opened, and it must not touch plain upstreams.
func TestWarmEncryptedReusesDoTConnection(t *testing.T) {
	addr, leaf, hits, conns := newDoTResolver(t, always(answer{rcode: dns.RcodeSuccess, ip: "203.0.113.241"}))
	p := newTestPool(t, []string{"tls://" + addr}, 2*time.Second, false, "")
	roots := x509.NewCertPool()
	roots.AddCert(leaf)
	p.tlsClient.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots}

	if _, _, _, err := p.ExchangeFor(query("warm-first.example."), ""); err != nil {
		t.Fatalf("first query: %v", err)
	}
	before := conns.Load()
	if before == 0 {
		t.Fatal("first query did not open a DoT connection")
	}

	p.WarmEncrypted(context.Background())
	p.WarmEncrypted(context.Background())

	if got := conns.Load(); got != before {
		t.Fatalf("keep-warm opened %d new connections, want reuse", got-before)
	}
	if hits.Load() < 3 {
		t.Fatalf("resolver saw %d queries, want the first plus two keep-warm queries", hits.Load())
	}

	plain := newTestResolver(t, always(answer{rcode: dns.RcodeSuccess, ip: "203.0.113.242"}), nil)
	plainPool := newTestPool(t, []string{plain.addr}, 2*time.Second, false, "")
	plainPool.WarmEncrypted(context.Background())
	if plain.udpHits.Load() != 0 {
		t.Fatalf("keep-warm sent %d queries to a plain upstream", plain.udpHits.Load())
	}
}

// The dashboard counters must reflect the three paths: a hedge, a failure-driven
// failover, and a plain timeout.
func TestUpstreamMetricsCountHedgeFailoverAndTimeout(t *testing.T) {
	slow := newTestResolver(t, always(answer{
		rcode: dns.RcodeSuccess,
		ip:    "203.0.113.243",
		delay: 300 * time.Millisecond,
	}), nil)
	fast := newTestResolver(t, always(answer{rcode: dns.RcodeSuccess, ip: "203.0.113.244"}), nil)
	hedged := newTestPool(t, []string{slow.addr, fast.addr}, 2*time.Second, false, "")
	adaptiveWithCurrent(t, hedged, slow.addr)
	if _, _, _, err := hedged.ExchangeFor(query("metrics-hedge.example."), ""); err != nil {
		t.Fatalf("hedged query: %v", err)
	}
	if got := hedged.Metrics(); got.Hedges == 0 || got.HedgeLosses == 0 {
		t.Fatalf("hedge metrics = %+v, want at least one hedge and one loss", got)
	}

	refused := "127.0.0.1:1"
	failedOver := newTestPool(t, []string{refused, fast.addr}, 2*time.Second, false, "")
	adaptiveWithCurrent(t, failedOver, refused)
	if _, _, _, err := failedOver.ExchangeFor(query("metrics-failover.example."), ""); err != nil {
		t.Fatalf("failover query: %v", err)
	}
	if got := failedOver.Metrics(); got.Failovers == 0 {
		t.Fatalf("failover metrics = %+v, want at least one failover", got)
	}

	hang := newTestResolver(t, always(answer{silent: true}), nil)
	timedOut := newTestPool(t, []string{hang.addr}, 300*time.Millisecond, false, "")
	if _, _, _, err := timedOut.ExchangeFor(query("metrics-timeout.example."), ""); err == nil {
		t.Fatal("single hanging resolver unexpectedly answered")
	}
	if got := timedOut.Metrics(); got.Timeouts == 0 {
		t.Fatalf("timeout metrics = %+v, want at least one timeout", got)
	}
}

// Keep-warm must not touch every encrypted resolver: active + next only.
func TestWarmTargetsActiveAndNextOnly(t *testing.T) {
	p := newTestPool(t, []string{
		"1.1.1.1:53",
		"tls://a.example:853",
		"tls://b.example:853",
		"tls://c.example:853",
	}, 2*time.Second, false, "")
	p.SetAdaptiveCurrent("tls://b.example:853")

	got := p.warmTargets()
	if len(got) != 2 || got[0] != "tls://b.example:853" || got[1] != "tls://a.example:853" {
		t.Fatalf("warm targets = %v, want [active b, next a]", got)
	}

	p.SetAdaptiveCurrent("1.1.1.1:53")
	got = p.warmTargets()
	if len(got) != 2 || got[0] != "tls://a.example:853" || got[1] != "tls://b.example:853" {
		t.Fatalf("warm targets with a plain active = %v, want the first two encrypted", got)
	}
}

// The constructor must only build a bootstrap resolver when servers are set.
func TestBootstrapResolverOnlyWhenConfigured(t *testing.T) {
	plain := newTestPool(t, []string{"1.1.1.1:53"}, 2*time.Second, false, "")
	if plain.resolver != nil {
		t.Fatal("pool without bootstrap built a resolver")
	}
	withBootstrap := NewUpstreamPoolWithBootstrap([]string{"https://dns.example/dns-query"}, 2*time.Second, false, "", []string{"1.1.1.1:53"})
	if withBootstrap.resolver == nil {
		t.Fatal("pool with bootstrap did not build a resolver")
	}
	if withBootstrap.tlsClient.Dialer == nil || withBootstrap.tlsClient.Dialer.Resolver != withBootstrap.resolver {
		t.Fatal("DoT client is not using the bootstrap resolver")
	}
	dohTransport, ok := withBootstrap.httpClient.Transport.(*http.Transport)
	if !ok || dohTransport.DialContext == nil {
		t.Fatal("DoH transport is not using the bootstrap resolver")
	}
	h3Transport, ok := withBootstrap.h3Client.Transport.(*http3.Transport)
	if !ok || h3Transport.Dial == nil {
		t.Fatal("DoH3 transport is not using the bootstrap resolver")
	}
}
