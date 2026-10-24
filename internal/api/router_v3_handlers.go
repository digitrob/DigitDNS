package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"digitdns/internal/core/upstream"
	"digitdns/internal/database"
	"digitdns/internal/httpx"
	"digitdns/internal/netutil"
	"digitdns/internal/service"
)

// -----------------------------------------------------------------------------
// Auth
// -----------------------------------------------------------------------------

// handleV3Login mints a dashboard session from username/password (+TOTP). The
// KDF, lockout tracker and replay guard all live in the web backend so this
// route and the panel's own login cannot drift about what a valid credential is.
func (a *API) handleV3Login(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeProblemAt(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "POST only")
		return
	}
	if a.v3Backend == nil {
		v3BackendUnavailable(w, r)
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Code     string `json:"code"`
	}
	if !decodeStrict(w, r, &req) {
		return
	}
	token, twoFactorRequired, err := a.v3Backend.Login(r, req.Username, req.Password, req.Code)
	if err != nil {
		if twoFactorRequired {
			w.Header().Set("Content-Type", "application/problem+json")
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"type":               "https://digitdns.dev/problems/unauthorized",
				"title":              "Unauthorized",
				"status":             http.StatusUnauthorized,
				"detail":             "invalid credentials",
				"twofactor_required": true,
			})
			return
		}
		writeProblemAt(w, r, http.StatusUnauthorized, "unauthorized", err.Error())
		return
	}
	username, weak := a.adminIdentity()
	_ = json.NewEncoder(w).Encode(map[string]any{
		"token":               token,
		"username":            username,
		"password_weak":       weak,
		"is_default_password": weak,
		"expires_in":          int(service.DefaultSessionIdleTimeout.Seconds()),
	})
}

func (a *API) handleV3Logout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeProblemAt(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "POST only")
		return
	}
	token := bearerToken(r)
	if !strings.HasPrefix(token, service.SessionTokenPrefix) {
		writeProblemAt(w, r, http.StatusBadRequest, "invalid_request", "not a dashboard session: the REST API key has no session to revoke")
		return
	}
	if a.sessions != nil {
		a.sessions.Delete(token)
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
}

func (a *API) handleV3Me(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeProblemAt(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "GET or HEAD only")
		return
	}
	username, weak := a.adminIdentity()
	resp := map[string]any{
		"username":            username,
		"password_weak":       weak,
		"is_default_password": weak,
	}
	if a.v3Backend != nil {
		resp["auth"] = a.v3Backend.AuthSnapshot()
	}
	_ = json.NewEncoder(w).Encode(resp)
}

func (a *API) handleV3Unlock(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeProblemAt(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "POST only")
		return
	}
	if a.v3Backend == nil {
		v3BackendUnavailable(w, r)
		return
	}
	var req struct {
		Code string `json:"code"`
		IP   string `json:"ip"`
	}
	if !decodeStrict(w, r, &req) {
		return
	}
	resp, err := a.v3Backend.Unlock(r, req.IP)
	if err != nil {
		writeProblemAt(w, r, backendProblemStatus(err), "invalid_request", err.Error())
		return
	}
	_ = json.NewEncoder(w).Encode(resp)
}

func (a *API) handleV32FAStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeProblemAt(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "GET or HEAD only")
		return
	}
	if a.v3Backend == nil {
		v3BackendUnavailable(w, r)
		return
	}
	_ = json.NewEncoder(w).Encode(a.v3Backend.AuthSnapshot())
}

func (a *API) handleV32FASetup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeProblemAt(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "POST only")
		return
	}
	if a.v3Backend == nil {
		v3BackendUnavailable(w, r)
		return
	}
	var req struct {
		Username        string `json:"username"`
		CurrentPassword string `json:"current_password"`
	}
	if !decodeStrict(w, r, &req) {
		return
	}
	resp, err := a.v3Backend.Setup2FA(r, req.Username, req.CurrentPassword)
	if err != nil {
		writeProblemAt(w, r, backendProblemStatus(err), "invalid_request", err.Error())
		return
	}
	_ = json.NewEncoder(w).Encode(resp)
}

func (a *API) handleV32FAEnable(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeProblemAt(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "POST only")
		return
	}
	if a.v3Backend == nil {
		v3BackendUnavailable(w, r)
		return
	}
	var req struct {
		Username        string `json:"username"`
		CurrentPassword string `json:"current_password"`
		Code            string `json:"code"`
	}
	if !decodeStrict(w, r, &req) {
		return
	}
	resp, err := a.v3Backend.Enable2FA(r, req.Username, req.CurrentPassword, req.Code)
	if err != nil {
		writeProblemAt(w, r, backendProblemStatus(err), "invalid_request", err.Error())
		return
	}
	_ = json.NewEncoder(w).Encode(resp)
}

func (a *API) handleV32FADisable(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeProblemAt(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "POST only")
		return
	}
	if a.v3Backend == nil {
		v3BackendUnavailable(w, r)
		return
	}
	var req struct {
		Username        string `json:"username"`
		CurrentPassword string `json:"current_password"`
		Code            string `json:"code"`
	}
	if !decodeStrict(w, r, &req) {
		return
	}
	resp, err := a.v3Backend.Disable2FA(r, req.Username, req.CurrentPassword, req.Code)
	if err != nil {
		writeProblemAt(w, r, backendProblemStatus(err), "invalid_request", err.Error())
		return
	}
	_ = json.NewEncoder(w).Encode(resp)
}

