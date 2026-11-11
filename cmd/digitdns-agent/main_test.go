package main

import (
	"testing"

	"golang.org/x/net/http2"
)

func TestJitteredIntervalBounds(t *testing.T) {
	base := agentHeartbeatBase
	for i := 0; i < 200; i++ {
		d := jitteredInterval(base, 0)
		if d < base*4/5 || d > base*6/5 {
			t.Fatalf("jitteredInterval(%v, 0) = %v, want within ±20%%", base, d)
		}
	}
}

func TestHeartbeatBackoffCapsAtOneMinute(t *testing.T) {
	for i := 0; i < 200; i++ {
		d := jitteredIntervalCapped(agentHeartbeatBase, 100, agentHeartbeatMaxBackoff)
		if d < agentHeartbeatMaxBackoff*4/5 || d > agentHeartbeatMaxBackoff*6/5 {
			t.Fatalf("heartbeat backoff = %v, want around %v", d, agentHeartbeatMaxBackoff)
		}
	}
}

func TestGoAwayIsATransportDrop(t *testing.T) {
	if !isTransportDrop(http2.GoAwayError{}) {
		t.Fatal("HTTP/2 GOAWAY must re-arm the long-poll immediately")
	}
}

func TestJitteredIntervalBacksOff(t *testing.T) {
	base := agentSyncBase
	short := jitteredInterval(base, 0)
	long := jitteredInterval(base, 4)
	if long <= short {
		t.Fatalf("backoff did not grow: %v then %v", short, long)
	}
	// The cap is applied before jitter, so the result is bounded by the cap
	// plus the ±20% spread.
	capped := jitteredInterval(base, 100)
	if capped < agentSyncMaxBackoff*4/5 || capped > agentSyncMaxBackoff*6/5 {
		t.Fatalf("capped interval = %v, want around %v", capped, agentSyncMaxBackoff)
	}
}
