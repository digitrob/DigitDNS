package database

import (
	"errors"
	"testing"
)

// The auto-renew flags are stored inverted so the zero value keeps renewal ON.
// These tests pin that reading of "absent means renew", the positive form the
// snapshot exposes, and the rollback-on-persist-failure contract both setters
// share with the rest of the access layer.

// A nil SubscriptionSettings — a caller before migration — reads as renewal on.
func TestSubscriptionNilSnapshotRenewsByDefault(t *testing.T) {
	var s *SubscriptionSettings
	if !s.Snapshot().AutoRenewACME {
		t.Error("nil receiver: AutoRenewACME = false, want true (absent means renew)")
	}
}

// A zero-valued record — every record written before the field existed —
// reads as renewal on, which is what makes adding the field a no-op for
// existing installs.
func TestSubscriptionZeroRecordRenewsByDefault(t *testing.T) {
	s := &SubscriptionSettings{}
	if !s.Snapshot().AutoRenewACME {
		t.Error("zero record: AutoRenewACME = false, want true")
	}
}

// Apply carries the positive snapshot value into the inverted stored field,
// and the snapshot reads it back.
func TestSubscriptionApplyRoundTripsAutoRenew(t *testing.T) {
	s := &SubscriptionSettings{}
	if err := s.Apply(SubscriptionSnapshot{AutoRenewACME: false}, nil); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if !s.NoAutoRenewACME {
		t.Error("stored NoAutoRenewACME = false after Apply{AutoRenewACME:false}, want true")
	}
	if s.Snapshot().AutoRenewACME {
		t.Error("snapshot AutoRenewACME = true after disabling, want false")
	}
}

// A failed persist restores the previous auto-renew choice along with the
// rest of the record.
func TestSubscriptionApplyRestoresAutoRenewOnFailure(t *testing.T) {
	s := &SubscriptionSettings{}
	// Start from renewal ON (the zero value) and attempt to turn it off with a
	// persist that fails.
	err := s.Apply(SubscriptionSnapshot{AutoRenewACME: false}, func(*SubscriptionSettings) error {
		return errors.New("disk full")
	})
	if err == nil {
		t.Fatal("Apply returned nil, want the persist error")
	}
	if !s.Snapshot().AutoRenewACME {
		t.Error("auto-renew was left disabled after a failed persist; want it restored to enabled")
	}
}

// IsDotAutoRenewEnabled / SetDotAutoRenew round-trip and default the same way.
func TestDotAutoRenewAccessors(t *testing.T) {
	var nilTLS *TLSSettings
	if !nilTLS.IsDotAutoRenewEnabled() {
		t.Error("nil receiver: IsDotAutoRenewEnabled = false, want true")
	}

	tls := &TLSSettings{}
	if !tls.IsDotAutoRenewEnabled() {
		t.Error("zero record: IsDotAutoRenewEnabled = false, want true")
	}
	if !tls.Snapshot().DotAutoRenewACME {
		t.Error("zero record: snapshot DotAutoRenewACME = false, want true")
	}

	if err := tls.SetDotAutoRenew(false, nil); err != nil {
		t.Fatalf("SetDotAutoRenew(false): %v", err)
	}
	if tls.IsDotAutoRenewEnabled() {
		t.Error("IsDotAutoRenewEnabled = true after opting out, want false")
	}
	if !tls.NoDotAutoRenewACME {
		t.Error("stored NoDotAutoRenewACME = false after opting out, want true")
	}
	if tls.Snapshot().DotAutoRenewACME {
		t.Error("snapshot DotAutoRenewACME = true after opting out, want false")
	}
}

// A failed persist restores the DoH/DoT auto-renew choice.
func TestSetDotAutoRenewRestoresOnFailure(t *testing.T) {
	tls := &TLSSettings{}
	err := tls.SetDotAutoRenew(false, func(*TLSSettings) error {
		return errors.New("disk full")
	})
	if err == nil {
		t.Fatal("SetDotAutoRenew returned nil, want the persist error")
	}
	if !tls.IsDotAutoRenewEnabled() {
		t.Error("DoH/DoT auto-renew was left disabled after a failed persist; want it restored")
	}
}