func (a *API) handleV3SSETicket(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeProblemAt(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "POST only")
		return
	}
	if a.v3Backend == nil {
		v3BackendUnavailable(w, r)
		return
	}
	resp, err := a.v3Backend.SSETicket(presentedSessionKey(r))
	if err != nil {
		writeProblemAt(w, r, backendProblemStatus(err), "invalid_request", err.Error())
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(resp)
}

// adminIdentity reads the configured admin username and weak-password flag
// without exposing the verifier.
func (a *API) adminIdentity() (string, bool) {
	user, _, weak := a.settings.AdminCredentials()
	if user == "" {
		user = "admin"
	}
	return user, weak
}

// presentedSessionKey is the credential a request arrived with, used to key the
// per-session SSE-ticket cap. Falls back to the transport peer for a key-auth
// caller that has no session.
func presentedSessionKey(r *http.Request) string {
	if t := bearerToken(r); strings.HasPrefix(t, service.SessionTokenPrefix) {
		return t
	}
	return netutil.PeerIP(r)
}

// -----------------------------------------------------------------------------
// Clients
// -----------------------------------------------------------------------------

// v3ClientDTO is the symmetric client shape. It carries the full stored record
// plus the computed quota view, matching v2's field names so a v2 caller's
// parser keeps working.
//
// API 3.1 adds the three figures the web card has always read off the legacy
// shape: TotalQueries and LastSeen live on the stored record, NextTrafficReset
// is computed by the service. They are additive fields, so a v3 caller written
// before 3.1 still parses the response unchanged.
type v3ClientDTO struct {
	ID                string   `json:"id"`
	UUID              string   `json:"uuid"`
	DisplayName       string   `json:"name"`
	Token             string   `json:"token"`
	AllowedIPs        []string `json:"allowed_ips"`
	TrafficLimitGB    float64  `json:"traffic_limit_gb"`
	TrafficUsedBytes  uint64   `json:"traffic_used_bytes"`
	TrafficResetCycle string   `json:"traffic_reset_cycle"`
	QuotaExceeded     bool     `json:"quota_exceeded"`
	ExpiresAt         string   `json:"expires_at,omitempty"`
	Enabled           bool     `json:"enabled"`
	Note              string   `json:"note,omitempty"`
	PolicyIDs         []string `json:"custom_policies"`
	MaxDevices        int      `json:"max_devices"`
	RegisterSecret    string   `json:"register_secret,omitempty"`
	TotalQueries      uint64   `json:"total_queries"`
	LastSeen          string   `json:"last_seen,omitempty"`
	NextTrafficReset  string   `json:"next_traffic_reset,omitempty"`
}

func v3ViewToDTO(c service.ClientView) v3ClientDTO {
	dto := v3ClientDTO{
		ID:                c.ID,
		UUID:              c.UUID,
		DisplayName:       c.Name,
		Token:             c.Token,
		AllowedIPs:        c.AllowedIPs,
		TrafficLimitGB:    c.TrafficLimitGB,
		TrafficUsedBytes:  c.TrafficUsedBytes,
		TrafficResetCycle: c.TrafficResetCycle,
		QuotaExceeded:     c.QuotaExceeded,
		Enabled:           c.Enabled,
		Note:              c.Note,
		PolicyIDs:         c.CustomPolicies,
		MaxDevices:        database.NormalizeMaxDevices(c.MaxDevices),
		RegisterSecret:    c.RegisterSecret,
		TotalQueries:      c.TotalQueries,
	}
	if !c.ExpiresAt.IsZero() {
		dto.ExpiresAt = c.ExpiresAt.UTC().Format(timeFormatRFC3339)
	}
	// Omitted rather than sent as the zero instant: a caller that forgets to
	// check would otherwise render "last seen year 1" for an account that has
	// never registered an address.
	if !c.LastSeen.IsZero() {
		dto.LastSeen = c.LastSeen.UTC().Format(timeFormatRFC3339)
	}
	if c.NextTrafficReset != nil && !c.NextTrafficReset.IsZero() {
		dto.NextTrafficReset = c.NextTrafficReset.UTC().Format(timeFormatRFC3339)
	}
	return dto
}

