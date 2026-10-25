package database

import (
	"errors"
)

// Runtime access to the operator-owned diagnostic target list.
//
// One *DiagnosticSettings is shared between the dashboard handlers that add and
// remove endpoints and the run handler that dials the list. Both run on the HTTP
// server's goroutines, so a save and a run can overlap, and the same
// string/slice tearing argument as settings_access.go applies: a caller ranging
// over the live slice while a save replaced it would dial a half-written list.
// The lock lives with the field, and every accessor returns a copy.

// ErrDiagnosticsNil is returned by mutators called on a nil *DiagnosticSettings,
// which means the daemon was wired up wrong rather than that the edit failed.
var ErrDiagnosticsNil = errors.New("diagnostic settings are not initialised")

// GetTargets returns a copy of the configured endpoints, in display order. A nil
// receiver returns nil, which the runner reads as "nothing configured" — the
// honest answer for a missing record, and the state the dashboard nudges the
// operator out of with its empty-state copy.
func (d *DiagnosticSettings) GetTargets() []DiagnosticTarget {
	if d == nil {
		return nil
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	return append([]DiagnosticTarget(nil), d.Targets...)
}

// SetTargets replaces the whole list with a copy of targets and returns the
// stored copy. It is what the dashboard's add/remove handlers converge on: both
// compute the new list from GetTargets and hand it back here, so there is a single
// writer and no read-modify-write window for two concurrent edits to interleave.
func (d *DiagnosticSettings) SetTargets(targets []DiagnosticTarget) ([]DiagnosticTarget, error) {
	if d == nil {
		return nil, ErrDiagnosticsNil
	}
	stored := append([]DiagnosticTarget(nil), targets...)
	if len(stored) == 0 {
		stored = nil
	}
	d.mu.Lock()
	d.Targets = stored
	d.mu.Unlock()
	return append([]DiagnosticTarget(nil), stored...), nil
}

// DiagnosticsTargetsSnapshot is a consistent copy of the diagnostic record for
// callers that want to serialise it into a response. It mirrors the settings
// snapshot convention: the fields agree with each other because they were copied
// under one lock acquisition.
type DiagnosticsTargetsSnapshot struct {
	Targets []DiagnosticTarget `json:"targets"`
}

// Snapshot returns a copy suitable for a response body, with a non-nil (but
// possibly empty) Targets slice so the JSON is [] rather than null — the
// dashboard's list renderer treats null and [] the same, but an empty array is
// the shape the documented contract shows.
func (d *DiagnosticSettings) Snapshot() DiagnosticsTargetsSnapshot {
	targets := d.GetTargets()
	if targets == nil {
		targets = []DiagnosticTarget{}
	}
	return DiagnosticsTargetsSnapshot{Targets: targets}
}
