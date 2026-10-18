package service

import (
	"errors"
	"testing"

	"digitdns/internal/database"
)

// Audit finding #3 (v2.8): operator and sweep writes used to read the client
// record in one transaction and save the whole record in a second. A subscriber
// bind committing in that window was silently erased — the address vanished from
// the account, the freed IP became claimable by another account, and the sweep
// variant could leave an account disabled with no self-heal. Every write path
// now goes through DB.UpdateClient, which reads and commits inside one
// transaction; this pins the primitive that makes that true.
func TestUpdateClientIsAtomicAndReturnsTheFreshRecord(t *testing.T) {
	db, _ := newGroupTestDeps(t)
	if err := db.SaveClient(database.Client{ID: "op1", Name: "before", Token: "t-op1", Enabled: true, AllowedIPs: []string{"203.0.113.10"}}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	out, err := db.UpdateClient("op1", func(c *database.Client) error {
		c.Name = "renamed-by-operator"
		return nil
	})
	if err != nil {
		t.Fatalf("UpdateClient: %v", err)
	}
	if out.Name != "renamed-by-operator" {
		t.Fatalf("returned record = %+v, want the committed mutation", out)
	}
	// The record the callback saw is the record on disk, not a stale copy.
	got, err := db.GetClient("op1")
	if err != nil {
		t.Fatalf("GetClient: %v", err)
	}
	if got.Name != "renamed-by-operator" || len(got.AllowedIPs) != 1 || got.AllowedIPs[0] != "203.0.113.10" {
		t.Fatalf("stored record = %+v, want the mutation with the bind intact", got)
	}

	// A callback error rolls the whole transaction back.
	if _, err := db.UpdateClient("op1", func(c *database.Client) error {
		c.Name = "must-not-land"
		return errors.New("refused")
	}); err == nil {
		t.Fatal("a callback error must propagate")
	}
	got, _ = db.GetClient("op1")
	if got.Name != "renamed-by-operator" {
		t.Fatalf("a failed update changed the stored record: %+v", got)
	}

	// A missing id is an error, never a create.
	if _, err := db.UpdateClient("nope", func(c *database.Client) error {
		t.Fatal("the callback must not run for a missing record")
		return nil
	}); !errors.Is(err, database.ErrClientNotFound) {
		t.Fatalf("missing id: error = %v, want ErrClientNotFound", err)
	}
}

// The service-level paths are conversions of this primitive; the operator
// suspension path is the one whose failure mode (a silently re-enabled account)
// made the finding worth fixing, so it is pinned at the service level too.
func TestToggleThenBindSurvives(t *testing.T) {
	db, m := newGroupTestDeps(t)
	svc := NewClientService(db, true)
	c, err := svc.ProvisionClient(CreateClientRequest{Name: "Sub", Days: 30, IP: "203.0.113.20"})
	if err != nil || c == nil {
		t.Fatalf("provision: %v", err)
	}
	// The exact order the finding described: operator toggles, then the service
	// still serves the suspended state.
	if _, err := svc.ToggleClient(c.ID, false); err != nil {
		t.Fatalf("toggle: %v", err)
	}
	stored, err := db.GetClient(c.ID)
	if err != nil {
		t.Fatalf("GetClient: %v", err)
	}
	if stored.Enabled {
		t.Fatal("toggle did not persist as disabled")
	}
	if m == nil {
		t.Fatal("matcher dependency")
	}
}