func (a *API) handleV3Clients(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet, http.MethodHead:
		clients, err := a.clients.ListClientViews()
		if err != nil {
			writeProblemAt(w, r, http.StatusInternalServerError, "internal", err.Error())
			return
		}
		limit, start, detail := v3Limit(r)
		if detail != "" {
			writeProblemAt(w, r, http.StatusBadRequest, "invalid_request", detail)
			return
		}
		if start > len(clients) {
			start = len(clients)
		}
		end := start + limit
		if end > len(clients) {
			end = len(clients)
		}
		items := make([]v3ClientDTO, 0, end-start)
		for _, c := range clients[start:end] {
			items = append(items, v3ViewToDTO(c))
		}
		next := ""
		if end < len(clients) {
			next = strconv.Itoa(end)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items":       items,
			"next_cursor": next,
			"allow_all":   a.clients.IsAllowAll(),
		})

	case http.MethodPost:
		var req struct {
			DisplayName     string     `json:"name"`
			ValidityDays    int        `json:"validity_days"`
			ExpiresAt       *time.Time `json:"expires_at"`
			AllowedIPs      []string   `json:"allowed_ips"`
			QuotaLimitGB    float64    `json:"traffic_limit_gb"`
			QuotaResetCycle string     `json:"traffic_reset_cycle"`
			PolicyIDs       []string   `json:"custom_policies"`
			Note            string     `json:"note"`
			MaxDevices      int        `json:"max_devices"`
		}
		if !decodeStrict(w, r, &req) {
			return
		}
		if strings.TrimSpace(req.DisplayName) == "" {
			writeProblemAt(w, r, http.StatusBadRequest, "invalid_request", "name is required")
			return
		}
		// API 3.1: an absolute expiry wins over validity_days, mirroring the
		// service's own precedence and the panel's date picker. A zero timestamp
		// is the same "no expiry" sentinel PATCH uses, so it is left unset.
		var expiresAt time.Time
		if req.ExpiresAt != nil && !req.ExpiresAt.IsZero() {
			expiresAt = *req.ExpiresAt
		}
		client, err := a.clients.ProvisionClient(service.CreateClientRequest{
			Name:              req.DisplayName,
			Days:              req.ValidityDays,
			ExpiresAt:         expiresAt,
			IPs:               req.AllowedIPs,
			TrafficLimitGB:    req.QuotaLimitGB,
			TrafficResetCycle: req.QuotaResetCycle,
			Note:              req.Note,
			CustomPolicies:    req.PolicyIDs,
			MaxDevices:        req.MaxDevices,
		})
		if err != nil {
			httpx.WriteClientError(w, err)
			return
		}
		w.Header().Set("Location", "/api/v3/clients/"+client.ID)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(v3ViewToDTO(a.clients.ViewClient(client)))

	default:
		writeProblemAt(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "GET, HEAD or POST")
	}
}

