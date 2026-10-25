package api

import (
	"crypto/subtle"
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"digitdns/internal/core/cache"
	"digitdns/internal/core/matcher"
	"digitdns/internal/core/upstream"
	"digitdns/internal/database"
	"digitdns/internal/httpx"
	"digitdns/internal/netutil"
	"digitdns/internal/service"
)

type API struct {
	db          *database.DB
	clients     *service.ClientService
	stats       *service.StatsService
	cache       *cache.Cache
	matcher     *matcher.Matcher
	upstreams   *upstream.UpstreamPool
	settings    *database.ServerSettings
	tlsSettings *database.TLSSettings
	sessions    *service.SessionManager

	// totpGate is injected by the web server (this package cannot import it)
	// so the state-changing POSTs here face the same second factor the
	// dashboard's equivalents enforce. nil in harnesses: the gate then reads
	// as "2FA not in force", which is exactly what the dashboard's gate does
	// when TOTP is off.
	totpGate func(*http.Request) bool

	// v3Backend carries the dashboard-coupled operations the v3 router exposes
	// without re-implementing them. Injected by the web server via SetV3Backend;
	// nil in bare harnesses, where the backend-dependent routes answer 503.
	v3Backend V3Backend
}

// masterKeyMatches compares a token against the master REST key in constant time.
// It is shared by the v1/v2 middleware and the v3 gate so the two cannot drift
// on what "the master key" means.
func (a *API) masterKeyMatches(token string) bool {
	master := a.settings.GetAPIKey()
	if token == "" || master == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(token), []byte(master)) == 1
}

// bearerToken reads the credential from X-API-Key or an Authorization: Bearer
// header, preferring the explicit key header. The same derivation every gate in
// this package uses.
func bearerToken(r *http.Request) string {
	if k := r.Header.Get("X-API-Key"); k != "" {
		return k
	}
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
	}
	return ""
}

// SetTOTPGate attaches the second-factor predicate for credential-changing
// POSTs. The dashboard enforces it on /api/settings/regenerate-api-key; the
// v1 route does the same work and must not become the unguarded copy of it.
func (a *API) SetTOTPGate(g func(*http.Request) bool) {
	a.totpGate = g
}

func NewAPI(
	db *database.DB,
	clients *service.ClientService,
	stats *service.StatsService,
	cache *cache.Cache,
	matcher *matcher.Matcher,
	upstreams *upstream.UpstreamPool,
	settings *database.ServerSettings,
	tlsSettings *database.TLSSettings,
	sessions *service.SessionManager,
) *API {
	return &API{
		db:          db,
		clients:     clients,
		stats:       stats,
		cache:       cache,
		matcher:     matcher,
		upstreams:   upstreams,
		settings:    settings,
		tlsSettings: tlsSettings,
		sessions:    sessions,
	}
}

// SecurityMiddleware validates APIBind access & API-Key authentication
func (a *API) SecurityMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		// 1. Enforce the APIBind access rule against the immediate peer,
		// deliberately not against netutil.ClientIP: a forwarding header must
		// never carry a request past a listener the operator restricted. The rule
		// is an address or CIDR (netutil.ParseAPIBind) whose default is loopback;
		// loopback is always accepted, so binding the API to e.g. a tailnet block
		// does not also lock out the machine's own scripts and the root-local
		// control plane. v3.1.0: the rule may be a list of such entries, any one of
		// which admits a peer; GetAPIExposure resolves the list-vs-single precedence
		// so the gate and the status endpoints cannot disagree about which is in force.
		if list, single := a.settings.GetAPIExposure(); !apiExposureAllows(list, single, netutil.PeerIP(r)) {
			httpx.WriteJSONError(w, http.StatusForbidden,
				fmt.Sprintf("REST API is not exposed to this address (%s). Set api_bind to 0.0.0.0 or a matching IP/CIDR in Settings to allow remote access.", apiExposureLabel(list, single)))
			return
		}

		// 2. Validate Authentication (X-API-Key, Bearer Token, or live dashboard session)
		apiKey := r.Header.Get("X-API-Key")
		authHeader := r.Header.Get("Authorization")
		if authHeader != "" && strings.HasPrefix(authHeader, "Bearer ") {
			apiKey = strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer "))
		}

		// One read of the master key, not two: the emptiness test and the comparison
		// used to be separate field reads, so a rotation landing between them meant
		// the request was compared against a key different from the one that had just
		// been found non-empty.
		var authorized bool
		if master := a.settings.GetAPIKey(); apiKey != "" && master != "" {
			if subtle.ConstantTimeCompare([]byte(apiKey), []byte(master)) == 1 {
				authorized = true
			}
		}
		if !authorized && a.sessions != nil {
			authorized = a.sessions.Validate(apiKey)
		}
		if !authorized {
			httpx.WriteJSONError(w, http.StatusUnauthorized, "Unauthorized: Invalid or missing API Key")
			return
		}

		next(w, r)
	}
}

