package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// v1 was removed at its published sunset (1 July 2026). These tests are the fence
// around that decision: every retired path must answer 404 rather than 401 or 200,
// the v2 contract must still carry the same auth and method rules, and the
// unversioned restart route the dashboard calls must still work.
func TestRetiredV1PathsAreGone(t *testing.T) {
	apiInst, _, _, cleanup := setupTestAPI(t)
	defer cleanup()

	mux := http.NewServeMux()
	apiInst.RegisterRoutes(mux)

	// The exact set v1 used to mount. A path that still answers anything other
	// than 404 means a route survived the removal.
	retired := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/v1/version"},
		{http.MethodGet, "/api/v1/status"},
		{http.MethodGet, "/api/v1/clients"},
		{http.MethodPost, "/api/v1/clients"},
		{http.MethodGet, "/api/v1/clients/c4b8e219"},
		{http.MethodPost, "/api/v1/clients/c4b8e219/reset-traffic"},
		{http.MethodGet, "/api/v1/policies"},
		{http.MethodPost, "/api/v1/cache/flush"},
		{http.MethodGet, "/api/v1/api-key"},
		{http.MethodPost, "/api/v1/api-key"},
		{http.MethodGet, "/api/v1/docs"},
	}
	for _, r := range retired {
		req := httptest.NewRequest(r.method, r.path, nil)
		req.Header.Set("X-API-Key", testKey)
		req.RemoteAddr = "127.0.0.1:12345"
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Errorf("%s %s = %d, want 404 (the v1 contract was removed at its sunset)",
				r.method, r.path, w.Code)
		}
	}
}

// The successor has to keep the two properties security reviews relied on for v1:
// the status route rejects anonymous callers, and a wrong verb is refused with the
// verbs it does accept.
func TestV2KeepsTheAuthAndMethodRulesV1Had(t *testing.T) {
	apiInst, _, _, cleanup := setupTestAPI(t)
	defer cleanup()

	mux := http.NewServeMux()
	apiInst.RegisterRoutes(mux)

	anon := httptest.NewRequest(http.MethodGet, "/api/v2/status", nil)
	anon.RemoteAddr = "127.0.0.1:12345"
	anonRec := httptest.NewRecorder()
	mux.ServeHTTP(anonRec, anon)
	if anonRec.Code != http.StatusUnauthorized {
		t.Errorf("anonymous GET /api/v2/status = %d, want 401", anonRec.Code)
	}

	ok := authedRequest(http.MethodGet, "/api/v2/status", nil)
	okRec := httptest.NewRecorder()
	mux.ServeHTTP(okRec, ok)
	if okRec.Code != http.StatusOK {
		t.Fatalf("authenticated GET /api/v2/status = %d — %s", okRec.Code, okRec.Body.String())
	}

	wrongVerb := authedRequest(http.MethodPost, "/api/v2/status", nil)
	wrongRec := httptest.NewRecorder()
	mux.ServeHTTP(wrongRec, wrongVerb)
	if wrongRec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /api/v2/status = %d, want 405", wrongRec.Code)
	}
	if allow := wrongRec.Header().Get("Allow"); !strings.Contains(allow, "GET") {
		t.Errorf("POST /api/v2/status Allow = %q, want it to name GET", allow)
	}
}

// /api/server/restart was never a v1 route; the dashboard's Restart button has
// always called it. It must survive the v1 sweep, still authenticated.
func TestServerRestartStillServesTheDashboard(t *testing.T) {
	apiInst, _, _, cleanup := setupTestAPI(t)
	defer cleanup()

	mux := http.NewServeMux()
	apiInst.RegisterRoutes(mux)

	anon := httptest.NewRequest(http.MethodPost, "/api/server/restart", nil)
	anon.RemoteAddr = "127.0.0.1:12345"
	anonRec := httptest.NewRecorder()
	mux.ServeHTTP(anonRec, anon)
	if anonRec.Code != http.StatusUnauthorized {
		t.Errorf("anonymous POST /api/server/restart = %d, want 401", anonRec.Code)
	}

	ok := authedRequest(http.MethodPost, "/api/server/restart", nil)
	okRec := httptest.NewRecorder()
	mux.ServeHTTP(okRec, ok)
	if okRec.Code != http.StatusOK {
		t.Errorf("authenticated POST /api/server/restart = %d — %s", okRec.Code, okRec.Body.String())
	}
}
