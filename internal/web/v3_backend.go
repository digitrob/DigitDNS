package web

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"

	"digitdns/internal/api"
	"digitdns/internal/auth"
	"digitdns/internal/netutil"
)

// v3Backend adapts *WebServer onto the api.V3Backend interface the v3 router
// consumes. Every method either runs the dashboard's own handler (so the REST
// answer is byte-identical to the panel's) or calls the same unexported helper
// the handler does. This is what keeps "full parity" honest: there is one
// implementation per feature, and it is the one the buttons use.
type v3Backend struct{ ws *WebServer }

// NewV3Backend builds the adapter. main/web wiring passes it to
// (*api.API).SetV3Backend.
func NewV3Backend(ws *WebServer) api.V3Backend { return &v3Backend{ws: ws} }

// call runs a dashboard handler and returns its status, parsed JSON body (nil
// when the body is not JSON) and the raw bytes. A handler that answers an error
// is mapped onto the v3 sentinels so the router can pick the right problem
// status without knowing web types.
func call(h http.HandlerFunc, r *http.Request) (int, map[string]any, []byte, error) {
	rec := httptest.NewRecorder()
	h(rec, r)
	body := rec.Body.Bytes()
	var parsed map[string]any
	_ = json.Unmarshal(body, &parsed)
	if rec.Code >= 400 {
		msg := "request failed"
		if parsed != nil {
			if e, ok := parsed["error"].(string); ok && e != "" {
				msg = e
			}
		}
		return rec.Code, parsed, body, v3FromStatus(rec.Code, msg)
	}
	return rec.Code, parsed, body, nil
}

// v3FromStatus maps an HTTP status from a delegated handler onto a v3 sentinel
// error so the router's problem status matches.
func v3FromStatus(status int, msg string) error {
	switch status {
	case http.StatusUnauthorized:
		return api.V3Error(api.ErrV3Unauthorized, msg)
	case http.StatusNotFound:
		return api.V3Error(api.ErrV3NotFound, msg)
	case http.StatusConflict:
		return api.V3Error(api.ErrV3Conflict, msg)
	case http.StatusServiceUnavailable:
		return api.V3Error(api.ErrV3Unavailable, msg)
	default:
		return errors.New(msg)
	}
}

// cloneWithBody returns a shallow copy of r (same context and URL) carrying a
// fresh body re-encoded from body, so a delegated handler that re-reads the body
// sees exactly the payload the v3 caller sent.
func cloneWithBody(r *http.Request, body map[string]any) (*http.Request, error) {
	buf, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	r2 := r.Clone(r.Context())
	r2.Body = http.NoBody
	r2 = r2.WithContext(r.Context())
	r2.Body = nopCloser{bytes.NewReader(buf)}
	r2.ContentLength = int64(len(buf))
	r2.Header = r.Header.Clone()
	r2.Header.Set("Content-Type", "application/json")
	return r2, nil
}

type nopCloser struct{ *bytes.Reader }

func (nopCloser) Close() error { return nil }

// -----------------------------------------------------------------------------
// Auth
// -----------------------------------------------------------------------------

// Login performs a full dashboard login through the same KDF gate, lockout
// tracker, TOTP validation and replay guard, then mints a session. It mirrors
// handleAuthLogin's checks so the two cannot diverge on what a valid credential
// is.
func (b *v3Backend) Login(r *http.Request, username, password, code string) (string, bool, error) {
	ws := b.ws
	clientIP := peerFor(ws, r)
	if ws.loginLimiter != nil && ws.loginLimiter.IsBlocked(clientIP) {
		return "", false, api.V3Error(api.ErrV3Unauthorized, "too many failed login attempts; this address is locked for 10 minutes")
	}

	ok, _ := ws.verifyAdminPassword(username, password)
	twoFactorRequired := false
	if ok && ws.authSettings.TOTPEnabledNow() {
		c := strings.TrimSpace(code)
		if !auth.ValidateTOTP(ws.authSettings.GetTOTPSecret(), c) || !ws.totpReplay.spend(c) {
			ok = false
			twoFactorRequired = true
		}
	}
	if !ok {
		if ws.loginLimiter != nil {
			ws.loginLimiter.RecordFailure(clientIP)
		}
		if twoFactorRequired {
			return "", true, api.V3Error(api.ErrV3Unauthorized, "invalid credentials")
		}
		return "", false, api.V3Error(api.ErrV3Unauthorized, "invalid credentials")
	}
	if ws.loginLimiter != nil {
		ws.loginLimiter.RecordSuccess(clientIP)
	}
	if ws.sessions == nil {
		return "", false, api.V3Error(api.ErrV3Unavailable, "sessions are not initialised")
	}
	return ws.sessions.Create(), false, nil
}

