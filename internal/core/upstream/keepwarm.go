package upstream

import (
	"context"
	"log"
	"slices"
	"sync"
	"time"

	"digitdns/internal/core/resolution"

	"github.com/miekg/dns"
)

const (
	// keepWarmInterval is shorter than the DoT idle timeout (30 s) and the DoQ
	// idle timeout (60 s), so a single small query keeps the connection and its
	// TLS/QUIC session alive between real client queries.
	keepWarmInterval = 20 * time.Second
	keepWarmTimeout  = 5 * time.Second
)

// StartKeepWarm runs the background connection-maintenance loop. The loop reads
// its scope and interval from the live policy each tick, so a settings save
// takes effect without a restart — including turning it on after a start with
// "off". The goroutine is harmless when the scope is off: it just sleeps.
func (p *UpstreamPool) StartKeepWarm(ctx context.Context) {
	if p == nil {
		return
	}
	if mode := p.KeepWarmMode(); mode != resolution.KeepWarmOff {
		log.Printf("[Upstream] Keep-warm: %s · %d encrypted upstream connection(s) every %s",
			keepWarmDescription(mode), len(p.warmTargets()), p.KeepWarmInterval())
	}
	go func() {
		for {
			timer := time.NewTimer(p.KeepWarmInterval())
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-p.keepWarmWake:
				// A save changed the scope or interval. Re-arm with the new
				// value instead of waiting out the old timer.
				if !timer.Stop() {
					<-timer.C
				}
				continue
			case <-timer.C:
			}
			if p.KeepWarmMode() == resolution.KeepWarmOff {
				continue
			}
			p.WarmEncrypted(ctx)
		}
	}()
}

// KeepWarmMode is the live keep-warm scope.
func (p *UpstreamPool) KeepWarmMode() string {
	if p == nil {
		return resolution.KeepWarmOff
	}
	mode, _ := p.keepWarmMode.Load().(string)
	if mode == "" {
		return resolution.KeepWarmActiveNext
	}
	return mode
}

// KeepWarmInterval is the live refresh interval.
func (p *UpstreamPool) KeepWarmInterval() time.Duration {
	if p == nil {
		return keepWarmInterval
	}
	if ns := p.keepWarmEvery.Load(); ns > 0 {
		return time.Duration(ns)
	}
	return keepWarmInterval
}

