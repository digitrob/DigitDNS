package version_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// version.json drives the binary's embedded version and the update feed; the
// offline bundle ships its own copy so an air-gapped install's startup drift
// check has something to compare. They have drifted before (the bundle carried
// 3.0.0 while the daemon shipped 3.4.7), and the only symptom was a warning in
// a log nobody reads until something else breaks. This test makes the drift a
// build failure instead.
func TestOfflineBundleVersionMatchesTheRepository(t *testing.T) {
	type doc struct {
		Version string `json:"version"`
		Channel string `json:"channel"`
	}
	read := func(path string) doc {
		t.Helper()
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		var d doc
		if err := json.Unmarshal(raw, &d); err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		return d
	}

	root := read("../../version.json")
	bundle := read("../../offline-bundle/version.json")
	if root.Version != bundle.Version || root.Channel != bundle.Channel {
		t.Fatalf("offline bundle version drift: version.json=%q/%q offline-bundle=%q/%q — update offline-bundle/version.json when bumping the release",
			root.Version, root.Channel, bundle.Version, bundle.Channel)
	}
}

// The install one-liner fetches config/restore/uninstall from the tag named in
// scripts/install.sh. That default has been forgotten on past releases, which
// made a fresh install pull helpers from an old tag while the README advertised a
// newer one. Pin it to version.json, and pin the offline bundle's human-facing
// title at the same time.
func TestInstallerAndBundleDocsFollowTheVersion(t *testing.T) {
	raw, err := os.ReadFile("../../version.json")
	if err != nil {
		t.Fatalf("read version.json: %v", err)
	}
	var doc struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse version.json: %v", err)
	}

	installer, err := os.ReadFile("../../scripts/install.sh")
	if err != nil {
		t.Fatalf("read scripts/install.sh: %v", err)
	}
	wantRef := `DIGITDNS_REF="${DIGITDNS_REF:-v` + doc.Version + `}"`
	if !strings.Contains(string(installer), wantRef) {
		t.Errorf("scripts/install.sh does not default DIGITDNS_REF to v%s — a fresh install would fetch helpers from the wrong tag; expected %s",
			doc.Version, wantRef)
	}

	bundle, err := os.ReadFile("../../offline-bundle/README.txt")
	if err != nil {
		t.Fatalf("read offline-bundle/README.txt: %v", err)
	}
	if !strings.Contains(string(bundle), "DigitDNS v"+doc.Version) {
		t.Errorf("offline-bundle/README.txt does not name v%s; update its title with the release", doc.Version)
	}
}
