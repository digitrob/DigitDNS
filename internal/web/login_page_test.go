package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Phase A: the standalone sign-in document.
//
// Before this page existed, /<admin-path>/dash/login served the full dashboard
// shell — index.html plus app.js plus the chart and icon libraries — to every
// unauthenticated visitor holding the namespace. The standalone document is
// self-contained: no external asset requests, one small inline script, and the
// namespace spliced in at serve time. These tests pin the routing, the rewrite,
// and the self-containment.

// TestLoginDocumentServesUnderTheAdminNamespace: /<p>/login answers 200 with the
// namespace injected and the placeholder gone.
func TestLoginDocumentServesUnderTheAdminNamespace(t *testing.T) {
	ws, _, cleanup := setupTestWebServer(t)
	defer cleanup()
	h := ws.BuildHandler()
	ap := ws.AdminPath()

	req := httptest.NewRequest(http.MethodGet, "/"+ap+"/login", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("GET /%s/login = %d, want 200 — body: %.200s", ap, w.Code, w.Body.String())
	}
	doc := w.Body.String()
	if strings.Contains(doc, adminBasePlaceholder) {
		t.Error("the served login document still carries the namespace placeholder — it was not injected")
	}
	if !strings.Contains(doc, `var ADMIN_BASE = "/`+ap+`"`) {
		t.Errorf("the served login document does not carry the %q namespace in ADMIN_BASE", ap)
	}
	if !strings.Contains(doc, `id="login-form"`) || !strings.Contains(doc, `id="login-code"`) {
		t.Error("the login document is missing the form or the two-factor code field")
	}
}

