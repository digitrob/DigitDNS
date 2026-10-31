package web

import (
	"strings"
	"testing"
)

// TestDomainsRemovalReachesEveryList pins the fix for the row button that did
// nothing on blocked and direct domains.
//
// The Domains table is one list with four statuses, but the Remove handler kept
// editing only custom_proxied and custom_proxied_disabled — the shape from when
// the table showed proxied domains alone. A blocked or direct row was in neither
// array, so the click left it on screen. The handler now goes through the shared
// helper, which also clears the hidden wildcard twin and the "Added" stamp.
func TestDomainsRemovalReachesEveryList(t *testing.T) {
	app := readAsset(t, "js/app.js")

	body := app[strings.Index(app, `btn.classList.contains('remove-proxied')`):]
	if i := strings.Index(body, `btn.classList.contains('proxied-pause-btn')`); i > 0 {
		body = body[:i]
	}
	if len(body) < 50 {
		t.Fatal("could not slice the remove-proxied branch out of js/app.js — the scan is broken")
	}
	if !strings.Contains(body, "removeDomainEverywhere(") {
		t.Error("the Domains row's Remove button no longer uses removeDomainEverywhere; " +
			"editing only the proxied lists leaves blocked and direct rows on screen")
	}

	helper := app[strings.Index(app, "function removeDomainEverywhere"):]
	helper = helper[:strings.Index(helper, "// proxiedFilterNeedle")]
	if !strings.Contains(helper, "DOMAIN_RULE_LISTS") {
		t.Error("removeDomainEverywhere does not walk DOMAIN_RULE_LISTS")
	}
	lists := app[strings.Index(app, "const DOMAIN_RULE_LISTS"):]
	lists = lists[:strings.Index(lists, "\n")]
	for _, list := range []string{"custom_proxied", "custom_proxied_disabled", "custom_blocked", "custom_direct"} {
		if !strings.Contains(lists, list) {
			t.Errorf("DOMAIN_RULE_LISTS is missing %s", list)
		}
	}
	if !strings.Contains(helper, "custom_domain_added") {
		t.Error("removeDomainEverywhere leaves the domain's Added stamp behind")
	}
}

// TestQuickActionMenuHasNoRemoveAction: the popup is a routing shortcut only.
// Removing a managed domain belongs to the Domains table's own row button and the
// bulk bar, so the shared menu carries no destructive entry on either surface.
func TestQuickActionMenuHasNoRemoveAction(t *testing.T) {
	doc := readAsset(t, "index.html")
	menu := doc[strings.Index(doc, `id="stream-action-menu"`):]
	menu = menu[:strings.Index(menu, "</div>\n\n    <!-- MOBILE STICKY")]
	if strings.Contains(menu, `data-action="remove"`) {
		t.Error("the quick-action menu still offers Remove; removal stays on the Domains row and bulk bar")
	}

	app := readAsset(t, "js/app.js")
	if strings.Contains(app, `action === 'remove'`) {
		t.Error("applyStreamAction still carries a Remove branch for a menu item that no longer exists")
	}
	if strings.Contains(app, `[data-action="remove"]`) {
		t.Error("openStreamActionMenu still looks for the removed menu item")
	}
}
