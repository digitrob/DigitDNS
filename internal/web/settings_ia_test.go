package web

import (
	"regexp"
	"strings"
	"testing"
)

// sectionSegment returns the slice of index.html that belongs to one top-level
// tab. The document is hand-assembled and every tab is a sibling <section> at
// the same indentation, so the next one is a reliable end marker.
func sectionSegment(t *testing.T, doc, id string) string {
	t.Helper()
	start := strings.Index(doc, `id="`+id+`"`)
	if start < 0 {
		t.Fatalf("index.html has no section id=%q", id)
	}
	rest := doc[start:]
	end := len(rest)
	for _, marker := range []string{"\n      <section id=\"tab-", "\n      <footer", "\n      <!-- FOOTER"} {
		if i := strings.Index(rest[1:], marker); i >= 0 {
			if i+1 < end {
				end = i + 1
			}
		}
	}
	return rest[:end]
}

// TestResolverCardsLiveOnResolversTab pins the IA split. Without this, a card
// can be copied instead of moved and every other gate still passes: the ids
// resolve, both tabs render, and the duplicate is only visible as two forms
// writing the same setting. The five cards are the contract between the
// Resolvers tab and the loaders in app.js.
func TestResolverCardsLiveOnResolversTab(t *testing.T) {
	doc := readAsset(t, "index.html")
	resolvers := sectionSegment(t, doc, "tab-resolvers")
	settings := sectionSegment(t, doc, "tab-rules")

	for _, id := range []string{
		"transports-card",
		"doh-tokens-card",
		"dnssec-card",
		"proxy-leak-card",
		"proxy-ipv6-card",
	} {
		if got := strings.Count(doc, `id="`+id+`"`); got != 1 {
			t.Errorf("%s appears %d time(s) in index.html, want exactly 1", id, got)
		}
		if !strings.Contains(resolvers, `id="`+id+`"`) {
			t.Errorf("%s is not inside #tab-resolvers", id)
		}
		if strings.Contains(settings, `id="`+id+`"`) {
			t.Errorf("%s is still inside #tab-rules — the card was copied, not moved", id)
		}
	}

	for _, id := range []string{
		"server-location-card", "twofa-card", "server-update-card",
		"admin-path-card", "admin-credentials-card", "subscription-card",
		"tls-card", "blocked-card", "dns-rewrites-card", "custom-groups-card",
		"stats-settings-card", "alerts-card", "about-card",
	} {
		if !strings.Contains(settings, `id="`+id+`"`) {
			t.Errorf("%s is missing from #tab-rules", id)
		}
	}
}

// TestSettingsJumpTargetsExist is the guard for both jump bars and for the
// command palette, which builds its Settings/Resolvers entries from the same
// attribute. A chip that names a removed id is a dead control.
func TestSettingsJumpTargetsExist(t *testing.T) {
	doc := readAsset(t, "index.html")
	re := regexp.MustCompile(`data-settings-jump="([^"]+)"`)
	seen := 0
	for _, m := range re.FindAllStringSubmatch(doc, -1) {
		target := m[1]
		if target == "top" {
			continue
		}
		seen++
		if !strings.Contains(doc, `id="`+target+`"`) {
			t.Errorf("data-settings-jump=%q has no matching id in index.html", target)
		}
	}
	if seen == 0 {
		t.Fatal("index.html has no settings jump targets — the scan is broken")
	}
}

// TestSwitchTabLiteralsHavePanels catches the class of bug that made the
// transports notice blank the page: switchTab('settings') named a tab id that
// does not exist, so every .tab-content was hidden and nothing was shown.
func TestSwitchTabLiteralsHavePanels(t *testing.T) {
	app := readAsset(t, "js/app.js")
	html := readAsset(t, "index.html")
	re := regexp.MustCompile(`switchTab\('([a-z0-9-]+)'`)
	for _, m := range re.FindAllStringSubmatch(app, -1) {
		tab := m[1]
		if !strings.Contains(html, `id="tab-`+tab+`"`) {
			t.Errorf("switchTab(%q) has no #tab-%s panel", tab, tab)
		}
	}
}
