package web

import (
	"os"
	"strings"
	"testing"
)

// The fork's identity is a product decision, not an internal one: the browser is
// served "DigitDNS", the Proxied section keeps its renamed title, and the
// AGPL-3.0 requires the upstream work be credited in the running app and in the
// repository. These pins are cheap, and they are what stops a future upstream
// merge (see .github/workflows/upstream-sync.yml) from quietly reverting the
// rebrand or dropping the attribution.

// repoFile reads a file relative to this package's directory (`go test` runs
// with internal/web as the working directory).
func repoFile(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	return string(b)
}

func TestDashboardIsBrandedDigitDNS(t *testing.T) {
	html := readAsset(t, "index.html")
	if !strings.Contains(html, "<title>DigitDNS — Anti-Sanction SmartDNS Controller</title>") {
		t.Error("index.html title is no longer the DigitDNS brand")
	}
	for _, marker := range []string{
		`data-tab="proxied"`,
		"Proxied Domains",
		`id="proxied-sort"`,
		`id="proxied-load-size"`,
	} {
		if !strings.Contains(html, marker) {
			t.Errorf("index.html is missing %q — the Proxied Domains section regressed", marker)
		}
	}
}

func TestDashboardCarriesAGPLAttribution(t *testing.T) {
	html := readAsset(t, "index.html")
	for _, marker := range []string{"AGPL-3.0", "Based on", "HyperDNS", "Digitrob"} {
		if !strings.Contains(html, marker) {
			t.Errorf("index.html is missing the attribution token %q", marker)
		}
	}
}

func TestRepositoryShipsUpstreamNoticeAndAttribution(t *testing.T) {
	notice := repoFile(t, "../../NOTICE")
	for _, marker := range []string{
		"AGPL", "IzumiRain/HyperDNS", "digitrob/DigitDNS", "fork",
	} {
		if !strings.Contains(notice, marker) {
			t.Errorf("NOTICE is missing %q", marker)
		}
	}
	readme := repoFile(t, "../../README.md")
	if !strings.Contains(readme, "IzumiRain/HyperDNS") || !strings.Contains(readme, "AGPL-3.0") {
		t.Error("README.md lost its upstream attribution or AGPL licence mention")
	}
}

func TestSelfUpdateRepointedToFork(t *testing.T) {
	src := readGoSource(t, "../selfupdate/selfupdate.go")
	for _, pin := range []string{
		`ghOwner  = "digitrob"`,
		`ghRepo   = "DigitDNS"`,
		`ghBranch = "custom"`,
	} {
		if !strings.Contains(src, pin) {
			t.Errorf("internal/selfupdate/selfupdate.go no longer pins %q — a fork build would follow upstream", pin)
		}
	}
}

func TestUpstreamSyncWorkflowExists(t *testing.T) {
	wf := repoFile(t, "../../.github/workflows/upstream-sync.yml")
	for _, marker := range []string{"IzumiRain/HyperDNS", "custom", "schedule"} {
		if !strings.Contains(wf, marker) {
			t.Errorf("upstream-sync.yml is missing %q", marker)
		}
	}
}
