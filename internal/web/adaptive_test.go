package web

import (
	"context"
	"testing"
	"time"

	"digitdns/internal/core/resolution"
)

// The scheduler sleeps for the configured interval instead of polling every
// 30 s, so the delay calculation is the whole contract. It has to fall back to a
// slow idle tick when Adaptive is not selected, or a strategy switch would be
// noticed only on the next settings save.
func TestNextAdaptiveDelayTracksIntervalAndIdlesOtherwise(t *testing.T) {
	ws, _, cleanup := setupTestWebServer(t)
	defer cleanup()

	set := func(strategy string, intervalMinutes int, lastRun time.Time) {
		ws.dnsMu.Lock()
		cfg := ws.mutableDNSLocked()
		cfg.DefaultStrategy = strategy
		cfg.AdaptiveIntervalMinutes = intervalMinutes
		if lastRun.IsZero() {
			cfg.AdaptiveLastRun = ""
		} else {
			cfg.AdaptiveLastRun = lastRun.Format(time.RFC3339)
		}
		ws.dnsMu.Unlock()
	}

	set(resolution.StrategyHedged, 60, time.Time{})
	if got := ws.nextAdaptiveDelay(); got != adaptiveIdleCheck {
		t.Errorf("non-adaptive delay = %v, want %v", got, adaptiveIdleCheck)
	}

	set(resolution.StrategyAdaptive, 60, time.Time{})
	if got := ws.nextAdaptiveDelay(); got != time.Second {
		t.Errorf("never-run delay = %v, want 1s", got)
	}

	set(resolution.StrategyAdaptive, 15, time.Now().Add(-10*time.Minute))
	if got := ws.nextAdaptiveDelay(); got < 4*time.Minute || got > 5*time.Minute {
		t.Errorf("delay after a run 10m ago with a 15m interval = %v, want ~5m", got)
	}

	set(resolution.StrategyAdaptive, 15, time.Now().Add(-30*time.Minute))
	if got := ws.nextAdaptiveDelay(); got != time.Second {
		t.Errorf("overdue delay = %v, want 1s", got)
	}
}

// The startup selection is forced, but it still has to respect the strategy: a
// daemon configured for a non-Adaptive mode must not switch resolvers at boot.
func TestRunAdaptiveStartupIgnoresNonAdaptiveStrategy(t *testing.T) {
	ws, _, cleanup := setupTestWebServer(t)
	defer cleanup()

	ws.dnsMu.Lock()
	ws.mutableDNSLocked().DefaultStrategy = resolution.StrategyHedged
	ws.dnsMu.Unlock()

	if err := ws.runAdaptiveStartup(context.Background()); err != nil {
		t.Fatalf("runAdaptiveStartup on a non-adaptive install: %v", err)
	}
}
