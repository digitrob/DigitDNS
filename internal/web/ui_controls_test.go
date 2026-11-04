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

// The historical dashboard block is separate from the live card grid: its ids
// are the contract between index.html, the v3 history endpoints and app.js. The
// history cards also opt into the dashboard layout engine through their own
// data-dash-grid group.
func TestDashboardHasTrafficHistorySection(t *testing.T) {
	doc := readAsset(t, "index.html")
	for _, id := range []string{
		"history-section", "history-range", "history-refresh-btn", "historyChart",
		"history-total", "history-blocked", "history-proxied",
		"history-cache", "history-clients", "history-latency",
		"history-top-domains", "history-top-clients", "history-top-upstreams",
	} {
		if !strings.Contains(doc, `id="`+id+`"`) {
			t.Errorf("index.html is missing history element %q", id)
		}
	}
	if !strings.Contains(doc, `data-dash-grid="history"`) || !strings.Contains(doc, `data-dash-card="history-chart"`) {
		t.Error("history cards are not part of the dashboard layout editor")
	}
	for _, rangeValue := range []string{"1h", "24h", "7d", "30d", "90d"} {
		if !strings.Contains(doc, `data-history-range="`+rangeValue+`"`) {
			t.Errorf("index.html is missing the %s history range button", rangeValue)
		}
	}
	// Exports are actions, not time selectors: they follow the last range chip so
	// they can sit at the far right of the toolbar.
	lastRange := strings.Index(doc, `data-history-range="90d"`)
	csv := strings.Index(doc, `id="history-export-csv"`)
	json := strings.Index(doc, `id="history-export-json"`)
	if lastRange < 0 || csv < 0 || json < 0 || csv < lastRange || json < csv {
		t.Error("CSV/JSON exports must come after the history range buttons")
	}
	if !strings.Contains(doc, `class="history-export-btn`) {
		t.Error("history exports have no dedicated action style")
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

// Ad blocking has its own sidebar page now, not a Settings card. These ids are
// the contract between index.html and the loader in app.js; a rename on one side
// would leave an empty shell that looks like a backend failure.
func TestFiltersPageHasAdBlockingCard(t *testing.T) {
	doc := readAsset(t, "index.html")
	page := strings.Index(doc, `id="tab-filters"`)
	card := strings.Index(doc, `id="filtering-card"`)
	if page < 0 || card < page {
		t.Fatalf("filtering card is not inside the filters page: page=%d card=%d", page, card)
	}
	if !strings.Contains(doc, `data-tab="filters"`) || !strings.Contains(doc, `data-route="/filters"`) {
		t.Error("index.html is missing the filters sidebar entry")
	}
	for _, id := range []string{
		"filtering-card", "filtering-enabled-input", "filtering-mode-select",
		"filtering-ttl-input", "filtering-interval-input",
		"filtering-allowlist-input", "filtering-save-settings-btn",
		"filtering-refresh-all-btn", "filtering-add-url-btn", "filtering-file-input",
		"filtering-name-input", "filtering-url-input", "filtering-homepage-input",
		"filtering-presets", "filtering-preset-search",
		"filtering-recommended-base-btn", "filtering-show-custom-btn",
		"filtering-custom-panel", "filtering-summary", "filtering-preset-summary", "filtering-error",
	} {
		if !strings.Contains(doc, `id="`+id+`"`) {
			t.Errorf("index.html is missing filtering element %q", id)
		}
	}
}
