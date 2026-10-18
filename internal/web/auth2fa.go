package web

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"digitdns/internal/auth"
	"digitdns/internal/database"
	"digitdns/internal/httpx"
)

// The authentication surface: TOTP enrollment and the change-credential
// endpoints, plus the shared checks every credential-changing endpoint runs
// when two-factor is in force.
//
// Two rules this file enforces everywhere:
//
//  1. Second factor for second-factor changes. Every endpoint here — and the
//     password-change and API-key endpoints — re-asks for the current password,
//     and when TOTP is enabled also for a current one-time code. An attacker
//     holding a stolen session token can therefore look, but not touch.
//  2. One failure string. Every credential check answers "Invalid credentials".
//     A different message per path would be an oracle telling a prober which
//     paths exist.

// SetAuthSettings attaches the auth record (test harness convenience; main
// wires it after construction, like SetSubscriptionSettings).
func (ws *WebServer) SetAuthSettings(a *database.AuthSettings) {
	ws.authSettings = a
}

// totpReplayGuard makes a validated TOTP code single-use across every gated
// endpoint. Without it one observed code authorised any number of concurrent
// calls in its 90-second window — login, unlock and key rotation included.
// The key is the code itself and the entry lives 90 seconds (the ±1-step
// validity the RFC allows), swept opportunistically on insert.
type totpReplayGuard struct {
	mu   sync.Mutex
	seen map[string]time.Time
}

func newTOTPReplayGuard() *totpReplayGuard {
	return &totpReplayGuard{seen: map[string]time.Time{}}
}

// spend reports whether this code has not been used yet, and records it.
func (g *totpReplayGuard) spend(code string) bool {
	if g == nil || code == "" {
		return true
	}
	now := time.Now()
	g.mu.Lock()
	defer g.mu.Unlock()
	for k, until := range g.seen {
		if until.Before(now) {
			delete(g.seen, k)
		}
	}
	if _, ok := g.seen[code]; ok {
		return false
	}
	g.seen[code] = now.Add(90 * time.Second)
	return true
}

// totpGate enforces the second factor on an already-authenticated,
// credential-changing endpoint. When two-factor is in force the request body
// must carry a current, not-yet-spent one-time code; without or with a wrong
// one the request is answered 401 and false comes back. When 2FA is off it is
// a no-op, so a pre-enrollment deployment keeps working unchanged.
func (ws *WebServer) totpGate(w http.ResponseWriter, code string) bool {
	if !ws.authSettings.TOTPEnabledNow() {
		return true
	}
	code = strings.TrimSpace(code)
	if !auth.ValidateTOTP(ws.authSettings.GetTOTPSecret(), code) || !ws.totpReplay.spend(code) {
		httpx.WriteJSONError(w, http.StatusUnauthorized, "Invalid credentials")
		return false
	}
	return true
}

