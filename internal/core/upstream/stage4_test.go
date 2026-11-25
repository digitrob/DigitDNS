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
