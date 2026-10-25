package version_test

import (
	"encoding/json"
	"os"
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
