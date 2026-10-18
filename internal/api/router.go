package api

import (
	"crypto/subtle"
	_ "embed"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"digitdns/internal/core/cache"
	"digitdns/internal/core/matcher"
	"digitdns/internal/core/upstream"
	"digitdns/internal/crypto"
	"digitdns/internal/database"
	"digitdns/internal/httpx"
	"digitdns/internal/netutil"
	"digitdns/internal/service"
	"digitdns/internal/version"
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
		// control plane.
		if bind := a.settings.GetAPIBind(); !netutil.APIBindAllowsPeer(bind, netutil.PeerIP(r)) {
			httpx.WriteJSONError(w, http.StatusForbidden,
				fmt.Sprintf("REST API is not exposed to this address (api_bind = %s). Set it to 0.0.0.0 or a matching IP/CIDR in Settings to allow remote access.", bindLabel(bind)))
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

// BindGate applies the APIBind exposure rule to the routes that carry no
// key: build metadata and the docs page. When the REST API is bound to
// localhost, an external caller gets the same 403 every authenticated route
// gives — the endpoint map and the build version are part of the API surface
// and do not leak past a localhost bind either. Without this gate the docs
// page (and only it) answered remote browsers on an install whose operator
// believed the whole REST surface was loopback-only.
func (a *API) BindGate(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if bind := a.settings.GetAPIBind(); !netutil.APIBindAllowsPeer(bind, netutil.PeerIP(r)) {
			httpx.WriteJSONError(w, http.StatusForbidden,
				fmt.Sprintf("REST API is not exposed to this address (api_bind = %s). Set it to 0.0.0.0 or a matching IP/CIDR in Settings to allow remote access.", bindLabel(bind)))
			return
		}
		next(w, r)
	}
}

// RegisterRoutes attaches all /api/v1/ endpoints to the HTTP mux. Every
// authenticated route carries the v1 deprecation markers (v2 is the
// successor contract; the versioned transition keeps v1 working — with the
// same security fixes — until its sunset).
func (a *API) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/version", a.DeprecateV1(a.BindGate(a.handleVersion)))
	mux.HandleFunc("/api/v1/status", a.DeprecateV1(a.SecurityMiddleware(a.handleStatus)))
	mux.HandleFunc("/api/v1/clients", a.DeprecateV1(a.SecurityMiddleware(a.handleClients)))
	mux.HandleFunc("/api/v1/clients/", a.DeprecateV1(a.SecurityMiddleware(a.handleClientAction)))
	mux.HandleFunc("/api/v1/policies", a.DeprecateV1(a.SecurityMiddleware(a.handlePolicies)))
	mux.HandleFunc("/api/v1/cache/flush", a.DeprecateV1(a.SecurityMiddleware(a.handleFlushCache)))
	mux.HandleFunc("/api/v1/api-key", a.DeprecateV1(a.SecurityMiddleware(a.handleAPIKey)))
	mux.HandleFunc("/api/v1/docs", a.DeprecateV1(a.BindGate(a.handleDocs)))
	mux.HandleFunc("/api/v2/docs", a.BindGate(a.handleDocsV2))
	mux.HandleFunc("/api/v2/openapi.json", a.BindGate(a.handleOpenAPIV2))
	mux.HandleFunc("/api/server/restart", a.DeprecateV1(a.SecurityMiddleware(a.handleRestart)))

	// v2: the successor contract (RFC 9457 errors, cursor pagination,
	// symmetric DTOs, strict decoding).
	a.RegisterRoutesV2(mux)
}

// handleVersion is intentionally unauthenticated: build metadata is not sensitive.
//
// GET and HEAD only, like every other read-only endpoint here. This one and handleStatus
// were the two the method-guard pass missed, and both answered a PUT or a DELETE with 200
// and a body — a caller using the wrong verb was told it had worked.
func (a *API) handleVersion(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		httpx.WriteMethodNotAllowed(w, "GET, HEAD")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(version.Get())
}

