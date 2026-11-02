package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The container healthcheck points at a path that must survive an admin-path
// rotation. It used to hit /<admin>/api/v2/version, so regenerating the hidden
// path marked a perfectly healthy container unhealthy - the check and the thing
// it checked had drifted apart.
func TestHealthzIsPublicAndContentless(t *testing.T) {
	ws, _, cleanup := setupTestWebServer(t)
	defer cleanup()

	h := ws.BuildHandler()

	for _, method := range []string{http.MethodGet, http.MethodHead} {
		req := httptest.NewRequest(method, "/healthz", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("%s /healthz = %d, want 200", method, rec.Code)
		}
		if method == http.MethodGet {
			if got := strings.TrimSpace(rec.Body.String()); got != `{"status":"ok"}` {
				t.Errorf("body = %q, want the fixed status object", got)
			}
			// Nothing that changes per install, nothing that identifies one.
			adminPath := ws.AdminPath()
			if adminPath != "" && strings.Contains(rec.Body.String(), adminPath) {
				t.Error("the health body carries the admin path")
			}
		}
	}

	// A write verb is refused rather than silently served.
	req := httptest.NewRequest(http.MethodPost, "/healthz", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /healthz = %d, want 405", rec.Code)
	}
	if rec.Header().Get("Allow") != "GET, HEAD" {
		t.Errorf("Allow = %q, want \"GET, HEAD\"", rec.Header().Get("Allow"))
	}
}

// It must be reachable without a session: the probe runs inside the container
// with no cookie and no token.
func TestHealthzNeedsNoSession(t *testing.T) {
	ws, _, cleanup := setupTestWebServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	ws.BuildHandler().ServeHTTP(rec, req)

	if rec.Code == http.StatusUnauthorized {
		t.Fatal("/healthz requires a session - the container probe cannot authenticate")
	}
}
