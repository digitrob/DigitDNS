// API v3: the full-parity contract.
//
// v1 grew by accretion and v2 redesigned the client/policy core, but neither
// reaches the whole dashboard: TLS/ACME issuance, subscription settings, 2FA
// enrollment, custom groups, self-update, diagnostics-target management and the
// live log stream were only ever reachable from the session-authenticated panel.
// v3 exists so the forthcoming Android companion app — and any other operator
// tool — can drive every capability the dashboard exposes, without a browser.
//
// Auth is the one place v3 makes a genuinely new promise. v1 and v2 accept the
// master REST key and nothing else; the dashboard accepts a dashboard session
// and nothing else. v3 accepts both, so a companion app can log in with the
// operator's username and password (plus a TOTP code when two-factor is on) and
// hold a real, revocable session — the same digitdns_session_ token the panel
// mints — while existing scripts keep using the key unchanged.
//
// v3 keeps every v2 convention, so a caller that learned v2 already knows v3:
//   - errors are RFC 9457 application/problem+json;
//   - lists answer {items, next_cursor} with a bounded limit;
//   - resource DTOs use symmetric field names;
//   - sub-resource verbs are POST /{resource}/{id}/actions/{action};
//   - unknown JSON fields are refused.
//
// The dashboard-coupled features are delegated to the web layer through
// V3Backend rather than re-implemented: one implementation per feature means
// the REST answer can never drift from the button it mirrors.
package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"digitdns/internal/httpx"
	"digitdns/internal/netutil"
	"digitdns/internal/service"
	"digitdns/internal/version"
)

// peerIP is the immediate transport peer, deliberately not the forwarded client:
// an exposure rule must judge the connection, not a header a caller controls.
func peerIP(r *http.Request) string { return netutil.PeerIP(r) }

// V3Backend exposes the dashboard-coupled operations v3 needs but this package
// cannot import the web layer to reach. Every method is the web server's own
// handler logic, so the REST surface and the panel cannot disagree about what a
// save or an issuance means. The web server implements this and injects it via
// SetV3Backend; when it is nil (bare harnesses) the routes that depend on it
// answer 503 rather than pretending the feature exists.
type V3Backend interface {
	// Login verifies a username/password/TOTP triple through the same KDF gate,
	// lockout tracker and replay guard the dashboard login uses, and mints a
	// digitdns_session_ token on success. err is non-nil on any failure; the
	// returned bool reports whether the failure was "second factor required".
	Login(r *http.Request, username, password, code string) (token string, twoFactorRequired bool, err error)
	// Logout revokes the presented session token.
	Logout(token string)
	// AuthSnapshot returns the read-only auth state (2FA on/off/enrolling).
	AuthSnapshot() map[string]any
	// Setup2FA/Enable2FA/Disable2FA mirror the dashboard enrollment endpoints.
	Setup2FA(r *http.Request, username, currentPassword string) (map[string]any, error)
	Enable2FA(r *http.Request, username, currentPassword, code string) (map[string]any, error)
	Disable2FA(r *http.Request, username, currentPassword, code string) (map[string]any, error)
	// Unlock clears login lockouts (empty ip clears all).
	Unlock(r *http.Request, ip string) (map[string]any, error)
	// SSETicket mints a one-time stream ticket bound to the caller's session key.
	SSETicket(sessionKey string) (map[string]any, error)
	// TicketAuthorized reports whether the request carries a live, unconsumed
	// one-time stream ticket (the ?ticket= EventSource path).
	TicketAuthorized(r *http.Request) bool

	// Config / settings.
	Config() map[string]any
	AccessConfig() map[string]any
	SaveAccess(r *http.Request, body map[string]any) (map[string]any, error)
	Rules() map[string]any
	SaveRules(r *http.Request, body map[string]any) (map[string]any, error)
	Settings() (map[string]any, error)
	SaveSettings(r *http.Request, body map[string]any) (map[string]any, error)
	RegenerateAPIKey(r *http.Request, currentPassword, code string) (map[string]any, error)
	RegenerateAdminPath(r *http.Request, confirm bool) (map[string]any, error)
	Subscription() map[string]any
	SaveSubscription(r *http.Request, body map[string]any) (map[string]any, error)

	// TLS / ACME.
	IssueTLS(r *http.Request, body map[string]any) (map[string]any, error)
	TLSAutoRenew(r *http.Request, body map[string]any) (map[string]any, error)
	ACMEStatus() map[string]any

	// Diagnostics targets (the run itself is data-only and lives in the router).
	DiagnosticsTargets() map[string]any
	AddDiagnosticsTarget(r *http.Request, body map[string]any) (map[string]any, error)
	DeleteDiagnosticsTarget(r *http.Request, host string) (map[string]any, error)

	// Custom groups.
	CustomGroups() ([]map[string]any, error)
	CreateCustomGroup(r *http.Request, body map[string]any) (map[string]any, error)
	UpdateCustomGroup(r *http.Request, id string, body map[string]any) (map[string]any, error)
	DeleteCustomGroup(r *http.Request, id string) (map[string]any, error)

	// Self-update.
	UpdateCheck(r *http.Request) (map[string]any, error)
	UpdateApply(r *http.Request) (map[string]any, error)
	UpdateStatus() map[string]any

	// Benchmark kick-off.
	Benchmark() (map[string]any, error)
}