// BindGate applies the APIBind exposure rule to the routes that carry no key:
// build metadata (version). When the REST API is bound to localhost, an
// external caller gets the same 403 every authenticated route gives — the
// build version is part of the API surface and does not leak past a localhost
// bind either. The docs pages are gated by BindGateDocs instead, which applies
// the same rule but answers a browser with an explanatory HTML page.
func (a *API) BindGate(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if list, single := a.settings.GetAPIExposure(); !apiExposureAllows(list, single, netutil.PeerIP(r)) {
			httpx.WriteJSONError(w, http.StatusForbidden,
				fmt.Sprintf("REST API is not exposed to this address (%s). Set api_bind to 0.0.0.0 or a matching IP/CIDR in Settings to allow remote access.", apiExposureLabel(list, single)))
			return
		}
		next(w, r)
	}
}

// BindGateDocs is BindGate for the documentation pages. It applies the same
// exposure rule — the docs are NOT public — but answers a rejected browser with
// an HTML page that explains the rule and how to satisfy it, instead of the raw
// {"error":...} JSON. A human who clicked the dashboard's API Reference button
// is the caller here, and a wall of JSON tells them nothing actionable; the API
// data routes keep the JSON 403, because their callers are scripts parsing a
// body, not people reading a page.
func (a *API) BindGateDocs(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		list, single := a.settings.GetAPIExposure()
		if !apiExposureAllows(list, single, netutil.PeerIP(r)) {
			a.writeDocsForbidden(w, apiExposureLabel(list, single))
			return
		}
		next(w, r)
	}
}

// writeDocsForbidden renders the gated-docs page. It is deliberately
// self-contained (inline CSS, no external assets) and lives in this package
// because internal/api cannot import internal/web to reuse the landing page.
func (a *API) writeDocsForbidden(w http.ResponseWriter, exposure string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusForbidden)
	admin := a.settings.GetAdminPath()
	docsPath := "/api/v3/docs"
	if admin != "" {
		docsPath = "/" + admin + "/api/v3/docs"
	}
	_, _ = fmt.Fprintf(w, `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>DigitDNS API Docs — Not Exposed</title>
  <style>
    :root { color-scheme: dark; }
    body { margin: 0; min-height: 100vh; display: flex; align-items: center; justify-content: center;
           background: #0b1020; color: #e6e7eb;
           font-family: system-ui,-apple-system,Segoe UI,Roboto,Helvetica,Arial,sans-serif; }
    .card { max-width: 640px; margin: 2rem; padding: 2rem 2.25rem; background: #121a30;
            border: 1px solid #24304f; border-radius: 16px; }
    h1 { margin: 0 0 .5rem; font-size: 1.25rem; color: #fff; }
    p { line-height: 1.55; color: #c3c8d4; }
    code { background: #0b1020; border: 1px solid #24304f; border-radius: 6px;
           padding: .1rem .35rem; font-size: .9em; color: #a9d2ff; }
    .rule { margin: .75rem 0; padding: .6rem .8rem; background: #0b1020; border-left: 3px solid #f59e0b;
            border-radius: 6px; font-family: ui-monospace,SFMono-Regular,Menlo,monospace; font-size: .85rem; color: #fcd34d; }
    ol { color: #c3c8d4; line-height: 1.7; padding-left: 1.2rem; }
    a { color: #67e8f9; }
  </style>
</head>
<body>
  <div class="card">
    <h1>Documentation is not exposed to this address</h1>
    <p>This DigitDNS instance binds its REST API (and these docs) to specific
      addresses. Your current address is not one of them, so the page is withheld.</p>
    <div class="rule">%s</div>
    <p>To reach the docs, either:</p>
    <ol>
      <li>Add your current IP or its CIDR in <code>Settings &rarr; API Network Binding</code>, then reload this page.</li>
      <li>Connect over a network already permitted by the rule above (for example a VPN/tailnet address), then reload.</li>
      <li>Open an SSH tunnel and browse over loopback, which is always allowed:<br>
        <code>ssh -L 8080:127.0.0.1:&lt;web-port&gt; root@&lt;server&gt;</code><br>
        then open <code>http://127.0.0.1:8080%s</code></li>
    </ol>
    <p style="color:#8b93a7;font-size:.85rem">The API itself is running normally; only this
      documentation page is gated. HTTP 403.</p>
  </div>
</body>
</html>`, exposure, docsPath)
}

// apiExposureAllows applies the resolved exposure rule: a non-empty list is the
// multi-CIDR rule (any entry may admit), and an empty list falls back to the
// single legacy value. A caller passes the two results of
// ServerSettings.GetAPIExposure(), which has already made one of them meaningful.
func apiExposureAllows(list []string, single, peer string) bool {
	if len(list) > 0 {
		return netutil.APIBindListAllowsPeer(list, peer)
	}
	return netutil.APIBindAllowsPeer(single, peer)
}

// apiExposureLabel renders the rule for an error message. A list is joined with
// commas; an empty one falls back to the single value, which renders as before.
func apiExposureLabel(list []string, single string) string {
	if len(list) > 0 {
		return "api_bind_list = " + strings.Join(list, ", ")
	}
	return "api_bind = " + bindLabel(single)
}

