package migrate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"
)

// These hashes pin the normalized importer output for the two fixture formats.
// The importer is additive and previewable: a change to how a source field is
// mapped must be a deliberate golden update, not a side effect of a UI change.
func goldenBundleHash(t *testing.T, name string) string {
	t.Helper()
	data := readFixture(t, name)
	var bundle Bundle
	var err error
	if name == "adguard-home.yaml" {
		bundle, err = ParseAdGuardYAML(data)
	} else {
		bundle, err = ParsePiHoleTOML(data)
	}
	if err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	bundle = bundle.Normalize()
	if err := bundle.Validate(); err != nil {
		t.Fatalf("validate %s: %v", name, err)
	}
	encoded, err := json.Marshal(bundle)
	if err != nil {
		t.Fatalf("marshal %s: %v", name, err)
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

func TestAdGuardImportGolden(t *testing.T) {
	const want = "c10b989c7700c1a78791f70ebca54051b5525feee9d8a2d93208d79c42e7d77b"
	if got := goldenBundleHash(t, "adguard-home.yaml"); got != want {
		t.Fatalf("normalized AdGuard bundle hash = %s, want %s", got, want)
	}
}

func TestPiHoleImportGolden(t *testing.T) {
	const want = "c900c6e4a52f346cc1b11ed551988f661409dd0e609f7f170ee6727b3d51dd8f"
	if got := goldenBundleHash(t, "pihole.toml"); got != want {
		t.Fatalf("normalized Pi-hole bundle hash = %s, want %s", got, want)
	}
}