func (a *API) handleV3ClientItem(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/v3/clients/")
	rest = strings.TrimSpace(strings.TrimSuffix(rest, "/"))

	if clientID, action, ok := strings.Cut(rest, "/actions/"); ok {
		if r.Method != http.MethodPost {
			writeProblemAt(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "POST only")
			return
		}
		a.v3ClientAction(w, r, clientID, action)
		return
	}

	id := rest
	switch r.Method {
	case http.MethodGet, http.MethodHead:
		client, err := a.clients.GetClient(id)
		if err != nil {
			writeProblemAt(w, r, http.StatusNotFound, "not_found", "no client with that id")
			return
		}
		_ = json.NewEncoder(w).Encode(v3ViewToDTO(a.clients.ViewClient(client)))

	case http.MethodPatch, http.MethodPut:
		var req v3PatchRequest
		if !decodeStrict(w, r, &req) {
			return
		}
		updated, err := a.clients.UpdateClient(id, req.toService())
		if err != nil {
			httpx.WriteClientError(w, err)
			return
		}
		_ = json.NewEncoder(w).Encode(v3ViewToDTO(a.clients.ViewClient(updated)))

	case http.MethodDelete:
		if err := a.clients.DeleteClient(id); err != nil {
			httpx.WriteClientError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		writeProblemAt(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "GET, HEAD, PATCH, PUT or DELETE")
	}
}

// v3PatchRequest carries only the fields a PATCH/PUT may change; pointers keep
// "not sent" distinct from "sent empty".
//
// API 3.1 adds UUID and ExpiresAt. ExpiresAt follows the legacy contract the
// panel uses: the zero timestamp (0001-01-01T00:00:00Z) means "no expiry", and
// an omitted (or null) field leaves the stored date alone. There is deliberately
// no way to send an empty string — that would decode as a malformed timestamp
// and answer 400 rather than silently meaning lifetime.
type v3PatchRequest struct {
	DisplayName     *string    `json:"name"`
	UUID            *string    `json:"uuid"`
	AllowedIPs      *[]string  `json:"allowed_ips"`
	QuotaLimitGB    *float64   `json:"traffic_limit_gb"`
	QuotaResetCycle *string    `json:"traffic_reset_cycle"`
	Note            *string    `json:"note"`
	Enabled         *bool      `json:"enabled"`
	ValidityDaysAdd *int       `json:"validity_days_add"`
	ExpiresAt       *time.Time `json:"expires_at"`
	MaxDevices      *int       `json:"max_devices"`
}

func (p v3PatchRequest) toService() service.UpdateClientRequest {
	req := service.UpdateClientRequest{}
	if p.DisplayName != nil {
		req.Name = p.DisplayName
	}
	if p.UUID != nil {
		req.UUID = p.UUID
	}
	if p.AllowedIPs != nil {
		req.AllowedIPs = p.AllowedIPs
	}
	if p.QuotaLimitGB != nil {
		req.TrafficLimitGB = p.QuotaLimitGB
	}
	if p.QuotaResetCycle != nil {
		req.TrafficResetCycle = p.QuotaResetCycle
	}
	if p.Note != nil {
		req.Note = p.Note
	}
	if p.Enabled != nil {
		req.Enabled = p.Enabled
	}
	if p.ValidityDaysAdd != nil {
		req.DaysToAdd = p.ValidityDaysAdd
	}
	if p.ExpiresAt != nil {
		req.ExpiresAt = p.ExpiresAt
	}
	if p.MaxDevices != nil {
		req.MaxDevices = p.MaxDevices
	}
	return req
}

func (a *API) v3ClientAction(w http.ResponseWriter, r *http.Request, clientID, action string) {
	switch action {
	case "renew":
		var req struct {
			ExtendDays int `json:"extend_days"`
		}
		_ = decodeOptional(w, r, &req)
		if req.ExtendDays <= 0 {
			writeProblemAt(w, r, http.StatusBadRequest, "invalid_request", "extend_days must be greater than 0")
			return
		}
		if err := a.clients.RenewClient(clientID, req.ExtendDays); err != nil {
			httpx.WriteClientError(w, err)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
	case "toggle":
		var req struct {
			Enabled bool `json:"enabled"`
		}
		if !decodeStrict(w, r, &req) {
			return
		}
		if _, err := a.clients.ToggleClient(clientID, req.Enabled); err != nil {
			httpx.WriteClientError(w, err)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
	case "add_ip":
		var req struct {
			IP string `json:"ip"`
		}
		if !decodeStrict(w, r, &req) {
			return
		}
		if err := a.clients.SetClientIP(clientID, req.IP); err != nil {
			httpx.WriteClientError(w, err)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
	case "remove_ip":
		var req struct {
			IP string `json:"ip"`
		}
		if !decodeStrict(w, r, &req) {
			return
		}
		if err := a.clients.RemoveClientIP(clientID, req.IP); err != nil {
			httpx.WriteClientError(w, err)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
	case "reset-traffic":
		if err := a.clients.ResetClientTraffic(clientID); err != nil {
			httpx.WriteClientError(w, err)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"reset": true})
	case "regenerate-uuid":
		newUUID, err := a.clients.RegenerateUUID(clientID)
		if err != nil {
			httpx.WriteClientError(w, err)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"uuid": newUUID})
	case "regenerate-register-secret":
		secret, err := a.clients.RegenerateRegisterSecret(clientID)
		if err != nil {
			httpx.WriteClientError(w, err)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"register_secret": secret})
	default:
		writeProblemAt(w, r, http.StatusNotFound, "unknown_action",
			"actions are renew, toggle, add_ip, remove_ip, reset-traffic, regenerate-uuid, regenerate-register-secret")
	}
}

// -----------------------------------------------------------------------------
// Policies
// -----------------------------------------------------------------------------

func (a *API) handleV3Policies(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet, http.MethodHead:
		policies, err := a.db.ListPolicies()
		if err != nil {
			writeProblemAt(w, r, http.StatusInternalServerError, "internal", err.Error())
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"items": policies, "catalog": []any{}})
	case http.MethodPost:
		var p database.Policy
		if !decodeStrict(w, r, &p) {
			return
		}
		if strings.TrimSpace(p.Key) == "" {
			writeProblemAt(w, r, http.StatusBadRequest, "invalid_request", "key is required")
			return
		}
		if err := a.db.SavePolicy(p); err != nil {
			writeProblemAt(w, r, http.StatusInternalServerError, "internal", err.Error())
			return
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(p)
	default:
		writeProblemAt(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "GET, HEAD or POST")
	}
}

// -----------------------------------------------------------------------------
// Custom groups (delegated to the web backend, which owns the service)
// -----------------------------------------------------------------------------

func (a *API) handleV3CustomGroups(w http.ResponseWriter, r *http.Request) {
	if a.v3Backend == nil {
		v3BackendUnavailable(w, r)
		return
	}
	switch r.Method {
	case http.MethodGet, http.MethodHead:
		groups, err := a.v3Backend.CustomGroups()
		if err != nil {
			writeProblemAt(w, r, http.StatusInternalServerError, "internal", err.Error())
			return
		}
		if groups == nil {
			groups = []map[string]any{}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"items": groups})
	case http.MethodPost:
		body, ok := v3Body(r)
		if !ok {
			writeProblemAt(w, r, http.StatusBadRequest, "invalid_request", "invalid JSON body")
			return
		}
		resp, err := a.v3Backend.CreateCustomGroup(r, body)
		if err != nil {
			writeProblemAt(w, r, backendProblemStatus(err), "invalid_request", err.Error())
			return
		}
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(resp)
	default:
		writeProblemAt(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "GET, HEAD or POST")
	}
}

func (a *API) handleV3CustomGroupItem(w http.ResponseWriter, r *http.Request) {
	if a.v3Backend == nil {
		v3BackendUnavailable(w, r)
		return
	}
	id := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v3/custom-groups/"), "/")
	if id == "" {
		writeProblemAt(w, r, http.StatusBadRequest, "invalid_request", "missing group id")
		return
	}
	// Icon sub-path is not exposed over v3 (binary assets are a dashboard
	// concern); only the record itself.
	switch r.Method {
	case http.MethodPut, http.MethodPatch:
		body, ok := v3Body(r)
		if !ok {
			writeProblemAt(w, r, http.StatusBadRequest, "invalid_request", "invalid JSON body")
			return
		}
		resp, err := a.v3Backend.UpdateCustomGroup(r, id, body)
		if err != nil {
			writeProblemAt(w, r, backendProblemStatus(err), "invalid_request", err.Error())
			return
		}
		_ = json.NewEncoder(w).Encode(resp)
	case http.MethodDelete:
		resp, err := a.v3Backend.DeleteCustomGroup(r, id)
		if err != nil {
			writeProblemAt(w, r, backendProblemStatus(err), "invalid_request", err.Error())
			return
		}
		_ = json.NewEncoder(w).Encode(resp)
	default:
		writeProblemAt(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "PUT, PATCH or DELETE")
	}
}