// RegisterRoutes attaches the versioned REST API to the HTTP mux.
//
// v1 was retired at its published sunset (1 July 2026): its routes are gone
// rather than deprecated, because a header saying "do not use this" is not what
// an expiry date promised. Everything v1 could do is served by v2 (same
// security middleware, typed DTOs and cursor pagination) and by v3 for
// dashboard parity. /api/server/restart is not versioned — the panel's own
// Restart button calls it — so it stays where it was, without the markers.
func (a *API) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v2/docs", a.BindGateDocs(a.handleDocsV2))
	mux.HandleFunc("/api/v2/openapi.json", a.BindGateDocs(a.handleOpenAPIV2))
	mux.HandleFunc("/api/server/restart", a.SecurityMiddleware(a.handleRestart))

	// v2: the successor contract (RFC 9457 errors, cursor pagination,
	// symmetric DTOs, strict decoding).
	a.RegisterRoutesV2(mux)

	// v3: full dashboard parity, with login (user/password + TOTP) sessions as
	// well as the master REST key. The dashboard-coupled routes delegate to the
	// web backend injected via SetV3Backend.
	a.RegisterRoutesV3(mux)
}

func (a *API) handleRestart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.WriteMethodNotAllowed(w, "POST")
		return
	}
	// Restart endpoint flushes the cache. A full daemon restart with listener
	// teardown/rebuild requires process control outside the API's scope.
	// For now, this provides cache invalidation which is the primary operator need.
	if a.cache != nil {
		a.cache.Flush()
	}
	_ = json.NewEncoder(w).Encode(map[string]bool{"restarted": true})
}

func (a *API) handleDocsV2(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		httpx.WriteMethodNotAllowed(w, "GET, HEAD")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	admin := adminServerURL(a.settings.GetAdminPath())
	html := fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>DigitDNS API v2 Docs</title>
  <link rel="stylesheet" type="text/css" href="%s/swagger/swagger-ui.css">
  <style>
    body { margin: 0; background: #0b1020; color: #e6e7eb; font-family: system-ui,-apple-system,Segoe UI,Roboto,Helvetica,Arial,sans-serif; }
    .swagger-ui { filter: invert(88%%) hue-rotate(180deg); }
  </style>
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="%s/swagger/swagger-ui-bundle.js"></script>
  <script>
    var viewerLoaded = typeof SwaggerUIBundle === 'function';
    if (viewerLoaded) SwaggerUIBundle({ dom_id: '#swagger-ui', url: '%s/api/v2/openapi.json', deepLinking: true, presets: [SwaggerUIBundle.presets.apis, SwaggerUIBundle.presets.operations] });
  </script>
</body>
</html>`, admin+"/swagger/swagger-ui.css", admin+"/swagger/swagger-ui-bundle.js", admin)
	_, _ = w.Write([]byte(html))
}

func (a *API) handleDocsV3(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		httpx.WriteMethodNotAllowed(w, "GET, HEAD")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	admin := adminServerURL(a.settings.GetAdminPath())
	html := fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>DigitDNS API v3 Docs</title>
  <link rel="stylesheet" type="text/css" href="%s/swagger/swagger-ui.css">
  <style>
    body { margin: 0; background: #0b1020; color: #e6e7eb; font-family: system-ui,-apple-system,Segoe UI,Roboto,Helvetica,Arial,sans-serif; }
    .swagger-ui { filter: invert(88%%) hue-rotate(180deg); }
  </style>
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="%s/swagger/swagger-ui-bundle.js"></script>
  <script>
    var viewerLoaded = typeof SwaggerUIBundle === 'function';
    if (viewerLoaded) SwaggerUIBundle({ dom_id: '#swagger-ui', url: '%s/api/v3/openapi.json', deepLinking: true, presets: [SwaggerUIBundle.presets.apis, SwaggerUIBundle.presets.operations] });
  </script>
</body>
</html>`, admin+"/swagger/swagger-ui.css", admin+"/swagger/swagger-ui-bundle.js", admin)
	_, _ = w.Write([]byte(html))
}

//go:embed openapi/v2.json
var openAPIV2Spec []byte

//go:embed openapi/v3.json
var openAPIV3Spec []byte

func (a *API) handleOpenAPIV3(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		httpx.WriteMethodNotAllowed(w, "GET, HEAD")
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	_, _ = w.Write(openAPIV3Spec)
}

func (a *API) handleOpenAPIV2(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		httpx.WriteMethodNotAllowed(w, "GET, HEAD")
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	_, _ = w.Write(openAPIV2Spec)
}

// adminServerURL builds the OpenAPI "servers" entry from the install's hidden
// admin path. The spec document is served below the same namespace it
// describes, so an absolute path is what Try it out needs; a relative "/"
// would resolve every request against the host root, where nothing has been
// mounted since v2.1. A record with no path (never migrated) degrades to the
// pre-v2.1 root, which is the honest answer for that build state.
func adminServerURL(adminPath string) string {
	if adminPath == "" {
		return "/"
	}
	return "/" + adminPath
}

func bindLabel(bind string) string {
	b := strings.TrimSpace(bind)
	if b == "" {
		return "127.0.0.1 (default)"
	}
	return b
}
