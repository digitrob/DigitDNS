package web

import (
	"strings"
	"testing"
)

// An expired session used to flash "Session expired" on the dashboard during
// login. Two things combined: the history loader wrote the 401 into its card,
// and the login success path hid the overlay before reloading, so the stale
// card was visible for a frame. These pin the three places that now keep the
// re-auth silent.
func TestExpiredSessionDoesNotFlashDuringLogin(t *testing.T) {
	app := readAsset(t, "js/app.js")

	if strings.Contains(app, "hideLoginModal") {
		t.Error("the login overlay is hidden before the reload again. The dashboard " +
			"underneath may still hold the previous session's error text, which is the " +
			"expired-session flash operators saw for a frame.")
	}

	historyFetch := functionBody(t, app, "historyFetch")
	if !strings.Contains(historyFetch, "err.status = 401") {
		t.Error("historyFetch no longer marks a 401, so loadHistory cannot distinguish " +
			"an expired session from a real history failure.")
	}

	loadHistory := functionBody(t, app, "loadHistory")
	if !strings.Contains(loadHistory, "e.status === 401") {
		t.Error("loadHistory paints an error for an expired session. That line sits " +
			"behind the login gate and is revealed when the gate closes.")
	}

	toast := functionBody(t, app, "showToast")
	if !strings.Contains(toast, "session (?:has )?expired") {
		t.Error("showToast no longer intercepts the auth-gate message, so a stray 401 " +
			"toast can flash over the login overlay.")
	}
}