// -----------------------------------------------------------------------------
// Rules & access (delegated)
// -----------------------------------------------------------------------------

func (a *API) handleV3Rules(w http.ResponseWriter, r *http.Request) {
	if a.v3Backend == nil {
		v3BackendUnavailable(w, r)
		return
	}
	switch r.Method {
	case http.MethodGet, http.MethodHead:
		_ = json.NewEncoder(w).Encode(a.v3Backend.Rules())
	case http.MethodPost, http.MethodPut:
		body, ok := v3Body(r)
		if !ok {
			writeProblemAt(w, r, http.StatusBadRequest, "invalid_request", "invalid JSON body")
			return
		}
		resp, err := a.v3Backend.SaveRules(r, body)
		if err != nil {
			writeProblemAt(w, r, backendProblemStatus(err), "invalid_request", err.Error())
			return
		}
		_ = json.NewEncoder(w).Encode(resp)
	default:
		writeProblemAt(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "GET, HEAD, POST or PUT")
	}
}

func (a *API) handleV3Access(w http.ResponseWriter, r *http.Request) {
	if a.v3Backend == nil {
		v3BackendUnavailable(w, r)
		return
	}
	switch r.Method {
	case http.MethodGet, http.MethodHead:
		_ = json.NewEncoder(w).Encode(a.v3Backend.AccessConfig())
	case http.MethodPost, http.MethodPut:
		body, ok := v3Body(r)
		if !ok {
			writeProblemAt(w, r, http.StatusBadRequest, "invalid_request", "invalid JSON body")
			return
		}
		resp, err := a.v3Backend.SaveAccess(r, body)
		if err != nil {
			writeProblemAt(w, r, backendProblemStatus(err), "invalid_request", err.Error())
			return
		}
		_ = json.NewEncoder(w).Encode(resp)
	default:
		writeProblemAt(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "GET, HEAD, POST or PUT")
	}
}

func (a *API) handleV3AccessMode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodPut {
		writeProblemAt(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "POST or PUT")
		return
	}
	var req struct {
		AllowAll *bool `json:"allow_all"`
	}
	if !decodeStrict(w, r, &req) {
		return
	}
	if req.AllowAll == nil {
		writeProblemAt(w, r, http.StatusBadRequest, "invalid_request", "allow_all is required")
		return
	}
	if err := a.clients.SetAllowAll(*req.AllowAll); err != nil {
		writeProblemAt(w, r, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"allow_all": a.clients.IsAllowAll()})
}

// -----------------------------------------------------------------------------
// Upstreams
// -----------------------------------------------------------------------------

// handleV3UpstreamStats: GET the per-upstream latency/health snapshot the panel's
// resolver-racer card reads. The web dashboard gets this from its own /api/stats
// map; v3 keeps the same shape so a client does not have to know the panel exists.
func (a *API) handleV3UpstreamStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeProblemAt(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "GET or HEAD only")
		return
	}
	// GetUpstreamStats on a nil pool panics, and that panic would land on a poll.
	stats := []upstream.UpstreamStat{}
	if a.upstreams != nil {
		stats = a.upstreams.GetUpstreamStats()
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"items": stats})
}

func (a *API) handleV3Upstreams(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet, http.MethodHead:
		var list []string
		if a.upstreams != nil {
			list = a.upstreams.GetUpstreams()
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"items": list})
	case http.MethodPost:
		var req struct {
			Address string `json:"address"`
		}
		if !decodeStrict(w, r, &req) {
			return
		}
		addr := strings.TrimSpace(req.Address)
		if addr == "" {
			writeProblemAt(w, r, http.StatusBadRequest, "invalid_request", "address is required")
			return
		}
		if a.upstreams == nil {
			writeProblemAt(w, r, http.StatusServiceUnavailable, "unavailable", "no upstream pool")
			return
		}
		canonical, ok := upstream.NormalizeUpstream(addr)
		if !ok {
			writeProblemAt(w, r, http.StatusBadRequest, "invalid_request", "invalid upstream address")
			return
		}
		list := a.upstreams.GetUpstreams()
		for _, u := range list {
			if u == canonical {
				writeProblemAt(w, r, http.StatusConflict, "conflict", "upstream already exists")
				return
			}
		}
		list = append(list, canonical)
		a.upstreams.SetUpstreams(list)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"items": list})
	default:
		writeProblemAt(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "GET, HEAD or POST")
	}
}

