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

// The Explain result is now a second entry point into the live log's rule menu.
// Source contracts cover the shared renderer, the delegated button wiring and the
// post-save lookup refresh: a later edit must not leave one of the two Explain
// surfaces with a stale result or a button that never opens the menu.
func TestExplainCanOpenRuleMenu(t *testing.T) {
	app := jsLineCommentRe.ReplaceAllString(readAsset(t, "js/app.js"), "")
	for _, want := range []string{
		"function renderExplain(data, targetId = 'explain-result', source = 'diagnostics')",
		"renderExplain(data, 'tools-explain-result', 'tools');",
		"data-explain-quick-action",
		"openStreamActionMenu(btn);",
		"streamActionExplainSource = trigger.dataset.explainSource || '';",
		"[data-explain-quick-action][aria-expanded=\"true\"]",
		"streamActionExplainSource === 'tools' ? toolsExplain",
		"streamActionExplainSource === 'diagnostics' ? runExplain",
		"async function applyStreamAction(action, onSaved)",
		"if (typeof onSaved === 'function') await onSaved();",
	} {
		if !strings.Contains(app, want) {
			t.Fatalf("js/app.js no longer contains %q", want)
		}
	}
	if !strings.Contains(app, "e.target.closest('[data-explain-quick-action]') || e.target.closest('#stream-action-menu')") {
		t.Fatal("an Explain rule button click would close the menu it just opened")
	}
}

// Action pictograms must be derived from the action key, not the badge colour:
// PROXY FAILED uses the red block chip but needs an alert icon, not a slash.
func TestLiveLogActionPictograms(t *testing.T) {
	app := jsLineCommentRe.ReplaceAllString(readAsset(t, "js/app.js"), "")
	icons := strings.Index(app, "const STREAM_ICON_PATHS = {")
	badgeStart := strings.Index(app, "const STREAM_ACTION_BADGES = {")
	if icons < 0 || badgeStart < 0 || icons > badgeStart {
		t.Fatal("icon paths must be declared before action badges — the badge map calls the helper at load time")
	}
	start := badgeStart
	end := strings.Index(app[start:], "\n};")
	if end < 0 {
		t.Fatal("action badge map has no closing brace — the scan is broken")
	}
	badges := app[start : start+end]
	for _, want := range []string{
		"streamActionBadge('badge-proxy', 'PROXY', 'shield')",
		"streamActionBadge('badge-block', 'Blocked', 'slash')",
		"streamActionBadge('badge-block', 'PROXY FAILED', 'alert')",
		"streamActionBadge('badge-direct', 'DIRECT', 'cornerUpRight')",
		"streamActionBadge('badge-cached', 'STALE', 'clock')",
		"streamActionBadge('badge-rewrite', 'Rewrite', 'edit3')",
	} {
		if !strings.Contains(badges, want) {
			t.Fatalf("action badge map lost %q", want)
		}
	}
	if !strings.Contains(app, `class="${klass}" aria-hidden="true"`) {
		t.Fatal("status pictograms are not marked decorative")
	}

	css := readAsset(t, "css/style.css")
	for _, want := range []string{".stream-action-badge-icon", "width: 11px", "height: 11px", "width: 10rem"} {
		if !strings.Contains(css, want) {
			t.Fatalf("style.css lost %q", want)
		}
	}
}

// The block chip reuses the rule-name classifier from the info popover. Keeping
// that link in source avoids a second category table that could drift.
func TestLiveLogBlockCategoryChip(t *testing.T) {
	app := jsLineCommentRe.ReplaceAllString(readAsset(t, "js/app.js"), "")
	for _, want := range []string{
		"const blockCategory = action === 'BLOCK' ? blocklistCategory(ruleStr) : '';",
		"{ Malware: 'block-cat-malware', Ads: 'block-cat-ads', Tracker: 'block-cat-tracker' }",
		"${blockCategoryChip}Blocked by",
		"streamIconSvg('slash', 'stream-action-badge-icon block-detail-icon')",
	} {
		if !strings.Contains(app, want) {
			t.Fatalf("js/app.js lost %q", want)
		}
	}
	css := readAsset(t, "css/style.css")
	for _, want := range []string{".block-cat-malware", ".block-cat-ads", ".block-cat-tracker"} {
		if !strings.Contains(css, want) {
			t.Fatalf("style.css lost %q", want)
		}
	}
}

// A quick action governs future lookups; the badge on a received query is the
// record of what that query actually did. The source contract keeps the old
// badge intact and greys it only after the save succeeds, keyed by query id so
// polling refreshes do not lose the marker.
func TestLiveLogChangedActionIsMarkedAfterSave(t *testing.T) {
	app := jsLineCommentRe.ReplaceAllString(readAsset(t, "js/app.js"), "")
	for _, want := range []string{
		"const streamChangedQueries = new Map();",
		"function streamQueryKey(q)",
		"function streamQueryChange(q)",
		"function markStreamQueryChanged(q, to)",
		"if (!streamActionExplainSource && logBadge)",
		"if ((q.domain || '') !== streamActionDomain) return;",
		"markStreamQueryChanged(q, logBadge);",
		"streamChangedQueries.clear();",
		"const change = streamQueryChange(q);",
		`class="stream-action-btn${changedClass}"`,
	} {
		if !strings.Contains(app, want) {
			t.Fatalf("js/app.js no longer contains %q", want)
		}
	}
	if strings.Contains(app, "q.action = logBadge") {
		t.Fatal("a live-log decision still rewrites the action a past query received")
	}
	saveAt := strings.Index(app, "async function applyStreamAction")
	markAt := strings.Index(app, "markStreamQueryChanged(q, logBadge);")
	if saveAt < 0 || markAt < saveAt {
		t.Fatal("the changed marker must be written after the rule save is known to succeed")
	}
	css := readAsset(t, "css/style.css")
	for _, want := range []string{
		".stream-action-btn.stream-action-changed .badge",
		"grayscale(1)",
		"opacity: 0.55",
		"text-decoration: line-through",
	} {
		if !strings.Contains(css, want) {
			t.Fatalf("style.css lost %q", want)
		}
	}
}

// The service form must suggest icons for a domain as well as a word. Hiding the
// choices also cancels the debounce and the sequence of any in-flight reply, so
// a slow older search cannot repaint suggestions under a changed field.
func TestServiceIconSuggestionsHandleDomainsAndCancelStaleSearches(t *testing.T) {
	app := jsLineCommentRe.ReplaceAllString(readAsset(t, "js/app.js"), "")
	for _, want := range []string{
		"function serviceIconQuery(raw)",
		"const q = serviceIconQuery(raw);",
		"iconSearchSeq++;",
		"clearTimeout(iconSearchTimer);",
		"hideIconResults();",
		"iconSearchTimer = setTimeout(() => searchServiceIcons(value), 350);",
	} {
		if !strings.Contains(app, want) {
			t.Fatalf("js/app.js no longer contains %q", want)
		}
	}
	html := readAsset(t, "index.html")
	if !strings.Contains(html, "A domain fetches its favicon and suggests library icons") {
		t.Fatal("index.html still says a domain cannot offer icon suggestions")
	}
}