func (b *v3Backend) Logout(token string) {
	if b.ws.sessions != nil {
		b.ws.sessions.Delete(token)
	}
}

func (b *v3Backend) AuthSnapshot() map[string]any {
	snap := b.ws.authSettings.Snapshot()
	out := map[string]any{
		"totp_enabled":    snap.TOTPEnabled,
		"totp_configured": snap.TOTPConfigured,
		"totp_enrolling":  snap.TOTPEnrolling,
	}
	if snap.OTPAuthURI != "" && snap.OTPAuthURI != "present" {
		out["otpauth_uri"] = "present"
	} else if snap.OTPAuthURI != "" {
		out["otpauth_uri"] = snap.OTPAuthURI
	}
	return out
}

func (b *v3Backend) Setup2FA(r *http.Request, username, currentPassword string) (map[string]any, error) {
	return b.delegateBody(r, map[string]any{"username": username, "current_password": currentPassword}, b.ws.handleAuth2FASetup)
}

func (b *v3Backend) Enable2FA(r *http.Request, username, currentPassword, code string) (map[string]any, error) {
	return b.delegateBody(r, map[string]any{"username": username, "current_password": currentPassword, "code": code}, b.ws.handleAuth2FAEnable)
}

func (b *v3Backend) Disable2FA(r *http.Request, username, currentPassword, code string) (map[string]any, error) {
	return b.delegateBody(r, map[string]any{"username": username, "current_password": currentPassword, "code": code}, b.ws.handleAuth2FADisable)
}

func (b *v3Backend) Unlock(r *http.Request, ip string) (map[string]any, error) {
	code := ""
	return b.delegateBody(r, map[string]any{"ip": ip, "code": code}, b.ws.handleAuthUnlock)
}

func (b *v3Backend) SSETicket(sessionKey string) (map[string]any, error) {
	ticket := b.ws.sseTickets.mint(sessionKey)
	if ticket == "" {
		return nil, api.V3Error(api.ErrV3Conflict, "too many outstanding stream tickets; open fewer tabs or wait for them to expire")
	}
	return map[string]any{"ticket": ticket, "expires_in": 60}, nil
}

func (b *v3Backend) TicketAuthorized(r *http.Request) bool {
	return b.ws.sseTicketAuthorized(r)
}

// delegateBody re-encodes body into a fresh request and runs a body-reading
// dashboard handler on it, returning the parsed JSON response.
func (b *v3Backend) delegateBody(r *http.Request, body map[string]any, h http.HandlerFunc) (map[string]any, error) {
	r2, err := cloneWithBody(r, body)
	if err != nil {
		return nil, err
	}
	_, parsed, _, err := call(h, r2)
	if err != nil {
		return nil, err
	}
	if parsed == nil {
		parsed = map[string]any{"success": true}
	}
	return parsed, nil
}

// -----------------------------------------------------------------------------
// Config / settings / access / rules
// -----------------------------------------------------------------------------

func (b *v3Backend) Config() map[string]any {
	rec := httptest.NewRecorder()
	b.ws.handleConfig(rec, httptest.NewRequest(http.MethodGet, "/api/config", nil))
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return out
}

func (b *v3Backend) AccessConfig() map[string]any {
	rec := httptest.NewRecorder()
	b.ws.handleConfigAccess(rec, httptest.NewRequest(http.MethodGet, "/api/config/access", nil))
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return out
}

func (b *v3Backend) SaveAccess(r *http.Request, body map[string]any) (map[string]any, error) {
	return b.delegateBody(r, body, b.ws.handleConfigAccess)
}

func (b *v3Backend) Rules() map[string]any {
	return b.ws.loadRulesConfig()
}

func (b *v3Backend) SaveRules(r *http.Request, body map[string]any) (map[string]any, error) {
	return b.delegateBody(r, body, b.ws.handleConfigRules)
}