func (a *API) handleV3UpstreamItem(w http.ResponseWriter, r *http.Request) {
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v3/upstreams/"), "/")
	if rest == "" {
		writeProblemAt(w, r, http.StatusBadRequest, "invalid_request", "missing upstream or action")
		return
	}
	// benchmark, reorder and delete are the actions.
	switch rest {
	case "benchmark":
		a.v3UpstreamBenchmark(w, r)
		return
	case "reorder":
		a.v3UpstreamReorder(w, r)
		return
	case "delete":
		a.v3UpstreamDelete(w, r)
		return
	}
	writeProblemAt(w, r, http.StatusNotFound, "not_found", "unknown upstream action")
}

// v3UpstreamDelete removes one entry from the pool, mirroring the dashboard's
// POST /api/upstreams/delete. POST rather than DELETE because the address rides in
// the body and removing the pool's last working resolver is not a safe verb; the
// canonical form is matched so a DoT/DoH string added through the dashboard can be
// removed by the same string.
func (a *API) v3UpstreamDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeProblemAt(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "POST only")
		return
	}
	if a.upstreams == nil {
		writeProblemAt(w, r, http.StatusServiceUnavailable, "unavailable", "no upstream pool")
		return
	}
	var req struct {
		Address string `json:"address"`
	}
	if !decodeStrict(w, r, &req) {
		return
	}
	addr := strings.TrimSpace(req.Address)
	if addr == "" {
		writeProblemAt(w, r, http.StatusBadRequest, "invalid_request", "address is required")
		return
	}
	canonical, ok := upstream.NormalizeUpstream(addr)
	if !ok {
		writeProblemAt(w, r, http.StatusBadRequest, "invalid_request", "invalid upstream address")
		return
	}
	current := a.upstreams.GetUpstreams()
	list := make([]string, 0, len(current))
	found := false
	for _, u := range current {
		if u == canonical {
			found = true
			continue
		}
		list = append(list, u)
	}
	if !found {
		writeProblemAt(w, r, http.StatusNotFound, "not_found", "upstream not found")
		return
	}
	a.upstreams.SetUpstreams(list)
	_ = json.NewEncoder(w).Encode(map[string]any{"items": list})
}

func (a *API) v3UpstreamBenchmark(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeProblemAt(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "POST only")
		return
	}
	if a.v3Backend == nil {
		v3BackendUnavailable(w, r)
		return
	}
	resp, err := a.v3Backend.Benchmark()
	if err != nil {
		writeProblemAt(w, r, backendProblemStatus(err), "invalid_request", err.Error())
		return
	}
	_ = json.NewEncoder(w).Encode(resp)
}

// v3UpstreamReorder replaces the pool with the caller's order. There is no
// reorder primitive on the pool, so this is SetUpstreams with the given list
// after validating every entry is already present exactly once.
func (a *API) v3UpstreamReorder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodPut {
		writeProblemAt(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "POST or PUT")
		return
	}
	if a.upstreams == nil {
		writeProblemAt(w, r, http.StatusServiceUnavailable, "unavailable", "no upstream pool")
		return
	}
	var req struct {
		Order []string `json:"order"`
	}
	if !decodeStrict(w, r, &req) {
		return
	}
	current := a.upstreams.GetUpstreams()
	want := make(map[string]bool, len(current))
	for _, u := range current {
		want[u] = true
	}
	if len(req.Order) != len(current) {
		writeProblemAt(w, r, http.StatusBadRequest, "invalid_request", "order must list every upstream exactly once")
		return
	}
	seen := make(map[string]bool, len(req.Order))
	canon := make([]string, 0, len(req.Order))
	for _, u := range req.Order {
		c, ok := upstream.NormalizeUpstream(strings.TrimSpace(u))
		if !ok || !want[c] || seen[c] {
			writeProblemAt(w, r, http.StatusBadRequest, "invalid_request", "order must list every upstream exactly once and contain no unknown entries")
			return
		}
		seen[c] = true
		canon = append(canon, c)
	}
	a.upstreams.SetUpstreams(canon)
	_ = json.NewEncoder(w).Encode(map[string]any{"items": canon})
}

func (a *API) handleV3Benchmark(w http.ResponseWriter, r *http.Request) {
	a.v3UpstreamBenchmark(w, r)
}

// -----------------------------------------------------------------------------
// Settings / config / TLS / subscription (delegated)
// -----------------------------------------------------------------------------

func (a *API) handleV3Settings(w http.ResponseWriter, r *http.Request) {
	if a.v3Backend == nil {
		v3BackendUnavailable(w, r)
		return
	}
	switch r.Method {
	case http.MethodGet, http.MethodHead:
		resp, err := a.v3Backend.Settings()
		if err != nil {
			writeProblemAt(w, r, http.StatusInternalServerError, "internal", err.Error())
			return
		}
		_ = json.NewEncoder(w).Encode(resp)
	case http.MethodPost, http.MethodPut:
		body, ok := v3Body(r)
		if !ok {
			writeProblemAt(w, r, http.StatusBadRequest, "invalid_request", "invalid JSON body")
			return
		}
		resp, err := a.v3Backend.SaveSettings(r, body)
		if err != nil {
			writeProblemAt(w, r, backendProblemStatus(err), "invalid_request", err.Error())
			return
		}
		_ = json.NewEncoder(w).Encode(resp)
	default:
		writeProblemAt(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "GET, HEAD, POST or PUT")
	}
}