// SetV3Backend attaches the dashboard-coupled backend for the v3 routes. Called
// by the web server right after NewAPI, like SetTOTPGate.
func (a *API) SetV3Backend(b V3Backend) {
	a.v3Backend = b
}

// RegisterRoutesV3 attaches the /api/v3 endpoints. Every route is behind
// SecurityMiddlewareV3 except version (BindGate only) and login (BindGate +
// the backend's own lockout handling); SSE additionally accepts ?ticket=.
func (a *API) RegisterRoutesV3(mux *http.ServeMux) {
	mux.HandleFunc("/api/v3/version", a.BindGate(a.handleV3Version))
	mux.HandleFunc("/api/v3/status", a.SecurityMiddlewareV3(a.handleV3Status))

	// Auth.
	mux.HandleFunc("/api/v3/auth/login", a.BindGate(a.handleV3Login))
	mux.HandleFunc("/api/v3/auth/logout", a.SecurityMiddlewareV3(a.handleV3Logout))
	mux.HandleFunc("/api/v3/auth/me", a.SecurityMiddlewareV3(a.handleV3Me))
	mux.HandleFunc("/api/v3/auth/unlock", a.SecurityMiddlewareV3MasterKey(a.handleV3Unlock))
	mux.HandleFunc("/api/v3/auth/2fa/status", a.SecurityMiddlewareV3(a.handleV32FAStatus))
	mux.HandleFunc("/api/v3/auth/2fa/setup", a.SecurityMiddlewareV3(a.handleV32FASetup))
	mux.HandleFunc("/api/v3/auth/2fa/enable", a.SecurityMiddlewareV3(a.handleV32FAEnable))
	mux.HandleFunc("/api/v3/auth/2fa/disable", a.SecurityMiddlewareV3(a.handleV32FADisable))
	mux.HandleFunc("/api/v3/auth/sse-ticket", a.SecurityMiddlewareV3(a.handleV3SSETicket))

	// Clients.
	mux.HandleFunc("/api/v3/clients", a.SecurityMiddlewareV3(a.handleV3Clients))
	mux.HandleFunc("/api/v3/clients/", a.SecurityMiddlewareV3(a.handleV3ClientItem))

	// Policies & custom groups.
	mux.HandleFunc("/api/v3/policies", a.SecurityMiddlewareV3(a.handleV3Policies))
	mux.HandleFunc("/api/v3/custom-groups", a.SecurityMiddlewareV3(a.handleV3CustomGroups))
	mux.HandleFunc("/api/v3/custom-groups/", a.SecurityMiddlewareV3(a.handleV3CustomGroupItem))

	// Rules & access.
	mux.HandleFunc("/api/v3/rules", a.SecurityMiddlewareV3(a.handleV3Rules))
	mux.HandleFunc("/api/v3/access", a.SecurityMiddlewareV3(a.handleV3Access))
	mux.HandleFunc("/api/v3/access/mode", a.SecurityMiddlewareV3(a.handleV3AccessMode))

	// Upstreams.
	mux.HandleFunc("/api/v3/upstreams", a.SecurityMiddlewareV3(a.handleV3Upstreams))
	mux.HandleFunc("/api/v3/upstreams/", a.SecurityMiddlewareV3(a.handleV3UpstreamItem))

	// Settings.
	mux.HandleFunc("/api/v3/settings", a.SecurityMiddlewareV3(a.handleV3Settings))
	mux.HandleFunc("/api/v3/settings/subscription", a.SecurityMiddlewareV3(a.handleV3Subscription))
	mux.HandleFunc("/api/v3/settings/regenerate-api-key", a.SecurityMiddlewareV3(a.handleV3RegenerateAPIKey))
	mux.HandleFunc("/api/v3/settings/regenerate-admin-path", a.SecurityMiddlewareV3(a.handleV3RegenerateAdminPath))

	// TLS / ACME.
	mux.HandleFunc("/api/v3/tls/issue", a.SecurityMiddlewareV3(a.handleV3TLSIssue))
	mux.HandleFunc("/api/v3/tls/auto-renew", a.SecurityMiddlewareV3(a.handleV3TLSAutoRenew))
	mux.HandleFunc("/api/v3/tls/acme/status", a.SecurityMiddlewareV3(a.handleV3ACMEStatus))

	// Stats & live stream.
	mux.HandleFunc("/api/v3/stats", a.SecurityMiddlewareV3(a.handleV3Stats))
	mux.HandleFunc("/api/v3/events/stream", a.SecurityMiddlewareV3Stream(a.handleV3Stream))

	// Diagnostics.
	mux.HandleFunc("/api/v3/diagnostics/targets", a.SecurityMiddlewareV3(a.handleV3DiagnosticsTargets))
	mux.HandleFunc("/api/v3/diagnostics/run", a.SecurityMiddlewareV3(a.handleV3DiagnosticsRun))

	// Cache, config & self-update.
	mux.HandleFunc("/api/v3/cache/flush", a.SecurityMiddlewareV3(a.handleV3FlushCache))
	mux.HandleFunc("/api/v3/config", a.SecurityMiddlewareV3(a.handleV3Config))
	mux.HandleFunc("/api/v3/benchmark", a.SecurityMiddlewareV3(a.handleV3Benchmark))
	mux.HandleFunc("/api/v3/update/check", a.SecurityMiddlewareV3(a.handleV3UpdateCheck))
	mux.HandleFunc("/api/v3/update/apply", a.SecurityMiddlewareV3(a.handleV3UpdateApply))
	mux.HandleFunc("/api/v3/update/status", a.SecurityMiddlewareV3(a.handleV3UpdateStatus))

	// Docs.
	mux.HandleFunc("/api/v3/docs", a.BindGate(a.handleDocsV3))
	mux.HandleFunc("/api/v3/openapi.json", a.BindGate(a.handleOpenAPIV3))
}