func (b *v3Backend) Settings() (map[string]any, error) {
	rec := httptest.NewRecorder()
	b.ws.handleSettings(rec, httptest.NewRequest(http.MethodGet, "/api/settings", nil))
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return out, nil
}

func (b *v3Backend) SaveSettings(r *http.Request, body map[string]any) (map[string]any, error) {
	return b.delegateBody(r, body, b.ws.handleSettings)
}

// ResolutionSettings is the dashboard's resolution view. The v3 router mounts
// this because the web mux's session-only gate rejects the master API key, which
// is a supported Android credential mode.
func (b *v3Backend) ResolutionSettings() (map[string]any, error) {
	rec := httptest.NewRecorder()
	b.ws.handleResolutionSettings(rec, httptest.NewRequest(http.MethodGet, "/api/settings/resolution", nil))
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return out, nil
}

func (b *v3Backend) SaveResolution(r *http.Request, body map[string]any) (map[string]any, error) {
	// The panel handler answers GET/POST; a v3 PUT is normalised to POST.
	post := r.Clone(r.Context())
	post.Method = http.MethodPost
	return b.delegateBody(post, body, b.ws.handleResolutionSettings)
}

func (b *v3Backend) RegenerateAPIKey(r *http.Request, currentPassword, code string) (map[string]any, error) {
	return b.delegateBody(r, map[string]any{"current_password": currentPassword, "code": code}, b.ws.handleRegenerateAPIKey)
}

func (b *v3Backend) RegenerateAdminPath(r *http.Request, confirm bool) (map[string]any, error) {
	return b.delegateBody(r, map[string]any{"confirm": confirm}, b.ws.handleRegenerateAdminPath)
}

func (b *v3Backend) Subscription() map[string]any {
	if b.ws.subSettings == nil {
		return map[string]any{}
	}
	snap := b.ws.subSettings.Snapshot()
	raw, _ := json.Marshal(snap)
	var out map[string]any
	_ = json.Unmarshal(raw, &out)
	return out
}

func (b *v3Backend) SaveSubscription(r *http.Request, body map[string]any) (map[string]any, error) {
	return b.delegateBody(r, body, b.ws.handleSubscriptionSettings)
}

// -----------------------------------------------------------------------------
// GeoIP database (server location). Status is a plain read; the refresh reuses
// the dashboard handler so the button and the daily loop cannot diverge.

func (b *v3Backend) GeoIPStatus() (map[string]any, error) {
	_, parsed, _, err := call(b.ws.handleGeoIPStatus, httptest.NewRequest(http.MethodGet, "/api/settings/geoip", nil))
	return parsed, err
}

func (b *v3Backend) GeoIPUpdate(r *http.Request) (map[string]any, error) {
	return b.delegateBody(r, nil, b.ws.handleGeoIPUpdate)
}

// TLS / ACME
// -----------------------------------------------------------------------------

func (b *v3Backend) IssueTLS(r *http.Request, body map[string]any) (map[string]any, error) {
	return b.delegateBody(r, body, b.ws.handleTLSSettings)
}

func (b *v3Backend) TLSAutoRenew(r *http.Request, body map[string]any) (map[string]any, error) {
	return b.delegateBody(r, body, b.ws.handleTLSAutoRenew)
}

func (b *v3Backend) ACMEStatus() map[string]any {
	rec := httptest.NewRecorder()
	b.ws.handleACMEStatus(rec, httptest.NewRequest(http.MethodGet, "/api/tls/acme/status", nil))
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return out
}

// -----------------------------------------------------------------------------
// Diagnostics targets
// -----------------------------------------------------------------------------

func (b *v3Backend) DiagnosticsTargets() map[string]any {
	rec := httptest.NewRecorder()
	b.ws.handleDiagnosticsTargets(rec, httptest.NewRequest(http.MethodGet, "/api/diagnostics/targets", nil))
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return out
}

func (b *v3Backend) AddDiagnosticsTarget(r *http.Request, body map[string]any) (map[string]any, error) {
	return b.delegateBody(r, body, b.ws.handleDiagnosticsTargets)
}