// WarmEncrypted sends one lightweight query to the encrypted upstreams the
// current scope selects. The whole pool is only warmed in "all" mode: the
// recommended "active_next" touches the resolver in use and the one a switch
// would pick next, and leaves the rest cold until first use.
//
// The result is deliberately not recorded as a probe or a live sample: this is
// connection maintenance, not a measurement, and it must not move the Adaptive
// ranking.
func (p *UpstreamPool) WarmEncrypted(ctx context.Context) {
	if p == nil {
		return
	}
	mode := p.KeepWarmMode()
	if mode == resolution.KeepWarmOff {
		return
	}
	targets := p.warmTargetsFor(mode)
	var wg sync.WaitGroup
	for _, addr := range targets {
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

// warmTargets is the recommended scope: the active resolver plus the next
// fastest encrypted one.
func (p *UpstreamPool) warmTargets() []string {
	return p.warmTargetsFor(resolution.KeepWarmActiveNext)
}

// warmTargetsFor picks the encrypted resolvers the scope selects. Plain
// upstreams need no connection kept alive, and a resolver in probation is
// skipped until it answers again.
func (p *UpstreamPool) warmTargetsFor(mode string) []string {
	if p == nil || mode == resolution.KeepWarmOff {
		return nil
	}
	p.mu.RLock()
	current := p.adaptiveCurrent
	servers := make([]string, len(p.upstreams))
	copy(servers, p.upstreams)
	p.mu.RUnlock()

	order := p.orderedByLatency(servers)
	out := make([]string, 0, 2)
	add := func(addr string) bool {
		if addr == "" || !isEncryptedUpstream(addr) || p.resolverProbation(addr) {
			return false
		}
		for _, existing := range out {
			if existing == addr {
				return false
			}
		}
		out = append(out, addr)
		return true
	}

	switch mode {
	case resolution.KeepWarmActive:
		if isEncryptedUpstream(current) && add(current) {
			return out
		}
		for _, addr := range order {
			if add(addr) {
				return out
			}
		}
	case resolution.KeepWarmAll:
		for _, addr := range order {
			add(addr)
		}
		return out
	default: // active_next
		add(current)
		for _, addr := range order {
			if len(out) >= 2 {
				break
			}
			add(addr)
		}
	}
	return out
}

func isEncryptedUpstream(addr string) bool {
	scheme, _ := splitUpstreamScheme(addr)
	switch scheme {
	case "tls", "https", "h3", "quic":
		return true
	default:
		return false
	}
}

func keepWarmDescription(mode string) string {
	switch mode {
	case resolution.KeepWarmOff:
		return "off"
	case resolution.KeepWarmActive:
		return "active resolver only"
	case resolution.KeepWarmAll:
		return "all encrypted resolvers"
	default:
		return "active + next"
	}
}

// AdvancedPolicy is the live, operator-tunable part of upstream maintenance.
type AdvancedPolicy struct {
	KeepWarm        string
	KeepWarmEvery   time.Duration
	Hedge           string
	CircuitCooldown time.Duration
	Fallback        []string
}

// SetAdvancedPolicy applies keep-warm, breaker cooldown and the fallback pool.
// Bootstrap DNS is intentionally absent: it is baked into the dialers when the
// pool is constructed, so a change needs a restart.
func (p *UpstreamPool) SetAdvancedPolicy(a AdvancedPolicy) {
	if p == nil {
		return
	}
	mode := resolution.NormalizeKeepWarm(a.KeepWarm)
	interval := a.KeepWarmEvery
	if interval <= 0 {
		interval = keepWarmInterval
	}
	cooldown := a.CircuitCooldown
	if cooldown <= 0 {
		cooldown = circuitCooldown
	}

	oldMode := p.KeepWarmMode()
	oldInterval := p.KeepWarmInterval()
	p.keepWarmMode.Store(mode)
	p.keepWarmEvery.Store(int64(interval))
	p.cooldownNs.Store(int64(cooldown))
	p.hedgeAggressive.Store(resolution.NormalizeHedge(a.Hedge) == resolution.HedgeAggressive)
	// Recompute the head start on the next query instead of waiting for the
	// once-a-second cache to age out.
	p.hedgeUpdated.Store(0)

	fallback := normalizeUpstreams(a.Fallback)
	current := p.fallback.Load()
	fallbackChanged := true
	if current == nil {
		fallbackChanged = len(fallback) > 0
	} else {
		fallbackChanged = !slices.Equal(current.GetUpstreams(), fallback)
	}
	if fallbackChanged {
		var fb *UpstreamPool
		if len(fallback) > 0 {
			// The fallback pool is deliberately built without a fallback of its
			// own, so a failing fallback cannot recurse.
			fb = NewUpstreamPoolWithBootstrap(fallback, p.timeout, false, "", p.bootstrap)
		}
		p.fallback.Store(fb)
		if len(fallback) > 0 {
			log.Printf("[Upstream] Fallback pool: %d resolver(s)", len(fallback))
		} else {
			log.Printf("[Upstream] Fallback pool: off")
		}
	}

	if oldMode != mode || oldInterval != interval {
		select {
		case p.keepWarmWake <- struct{}{}:
		default:
		}
		if mode == resolution.KeepWarmOff {
			log.Printf("[Upstream] Keep-warm: off")
		} else {
			log.Printf("[Upstream] Keep-warm: %s · %d encrypted upstream connection(s) every %s",
				keepWarmDescription(mode), len(p.warmTargetsFor(mode)), interval)
		}
	}
}
