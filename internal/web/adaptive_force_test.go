package web

import (
	"encoding/json"
	"net"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"digitdns/internal/core/upstream"
	"digitdns/internal/service"

	"github.com/miekg/dns"
)

// The Force response must carry the resolver numbers the selection just used.
// The panel used to repaint from its previous /api/stats payload, which is
// cached for a second, so the row the server had just selected kept showing the
// pre-switch latency. A plain GET keeps the smaller payload.
func TestAdaptiveRunResponseCarriesUpstreamStats(t *testing.T) {
	ws, _, cleanup := setupTestWebServer(t)
	defer cleanup()

	read := func(withStats bool) resolutionView {
		t.Helper()
		rec := httptest.NewRecorder()
		ws.writeResolution(rec, withStats)
		var view resolutionView
		if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil {
			t.Fatalf("decode resolution view: %v", err)
		}
		return view
	}

	if got := read(false); len(got.UpstreamStats) != 0 {
		t.Fatalf("settings read carried %d upstream stats, want none", len(got.UpstreamStats))
	}
	view := read(true)
	if len(view.UpstreamStats) != 1 {
		t.Fatalf("force response carried %d upstream stats, want 1", len(view.UpstreamStats))
	}
	if view.UpstreamStats[0].Address != "1.1.1.1:53" {
		t.Fatalf("force response stats = %+v, want the configured resolver", view.UpstreamStats)
	}
}

// Force switches from the measurements already in hand and does not queue a
// full benchmark or wait for the scheduler. That is the first-tap guarantee:
// tying the switch to a probe made the old resolver stay active until the
// benchmark finished, and a scheduled probe could outlast the client timeout.
func TestForceSelectsFromLatestMeasurementsWithoutBenchmark(t *testing.T) {
	ws, _, cleanup := setupTestWebServer(t)
	defer cleanup()

	fake := startWebTestDNS(t)
	ws.upstreams = upstream.NewUpstreamPool([]string{fake}, 500*time.Millisecond, false, "")
	ws.upstreams.BenchmarkAll() // the pool already has a measured resolver

	var runs atomic.Int32
	runner := service.NewBenchmarkRunner(func() {
		runs.Add(1)
		ws.upstreams.BenchmarkAll()
	})
	ws.SetControlState(runner, nil)

	decision, err := ws.runAdaptiveNow(true)
	if err != nil {
		t.Fatalf("Force run: %v", err)
	}
	if runs.Load() != 0 {
		t.Fatalf("benchmark runs = %d, want 0 — Force must switch without a probe", runs.Load())
	}
	if decision.Current != fake {
		t.Fatalf("Force chose %q, want the measured resolver %q", decision.Current, fake)
	}

	ws.dnsMu.Lock()
	persisted := ws.mutableDNSLocked().AdaptiveCurrent
	ws.dnsMu.Unlock()
	if persisted != fake {
		t.Fatalf("persisted choice = %q, want %q", persisted, fake)
	}
}

// The v3 Force response is what the Android app consumes. It must carry the
// racer rows too, so the app does not have to follow up with /upstreams/stats
// and race that endpoint's one-second cache.
func TestV3ForceResponseCarriesUpstreamStats(t *testing.T) {
	ws, _, cleanup := setupTestWebServer(t)
	defer cleanup()

	fake := startWebTestDNS(t)
	ws.upstreams = upstream.NewUpstreamPool([]string{fake}, 500*time.Millisecond, false, "")
	ws.upstreams.BenchmarkAll()

	out, err := NewV3Backend(ws).RunAdaptive()
	if err != nil {
		t.Fatalf("v3 Force run: %v", err)
	}
	stats, ok := out["upstream_stats"].([]any)
	if !ok || len(stats) != 1 {
		t.Fatalf("v3 Force upstream_stats = %#v, want one racer row", out["upstream_stats"])
	}
	if got, _ := out["adaptive_current"].(string); got != fake {
		t.Fatalf("v3 Force adaptive_current = %q, want %q", got, fake)
	}
}

// startWebTestDNS is a tiny loopback resolver for the Force timing test. It
// answers every A question immediately so the test measures the Force path, not
// a network.
func startWebTestDNS(t *testing.T) string {
	t.Helper()
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := &dns.Server{
		PacketConn: pc,
		Handler: dns.HandlerFunc(func(w dns.ResponseWriter, r *dns.Msg) {
			m := new(dns.Msg)
			m.SetReply(r)
			if len(r.Question) > 0 {
				if rr, err := dns.NewRR(r.Question[0].Name + " 30 IN A 203.0.113.250"); err == nil {
					m.Answer = append(m.Answer, rr)
				}
			}
			_ = w.WriteMsg(m)
		}),
	}
	go func() { _ = srv.ActivateAndServe() }()
	t.Cleanup(func() { _ = srv.Shutdown() })
	return pc.LocalAddr().String()
}