func (b *v3Backend) DeleteDiagnosticsTarget(r *http.Request, host string) (map[string]any, error) {
	return b.delegateBody(r, map[string]any{"host": host}, b.ws.handleDiagnosticsTargets)
}

// -----------------------------------------------------------------------------
// Custom groups
// -----------------------------------------------------------------------------

func (b *v3Backend) CustomGroups() ([]map[string]any, error) {
	rec := httptest.NewRecorder()
	b.ws.handleCustomGroups(rec, httptest.NewRequest(http.MethodGet, "/api/custom-groups", nil))
	var wrapper struct {
		Groups []json.RawMessage `json:"groups"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &wrapper)
	out := make([]map[string]any, 0, len(wrapper.Groups))
	for _, raw := range wrapper.Groups {
		var m map[string]any
		_ = json.Unmarshal(raw, &m)
		out = append(out, m)
	}
	return out, nil
}

func (b *v3Backend) CreateCustomGroup(r *http.Request, body map[string]any) (map[string]any, error) {
	return b.delegateBody(r, body, b.ws.handleCustomGroups)
}

func (b *v3Backend) UpdateCustomGroup(r *http.Request, id string, body map[string]any) (map[string]any, error) {
	r2, err := cloneWithBody(r, body)
	if err != nil {
		return nil, err
	}
	r2.URL.Path = "/api/custom-groups/" + id
	_, parsed, _, err := call(b.ws.handleCustomGroupByID, r2)
	if err != nil {
		return nil, err
	}
	if parsed == nil {
		parsed = map[string]any{"success": true}
	}
	return parsed, nil
}

func (b *v3Backend) DeleteCustomGroup(r *http.Request, id string) (map[string]any, error) {
	r2 := r.Clone(r.Context())
	r2.Method = http.MethodDelete
	r2.URL.Path = "/api/custom-groups/" + id
	_, parsed, _, err := call(b.ws.handleCustomGroupByID, r2)
	if err != nil {
		return nil, err
	}
	if parsed == nil {
		parsed = map[string]any{"success": true}
	}
	return parsed, nil
}

// -----------------------------------------------------------------------------
// Self-update & benchmark
// -----------------------------------------------------------------------------

func (b *v3Backend) UpdateCheck(r *http.Request) (map[string]any, error) {
	rec := httptest.NewRecorder()
	b.ws.handleUpdateCheck(rec, httptest.NewRequest(http.MethodGet, "/api/update/check", nil))
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if rec.Code >= 400 {
		return nil, v3FromStatus(rec.Code, "the update service is unavailable")
	}
	return out, nil
}

func (b *v3Backend) UpdateApply(r *http.Request) (map[string]any, error) {
	rec := httptest.NewRecorder()
	b.ws.handleUpdateApply(rec, httptest.NewRequest(http.MethodPost, "/api/update/apply", nil))
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if rec.Code >= 400 {
		msg := "the update service is unavailable"
		if out != nil {
			if e, ok := out["error"].(string); ok && e != "" {
				msg = e
			}
		}
		return nil, v3FromStatus(rec.Code, msg)
	}
	return out, nil
}

func (b *v3Backend) UpdateStatus() map[string]any {
	rec := httptest.NewRecorder()
	b.ws.handleUpdateStatus(rec, httptest.NewRequest(http.MethodGet, "/api/update/status", nil))
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return out
}

func (b *v3Backend) Benchmark() (map[string]any, error) {
	if b.ws.upstreams == nil {
		return nil, api.V3Error(api.ErrV3Unavailable, "no upstream pool configured")
	}
	if !b.ws.benchmark.Start() {
		return nil, api.V3Error(api.ErrV3Conflict, "a benchmark is already in progress")
	}
	return map[string]any{"started": true, "running": true, "message": "benchmark started; upstream latencies refresh as probes land"}, nil
}

// peerFor is the effective login-lockout key: the forwarded client when a
// declared proxy supplied it, else the transport peer — exactly what
// handleAuthLogin uses.
func peerFor(ws *WebServer, r *http.Request) string {
	return netutil.ClientIP(r)
}

// PersistUpstreams mirrors the dashboard's upstream persistence for the v3
// add/delete/reorder handlers, which mutate the pool directly.
func (b *v3Backend) PersistUpstreams(list []string) error {
	return b.ws.PersistUpstreams(list)
}