// TestLoginDocumentIsSelfContained: the standalone page must request nothing —
// no stylesheet, no script, no font from the network. Anything external would
// put admin front-end surface back in front of an unauthenticated visitor.
func TestLoginDocumentIsSelfContained(t *testing.T) {
	ws, _, cleanup := setupTestWebServer(t)
	defer cleanup()
	h := ws.BuildHandler()

	req := httptest.NewRequest(http.MethodGet, "/"+ws.AdminPath()+"/login", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	doc := w.Body.String()

	for _, ref := range []string{`src="`, `href="`, `src='`, `href='`} {
		searchDoc := strings.ReplaceAll(doc, `href="data:image/png;base64,`, `data-favicon="`)
		if i := strings.Index(searchDoc, ref); i >= 0 {
			// The embedded data favicon has no network request. The only other
			// acceptable occurrence is inside a comment; scan the rest of the
			// document from there for another one.
			rest := searchDoc[i+len(ref):]
			if j := strings.Index(rest, ref); j >= 0 || !strings.Contains(searchDoc[:i], "<!--") {
				t.Errorf("the login document references an external resource via %q — it must be fully self-contained", ref)
			}
		}
	}
	if strings.Contains(doc, "app.js") || strings.Contains(doc, "i18n.js") || strings.Contains(doc, "chart.umd") {
		t.Error("the login document pulls in the dashboard bundle — the whole point of the standalone page is that it does not")
	}
}

func TestLoginDocumentHasAccessibleHeadingAndEmbeddedFavicon(t *testing.T) {
	doc := readAsset(t, "login.html")
	if !strings.Contains(doc, `<h1 class="brand-name">DigitDNS</h1>`) {
		t.Error("standalone login has no page-level h1")
	}
	if !strings.Contains(doc, `<link rel="icon" href="data:image/png;base64,`) {
		t.Error("standalone login has no embedded favicon and causes a failing browser request")
	}
}

// TestLoginDocumentsOfferRememberMe: both sign-in surfaces must carry the tick and
// read the same storage key, or a login remembered on the standalone page would
// not come back in the dashboard's own overlay.
func TestLoginDocumentsOfferRememberMe(t *testing.T) {
	for _, name := range []string{"login.html", "index.html"} {
		doc := readAsset(t, name)
		if !strings.Contains(doc, `id="login-remember"`) {
			t.Errorf("%s has no remember-me tick", name)
		}
	}
	for _, name := range []string{"login.html", "js/app.js"} {
		doc := readAsset(t, name)
		if !strings.Contains(doc, "digitdns_remember") {
			t.Errorf("%s does not read the digitdns_remember key", name)
		}
	}
}

// TestRetiredDashLoginRedirects: bookmarks of /<p>/dash/login must land on the
// new page with a redirect that does not leak anything the target does not show.
// TestLoginDocumentStoresCanonicalSessionKey: the standalone page and the dashboard
// must agree on one localStorage key. Writing the legacy hyperdns_token while an older
// digitdns_token was still present made the panel boot with a dead token, answer 401
// and show its login overlay — the "asked me to sign in twice" report.
func TestLoginDocumentStoresCanonicalSessionKey(t *testing.T) {
	doc := readAsset(t, "login.html")
	if !strings.Contains(doc, `localStorage.setItem('digitdns_token'`) {
		t.Error("login.html does not store the new session under digitdns_token, the key app.js reads first")
	}
	if strings.Contains(doc, `localStorage.setItem('hyperdns_token'`) {
		t.Error("login.html still writes the legacy hyperdns_token key")
	}
	if !strings.Contains(doc, `localStorage.removeItem('hyperdns_token'`) {
		t.Error("login.html does not clear the legacy hyperdns_token key after a successful login")
	}
}

func TestRetiredDashLoginRedirects(t *testing.T) {
	ws, _, cleanup := setupTestWebServer(t)
	defer cleanup()
	h := ws.BuildHandler()

	req := httptest.NewRequest(http.MethodGet, "/"+ws.AdminPath()+"/dash/login", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("GET /<p>/dash/login = %d, want 302", w.Code)
	}
	if got := w.Header().Get("Location"); got != "/"+ws.AdminPath()+"/login" {
		t.Errorf("Location = %q, want /%s/login", got, ws.AdminPath())
	}
}

// TestLoginDocumentRewritesWhenThePathRegenerates: the login document must
// follow the namespace regeneration exactly like the dashboard document does.
func TestLoginDocumentRewritesWhenThePathRegenerates(t *testing.T) {
	ws, _, cleanup := setupTestWebServer(t)
	defer cleanup()

	s := ws.staticHandler()
	first := ws.AdminPath()

	w := httptest.NewRecorder()
	if !s.serveLogin(w, httptest.NewRequest(http.MethodGet, "/", nil), first) {
		t.Fatal("serveLogin reported no login document embedded")
	}
	if !strings.Contains(w.Body.String(), `var ADMIN_BASE = "/`+first+`"`) {
		t.Fatalf("the login document did not carry the %q namespace", first)
	}

	second := GenerateAdminPath()
	for second == first {
		second = GenerateAdminPath()
	}
	w = httptest.NewRecorder()
	s.serveLogin(w, httptest.NewRequest(http.MethodGet, "/", nil), second)
	if !strings.Contains(w.Body.String(), `var ADMIN_BASE = "/`+second+`"`) {
		t.Errorf("after regeneration the login document still serves the %q namespace", first)
	}
	if strings.Contains(w.Body.String(), `var ADMIN_BASE = "/`+first+`"`) {
		t.Error("the regenerated login document leaked the retired namespace")
	}
}

// TestLoginDocumentSurvivesAnEmptyAssetTree: a daemon built without the embed
// must answer 404 rather than an empty 200.
func TestLoginDocumentSurvivesAnEmptyAssetTree(t *testing.T) {
	s := newStaticServer(nil)
	w := httptest.NewRecorder()
	if s.serveLogin(w, httptest.NewRequest(http.MethodGet, "/login", nil), "0123456789abcdef") {
		t.Fatal("serveLogin reported success with no assets embedded")
	}
}
