package web

import (
	"strings"
	"testing"
)

// The dirty note is advisory inside the SPA, but beforeunload is a real browser
// prompt. An auth transition must clear it before any reload that follows a
// successful login, otherwise Firefox shows "Leave site?" at the moment the
// operator signs in. These scans pin that ordering in the real asset, not in a
// copy of the logic that could drift away from it.
func TestAuthTransitionsClearSettingsDirty(t *testing.T) {
	app := jsLineCommentRe.ReplaceAllString(readAsset(t, "js/app.js"), "")

	login := strings.Index(app, "fetch(api('/api/auth/login')")
	if login < 0 {
		t.Fatal("js/app.js has no /api/auth/login fetch — the scan is broken")
	}
	reload := strings.Index(app[login:], "window.location.reload();")
	if reload < 0 {
		t.Fatal("login success no longer reloads the dashboard — the scan is broken")
	}
	reload += login
	beforeReload := app[login:reload]
	if !strings.Contains(beforeReload, "clearSettingsDirty();") {
		t.Fatal("successful login reloads without clearing the settings dirty state")
	}

	signOut := strings.Index(app, "async function signOut()")
	if signOut < 0 {
		t.Fatal("js/app.js has no signOut() — the scan is broken")
	}
	modal := strings.Index(app[signOut:], "showLoginModal();")
	if modal < 0 {
		t.Fatal("signOut no longer returns to the login modal — the scan is broken")
	}
	modal += signOut
	beforeModal := app[signOut:modal]
	if !strings.Contains(beforeModal, "clearSettingsDirty();") {
		t.Fatal("sign-out leaves stale dirty state behind for the next navigation")
	}

	if !strings.Contains(app, "if (!settingsDirty.size) return;") {
		t.Fatal("beforeunload guard was removed; genuine unsaved edits must still be protected")
	}
}
