package bootstrap

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"digitdns/internal/crypto"
	"digitdns/internal/database"
)

// Audit finding #7: a migration marker that is valid JSON of the wrong type
// (number, string, array, or an object with non-boolean members) used to
// decode-fail in loadMigrationMarkers, and the caller then SKIPPED the
// allow_all=false convergence — booting a legacy install as an open resolver.
// The gate now (a) refuses to boot on the wrong-typed marker at the schema
// layer, and (b) converges allow_all to the safe default if the marker is ever
// unreadable, so the daemon can no longer fail open.
func TestWrongTypedMigrationMarkerFailsClosed(t *testing.T) {
	for _, bad := range []string{"42", `"v2.2"`, "[1,2]", "true", `{"whitelist_v2_2":"yes"}`, `{"whitelist_v2_2":1}`} {
		dir := t.TempDir()
		keyPath := filepath.Join(dir, "master.key")
		if _, err := crypto.LoadOrGenerateMasterKey(keyPath); err != nil {
			t.Fatalf("key: %v", err)
		}
		db, err := database.Create(filepath.Join(dir, "data.db"), mustCipher(t, keyPath))
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		// Legacy posture: public mode, plus the mis-typed marker.
		if err := db.SetSetting("allow_all", true); err != nil {
			t.Fatalf("allow_all: %v", err)
		}
		if err := db.SetSetting("migration", mustDecodeAny(bad)); err != nil {
			t.Fatalf("marker %s: %v", bad, err)
		}
		MigrateDefaultAllowAll(db)
		var allowAll bool
		if err := db.GetSetting("allow_all", &allowAll); err != nil {
			t.Fatalf("read allow_all: %v", err)
		}
		if allowAll {
			t.Fatalf("marker %s: allow_all stayed true — the resolver would boot PUBLIC", bad)
		}
		db.Close()

		// The database must also REFUSE to reopen: the schema gate treats the
		// wrong-typed marker as corruption rather than booting with it.
		_, err = database.OpenExisting(filepath.Join(dir, "data.db"), mustCipher(t, keyPath))
		if err == nil {
			t.Fatalf("marker %s: a wrong-typed migration marker must fail the boot validation", bad)
		}
	}
}

// A well-formed marker still behaves as before: absent = legacy first boot
// (converge once), unknown markers act as "not migrated", a boolean marker acts
// as migrated.
func TestValidMigrationMarkerBehaviourUnchanged(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "master.key")
	if _, err := crypto.LoadOrGenerateMasterKey(keyPath); err != nil {
		t.Fatalf("key: %v", err)
	}
	db, err := database.Create(filepath.Join(dir, "data.db"), mustCipher(t, keyPath))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer db.Close()

	// Absent marker + allow_all=true -> converged on first boot.
	if err := db.SetSetting("allow_all", true); err != nil {
		t.Fatalf("allow_all: %v", err)
	}
	MigrateDefaultAllowAll(db)
	var allowAll bool
	_ = db.GetSetting("allow_all", &allowAll)
	if allowAll {
		t.Fatal("absent marker must converge allow_all to false")
	}
}

func mustDecodeAny(raw string) any {
	var v any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		panic(err)
	}
	return v
}