func (a *API) handleV3Subscription(w http.ResponseWriter, r *http.Request) {
	if a.v3Backend == nil {
		v3BackendUnavailable(w, r)
		return
	}
	switch r.Method {
	case http.MethodGet, http.MethodHead:
		_ = json.NewEncoder(w).Encode(a.v3Backend.Subscription())
	case http.MethodPost, http.MethodPut:
		body, ok := v3Body(r)
		if !ok {
			writeProblemAt(w, r, http.StatusBadRequest, "invalid_request", "invalid JSON body")
			return
		}
		resp, err := a.v3Backend.SaveSubscription(r, body)
		if err != nil {
			writeProblemAt(w, r, backendProblemStatus(err), "invalid_request", err.Error())
			return
		}
		_ = json.NewEncoder(w).Encode(resp)
	default:
		writeProblemAt(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "GET, HEAD, POST or PUT")
	}
}

func (a *API) handleV3Config(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeProblemAt(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "GET or HEAD only")
		return
	}
	if a.v3Backend == nil {
		v3BackendUnavailable(w, r)
		return
	}
	_ = json.NewEncoder(w).Encode(a.v3Backend.Config())
}

// handleV3RegenerateAPIKey rotates the master REST key. It is a
// credential-changing act, so the backend re-asks for the current password and,
// when 2FA is on, a current code.
func (a *API) handleV3RegenerateAPIKey(w http.ResponseWriter, r *http.Request) {
	if a.v3Backend == nil {
		v3BackendUnavailable(w, r)
		return
	}
	if r.Method == http.MethodGet || r.Method == http.MethodHead {
		resp, err := a.v3Backend.Settings()
		if err != nil {
			writeProblemAt(w, r, http.StatusInternalServerError, "internal", err.Error())
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"api_key": resp["api_key"]})
		return
	}
	if r.Method != http.MethodPost {
		writeProblemAt(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "GET, HEAD or POST")
		return
	}
	var req struct {
		CurrentPassword string `json:"current_password"`
		Code            string `json:"code"`
	}
	if !decodeStrict(w, r, &req) {
		return
	}
	resp, err := a.v3Backend.RegenerateAPIKey(r, req.CurrentPassword, req.Code)
	if err != nil {
		writeProblemAt(w, r, backendProblemStatus(err), "invalid_request", err.Error())
		return
	}
	_ = json.NewEncoder(w).Encode(resp)
}

func (a *API) handleV3RegenerateAdminPath(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeProblemAt(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "POST only")
		return
	}
	if a.v3Backend == nil {
		v3BackendUnavailable(w, r)
		return
	}
	var req struct {
		Confirm bool `json:"confirm"`
	}
	if !decodeStrict(w, r, &req) {
		return
	}
	if !req.Confirm {
		writeProblemAt(w, r, http.StatusBadRequest, "invalid_request",
			"regenerating the admin path requires confirm=true — every bookmark, integration and session under the old path stops working immediately")
		return
	}
	resp, err := a.v3Backend.RegenerateAdminPath(r, true)
	if err != nil {
		writeProblemAt(w, r, backendProblemStatus(err), "invalid_request", err.Error())
		return
	}
	_ = json.NewEncoder(w).Encode(resp)
}

func (a *API) handleV3TLSIssue(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeProblemAt(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "POST only")
		return
	}
	if a.v3Backend == nil {
		v3BackendUnavailable(w, r)
		return
	}
	body, ok := v3Body(r)
	if !ok {
		writeProblemAt(w, r, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}
	resp, err := a.v3Backend.IssueTLS(r, body)
	if err != nil {
		writeProblemAt(w, r, backendProblemStatus(err), "invalid_request", err.Error())
		return
	}
	_ = json.NewEncoder(w).Encode(resp)
}

func (a *API) handleV3GeoIPStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeProblemAt(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "GET or HEAD only")
		return
	}
	if a.v3Backend == nil {
		v3BackendUnavailable(w, r)
		return
	}
	resp, err := a.v3Backend.GeoIPStatus()
	if err != nil {
		writeProblemAt(w, r, backendProblemStatus(err), "internal", err.Error())
		return
	}
	_ = json.NewEncoder(w).Encode(resp)
}

func (a *API) handleV3GeoIPUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeProblemAt(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "POST only")
		return
	}
	if a.v3Backend == nil {
		v3BackendUnavailable(w, r)
		return
	}
	resp, err := a.v3Backend.GeoIPUpdate(r)
	if err != nil {
		writeProblemAt(w, r, backendProblemStatus(err), "invalid_request", err.Error())
		return
	}
	_ = json.NewEncoder(w).Encode(resp)
}

func (a *API) handleV3TLSAutoRenew(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeProblemAt(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "POST only")
		return
	}
	if a.v3Backend == nil {
		v3BackendUnavailable(w, r)
		return
	}
	body, ok := v3Body(r)
	if !ok {
		writeProblemAt(w, r, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}
	resp, err := a.v3Backend.TLSAutoRenew(r, body)
	if err != nil {
		writeProblemAt(w, r, backendProblemStatus(err), "invalid_request", err.Error())
		return
	}
	_ = json.NewEncoder(w).Encode(resp)
}

func (a *API) handleV3ACMEStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeProblemAt(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "GET or HEAD only")
		return
	}
	if a.v3Backend == nil {
		v3BackendUnavailable(w, r)
		return
	}
	_ = json.NewEncoder(w).Encode(a.v3Backend.ACMEStatus())
}

