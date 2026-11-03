package web

import (
	"strings"
	"testing"
)

// The live-log refresh control is intentionally before the search field: it is
// the first action in the control row, and the stream-pause handler shares its
// paused-state contract with the refresh handler.
func TestLiveLogRefreshControlPrecedesSearch(t *testing.T) {
	doc := readAsset(t, "index.html")
	refresh := strings.Index(doc, `id="stream-refresh-btn"`)
	search := strings.Index(doc, `id="stream-search"`)
	if refresh < 0 {
		t.Fatal("index.html has no live-log refresh button")
	}
	if search < 0 {
		t.Fatal("index.html has no live-log search input")
	}
	if refresh > search {
		t.Error("the refresh button is after the search input; it belongs to the left of the search bar")
	}
	if !strings.Contains(doc[refresh:search], `aria-label="Refresh live log"`) {
		t.Error("the refresh button has no accessible label")
	}
}

// Settings is a compact card grid, with the wide cards opting back into a full
// row. The class must stay on the section: this test is the guard against a
// future markup edit that removes the layout without touching the CSS.
func TestSettingsTabUsesCompactGrid(t *testing.T) {
	doc := readAsset(t, "index.html")
	if !strings.Contains(doc, `id="tab-rules" class="tab-content space-y-6 hidden settings-layout"`) {
		t.Fatal("tab-rules is missing the settings-layout class")
	}
	if got := strings.Count(doc, "settings-wide"); got < 6 {
		t.Fatalf("settings-wide appears %d times, want at least 6 wide cards", got)
	}
}
