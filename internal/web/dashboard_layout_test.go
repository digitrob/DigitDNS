package web

import (
	"net/http"
	"testing"
)

// The arrangement is install-wide: saving it must come back in /api/config for
// every browser, and a bad document must be refused rather than stored.
func TestDashboardLayoutRoundTrips(t *testing.T) {
	_, h, tok, cleanup := authedServer(t)
	defer cleanup()

	body := `{"layout":{"order":{"metrics":["qps","cache"]},"cards":{"qps":{"w":3,"h":2}}}}`
	if w := postJSON(t, h, "/api/dashboard/layout", body, tok); w.Code != http.StatusOK {
		t.Fatalf("save = %d — %s", w.Code, w.Body.String())
	}

	cfg := decodeBody(t, authedGet(t, h, "/api/config", tok))
	layout, ok := cfg["dashboard_layout"].(map[string]any)
	if !ok {
		t.Fatalf("config.dashboard_layout = %#v", cfg["dashboard_layout"])
	}
	cards, _ := layout["cards"].(map[string]any)
	qps, _ := cards["qps"].(map[string]any)
	if qps["w"] != float64(3) || qps["h"] != float64(2) {
		t.Errorf("qps layout = %#v, want w=3 h=2", cards["qps"])
	}
}

func TestDashboardLayoutRejectsInvalidJSON(t *testing.T) {
	_, h, tok, cleanup := authedServer(t)
	defer cleanup()

	if w := postJSON(t, h, "/api/dashboard/layout", `{"layout":`, tok); w.Code != http.StatusBadRequest {
		t.Fatalf("invalid layout = %d, want 400", w.Code)
	}
}

func TestDashboardLayoutRejectsGET(t *testing.T) {
	_, h, tok, cleanup := authedServer(t)
	defer cleanup()

	if w := authedGet(t, h, "/api/dashboard/layout", tok); w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET = %d, want 405", w.Code)
	}
}