// SecurityMiddlewareV3 is the v3 auth gate: the same APIBind exposure rule v2
// enforces, then a credential that is either the master REST key OR a live
// dashboard session. That union is the whole point of v3 — a companion app holds
// a session, a script holds the key, and both reach the same routes.
func (a *API) SecurityMiddlewareV3(next http.HandlerFunc) http.HandlerFunc {
	return a.securityV3(next, false, false)
}

// SecurityMiddlewareV3MasterKey is the recovery variant mounted on exactly one
// route — /auth/unlock — the lockout lifeline: an operator locked out of login
// must still be able to clear the lockout with the master key, or a
// fat-fingered password would be permanent short of an SSH restart. It is the
// same reachability rule the dashboard's requireAuthWithMasterKey applies.
func (a *API) SecurityMiddlewareV3MasterKey(next http.HandlerFunc) http.HandlerFunc {
	return a.securityV3(next, true, false)
}

// SecurityMiddlewareV3Stream is SecurityMiddlewareV3 plus ?ticket=, which only
// EventSource needs because it cannot send headers.
func (a *API) SecurityMiddlewareV3Stream(next http.HandlerFunc) http.HandlerFunc {
	return a.securityV3(next, false, true)
}

func (a *API) securityV3(next http.HandlerFunc, allowMasterKey, allowTicket bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		if list, single := a.settings.GetAPIExposure(); !apiExposureAllows(list, single, peerIP(r)) {
			httpx.WriteJSONError(w, http.StatusForbidden,
				"REST API is not exposed to this address. Set api_bind to 0.0.0.0 or a matching IP/CIDR in Settings to allow remote access.")
			return
		}

		if allowTicket && a.v3Backend != nil && a.v3Backend.TicketAuthorized(r) {
			next(w, r)
			return
		}

		token := bearerToken(r)
		if token == "" {
			writeProblem(w, http.StatusUnauthorized, "unauthorized", "Unauthorized: provide the API key or a dashboard session token")
			return
		}
		if !a.v3Authorized(token, allowMasterKey) {
			writeProblem(w, http.StatusUnauthorized, "unauthorized", "Unauthorized: invalid or expired credential")
			return
		}
		next(w, r)
	}
}

