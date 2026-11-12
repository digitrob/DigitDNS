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
	"digitdns/internal/service"
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

func (b *v3Backend) Cancel2FA(r *http.Request) (map[string]any, error) {
	return b.delegateBody(r, map[string]any{}, b.ws.handleAuth2FACancel)
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

// RunAdaptive forces one Adaptive benchmark/selection cycle. The panel and the
// app call the same helper, so the button and the scheduler cannot drift.
func (b *v3Backend) RunAdaptive() (map[string]any, error) {
	if b.ws.upstreams == nil {
		return nil, api.V3Error(api.ErrV3Unavailable, "no upstream pool configured")
	}
	if _, err := b.ws.runAdaptiveNow(true); err != nil {
		if errors.Is(err, errAdaptiveNotSelected) || errors.Is(err, errAdaptiveBusy) {
			return nil, api.V3Error(api.ErrV3Conflict, err.Error())
		}
		return nil, err
	}
	return b.ResolutionSettings()
}

// DnssecSettings is the dashboard's DNSSEC policy view. The Android client uses
// the v3 route because its credential can be the master API key.
func (b *v3Backend) DnssecSettings() (map[string]any, error) {
	rec := httptest.NewRecorder()
	b.ws.handleDNSSECSettings(rec, httptest.NewRequest(http.MethodGet, "/api/settings/dnssec", nil))
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return out, nil
}

func (b *v3Backend) SaveDnssecSettings(r *http.Request, body map[string]any) (map[string]any, error) {
	// The panel handler answers GET/POST; a v3 PUT is normalised to POST.
	post := r.Clone(r.Context())
	post.Method = http.MethodPost
	return b.delegateBody(post, body, b.ws.handleDNSSECSettings)
}

// TransportSettings is the dashboard's DoQ/DoH3 card. The Android client uses
// the v3 route because its credential can be the master API key.
func (b *v3Backend) TransportSettings() (map[string]any, error) {
	rec := httptest.NewRecorder()
	b.ws.handleTransportSettings(rec, httptest.NewRequest(http.MethodGet, "/api/settings/transports", nil))
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return out, nil
}

func (b *v3Backend) SaveTransportSettings(r *http.Request, body map[string]any) (map[string]any, error) {
	post := r.Clone(r.Context())
	post.Method = http.MethodPost
	return b.delegateBody(post, body, b.ws.handleTransportSettings)
}

// ProxyLeakSettings is the dashboard's live proxy leak guard. The Android
// client uses the v3 route because its credential can be the master API key.
func (b *v3Backend) ProxyLeakSettings() (map[string]any, error) {
	rec := httptest.NewRecorder()
	b.ws.handleProxyLeakSettings(rec, httptest.NewRequest(http.MethodGet, "/api/settings/proxy-leak", nil))
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return out, nil
}

func (b *v3Backend) SaveProxyLeakSettings(r *http.Request, body map[string]any) (map[string]any, error) {
	// The panel handler answers GET/POST; a v3 PUT is normalised to POST.
	post := r.Clone(r.Context())
	post.Method = http.MethodPost
	return b.delegateBody(post, body, b.ws.handleProxyLeakSettings)
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

func (b *v3Backend) TLSCertDetails() map[string]any {
	rec := httptest.NewRecorder()
	b.ws.handleTLSCert(rec, httptest.NewRequest(http.MethodGet, "/api/tls/cert", nil))
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return out
}

func (b *v3Backend) InstallTLSCert(r *http.Request, body map[string]any) (map[string]any, error) {
	return b.delegateBody(r, body, b.ws.handleTLSCert)
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

func (b *v3Backend) Explain(domain, clientIP string) (map[string]any, error) {
	return b.ws.Explain(domain, clientIP)
}

func (b *v3Backend) AlertSettings() (map[string]any, error) {
	if b.ws.alerts == nil {
		return nil, nil
	}
	s, err := b.ws.alerts.Settings()
	if err != nil {
		return nil, err
	}
	return map[string]any{"alerts": redactAlerts(s), "history": b.ws.alerts.History()}, nil
}

func (b *v3Backend) SaveAlertSettings(r *http.Request, body map[string]any) (map[string]any, error) {
	if b.ws.alerts == nil {
		return nil, nil
	}
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	var req service.AlertSettings
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, err
	}
	if current, err := b.ws.alerts.Settings(); err == nil {
		byID := map[string]string{}
		for _, ch := range current.Channels {
			byID[ch.ID] = ch.Token
		}
		for i := range req.Channels {
			tok := strings.TrimSpace(req.Channels[i].Token)
			if (tok == "" || strings.HasPrefix(tok, "••")) && byID[req.Channels[i].ID] != "" {
				req.Channels[i].Token = byID[req.Channels[i].ID]
			}
		}
	}
	if err := validateAlerts(req); err != nil {
		return nil, err
	}
	if err := b.ws.alerts.SaveSettings(req); err != nil {
		return nil, err
	}
	return map[string]any{"alerts": redactAlerts(req), "history": b.ws.alerts.History()}, nil
}

func (b *v3Backend) TestAlerts(r *http.Request) (map[string]any, error) {
	if b.ws.alerts == nil {
		return nil, nil
	}
	failures := b.ws.alerts.SendTest(r.Context())
	return map[string]any{"ok": len(failures) == 0, "errors": failures}, nil
}

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
// Relay agents
// -----------------------------------------------------------------------------

func (b *v3Backend) Agents() ([]map[string]any, error) {
	rec := httptest.NewRecorder()
	b.ws.handleAgents(rec, httptest.NewRequest(http.MethodGet, "/api/agents", nil))
	var wrapper struct {
		Agents []json.RawMessage `json:"agents"`
	}
	if rec.Code >= 400 {
		var parsed map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &parsed)
		return nil, v3FromStatus(rec.Code, agentErrorMessage(parsed, "could not list agents"))
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &wrapper)
	out := make([]map[string]any, 0, len(wrapper.Agents))
	for _, raw := range wrapper.Agents {
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, nil
}

func (b *v3Backend) CreateAgent(r *http.Request, body map[string]any) (map[string]any, error) {
	return b.delegateBody(r, body, b.ws.handleAgents)
}

func (b *v3Backend) UpdateAgent(r *http.Request, id string, body map[string]any) (map[string]any, error) {
	return b.agentItem(r, id, http.MethodPut, "", body)
}

func (b *v3Backend) DeleteAgent(r *http.Request, id string) (map[string]any, error) {
	return b.agentItem(r, id, http.MethodDelete, "", nil)
}

func (b *v3Backend) SetAgentEnabled(r *http.Request, id string, enabled bool) (map[string]any, error) {
	action := "disable"
	if enabled {
		action = "enable"
	}
	return b.agentItem(r, id, http.MethodPost, action, nil)
}

func (b *v3Backend) RotateAgentKey(r *http.Request, id string) (map[string]any, error) {
	return b.agentItem(r, id, http.MethodPost, "rotate-key", nil)
}

func (b *v3Backend) AgentKey(r *http.Request, id string) (map[string]any, error) {
	return b.agentItem(r, id, http.MethodGet, "key", nil)
}

func (b *v3Backend) TestAgent(r *http.Request, id string) (map[string]any, error) {
	return b.agentItem(r, id, http.MethodPost, "test", nil)
}

// agentItem runs the panel's item handler for one id, optionally with a body.
// The panel route is /api/agents/<id>[/<action>], so the v3 caller's credential
// is irrelevant here: the v3 middleware has already authorised the request.
func (b *v3Backend) agentItem(
	r *http.Request,
	id, method, action string,
	body map[string]any,
) (map[string]any, error) {
	r2 := r.Clone(r.Context())
	r2.Method = method
	r2.URL.Path = "/api/agents/" + id
	if action != "" {
		r2.URL.Path += "/" + action
	}
	if body != nil {
		cloned, err := cloneWithBody(r2, body)
		if err != nil {
			return nil, err
		}
		r2 = cloned
	}
	_, parsed, _, err := call(b.ws.handleAgentByID, r2)
	if err != nil {
		return nil, err
	}
	if parsed == nil {
		parsed = map[string]any{"success": true}
	}
	return parsed, nil
}

func (b *v3Backend) TestRelayMode(r *http.Request) (map[string]any, error) {
	rec := httptest.NewRecorder()
	b.ws.handleRelayModeTest(rec, httptest.NewRequest(http.MethodPost, "/api/relay-mode/test", nil))
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if out == nil {
		out = map[string]any{"ok": false, "error": "relay mode test returned no response"}
	}
	return out, nil
}

func (b *v3Backend) RelayMode() map[string]any {
	return map[string]any{"relay_mode": b.ws.relayModeView()}
}

func (b *v3Backend) SaveRelayMode(r *http.Request, body map[string]any) (map[string]any, error) {
	return b.delegateBody(r, body, b.ws.handleRelayMode)
}

func (b *v3Backend) EngineStatus() map[string]any {
	rec := httptest.NewRecorder()
	b.ws.handleEngine(rec, httptest.NewRequest(http.MethodGet, "/api/engine", nil))
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if out == nil {
		out = map[string]any{"enabled": true}
	}
	return out
}

func (b *v3Backend) SaveEngine(r *http.Request, body map[string]any) (map[string]any, error) {
	return b.delegateBody(r, body, b.ws.handleEngine)
}

func (b *v3Backend) SystemLog(limit int) (map[string]any, error) {
	lines := []string{}
	if b.ws.systemLog != nil {
		lines = b.ws.systemLog.Lines(limit)
	}
	return map[string]any{"lines": lines}, nil
}

func (b *v3Backend) Egresses() ([]map[string]any, error) {
	rec := httptest.NewRecorder()
	b.ws.handleEgresses(rec, httptest.NewRequest(http.MethodGet, "/api/egresses", nil))
	var wrapper struct {
		Egresses []json.RawMessage `json:"egresses"`
	}
	if rec.Code >= 400 {
		var parsed map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &parsed)
		return nil, v3FromStatus(rec.Code, agentErrorMessage(parsed, "could not list egresses"))
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &wrapper)
	out := make([]map[string]any, 0, len(wrapper.Egresses))
	for _, raw := range wrapper.Egresses {
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, nil
}

func (b *v3Backend) CreateEgress(r *http.Request, body map[string]any) (map[string]any, error) {
	return b.delegateBody(r, body, b.ws.handleEgresses)
}

func (b *v3Backend) UpdateEgress(r *http.Request, id string, body map[string]any) (map[string]any, error) {
	return b.egressItem(r, id, http.MethodPut, "", body)
}

func (b *v3Backend) DeleteEgress(r *http.Request, id string) (map[string]any, error) {
	return b.egressItem(r, id, http.MethodDelete, "", nil)
}

func (b *v3Backend) SetEgressEnabled(r *http.Request, id string, enabled bool) (map[string]any, error) {
	action := "disable"
	if enabled {
		action = "enable"
	}
	return b.egressItem(r, id, http.MethodPost, action, nil)
}

func (b *v3Backend) TestEgress(r *http.Request, id string) (map[string]any, error) {
	return b.egressItem(r, id, http.MethodPost, "test", nil)
}

// egressItem runs the panel's item handler for one id. The v3 middleware has
// already authorised the caller; the cloned request only needs the panel path.
func (b *v3Backend) egressItem(
	r *http.Request,
	id, method, action string,
	body map[string]any,
) (map[string]any, error) {
	r2 := r.Clone(r.Context())
	r2.Method = method
	r2.URL.Path = "/api/egresses/" + id
	if action != "" {
		r2.URL.Path += "/" + action
	}
	if body != nil {
		cloned, err := cloneWithBody(r2, body)
		if err != nil {
			return nil, err
		}
		r2 = cloned
	}
	_, parsed, _, err := call(b.ws.handleEgressByID, r2)
	if err != nil {
		return nil, err
	}
	if parsed == nil {
		parsed = map[string]any{"success": true}
	}
	return parsed, nil
}

func agentErrorMessage(parsed map[string]any, fallback string) string {
	if parsed != nil {
		if e, ok := parsed["error"].(string); ok && e != "" {
			return e
		}
	}
	return fallback
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

func (b *v3Backend) UpdateMode(r *http.Request) (map[string]any, error) {
	return b.delegateRequest(r, b.ws.handleUpdateMode)
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

// -----------------------------------------------------------------------------
// Ad blocking / blocklists (delegated)
// -----------------------------------------------------------------------------

// FilteringStatus mirrors GET /api/filtering: settings, installed lists, the
// preset catalogue, cached preset counts and the compiled totals. The dashboard
// handler also kicks off the background catalogue count scan when its cache is
// stale, so the Android list and the panel show the same numbers.
func (b *v3Backend) FilteringStatus(r *http.Request) (map[string]any, error) {
	return b.delegateRequest(r, b.ws.handleFiltering)
}

func (b *v3Backend) SaveFilteringSettings(r *http.Request) (map[string]any, error) {
	// The panel handler answers POST only; a v3 PUT is normalised to POST.
	post := r.Clone(r.Context())
	post.Method = http.MethodPost
	return b.delegateRequest(post, b.ws.handleFilteringSettings)
}

func (b *v3Backend) RefreshFiltering(r *http.Request) (map[string]any, error) {
	return b.delegateRequest(r, b.ws.handleFilteringRefresh)
}

func (b *v3Backend) FilteringPresets(r *http.Request) (map[string]any, error) {
	return b.delegateRequest(r, b.ws.handleFilteringPresets)
}

func (b *v3Backend) FilterTest(r *http.Request) (map[string]any, error) {
	return b.delegateRequest(r, b.ws.handleFilterTest)
}

func (b *v3Backend) AddFilteringList(r *http.Request) (map[string]any, error) {
	return b.delegateRequest(r, b.ws.handleFilterLists)
}

func (b *v3Backend) FilteringListItem(r *http.Request, rest string) (map[string]any, error) {
	r2 := r.Clone(r.Context())
	r2.URL.Path = "/api/filtering/lists/" + rest
	return b.delegateRequest(r2, b.ws.handleFilterListItem)
}

// delegateRequest runs a dashboard handler on the v3 caller's request and returns
// its parsed JSON. The request body is not pre-read, which is what lets a large
// uploaded blocklist reach the handler's own 20 MB limit.
func (b *v3Backend) delegateRequest(r *http.Request, h http.HandlerFunc) (map[string]any, error) {
	code, parsed, _, err := call(h, r)
	if err != nil {
		return nil, err
	}
	if parsed == nil {
		if code == http.StatusNoContent {
			return map[string]any{"success": true}, nil
		}
		return map[string]any{}, nil
	}
	return parsed, nil
}
