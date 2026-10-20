package service

import (
	"testing"
	"time"

	"digitdns/internal/database"
)

// A CIDR entry an operator attaches to an account has to resolve by containment,
// not equality: the resolver reports a concrete peer address, never a network.
func TestAllowedCIDRMatchesByContainment(t *testing.T) {
	svc, db, cleanup := setupClientFixture(t)
	defer cleanup()

	if err := db.SaveClient(database.Client{
		ID: "net1", UUID: database.GenerateUUID(), Name: "office", Token: "tok-net1",
		AllowedIPs: []string{"203.0.113.0/24"}, Enabled: true, CreatedAt: time.Now(),
		RegisterSecret: "fixturesecret-net1",
	}); err != nil {
		t.Fatalf("SaveClient: %v", err)
	}
	svc.reloadCache()

	c, ok := svc.IsIPAllowed("203.0.113.55")
	if !ok || c == nil || c.ID != "net1" {
		t.Fatalf("an address inside the CIDR resolved to (%v, %v), want account net1", c, ok)
	}

	// Public mode allows everyone, so the outside address is only meaningfully
	// "refused" in whitelist mode.
	if err := svc.SetAllowAll(false); err != nil {
		t.Fatalf("SetAllowAll: %v", err)
	}
	if _, ok := svc.IsIPAllowed("198.51.100.1"); ok {
		t.Errorf("an address outside the CIDR was allowed")
	}
	if _, ok := svc.IsIPAllowed("203.0.113.55"); !ok {
		t.Errorf("the CIDR stopped matching after switching to whitelist mode")
	}
}

// A source that matches the server-wide allowlist must be served as an anonymous
// Public account (nil client) while whitelist mode is on, and not otherwise.
func TestServerWideAllowlistServesPublicButOnlyInWhitelistMode(t *testing.T) {
	svc, _, cleanup := setupClientFixture(t) // allowAll=true
	defer cleanup()

	if _, err := svc.SetResolverAllowCIDRs([]string{"198.51.100.0/24", "2001:db8::/32"}); err != nil {
		t.Fatalf("SetResolverAllowCIDRs: %v", err)
	}

	// Public mode: everything is already allowed, and the allowlisted source has
	// no account either.
	if c, ok := svc.IsIPAllowed("198.51.100.7"); !ok || c != nil {
		t.Fatalf("in public mode an allowlisted source resolved to (%v, %v)", c, ok)
	}

	// Whitelist mode: a non-account source is refused unless allowlisted.
	if err := svc.SetAllowAll(false); err != nil {
		t.Fatalf("SetAllowAll: %v", err)
	}
	if c, ok := svc.IsIPAllowed("198.51.100.7"); !ok || c != nil {
		t.Errorf("whitelist mode refused an allowlisted source: (%v, %v)", c, ok)
	}
	if _, ok := svc.IsIPAllowed("203.0.113.250"); ok {
		t.Errorf("whitelist mode allowed a source that is neither a client nor allowlisted")
	}

	got := svc.ResolverAllowCIDRs()
	if len(got) != 2 {
		t.Fatalf("ResolverAllowCIDRs returned %v, want two canonical entries", got)
	}
}

// SetResolverAllowCIDRs must reject a malformed entry rather than silently drop
// it: an operator who pasted a typo has to learn the allowlist did not change.
func TestSetResolverAllowCIDRsRejectsMalformed(t *testing.T) {
	svc, _, cleanup := setupClientFixture(t)
	defer cleanup()

	if _, err := svc.SetResolverAllowCIDRs([]string{"203.0.113.0/33"}); err == nil {
		t.Fatalf("a /33 prefix was accepted")
	}
	if got := svc.ResolverAllowCIDRs(); len(got) != 0 {
		t.Errorf("a rejected write left %v behind", got)
	}
}

// CIDR entries are exempt from MaxDevices: shrinking the limit trims host
// bindings but never the operator's network range.
func TestCIDRExemptFromMaxDevices(t *testing.T) {
	svc, db, cleanup := setupClientFixture(t)
	defer cleanup()

	if err := db.SaveClient(database.Client{
		ID: "mix1", UUID: database.GenerateUUID(), Name: "mix", Token: "tok-mix1",
		AllowedIPs: []string{"203.0.113.0/24", "198.51.100.1", "198.51.100.2"},
		Enabled:    true, CreatedAt: time.Now(), RegisterSecret: "fixturesecret-mix1",
	}); err != nil {
		t.Fatalf("SaveClient: %v", err)
	}
	svc.reloadCache()

	updated, err := svc.SetClientAllowedIPs("mix1", []string{"203.0.113.0/24", "198.51.100.1", "198.51.100.2", "198.51.100.3"})
	if err != nil {
		t.Fatalf("SetClientAllowedIPs: %v", err)
	}
	if len(updated.AllowedIPs) != 4 {
		t.Fatalf("SetClientAllowedIPs stored %v, want four entries", updated.AllowedIPs)
	}
	if _, ok := svc.IsIPAllowed("198.51.100.99"); !ok {
		t.Errorf("the CIDR stopped matching after SetClientAllowedIPs")
	}
}
