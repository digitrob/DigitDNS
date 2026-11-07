package selfupdate

import (
	"strings"
	"testing"
	"time"
)

// compareSemver decides whether the dashboard offers an update, so a wrong
// verdict here either nags on every load or hides a real release. These pin the
// numeric-core comparison and the suffix-stripping behaviour.
func TestCompareSemver(t *testing.T) {
	cases := []struct {
		a, b string
		want int // sign: >0 a newer, 0 equal, <0 a older
	}{
		{"2.6.0", "2.5.0", 1},
		{"2.5.0", "2.6.0", -1},
		{"2.5.0", "2.5.0", 0},
		{"2.5.1", "2.5.0", 1},
		{"2.10.0", "2.9.0", 1}, // numeric, not lexical
		{"3.0.0", "2.99.99", 1},
		{"v2.6.0", "2.5.0", 1},              // leading v tolerated
		{"2.6.0-beta", "2.5.0", 1},          // suffix ignored on the core compare
		{"2.5.0-beta.2", "2.5.0-beta.1", 0}, // suffix not compared -> equal cores
		{"2.5", "2.5.0", 0},                 // missing field reads as zero
	}
	for _, c := range cases {
		got := compareSemver(c.a, c.b)
		if (got > 0) != (c.want > 0) || (got < 0) != (c.want < 0) || (got == 0) != (c.want == 0) {
			t.Errorf("compareSemver(%q,%q)=%d, want sign of %d", c.a, c.b, got, c.want)
		}
	}
}

// A stale Running flag from a failed restart must not refuse every later click.
func TestApplyBusyOnlyWhileRecent(t *testing.T) {
	now := time.Now()
	if !applyBusy(Progress{Running: true, StartedAt: now.Add(-time.Minute)}, now) {
		t.Fatal("a recent apply should be busy")
	}
	if applyBusy(Progress{Running: true, StartedAt: now.Add(-10 * time.Minute)}, now) {
		t.Fatal("a stale apply should allow a retry")
	}
	if applyBusy(Progress{Running: false, StartedAt: now}, now) {
		t.Fatal("an idle updater should not be busy")
	}
	if applyBusy(Progress{Running: true}, now) {
		t.Fatal("a Running flag with no start time must be treated as stale")
	}
}

func TestContainerMarkerIsDetected(t *testing.T) {
	t.Setenv("DIGITDNS_CONTAINER", "1")
	if !runningInContainer() {
		t.Fatal("DIGITDNS_CONTAINER=1 was not detected")
	}
	u := New("digitdns", nil, t.TempDir())
	if !u.container {
		t.Fatal("Updater did not record the container marker")
	}
	// StartApply must refuse before touching the network. On Linux the message
	// names the pull/recreate command; on a dev machine it names the OS gate.
	if err := u.StartApply(); err == nil {
		t.Fatal("container install accepted an in-app update")
	} else if !strings.Contains(strings.ToLower(err.Error()), "container") && !strings.Contains(strings.ToLower(err.Error()), "linux") {
		t.Fatalf("unexpected refusal: %v", err)
	}
}

// The modal polls the status endpoint, so a stale Running flag must be cleared
// there — not only when the operator clicks Update again.
func TestProgressClearsStaleRunning(t *testing.T) {
	now := time.Now()
	u := &Updater{progress: Progress{
		Running:    true,
		Restarting: true,
		Phase:      "download",
		Message:    "Downloading…",
		StartedAt:  now.Add(-10 * time.Minute),
	}}
	got := u.Progress()
	if got.Running || got.Restarting {
		t.Fatalf("stale apply still reported as running: %+v", got)
	}
	if got.Phase != "error" || got.Error == "" {
		t.Fatalf("stale apply was not turned into an actionable error: %+v", got)
	}
	// The stored state is healed too, so every later poll agrees.
	if again := u.Progress(); again.Running || again.Error != got.Error {
		t.Fatalf("stale state was not persisted: %+v", again)
	}

	// A recent apply is returned untouched.
	u2 := &Updater{progress: Progress{Running: true, Phase: "download", StartedAt: time.Now()}}
	if live := u2.Progress(); !live.Running || live.Phase != "download" {
		t.Fatalf("live apply was modified: %+v", live)
	}
}