func (a *API) handleStatus(w http.ResponseWriter, r *http.Request) {
	// SecurityMiddleware has already answered OPTIONS by the time this runs, so this is
	// only ever a real request with a verb this endpoint has no meaning for.
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		httpx.WriteMethodNotAllowed(w, "GET, HEAD")
		return
	}
	stats := a.stats.GetLiveStats()
	publicIP, apiBind := a.settings.Endpoint()
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":    "healthy",
		"version":   version.Get(),
		"timestamp": time.Now(),
		"telemetry": stats,
		"public_ip": publicIP,
		"api_bind":  apiBind,
	})
}

func (a *API) handleClients(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	// HEAD runs the read path and net/http discards the body, so it costs nothing and
	// answers the uptime probes that send it. RFC 9110 §9.1 expects any resource serving
	// GET to serve HEAD; every readable endpoint in this file now does.
	case http.MethodGet, http.MethodHead:
		// Views, not stored records: an integration reading this list should not have
		// to reimplement the quota rule or the clamped-month arithmetic to know who is
		// cut off and when their allowance returns. Every field of the stored record is
		// still present and in the same place — the struct is embedded, so it flattens.
		clients, err := a.clients.ListClientViews()
		if err != nil {
			httpx.WriteJSONErrorFor(w, http.StatusInternalServerError, err)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"clients": clients})

	case http.MethodPost:
		var req struct {
			Name           string   `json:"name"`
			Days           int      `json:"days"`
			IP             string   `json:"ip"`
			TrafficLimitGB float64  `json:"traffic_limit_gb"`
			CustomPolicies []string `json:"custom_policies"`
			Note           string   `json:"note"`

			// "", "daily", "weekly" or "monthly": the period after which the quota
			// above comes back on its own. Omitting it keeps the pre-v1.5.0 behaviour,
			// where a spent limit stays spent until an operator resets it by hand.
			TrafficResetCycle string `json:"traffic_reset_cycle"`
		}
		// Bounded like the PUT below. An authenticated caller is still a caller: an
		// unbounded decode lets one request name a client whose length is whatever the
		// sender feels like sending, and the daemon holds all of it.
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1MB limit
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
			httpx.WriteJSONError(w, http.StatusBadRequest, "Invalid request payload")
			return
		}
		// One write, so nothing can half-succeed. This used to create the account and
		// then update it to attach the quota, the cycle, the note and the policies, with
		// the second error swallowed so the request still answered 201 — an integration
		// that provisioned a 50 GB monthly plan and lost that write got back a live
		// account with no limit at all. ProvisionClient also refuses an unsupported
		// cycle, and refuses it before anything is stored.
		client, err := a.clients.ProvisionClient(service.CreateClientRequest{
			Name: req.Name,
			Days: req.Days,
			IP:   req.IP,

			TrafficLimitGB:    req.TrafficLimitGB,
			TrafficResetCycle: req.TrafficResetCycle,

			Note:           req.Note,
			CustomPolicies: req.CustomPolicies,
		})
		if err != nil {
			httpx.WriteClientError(w, err)
			return
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(a.clients.ViewClient(client))

	default:
		httpx.WriteMethodNotAllowed(w, "GET, HEAD, POST")
	}
}

func (a *API) handleClientAction(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/v1/clients/")
	id = strings.TrimSpace(strings.TrimSuffix(id, "/"))

	// Both sub-actions mutate state and are documented as POST, so anything else
	// is rejected rather than silently treated as a POST.
	if clientID, ok := strings.CutSuffix(id, "/regenerate-uuid"); ok {
		if r.Method != http.MethodPost {
			httpx.WriteMethodNotAllowed(w, "POST")
			return
		}
		newUUID, err := a.clients.RegenerateUUID(clientID)
		if err != nil {
			// Not a blanket 404: this reached the database, and a bbolt write failure
			// answering "no such client" sends an integration hunting for a record
			// that is sitting right there in the list.
			httpx.WriteClientError(w, err)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"uuid": newUUID})
		return
	}

	if clientID, ok := strings.CutSuffix(id, "/reset-traffic"); ok {
		if r.Method != http.MethodPost {
			httpx.WriteMethodNotAllowed(w, "POST")
			return
		}
		if err := a.clients.ResetClientTraffic(clientID); err != nil {
			httpx.WriteClientError(w, err)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]bool{"reset": true})
		return
	}

	switch r.Method {
	case http.MethodGet, http.MethodHead:
		client, err := a.clients.GetClient(id)
		if err != nil {
			httpx.WriteJSONError(w, http.StatusNotFound, "Client not found")
			return
		}
		// Decorated with the live usage total, so a billing integration polling this
		// route sees what the resolver sees rather than what the last flush wrote.
		_ = json.NewEncoder(w).Encode(a.clients.ViewClient(client))

	case http.MethodPut:
		var req service.UpdateClientRequest
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1MB limit
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpx.WriteJSONError(w, http.StatusBadRequest, "Invalid JSON payload")
			return
		}
		updated, err := a.clients.UpdateClient(id, req)
		if err != nil {
			httpx.WriteClientError(w, err)
			return
		}
		// Changing the cycle moves the next reset, so the response carries the new
		// boundary rather than leaving the caller to derive it from the name it sent.
		_ = json.NewEncoder(w).Encode(a.clients.ViewClient(updated))

	case http.MethodDelete:
		if err := a.clients.DeleteClient(id); err != nil {
			httpx.WriteClientError(w, err)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]bool{"deleted": true})

	default:
		httpx.WriteMethodNotAllowed(w, "GET, HEAD, PUT, DELETE")
	}
}

