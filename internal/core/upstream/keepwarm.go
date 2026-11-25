package upstream

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/miekg/dns"
)

const (
	// keepWarmInterval is shorter than the DoT idle timeout (30 s) and the DoQ
	// idle timeout (60 s), so a single small query keeps the connection and its
	// TLS/QUIC session alive between real client queries.
	keepWarmInterval = 20 * time.Second
	keepWarmTimeout  = 5 * time.Second
)

// StartKeepWarm runs a small background loop that sends one query to every
// healthy encrypted upstream. It exists to remove the handshake from the first
// cache miss after a quiet period. Plain UDP/TCP upstreams are skipped: they
// have no persistent connection to keep.
func (p *UpstreamPool) StartKeepWarm(ctx context.Context) {
	if p == nil {
		return
	}
	p.mu.RLock()
	encrypted := 0
	for _, addr := range p.upstreams {
		if scheme, _ := splitUpstreamScheme(addr); scheme == "tls" || scheme == "https" || scheme == "h3" || scheme == "quic" {
			encrypted++
		}
	}
	p.mu.RUnlock()
	if encrypted == 0 {
		return
	}
	log.Printf("[Upstream] Keep-warm: refreshing %d encrypted upstream connection(s) every %s", encrypted, keepWarmInterval)
	go func() {
		ticker := time.NewTicker(keepWarmInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				p.WarmEncrypted(ctx)
			}
		}
	}()
}

// WarmEncrypted sends one lightweight query to each healthy encrypted upstream.
// The result is deliberately not recorded as a probe or a live sample: this is
// connection maintenance, not a measurement, and it must not move the Adaptive
// ranking.
func (p *UpstreamPool) WarmEncrypted(ctx context.Context) {
	if p == nil {
		return
	}
	p.mu.RLock()
	servers := make([]string, len(p.upstreams))
	copy(servers, p.upstreams)
	p.mu.RUnlock()

	var wg sync.WaitGroup
	for _, addr := range servers {
		scheme, _ := splitUpstreamScheme(addr)
		switch scheme {
		case "tls", "https", "h3", "quic":
		default:
			continue
		}
		if p.resolverProbation(addr) {
			continue
		}
		wg.Add(1)
		go func(upstream string) {
			defer wg.Done()
			q := new(dns.Msg)
			q.SetQuestion("cloudflare.com.", dns.TypeA)
			q.Id = dns.Id()
			qctx, cancel := context.WithTimeout(ctx, keepWarmTimeout)
			defer cancel()
			_, _, _ = p.queryOne(qctx, q, upstream)
		}(addr)
	}
	wg.Wait()
}
