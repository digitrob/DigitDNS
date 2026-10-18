package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"digitdns/internal/database"
)

// The v2.4 URL-source theme CSS never loaded a single byte: the static CSP's
// style-src 'self' blocked the cross-origin <link>. v2.7 widens style-src with
// the configured origin — and only on the portal routes, leaving the dashboard
// and API on the strict static policy.
func TestCSPPortalWidensForThemeURLSource(t *testing.T) {
	base := contentSecurityPolicy

	// No sub-settings: the static policy everywhere.
	ws := &WebServer{}
	if got := ws.cspFor(httptest.NewRequest(http.MethodGet, "/sub/abc", nil)); got != base {
		t.Fatalf("with no settings, portal CSP must be the static policy, got %s", got)
	}

	ws.subSettings = &database.SubscriptionSettings{}
	// Inline source: still the static policy, even on the portal.
	ws.subSettings.Apply(database.SubscriptionSnapshot{
		ThemeCSSSource: "inline", ThemeCSS: "body{color:red}",
	}, func(*database.SubscriptionSettings) error { return nil })
	if got := ws.cspFor(httptest.NewRequest(http.MethodGet, "/sub/abc", nil)); got != base {
		t.Fatalf("inline source must not widen CSP, got %s", got)
	}

	// URL source on the portal: style-src gains exactly the URL's origin.
	ws.subSettings.Apply(database.SubscriptionSnapshot{
		ThemeCSSSource: "url", ThemeCSSURL: "https://cdn.example.com/brand/portal.css? v=2",
	}, func(*database.SubscriptionSettings) error { return nil })
	ws.subSettings.Apply(database.SubscriptionSnapshot{
		ThemeCSSSource: "url", ThemeCSSURL: "https://cdn.example.com/brand/portal.css",
	}, func(*database.SubscriptionSettings) error { return nil })

	portal := ws.cspFor(httptest.NewRequest(http.MethodGet, "/sub/tok123", nil))
	if !strings.Contains(portal, "style-src 'self' 'unsafe-inline' https://cdn.example.com;") {
		t.Fatalf("portal CSP must widen style-src with the configured origin, got %s", portal)
	}
	if strings.Count(portal, "cdn.example.com") != 1 {
		t.Fatalf("portal CSP must mention the origin once, got %s", portal)
	}
	// Dashboard and API: strict.
	for _, path := range []string{"/api/stats", "/dash/", "/"} {
		if got := ws.cspFor(httptest.NewRequest(http.MethodGet, path, nil)); got != base {
			t.Fatalf("non-portal route %s must keep the strict CSP, got %s", path, got)
		}
	}

	// A URL with a path carries only its origin, never the path or query.
	if origin := ws.portalThemeStyleOrigin(); origin != "https://cdn.example.com" {
		t.Fatalf("portalThemeStyleOrigin = %q, want scheme://host only", origin)
	}

	// A non-http(s) or host-less URL never widens anything.
	ws.subSettings.Apply(database.SubscriptionSnapshot{
		ThemeCSSSource: "url", ThemeCSSURL: "javascript:alert(1)",
	}, func(*database.SubscriptionSettings) error { return nil })
	if got := ws.cspFor(httptest.NewRequest(http.MethodGet, "/sub/x", nil)); got != base {
		t.Fatalf("a javascript: URL must not widen the CSP, got %s", got)
	}
}