func (a *API) handlePolicies(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet, http.MethodHead:
		policies, err := a.db.ListPolicies()
		if err != nil {
			httpx.WriteJSONErrorFor(w, http.StatusInternalServerError, err)
			return
		}
		// The catalogue was described as "every gaming and streaming preset". This
		// build ships no presets, so the list is well-formed but empty; the route
		// stays for integrations that read it.
		_ = json.NewEncoder(w).Encode(map[string]any{
			"policies": policies,
			"catalog":  []any{},
		})

	case http.MethodPost:
		var p database.Policy
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1MB limit
		if err := json.NewDecoder(r.Body).Decode(&p); err != nil || p.Key == "" {
			httpx.WriteJSONError(w, http.StatusBadRequest, "Invalid request payload")
			return
		}
		if err := a.db.SavePolicy(p); err != nil {
			httpx.WriteJSONErrorFor(w, http.StatusInternalServerError, err)
			return
		}
		_ = json.NewEncoder(w).Encode(p)

	default:
		httpx.WriteMethodNotAllowed(w, "GET, HEAD, POST")
	}
}

func (a *API) handleFlushCache(w http.ResponseWriter, r *http.Request) {
	// POST-only, as API.md and the OpenAPI spec below both document it. It used to
	// accept any method, so a GET emptied the resolver's cache — and a GET is what a
	// link preview, a prefetcher or a browser's address-bar completion will issue on
	// its own. Every cached answer being dropped is not fatal, but it is a latency
	// spike on the next query for every subscriber at once, which on a resolver sold
	// for its ping is the whole product misbehaving.
	if r.Method != http.MethodPost {
		httpx.WriteMethodNotAllowed(w, "POST")
		return
	}
	if a.cache != nil {
		a.cache.Flush()
	}
	_ = json.NewEncoder(w).Encode(map[string]bool{"flushed": true})
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

func (a *API) handleAPIKey(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	switch r.Method {
	case http.MethodGet, http.MethodHead:
		key := a.settings.GetAPIKey()
		bind := a.settings.GetAPIBind()
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success":  true,
			"api_key":  key,
			"api_bind": bind,
			"data": map[string]string{
				"api_key":  key,
				"api_bind": bind,
			},
		})
	case http.MethodPost:
		// Regenerate Master API Key. The second factor first, exactly as the
		// dashboard's rotate endpoint enforces it: a live session is accepted
		// by SecurityMiddleware, so without this gate a hijacked session could
		// rotate the key with two-factor on — the one credential change the
		// dashboard refuses to do unguarded.
		if a.totpGate != nil && !a.totpGate(r) {
			httpx.WriteJSONError(w, http.StatusUnauthorized, "Invalid credentials")
			return
		}
		newKey := crypto.GenerateAPIKey()

		// The persist error used to be discarded, so a failed write answered with the
		// new key while the database kept the old one: every integration the operator
		// then reconfigured worked until the next restart and failed together after
		// it. UpdateAndPersist rolls the in-memory value back instead.
		if err := a.settings.UpdateAndPersist(
			func(m *database.MutableSettings) { m.APIKey = newKey },
			func(s *database.ServerSettings) error { return a.db.SetSetting("server", s) },
		); err != nil {
			log.Printf("[API] Could not persist the regenerated API key: %v", err)
			httpx.WriteJSONError(w, http.StatusInternalServerError, "Could not save the new API key")
			return
		}
		log.Printf("[API] REST API key regenerated")

		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"api_key": newKey,
			"data": map[string]string{
				"api_key": newKey,
			},
		})
	default:
		httpx.WriteMethodNotAllowed(w, "GET, HEAD, POST")
	}
}

