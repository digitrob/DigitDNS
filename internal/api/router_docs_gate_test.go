package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"digitdns/internal/service"
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

// TestDocsAcceptTheDashboardDocumentCookie is the fix for the dashboard's API
// Reference button on a loopback-only api_bind: a browser navigation carries no
// Authorization header, but it does carry the HttpOnly document cookie the
// panel set at login. The docs page is inlined, so this one request is the
// whole flow.
func TestDocsAcceptTheDashboardDocumentCookie(t *testing.T) {
	apiInst, _, settings, cleanup := setupTestAPI(t)
	defer cleanup()
	setAPIBind(t, settings, "127.0.0.1")
	mgr := service.NewSessionManager(time.Hour)
	apiInst.sessions = mgr
	token := mgr.Create()

	mux := http.NewServeMux()
	apiInst.RegisterRoutes(mux)

	for _, path := range []string{"/api/v3/docs", "/api/v2/docs"} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.RemoteAddr = "198.51.100.7:40000"
		req.AddCookie(&http.Cookie{Name: dashboardDocumentCookie, Value: token})
		mux.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("GET %s with a live dashboard cookie answered %d, want 200: %s",
				path, w.Code, w.Body.String())
			continue
		}
		body := w.Body.String()
		if !strings.Contains(body, `id="openapi-spec"`) {
			t.Errorf("GET %s did not inline the spec; the page would need a second, gated fetch", path)
		}
		if !strings.Contains(body, "/js/api-docs.js") {
			t.Errorf("GET %s does not load the external docs bootstrap; the CSP blocks an inline one", path)
		}
	}

	// The cookie is not a master key: an unknown value still falls through to
	// the exposure gate.
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v3/docs", nil)
	req.RemoteAddr = "198.51.100.7:40000"
	req.AddCookie(&http.Cookie{Name: dashboardDocumentCookie, Value: "digitdns_session_bogus"})
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("GET /api/v3/docs with a bogus cookie answered %d, want 403", w.Code)
	}
}

// TestDocsPagesFollowTheDashboardTheme pins the palette handoff: the dashboard
// keeps its theme in localStorage, which the server cannot read, so i18n.js
// mirrors it into the digitdns_theme cookie and the docs pages render in that
// palette instead of a fourth, slightly different dark. ?theme= stays available
// because the refused page has no script to toggle with.
func TestDocsPagesFollowTheDashboardTheme(t *testing.T) {
	apiInst, _, settings, cleanup := setupTestAPI(t)
	defer cleanup()
	setAPIBind(t, settings, "127.0.0.1")

	mux := http.NewServeMux()
	apiInst.RegisterRoutes(mux)

	for _, theme := range []string{"dark", "grey", "light"} {
		// The Swagger shell, reached from the dashboard with the cookie set.
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v3/docs", nil)
		req.RemoteAddr = "127.0.0.1:12345"
		req.AddCookie(&http.Cookie{Name: dashboardThemeCookie, Value: theme})
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("GET /api/v3/docs in %s = %d", theme, w.Code)
		}
		body := w.Body.String()
		if !strings.Contains(body, `data-theme="`+theme+`"`) {
			t.Errorf("docs page in %s does not carry data-theme=%q", theme, theme)
		}
		if theme == "light" && strings.Contains(body, "invert(88%)") {
			t.Error("the light docs page still inverts the vendored Swagger sheet")
		}
		if theme != "light" && !strings.Contains(body, "invert(88%)") {
			t.Errorf("the %s docs page does not invert the vendored light Swagger sheet", theme)
		}
	}

	// The refused page: the query wins over the cookie, so the theme links work
	// without a script.
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v3/docs?theme=light", nil)
	req.RemoteAddr = "198.51.100.7:40000"
	req.AddCookie(&http.Cookie{Name: dashboardThemeCookie, Value: "dark"})
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("GET /api/v3/docs?theme=light from a refused peer = %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, `data-theme="light"`) {
		t.Error("the refused page ignored ?theme=light")
	}
	if !strings.Contains(body, `href="?theme=dark"`) || !strings.Contains(body, `href="?theme=grey"`) {
		t.Error("the refused page does not offer the other two themes")
	}

	// No cookie and no query is the panel's default, not a third palette.
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/v3/docs", nil)
	req.RemoteAddr = "198.51.100.7:40000"
	mux.ServeHTTP(w, req)
	if body := w.Body.String(); !strings.Contains(body, `data-theme="dark"`) {
		t.Error("the refused page does not default to dark")
	}
}
