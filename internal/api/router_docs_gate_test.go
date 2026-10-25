package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// gatedDocsPaths is every documentation-style route. They are not public: the
// same APIBind exposure rule that guards the data routes applies to them, and
// the OpenAPI specs are on this list deliberately — a remote caller who is not
// allowed to reach the API must not read its spec either.
var gatedDocsPaths = []string{
	"/api/v2/docs",
	"/api/v2/openapi.json",
	"/api/v3/docs",
	"/api/v3/openapi.json",
}

// TestDocsAreGatedButExplainThemselves is the v3.2.2 contract: docs stay behind
// the exposure rule (they are not made public), but a browser that is refused
// gets an HTML page naming the rule and how to satisfy it, not the bare JSON an
// API client would parse. The data routes keep the JSON body, because their
// callers are scripts.
func TestDocsAreGatedButExplainThemselves(t *testing.T) {
	apiInst, _, settings, cleanup := setupTestAPI(t)
	defer cleanup()
	setAPIBind(t, settings, "127.0.0.1")

	mux := http.NewServeMux()
	apiInst.RegisterRoutes(mux)

	for _, path := range gatedDocsPaths {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.RemoteAddr = "198.51.100.7:40000"
		mux.ServeHTTP(w, req)

		if w.Code != http.StatusForbidden {
			t.Errorf("GET %s from a non-loopback peer answered %d, want 403 — the docs "+
				"route has lost its exposure gate.", path, w.Code)
			continue
		}
		if got := w.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/html") {
			t.Errorf("GET %s answered Content-Type %q, want text/html — the refused browser "+
				"should get the explanatory page, not a JSON body.", path, got)
		}
		body := w.Body.String()
		if !strings.Contains(body, "api_bind") {
			t.Errorf("GET %s refused without naming the rule in force; the page cannot tell "+
				"the operator what to change. Body: %s", path, body)
		}
		if !strings.Contains(body, "403") {
			t.Errorf("GET %s refused without stating the status; the page should say 403. Body: %s", path, body)
		}
	}
}

// TestDataRoutesStillAnswerJSONWhenRefused pins the asymmetry: the friendly HTML
// page is for the docs only. A refused data route must keep its JSON body, or a
// script parsing the error loses the shape it relies on.
func TestDataRoutesStillAnswerJSONWhenRefused(t *testing.T) {
	apiInst, _, settings, cleanup := setupTestAPI(t)
	defer cleanup()
	setAPIBind(t, settings, "127.0.0.1")

	mux := http.NewServeMux()
	apiInst.RegisterRoutes(mux)

	for _, path := range []string{"/api/v2/version", "/api/v2/status"} {
		w := httptest.NewRecorder()
		req := authedRequest(http.MethodGet, path, nil)
		req.RemoteAddr = "198.51.100.7:40000"
		mux.ServeHTTP(w, req)

		if w.Code != http.StatusForbidden {
			t.Errorf("GET %s from a non-loopback peer answered %d, want 403", path, w.Code)
			continue
		}
		if got := w.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
			t.Errorf("GET %s answered Content-Type %q, want application/json — a data route "+
				"must not switch to the docs HTML page.", path, got)
		}
	}
}

// TestDocsReachableFromAllowedPeer is the other half: when the peer is allowed,
// the gate lets the documentation through unchanged. Without this, a gate that
// refuses everyone would pass the test above.
func TestDocsReachableFromAllowedPeer(t *testing.T) {
	apiInst, _, settings, cleanup := setupTestAPI(t)
	defer cleanup()
	setAPIBind(t, settings, "127.0.0.1")

	mux := http.NewServeMux()
	apiInst.RegisterRoutes(mux)

	for _, path := range gatedDocsPaths {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.RemoteAddr = "127.0.0.1:12345"
		mux.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("GET %s from loopback answered %d, want 200: %s", path, w.Code, w.Body.String())
		}
	}
}
