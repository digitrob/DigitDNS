package bootstrap

import (
	"fmt"
	"log"
)

// migrationMarkerKey is the settings-bucket key holding the set of one-time
// boot migrations this database has already received. Each entry is written in
// the same transaction-free sequence as its effect (effect first, marker
// second), so a crash between the two re-runs the migration on next boot —
// which every migration must therefore tolerate.
const migrationMarkerKey = "migration"

// MigrateDefaultAllowAll converges a pre-v2.2.0 database onto the whitelist
// default.
//
// Before v2.2.0 the compiled default was allow_all=true and no record was ever
// stored until an operator touched the toggle, so a long-running install can
// sit on "no allow_all row at all" while genuinely meaning public mode. The
// v2.2.0 default is the opposite (unknown sources refused), and the user has
// decided an upgrading install follows it: the migration forces one write of
// allow_all=false and marks it done.
//
// Ordering guarantees, given the effect-first/marker-second write pair:
//
//   - Crash before the allow_all write: nothing changed; next boot retries.
//   - Crash between the two writes: allow_all=false is already correct; the
//     missing marker only causes one harmless rewrite of the same value.
//   - After the marker: never touched again — including by a later downgrade,
//     because the marker, not the version, is the gate.
//
// An operator who wants public mode back flips the dashboard toggle, which
// writes its own row; from then on the DB record wins over every default.
//
// fileAllowAll is true when this is a first boot (no persisted server record)
// and config.json explicitly set access.allow_all. There is no legacy database
// to converge in that case, and forcing false would silently discard the
// operator's written configuration — the file is the only source of truth at
// that moment. The marker is still written, so a later boot cannot flip it
// either.
func MigrateDefaultAllowAll(store SettingsWriter, fileAllowAll bool) {
	if store == nil {
		return
	}
	migrations, err := loadMigrationMarkers(store)
	if err != nil {
		// Unreadable marker state is reported and then handled FAIL-CLOSED:
		// rather than skipping the migration (which left a legacy install
		// running public), converge allow_all to the safe default and leave the
		// marker unwritten, so the next boot retries. The daemon still starts —
		// refusing to boot over a non-secret bookkeeping key would take the
		// whole resolver down for a bookmark — but it starts CLOSED.
		log.Printf("[Bootstrap] could not read migration markers (%v); converging allow_all to the safe default and retrying next boot", err)
		if err2 := store.SetSetting("allow_all", false); err2 != nil {
			log.Printf("[Bootstrap] could not persist the v2.2.0 whitelist default: %v", err2)
		}
		return
	}
	if migrations["whitelist_v2_2"] {
		return
	}
	if fileAllowAll {
		migrations["whitelist_v2_2"] = true
		if err := store.SetSetting(migrationMarkerKey, migrations); err != nil {
			log.Printf("[Bootstrap] kept config.json's allow_all value but could not write the migration marker: %v", err)
		}
		log.Printf("[Bootstrap] config.json sets access.allow_all; keeping it (no legacy database to converge)")
		return
	}
	if err := store.SetSetting("allow_all", false); err != nil {
		log.Printf("[Bootstrap] could not persist the v2.2.0 whitelist default (refusing sources by default): %v", err)
		return
	}
	migrations["whitelist_v2_2"] = true
	if err := store.SetSetting(migrationMarkerKey, migrations); err != nil {
		// See the crash-between-writes case above: the effect already landed;
		// the migration will simply run once more on some later boot and
		// rewrite the same value.
		log.Printf("[Bootstrap] whitelist default applied but the marker write failed (it will re-run harmlessly): %v", err)
		return
	}
	log.Printf("[Bootstrap] v2.2.0: client access whitelist enabled — unregistered sources are now refused. Re-enable public access from the dashboard if that is what you intended.")
}

// loadMigrationMarkers reads the marker set, treating an absent record as an
// empty set (a pre-v2.2.0 database) rather than an error.
//
// "Present but undecodable" is distinguished from "absent" and is a HARD error
// (audit finding #7, v2.8): a marker that is valid JSON of the wrong type — a
// number, a string, an array, or an object with non-boolean members — used to
// decode-fail here, and the caller then skipped the allow_all=false convergence
// and booted a legacy install as an OPEN resolver. The store's schema gate now
// also validates this key (see setting_repo.go's migration entry), which makes
// the wrong-typed marker unreachable at boot; this branch is the second fence.
func loadMigrationMarkers(store SettingsStore) (map[string]bool, error) {
	present, err := store.SettingExists(migrationMarkerKey)
	if err != nil {
		return nil, fmt.Errorf("inspect migration markers: %w", err)
	}
	if !present {
		return map[string]bool{}, nil
	}
	var markers map[string]bool
	if err := store.GetSetting(migrationMarkerKey, &markers); err != nil {
		return nil, fmt.Errorf("the stored migration marker is unreadable (corrupt or wrong type): %w", err)
	}
	if markers == nil {
		// A JSON null where a marker object belongs is the same class of
		// corruption; treat it as unreadable rather than "never migrated".
		return nil, fmt.Errorf("the stored migration marker is empty (expected a marker object)")
	}
	return markers, nil
}