// totpRequestGate is totpGate in request-shaped form, for the v1 REST router
// (internal/api cannot import this package to call the writer version, and
// its handlers own their own responses). The code comes from the JSON body
// only: a code is a credential, and this codebase's own rule for credentials
// is that they do not ride query strings, where access logs and proxy logs
// copy them.
func (ws *WebServer) totpRequestGate(r *http.Request) bool {
	if !ws.authSettings.TOTPEnabledNow() {
		return true
	}
	code := ""
	if r.Body != nil {
		var body struct {
			Code string `json:"code"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&body); err == nil {
			code = body.Code
		}
	}
	code = strings.TrimSpace(code)
	return auth.ValidateTOTP(ws.authSettings.GetTOTPSecret(), code) && ws.totpReplay.spend(code)
}

// mapGetter adapts a decoded JSON map.
type mapGetter map[string]any

func (m mapGetter) Get(key string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// decodeBodyMap decodes a JSON object into a mapGetter, bounded like every
// other body in the package.
func decodeBodyMap(w http.ResponseWriter, r *http.Request) (mapGetter, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var m map[string]any
	if err := json.NewDecoder(r.Body).Decode(&m); err != nil || m == nil {
		httpx.WriteJSONError(w, http.StatusBadRequest, "Invalid request format")
		return nil, false
	}
	return mapGetter(m), true
}

// handleAuth2FASetup opens a TOTP enrollment. Requires the current password;
// when 2FA is already active it refuses — disabling first is the deliberate
// downgrade step. The response carries the secret and the otpauth URI exactly
// once; the snapshot channel never repeats it.
func (ws *WebServer) handleAuth2FASetup(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		httpx.WriteMethodNotAllowed(w, "POST")
		return
	}
	body, ok := decodeBodyMap(w, r)
	if !ok {
		return
	}
	if ok, _ := ws.verifyAdminPassword(body.Get("username"), body.Get("current_password")); !ok {
		httpx.WriteJSONError(w, http.StatusUnauthorized, "Invalid credentials")
		return
	}

	secret := auth.GenerateTOTPSecret()
	if err := ws.authSettings.SetTOTPPending(secret, func(a *database.AuthSettings) error {
		return ws.db.SetSetting("auth", a)
	}); err != nil {
		httpx.WriteJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	adminUser, _, _ := ws.settings.AdminCredentials()
	if adminUser == "" {
		adminUser = "admin"
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success":     true,
		"secret":      secret,
		"otpauth_uri": auth.OTPAuthURI("DigitDNS", adminUser, secret),
	})
}

// handleAuth2FAEnable confirms an open enrollment: the code the phone is now
// showing must validate against the pending secret. On success the secret is
// promoted and every session dies — the operator re-logs in, now answering the
// authenticator prompt.
func (ws *WebServer) handleAuth2FAEnable(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		httpx.WriteMethodNotAllowed(w, "POST")
		return
	}
	body, ok := decodeBodyMap(w, r)
	if !ok {
		return
	}
	if ok, _ := ws.verifyAdminPassword(body.Get("username"), body.Get("current_password")); !ok {
		httpx.WriteJSONError(w, http.StatusUnauthorized, "Invalid credentials")
		return
	}

	confirmed, err := ws.authSettings.ConfirmTOTP(
		strings.TrimSpace(body.Get("code")),
		time.Now().UTC().Format(time.RFC3339),
		auth.ValidateTOTP,
		func(a *database.AuthSettings) error { return ws.db.SetSetting("auth", a) },
	)
	if err != nil {
		httpx.WriteJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !confirmed {
		// The enrollment stays open; nothing changed. The shared string keeps
		// wrong codes indistinguishable from wrong passwords.
		httpx.WriteJSONError(w, http.StatusUnauthorized, "Invalid credentials")
		return
	}

	if ws.sessions != nil {
		ws.sessions.DeleteAll()
	}
	log.Printf("[Web] Two-factor authentication enabled; all sessions invalidated")
	_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
}

// handleAuth2FADisable turns 2FA off. It demands both factors one last time:
// the current password and a current code. Sessions die with it, per the plan.
func (ws *WebServer) handleAuth2FADisable(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		httpx.WriteMethodNotAllowed(w, "POST")
		return
	}
	body, ok := decodeBodyMap(w, r)
	if !ok {
		return
	}
	if ok, _ := ws.verifyAdminPassword(body.Get("username"), body.Get("current_password")); !ok {
		httpx.WriteJSONError(w, http.StatusUnauthorized, "Invalid credentials")
		return
	}
	if ws.authSettings.TOTPEnabledNow() {
		code := strings.TrimSpace(body.Get("code"))
		if code == "" || !auth.ValidateTOTP(ws.authSettings.GetTOTPSecret(), code) {
			httpx.WriteJSONError(w, http.StatusUnauthorized, "Invalid credentials")
			return
		}
	} else {
		// Disabling when nothing is enabled is a no-op success, not an error:
		// the UI and the server agree without a second round trip.
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
		return
	}

	if err := ws.authSettings.DisableTOTP(func(a *database.AuthSettings) error {
		return ws.db.SetSetting("auth", a)
	}); err != nil {
		httpx.WriteJSONError(w, http.StatusInternalServerError, "Could not save the auth settings")
		return
	}
	if ws.sessions != nil {
		ws.sessions.DeleteAll()
	}
	log.Printf("[Web] Two-factor authentication disabled; all sessions invalidated")
	_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
}

// handleAuth2FAStatus is the read-only view the settings panel renders. It
// carries no secrets — that is enforced by the snapshot type itself.
func (ws *WebServer) handleAuth2FAStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		httpx.WriteMethodNotAllowed(w, "GET, HEAD")
		return
	}
	snap := ws.authSettings.Snapshot()
	// The pending enrollment's otpauth URI embeds the shared secret, and this
	// endpoint is reachable with any live session — no password, no code. The
	// URI therefore never leaves the setup response, which IS password-gated:
	// status only says whether an enrollment is open ("present"), and the
	// operator who reloads the page mid-enrollment re-runs setup (a fresh
	// secret; the old pending one is discarded).
	if snap.OTPAuthURI != "" && snap.OTPAuthURI != "present" {
		snap.OTPAuthURI = "present"
	}
	_ = json.NewEncoder(w).Encode(snap)
}