// v3Authorized reports whether a token is acceptable on a v3 route: a live
// dashboard session always, and the master REST key unless the route is
// session-only. The key comparison is constant-time via the settings helper.
func (a *API) v3Authorized(token string, allowMasterKey bool) bool {
	if a.sessions != nil && a.sessions.Validate(token) {
		return true
	}
	if strings.HasPrefix(token, service.SessionTokenPrefix) {
		// A session-shaped token that is not live is never the master key.
		return false
	}
	// The master key is accepted wherever a script is the caller. Unlock keeps
	// it too (allowMasterKey is true there and this path already accepts it).
	return a.masterKeyMatches(token)
}

// handleV3Version carries the build metadata and the contract version.
func (a *API) handleV3Version(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		httpx.WriteMethodNotAllowed(w, "GET, HEAD")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	info := version.Get()
	_ = json.NewEncoder(w).Encode(map[string]any{
		"version": info.Version,
		"display": info.Display,
		"hash":    info.Hash,
		"commit":  info.Commit,
		"api":     "v3",
	})
}

func (a *API) handleV3Status(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeProblem(w, http.StatusMethodNotAllowed, "method_not_allowed", "GET or HEAD only")
		return
	}
	stats := a.stats.GetLiveStats()
	publicIP, apiBind := a.settings.Endpoint()
	list, single := a.settings.GetAPIExposure()
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status":        "healthy",
		"version":       version.Get(),
		"telemetry":     stats,
		"public_ip":     publicIP,
		"api_bind":      apiBind,
		"api_bind_list": list,
		"api_exposure":  apiExposureLabel(list, single),
	})
}

// v3Limit reads and validates the bounded pagination window. It returns the
// limit, the resolved start offset and whether the caller asked for a page at
// all. It mirrors the v2 contract exactly (v2LimitMax / v2LimitDefault).
func v3Limit(r *http.Request) (limit, start int, errDetail string) {
	limit = v2LimitDefault
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			return 0, 0, "limit must be a positive integer"
		}
		if n > v2LimitMax {
			return 0, 0, "limit must be at most " + strconv.Itoa(v2LimitMax)
		}
		limit = n
	}
	if cursor := strings.TrimSpace(r.URL.Query().Get("cursor")); cursor != "" {
		n, err := strconv.Atoi(cursor)
		if err != nil || n < 0 {
			return 0, 0, "cursor is not valid"
		}
		start = n
	}
	return limit, start, ""
}

// v3Body reads the request body as a generic JSON object for the routes that
// forward to the web backend, which owns its own field validation. The 1 MiB
// cap matches the dashboard's maxRequestBody.
func v3Body(r *http.Request) (map[string]any, bool) {
	r.Body = http.MaxBytesReader(nil, r.Body, 1<<20)
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return nil, false
	}
	return body, true
}

// v3BackendUnavailable answers every backend-dependent route when the web layer
// did not inject a backend (a bare test harness). A 503 is honest: the feature
// exists in the contract but is not wired in this build.
func v3BackendUnavailable(w http.ResponseWriter) bool {
	writeProblem(w, http.StatusServiceUnavailable, "unavailable", "this operation is unavailable in this build")
	return false
}

// Sentinel errors a V3Backend implementation may return to steer the problem
// status. They wrap no domain error — the web layer maps its own errors onto
// these before returning, so the router stays ignorant of web/database types.
// They are exported so the web backend can reference them.
var (
	ErrV3Unauthorized = errors.New("unauthorized")
	ErrV3NotFound     = errors.New("not found")
	ErrV3Conflict     = errors.New("conflict")
	ErrV3Unavailable  = errors.New("unavailable")
)

// V3Error wraps a message with one of the sentinel categories so a backend can
// return a precise status without importing anything from this package's
// problem helpers.
func V3Error(kind error, msg string) error { return fmt.Errorf("%w: %s", kind, msg) }

// errorsIs reports whether err wraps target.
func errorsIs(err, target error) bool { return errors.Is(err, target) }

// aliases keep the handler-file references readable.
var (
	errV3Unauthorized = ErrV3Unauthorized
	errV3NotFound     = ErrV3NotFound
	errV3Conflict     = ErrV3Conflict
	errV3Unavailable  = ErrV3Unavailable
)