// -----------------------------------------------------------------------------
// Stats, stream, cache, diagnostics, update
// -----------------------------------------------------------------------------

func (a *API) handleV3Stats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeProblemAt(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "GET or HEAD only")
		return
	}
	_ = json.NewEncoder(w).Encode(a.stats.GetLiveStats())
}

func (a *API) handleV3Stream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeProblemAt(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "GET only")
		return
	}
	a.stats.ServeSSE(w, r)
}

func (a *API) handleV3FlushCache(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeProblemAt(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "POST only")
		return
	}
	a.cache.Flush()
	_ = json.NewEncoder(w).Encode(map[string]any{"flushed": true})
}

func (a *API) handleV3DiagnosticsTargets(w http.ResponseWriter, r *http.Request) {
	if a.v3Backend == nil {
		v3BackendUnavailable(w, r)
		return
	}
	switch r.Method {
	case http.MethodGet, http.MethodHead:
		_ = json.NewEncoder(w).Encode(a.v3Backend.DiagnosticsTargets())
	case http.MethodPost:
		body, ok := v3Body(r)
		if !ok {
			writeProblemAt(w, r, http.StatusBadRequest, "invalid_request", "invalid JSON body")
			return
		}
		resp, err := a.v3Backend.AddDiagnosticsTarget(r, body)
		if err != nil {
			writeProblemAt(w, r, backendProblemStatus(err), "invalid_request", err.Error())
			return
		}
		_ = json.NewEncoder(w).Encode(resp)
	case http.MethodDelete:
		host := strings.TrimSpace(r.URL.Query().Get("host"))
		if host == "" {
			var req struct {
				Host string `json:"host"`
			}
			_ = decodeOptional(w, r, &req)
			host = strings.TrimSpace(req.Host)
		}
		resp, err := a.v3Backend.DeleteDiagnosticsTarget(r, host)
		if err != nil {
			writeProblemAt(w, r, backendProblemStatus(err), "invalid_request", err.Error())
			return
		}
		_ = json.NewEncoder(w).Encode(resp)
	default:
		writeProblemAt(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "GET, HEAD, POST or DELETE")
	}
}

func (a *API) handleV3DiagnosticsRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeProblemAt(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "POST only")
		return
	}
	var rec database.DiagnosticSettings
	if a.db != nil {
		_ = a.db.GetSetting("diagnostics", &rec)
	}
	report := service.RunDiagnostics(rec.GetTargets())
	_ = json.NewEncoder(w).Encode(map[string]any{
		"overall_score":   report.OverallScore,
		"overall_quality": report.OverallQuality,
		"avg_latency_ms":  report.AvgLatencyMs,
		"reachable":       report.Reachable,
		"total":           report.Total,
		"results":         report.Results,
		"timestamp":       report.Timestamp,
	})
}

func (a *API) handleV3UpdateCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeProblemAt(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "GET or HEAD only")
		return
	}
	if a.v3Backend == nil {
		v3BackendUnavailable(w, r)
		return
	}
	resp, err := a.v3Backend.UpdateCheck(r)
	if err != nil {
		writeProblemAt(w, r, backendProblemStatus(err), "invalid_request", err.Error())
		return
	}
	_ = json.NewEncoder(w).Encode(resp)
}

func (a *API) handleV3UpdateApply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeProblemAt(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "POST only")
		return
	}
	if a.v3Backend == nil {
		v3BackendUnavailable(w, r)
		return
	}
	resp, err := a.v3Backend.UpdateApply(r)
	if err != nil {
		writeProblemAt(w, r, backendProblemStatus(err), "invalid_request", err.Error())
		return
	}
	_ = json.NewEncoder(w).Encode(resp)
}

func (a *API) handleV3UpdateStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeProblemAt(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "GET or HEAD only")
		return
	}
	if a.v3Backend == nil {
		v3BackendUnavailable(w, r)
		return
	}
	_ = json.NewEncoder(w).Encode(a.v3Backend.UpdateStatus())
}

// decodeOptional decodes a body into dst, tolerating an empty body (some
// actions carry no fields). Unknown fields are still refused.
func decodeOptional(w http.ResponseWriter, r *http.Request, dst any) bool {
	if r.Body == nil || r.ContentLength == 0 {
		return true
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		// An empty body is acceptable for optional payloads; anything else is a
		// real decode error the caller must know about.
		if err.Error() == "EOF" {
			return true
		}
		writeProblemAt(w, r, http.StatusBadRequest, "invalid_request", err.Error())
		return false
	}
	return true
}

// backendProblemStatus maps a backend error to a problem status. The web
// backend returns typed sentinel errors for the cases v3 wants to report
// precisely; everything else is a 400 (the request was the problem).
func backendProblemStatus(err error) int {
	switch {
	case errorsIs(err, errV3Unauthorized):
		return http.StatusUnauthorized
	case errorsIs(err, errV3NotFound):
		return http.StatusNotFound
	case errorsIs(err, errV3Conflict):
		return http.StatusConflict
	case errorsIs(err, errV3Unavailable):
		return http.StatusServiceUnavailable
	default:
		return http.StatusBadRequest
	}
}