func (a *API) handleDocs(w http.ResponseWriter, r *http.Request) {
	// The third endpoint the method-guard pass missed, and the only one of the three that is
	// both unauthenticated and reachable from a browser, so it is the one most likely to be
	// poked with an arbitrary verb. GET/HEAD, and the 405 goes out before the text/html header
	// below so the error still carries the JSON content type every other error in this file has.
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		httpx.WriteMethodNotAllowed(w, "GET, HEAD")
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// Swagger UI is vendored (web/swagger/, embedded, served same-origin under
	// the admin namespace), so this page runs under the dashboard-wide 'self'
	// policy with no per-route override — the CDN exception is gone.
	html := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
  <title>DigitDNS REST API v1 Documentation</title>
  <meta charset="utf-8"/>
  <link rel="stylesheet" type="text/css" href="%s/swagger/swagger-ui.css">
  <style>
    body { margin: 0; background: #0b0f19; }
    .swagger-ui { filter: invert(88%%) hue-rotate(180deg); }
    #offline-notice { max-width: 52rem; margin: 0 auto; padding: 2.5rem 1.5rem; color: #e5e7eb;
      font-family: ui-sans-serif, system-ui, "Segoe UI", Roboto, sans-serif; line-height: 1.65; }
    #offline-notice h1 { color: #00f0ff; font-size: 1.5rem; margin: 0 0 .25rem; }
    #offline-notice h2 { color: #e5e7eb; font-size: 1rem; margin: 2rem 0 .5rem; }
    #offline-notice p { color: #9ca3af; margin: .5rem 0; }
    #offline-notice code, #offline-notice li { font-family: ui-monospace, "Cascadia Code", Consolas, monospace; }
    #offline-notice code { color: #00f0ff; }
    #offline-notice ul { list-style: none; padding: 0; margin: .5rem 0; }
    #offline-notice li { border-left: 2px solid #1f2937; padding: .3rem 0 .3rem .75rem; font-size: .9rem; }
    #offline-notice b { color: #10b981; font-weight: 600; }
  </style>
</head>
<body>
  <div id="offline-notice">
    <h1>DigitDNS REST API v1</h1>
    <p>The interactive viewer normally loads here, but this browser could not load it
      from this origin. <b>The API itself is running normally</b> &mdash; only the viewer is
      missing. Every endpoint below is live right now; from the server itself:</p>
    <p><code>curl -s "http://127.0.0.1:8080/&lt;admin-path&gt;/api/v1/version"</code></p>
    <h2>Endpoints</h2>
    <ul>
      <li>GET&nbsp;&nbsp;&nbsp;/api/v1/version &mdash; build version and sync hash (no key)</li>
      <li>GET&nbsp;&nbsp;&nbsp;/api/v1/docs &mdash; this page (no key)</li>
      <li>GET&nbsp;&nbsp;&nbsp;/api/v2/docs &mdash; v2 API documentation</li>
      <li>GET&nbsp;&nbsp;&nbsp;/api/v2/openapi.json &mdash; v2 OpenAPI spec</li>
      <li>GET&nbsp;&nbsp;&nbsp;/api/v1/status &mdash; live telemetry: QPS, cache, latency, relays</li>
      <li>GET&nbsp;&nbsp;&nbsp;/api/v1/clients &mdash; list subscriber accounts, wrapped as <code>{"clients":[...]}</code></li>
      <li>POST&nbsp;&nbsp;/api/v1/clients &mdash; create one; body needs at least <code>name</code>, and
        omitting <code>days</code> creates an account that <b>never expires</b></li>
      <li>GET&nbsp;&nbsp;&nbsp;/api/v1/clients/{id} &mdash; one subscriber, unwrapped</li>
      <li>PUT&nbsp;&nbsp;&nbsp;/api/v1/clients/{id} &mdash; <code>days_to_add</code> renews (from now if the plan
        already lapsed); <code>allowed_ip</code> rebinds, and <code>""</code> unbinds; also
        <code>name</code>, <code>uuid</code>, <code>enabled</code>, <code>traffic_limit_gb</code>,
        <code>traffic_reset_cycle</code>, <code>expires_at</code>, <code>custom_policies</code> (stored,
        but it no longer narrows an account's rules), <code>note</code>. Unknown field names are
        ignored silently</li>
      <li>DELETE /api/v1/clients/{id} &mdash; delete permanently</li>
      <li>POST&nbsp;&nbsp;/api/v1/clients/{id}/regenerate-uuid &mdash; invalidates the old UUID at once</li>
      <li>POST&nbsp;&nbsp;/api/v1/clients/{id}/reset-traffic &mdash; consumed bytes back to zero</li>
      <li>GET&nbsp;&nbsp;&nbsp;/api/v1/policies &mdash; <code>catalog</code>: the built-in rule
        catalogue, which this build ships none of, so it is always <code>[]</code>; <code>policies</code>:
        the stored rule entries &mdash; the four <code>custom_*</code> lists the dashboard writes</li>
      <li>POST&nbsp;&nbsp;/api/v1/policies &mdash; store one rule entry under its <code>key</code>
        (e.g. <code>{"key":"custom_proxied","custom_domains":["example.net"]}</code>); the record is saved,
        not applied &mdash; a daemon restart is what reads it back</li>
      <li>POST&nbsp;&nbsp;/api/v1/cache/flush &mdash; empty the DNS cache</li>
      <li>GET&nbsp;&nbsp;&nbsp;/api/v1/api-key &mdash; read the master key and its bind address</li>
      <li>POST&nbsp;&nbsp;/api/v1/api-key &mdash; rotate it; every integration must be reconfigured</li>
      <li>POST&nbsp;&nbsp;/api/server/restart &mdash; reset runtime state (flushes the cache; not a process restart)</li>
    </ul>
    <h2>Selling access</h2>
    <p>Suspending or expiring an account only stops the resolver <i>recognising</i> it. While
      <code>access.allow_all</code> is at its default of <b>true</b> the daemon is an open resolver, so
      the address behind that account is then served as anonymous <code>Public</code> &mdash; which has no
      plan to expire and no quota to exceed. Traffic limits are still enforced on accounts it does
      recognise, but <b>revocation does nothing until <code>access.allow_all</code> is false</b>. Turn it
      off on any server you sell access to.</p>
    <h2>Authenticating</h2>
    <p>Send the master API key from Settings as <code>X-API-Key: &lt;key&gt;</code>, or as
      <code>Authorization: Bearer &lt;key&gt;</code>. Both are accepted everywhere except
      <code>/version</code> and this page, which need no key at all.</p>
    <p>A <b>403</b> instead of a 401 means the key is not the problem: the REST API is bound to
      localhost and the request did not come from the server itself. Turn on Public API in Settings,
      or call it over an SSH tunnel.</p>
    <h2>Getting the viewer back</h2>
    <p>The viewer ships inside this binary (<code>web/swagger/</code>); if it is missing, read
      <b>API.md</b> in the project root &mdash; it documents every endpoint above with request and
      response bodies.</p>
  </div>
  <div id="swagger-ui"></div>
  <script src="%s/swagger/swagger-ui-bundle.js"></script>
  <script>
    // The viewer is vendored same-origin, so a load failure now means a broken embed rather
    // than an unreachable CDN. When that happens SwaggerUIBundle is never defined, and calling it
    // throws inside an inline script: nothing renders and the operator gets a dark rectangle with no
    // explanation, indistinguishable from a broken server. So the static list above ships visible
    // and is hidden only once the real viewer is known to be here to replace it.
    var viewerLoaded = typeof SwaggerUIBundle === 'function';
    if (viewerLoaded) document.getElementById('offline-notice').style.display = 'none';
    if (viewerLoaded) SwaggerUIBundle({
      dom_id: '#swagger-ui',
      spec: {
        openapi: "3.0.0",
        info: {
          title: "DigitDNS Core REST API",
          version: "%s",
          description: "REST API for the DigitDNS SmartDNS Controller: AES-256-GCM encrypted subscribers, operator-maintained domain rule lists and real-time DNS telemetry.\n\n**Authentication.** Press Authorize and paste the master API key from Settings. It travels as the X-API-Key header, or as Authorization: Bearer <key> — both are accepted, and a live dashboard session token works in either.\n\n**If Try it out answers 403 rather than 401**, the key is not the problem: the REST API is bound to localhost and this browser is not on the server. Turn on Public API in Settings, or call the API from the machine itself."
        },
        servers: [{ url: "%s", description: "This DigitDNS instance. Since v2.1 the API lives below the install's hidden 16-hex admin path, so the base is that path and not the host root — Try it out resolves against this value." }],
        security: [{ ApiKeyAuth: [] }, { BearerAuth: [] }],
        components: {
          securitySchemes: {
            ApiKeyAuth: { type: "apiKey", in: "header", name: "X-API-Key" },
            BearerAuth: { type: "http", scheme: "bearer", description: "The same master key, or a dashboard session token." }
          }
        },
                paths: {
          "/api/v1/version": { "get": { "summary": "v", "security": [], "responses": { "200": { "description": "OK" } } } },
          "/api/v1/docs": { "get": { "summary": "d", "security": [], "responses": { "200": { "description": "OK" } } } },
          "/api/v1/status": { "get": { "summary": "s", "responses": { "200": { "description": "OK" }, "401": { "description": "x" } } } },
          "/api/v1/clients": { "get": { "summary": "l", "responses": { "200": { "description": "OK" }, "401": { "description": "x" } } }, "post": { "summary": "c", "responses": { "200": { "description": "OK" }, "401": { "description": "x" }, "400": { "description": "x" } } } },
          "/api/v1/clients/{id}": { "parameters": [ { name: "id", in: "path", required: true, schema: { type: "string" } } ], "get": { "summary": "g", "responses": { "200": { "description": "OK" }, "401": { "description": "x" }, "404": { "description": "x" } } }, "put": { "summary": "u", "responses": { "200": { "description": "OK" }, "401": { "description": "x" }, "404": { "description": "x" }, "400": { "description": "x" } } }, "delete": { "summary": "d", "responses": { "200": { "description": "OK" }, "401": { "description": "x" }, "404": { "description": "x" } } } },
          "/api/v1/clients/{id}/actions/regenerate-uuid": { "parameters": [ { name: "id", in: "path", required: true, schema: { type: "string" } } ], "post": { "summary": "r", "responses": { "200": { "description": "OK" }, "401": { "description": "x" }, "404": { "description": "x" } } } },
          "/api/v1/clients/{id}/actions/reset-traffic": { "parameters": [ { name: "id", in: "path", required: true, schema: { type: "string" } } ], "post": { "summary": "t", "responses": { "200": { "description": "OK" }, "401": { "description": "x" }, "404": { "description": "x" } } } },
          "/api/v1/policies": { "get": { "summary": "p", "responses": { "200": { "description": "OK" }, "401": { "description": "x" } } } },
          "/api/v1/cache/flush": { "post": { "summary": "f", "responses": { "200": { "description": "OK" }, "401": { "description": "x" } } } },
          "/api/v1/api-key": { "get": { "summary": "ak", "responses": { "200": { "description": "OK" }, "401": { "description": "x" } } } },
          "/api/v2/docs": { "get": { "summary": "v2d", "security": [], "responses": { "200": { "description": "OK" } } } },
          "/api/v2/openapi.json": { "get": { "summary": "v2o", "security": [], "responses": { "200": { "description": "OK" } } } },
          "/api/server/restart": { "post": { "summary": "rs", "responses": { "200": { "description": "OK" }, "401": { "description": "x" } } } }
        }
      }
    });
  </script>
</body>
</html>`, adminServerURL(a.settings.GetAdminPath()), adminServerURL(a.settings.GetAdminPath()), version.Get().Version, adminServerURL(a.settings.GetAdminPath()))
	_, _ = w.Write([]byte(html))
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

//go:embed openapi/v2.json
var openAPIV2Spec []byte

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
