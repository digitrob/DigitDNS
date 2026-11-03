package agent

import (
	"testing"
	"time"
)

func TestRefusalLogThrottlesAndReportsSuppressed(t *testing.T) {
	now := time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)
	rl := newRefusalLog(time.Minute, func() time.Time { return now })

	ok, suppressed := rl.allow("ip|203.0.113.5")
	if !ok || suppressed != 0 {
		t.Fatalf("first event = (%v, %d), want (true, 0)", ok, suppressed)
	}
	for i := 0; i < 10; i++ {
		ok, suppressed = rl.allow("ip|203.0.113.5")
		if ok || suppressed != 0 {
			t.Fatalf("suppressed event %d = (%v, %d), want (false, 0)", i, ok, suppressed)
		}
	}
	now = now.Add(time.Minute)
	ok, suppressed = rl.allow("ip|203.0.113.5")
	if !ok || suppressed != 10 {
		t.Fatalf("after interval = (%v, %d), want (true, 10)", ok, suppressed)
	}
}

func TestRefusalLogKeysAreIndependent(t *testing.T) {
	now := time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)
	rl := newRefusalLog(time.Minute, func() time.Time { return now })

	if ok, _ := rl.allow("ip|one"); !ok {
		t.Fatal("first key's first event was suppressed")
	}
	if ok, _ := rl.allow("ip|two"); !ok {
		t.Fatal("second key's first event was suppressed")
	}
	if ok, _ := rl.allow("ip|one"); ok {
		t.Fatal("repeated first key was not suppressed")
	}
}
