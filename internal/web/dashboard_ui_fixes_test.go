package web

import (
	"strings"
	"testing"
)

// The Live Traffic card carries two rates in one headline row. A source-level
// check is enough to pin the fix: if a later edit restores flex-wrap, drops a
// unit span, or puts the large single-number scale back, the card will wrap at
// three digits again on the six-across dashboard grid.
func TestLiveTrafficRowStaysOnOneLine(t *testing.T) {
	doc := readAsset(t, "index.html")

	start := strings.Index(doc, `<div id="stat-traffic-row"`)
	if start < 0 {
		t.Fatal("index.html has no stat-traffic-row — the scan is broken")
	}
	end := strings.Index(doc[start:], `>`)
	if end < 0 {
		t.Fatal("stat-traffic-row has no closing tag — the scan is broken")
	}
	tag := doc[start : start+end]
	if !strings.Contains(tag, "metric-value-traffic") {
		t.Fatal("Live Traffic row lost its custom layout class")
	}
	if strings.Contains(tag, "flex-wrap") {
		t.Fatal("Live Traffic row still carries the old flex-wrap class")
	}

	for _, id := range []string{"stat-speed-in-unit", "stat-speed-out-unit"} {
		if got := strings.Count(doc, `id="`+id+`"`); got != 1 {
			t.Fatalf("index.html has %d %s spans, want 1", got, id)
		}
	}

	css := readAsset(t, "css/style.css")
	cssStart := strings.Index(css, "\n.metric-value-traffic {")
	if cssStart < 0 {
		t.Fatal("style.css has no .metric-value-traffic rule — the scan is broken")
	}
	cssEnd := strings.Index(css[cssStart:], "}")
	if cssEnd < 0 {
		t.Fatal("metric-value-traffic rule has no closing brace — the scan is broken")
	}
	rule := css[cssStart : cssStart+cssEnd]
	if !strings.Contains(rule, "clamp(1.05rem, 6cqi, 1.55rem)") {
		t.Fatal("metric-value-traffic lost its compact clamp")
	}
	if !strings.Contains(rule, "flex-wrap: nowrap") || !strings.Contains(rule, "white-space: nowrap") {
		t.Fatal("metric-value-traffic no longer forbids wrapping")
	}
	const custom = `#tab-dashboard.dash-custom-layout [data-dash-grid="metrics"] .metric-value-traffic`
	customAt := strings.Index(css, custom)
	if customAt < 0 {
		t.Fatal("style.css no longer overrides the traffic scale in custom layouts")
	}
	if !strings.Contains(css[customAt:customAt+400], "clamp(1.05rem, 6cqi, 1.55rem)") {
		t.Fatal("custom layout still uses the large single-number traffic scale")
	}

	app := jsLineCommentRe.ReplaceAllString(readAsset(t, "js/app.js"), "")
	if !strings.Contains(app, "function formatTrafficSpeed(") {
		t.Fatal("js/app.js lost formatTrafficSpeed(); long KB/s values will wrap again")
	}
	for _, want := range []string{
		"setTxt('stat-speed-in-unit', speedIn.unit);",
		"setTxt('stat-speed-out-unit', speedOut.unit);",
		"const trafficTitle = `In ${speedIn.title} · Out ${speedOut.title}`;",
	} {
		if !strings.Contains(app, want) {
			t.Fatalf("js/app.js no longer contains %q", want)
		}
	}
}

// The two brand marks are the only persistent "home" affordance in the panel.
// They must both route to the dashboard, and the update button injected into the
// desktop brand row must keep opening its modal instead of navigating.
func TestBrandMarksNavigateToDashboard(t *testing.T) {
	doc := readAsset(t, "index.html")
	if got := strings.Count(doc, "data-home-link"); got != 2 {
		t.Fatalf("index.html has %d home links, want 2 (sidebar and mobile)", got)
	}
	for _, id := range []string{"sidebar-home-btn", "mobile-home-btn"} {
		if !strings.Contains(doc, `id="`+id+`"`) {
			t.Fatalf("index.html has no %s home link", id)
		}
	}

	app := jsLineCommentRe.ReplaceAllString(readAsset(t, "js/app.js"), "")
	fnAt := strings.Index(app, "function goToDashboardFromBrand(")
	if fnAt < 0 {
		t.Fatal("js/app.js has no brand home handler — the scan is broken")
	}
	end := strings.Index(app[fnAt:], "\n}")
	if end < 0 {
		t.Fatal("brand home handler has no closing brace — the scan is broken")
	}
	fn := app[fnAt : fnAt+end]
	for _, want := range []string{"#update-available-btn", "closeMobileMore();", "switchTab('dashboard');"} {
		if !strings.Contains(fn, want) {
			t.Fatalf("brand home handler no longer contains %q", want)
		}
	}
	if strings.Index(fn, "#update-available-btn") > strings.Index(fn, "switchTab('dashboard');") {
		t.Fatal("brand home handler routes before ignoring the update button")
	}
	if !strings.Contains(app, "initBrandHomeLinks();") {
		t.Fatal("brand home links are never wired up")
	}
}
