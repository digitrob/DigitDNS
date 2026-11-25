package web

import (
	"strings"
	"testing"
)

// These scans describe behavior that is otherwise impossible to assert with a
// Go unit test: the dashboard is plain JavaScript served as an embedded asset.
// They read the real source rather than a copy so a future refactor cannot keep
// the test green while dropping the behavior from the browser.

func TestLiveLogFollowChipAndDropDiagnostics(t *testing.T) {
	app := jsLineCommentRe.ReplaceAllString(readAsset(t, "js/app.js"), "")
	doc := readAsset(t, "index.html")

	for _, want := range []string{
		"stream-follow-btn",
		"streamPendingNew++",
		"function jumpToLatestStream()",
		"source.addEventListener('dropped'",
		"streamDropped = Math.max(0, Number(data.count) || 0)",
		"function streamStatusTooltip(state)",
	} {
		if !strings.Contains(app, want) {
			t.Errorf("js/app.js no longer contains %q", want)
		}
	}
	if !strings.Contains(doc, "id=\"stream-follow-btn\"") {
		t.Error("the live-log toolbar lost the follow chip")
	}
}

func TestVisibilityChangeSuspendsAndReopensTheStream(t *testing.T) {
	app := jsLineCommentRe.ReplaceAllString(readAsset(t, "js/app.js"), "")

	if !strings.Contains(app, "function suspendLiveStreamForVisibility()") {
		t.Fatal("the visibility suspension function was removed")
	}
	suspend := strings.Index(app, "suspendLiveStreamForVisibility();")
	if suspend < 0 {
		t.Fatal("a hidden tab no longer suspends the live stream")
	}
	// The visible branch of the same listener must reopen it; otherwise a tab
	// backgrounded once would stay disconnected for the rest of the session.
	if reopen := strings.Index(app[suspend:], "startLiveStream();"); reopen < 0 {
		t.Fatal("a visible tab no longer reopens the suspended stream")
	}
}

func TestLiveLogCellsFilterByDomainAndClient(t *testing.T) {
	app := jsLineCommentRe.ReplaceAllString(readAsset(t, "js/app.js"), "")

	for _, want := range []string{
		"stream-col-client stream-filter-cell",
		"stream-col-domain stream-filter-cell",
		"data-stream-filter=",
		"function applyStreamFilterFromCell(cell)",
		"searchEl.value = searchEl.value === value ? '' : value;",
		"if (filterCell) {",
	} {
		if !strings.Contains(app, want) {
			t.Errorf("js/app.js no longer contains %q", want)
		}
	}
}

func TestLatencyRowsHaveFourColourTiers(t *testing.T) {
	app := jsLineCommentRe.ReplaceAllString(readAsset(t, "js/app.js"), "")
	css := readAsset(t, "css/style.css")

	if !strings.Contains(app, "function latencyTierClass(ms)") {
		t.Fatal("the live log no longer assigns latency tiers")
	}
	for _, cls := range []string{"latency-fast", "latency-mid", "latency-slow", "latency-critical"} {
		if !strings.Contains(app, cls) {
			t.Errorf("js/app.js no longer uses %q", cls)
		}
		if !strings.Contains(css, "."+cls) {
			t.Errorf("style.css no longer styles %q", cls)
		}
	}
}

// Ranked history tables are fixed at ten rows. The old 10/50/all toolbar
// selector only changed DOM size, so it was removed; this test fails if it (or
// a replacement that fetches hundreds of rows per refresh) comes back.
func TestHistoryRankedListsAreFixedAtTen(t *testing.T) {
	app := jsLineCommentRe.ReplaceAllString(readAsset(t, "js/app.js"), "")
	doc := readAsset(t, "index.html")

	if strings.Contains(doc, "history-top-limit-btn") {
		t.Error("the history toolbar brought back the ranked-list limit control")
	}
	for _, want := range []string{
		"const HISTORY_TOP_LIMIT = 10",
		"limit=${HISTORY_TOP_LIMIT}",
	} {
		if !strings.Contains(app, want) {
			t.Errorf("js/app.js no longer contains %q", want)
		}
	}
}

func TestLiveLogDensityToggle(t *testing.T) {
	app := jsLineCommentRe.ReplaceAllString(readAsset(t, "js/app.js"), "")
	doc := readAsset(t, "index.html")
	css := readAsset(t, "css/style.css")

	if !strings.Contains(doc, "id=\"stream-density-btn\"") {
		t.Error("the live-log toolbar lost the density control")
	}
	for _, want := range []string{
		"function setStreamDense(dense)",
		"localStorage.setItem('digitdns_stream_dense'",
		"classList.toggle('stream-dense', streamDense)",
		"getElementById('stream-density-btn')?.addEventListener('click'",
	} {
		if !strings.Contains(app, want) {
			t.Errorf("js/app.js no longer contains %q", want)
		}
	}
	if !strings.Contains(css, ".stream-table.stream-dense") {
		t.Error("style.css no longer carries the dense-row rules")
	}
}
