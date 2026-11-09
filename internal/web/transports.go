package web

import (
	"encoding/json"
	"net/http"
	"strings"

	"digitdns/internal/httpx"
)

// transportSettingsView is the encrypted-DNS card's state for the two
// client-facing QUIC transports. Both are opt-in and applied live; the view
// always carries the effective port and auth mode, including the defaults an
// older "dns" record does not have yet.
type transportSettingsView struct {
	DoQEnabled     bool     `json:"doq_enabled"`
	DoQPort        int      `json:"doq_port"`
	DoQAuth        string   `json:"doq_auth"`
	DoQTokenDomain string   `json:"doq_token_domain"`
	DoH3Enabled    bool     `json:"doh3_enabled"`
	DoQTokens      []string `json:"doq_tokens"`
	// Actual listener state. A saved toggle is the desired state; these say
	// whether the bind succeeded, so a failed start is visible instead of
	// looking like a running endpoint.
	DoH3Listening bool `json:"doh3_listening"`
	DoQListening  bool `json:"doq_listening"`
}

const defaultDoQPort = 853

// transportSettingsView reads the saved flags and the token list. The tokens
// live in the shared "access" record, so they survive a DoH-token save and
// every other read-modify-write of that record.
func (ws *WebServer) transportSettingsView() transportSettingsView {
	ws.dnsMu.Lock()
	cfg := ws.mutableDNSLocked()
	view := transportSettingsView{
		DoQEnabled:     cfg.DoQEnabled,
		DoQPort:        cfg.DoQPort,
		DoQAuth:        cfg.DoQAuth,
		DoQTokenDomain: cfg.DoQTokenDomain,
		DoH3Enabled:    cfg.DoH3Enabled,
	}
	ws.dnsMu.Unlock()

	if view.DoQPort == 0 {
		view.DoQPort = defaultDoQPort
	}
	view.DoQAuth = normalizeDoQAuth(view.DoQAuth)
	view.DoQTokens = ws.loadAccessConfig().DoQTokens
	if view.DoQTokens == nil {
		view.DoQTokens = []string{}
	}
	if ws.transportStatus != nil {
		view.DoH3Listening, view.DoQListening = ws.transportStatus()
	}
	return view
}

func normalizeDoQAuth(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "ip":
		return "ip"
	case "sni-token":
		return "sni-token"
	default:
		return ""
	}
}

// normalizeDoQTokens lower-cases, trims and de-duplicates token labels. A
// label is also a DNS label because it becomes the leftmost SNI label, so the
// ACME label rules apply unchanged.
func normalizeDoQTokens(raw []string) ([]string, string) {
	seen := map[string]bool{}
	out := make([]string, 0, len(raw))
	for _, entry := range raw {
		token := strings.ToLower(strings.TrimSpace(strings.TrimSuffix(entry, ".")))
		if token == "" {
			continue
		}
		if err := validACMELabel(token); err != nil {
			return nil, "invalid DoQ token " + strings.TrimSpace(entry) + ": " + err.Error()
		}
		if seen[token] {
			continue
		}
		seen[token] = true
		out = append(out, token)
	}
	if out == nil {
		out = []string{}
	}
	return out, ""
}

// handleTransportSettings reads or writes the DoQ/DoH3 policy. A save is
// persisted first, then handed to the running server; a bind failure is
// reported to the caller and leaves the listener off.
func (ws *WebServer) handleTransportSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet, http.MethodHead:
		ws.writeTransportSettings(w)
	case http.MethodPost:
		ws.saveTransportSettings(w, r)
	default:
		httpx.WriteMethodNotAllowed(w, "GET, HEAD, POST")
	}
}

func (ws *WebServer) writeTransportSettings(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(ws.transportSettingsView())
}

func (ws *WebServer) saveTransportSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DoQEnabled     *bool     `json:"doq_enabled"`
		DoQPort        *int      `json:"doq_port"`
		DoQAuth        *string   `json:"doq_auth"`
		DoQTokenDomain *string   `json:"doq_token_domain"`
		DoH3Enabled    *bool     `json:"doh3_enabled"`
		DoQTokens      *[]string `json:"doq_tokens"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		httpx.WriteJSONError(w, http.StatusBadRequest, "Could not read the transport settings: "+err.Error())
		return
	}

	ws.dnsMu.Lock()
	cfg := ws.mutableDNSLocked()
	before := *cfg

	auth := normalizeDoQAuth(cfg.DoQAuth)
	if req.DoQAuth != nil {
		auth = normalizeDoQAuth(*req.DoQAuth)
		if auth == "" {
			ws.dnsMu.Unlock()
			httpx.WriteJSONError(w, http.StatusBadRequest, "doq_auth must be \"ip\" or \"sni-token\"")
			return
		}
	}
	port := cfg.DoQPort
	if port == 0 {
		port = defaultDoQPort
	}
	if req.DoQPort != nil {
		port = *req.DoQPort
		if port == 0 {
			port = defaultDoQPort
		}
		if port < 1 || port > 65535 {
			ws.dnsMu.Unlock()
			httpx.WriteJSONError(w, http.StatusBadRequest, "doq_port must be between 1 and 65535")
			return
		}
	}

	tokenDomain := strings.ToLower(strings.TrimSpace(cfg.DoQTokenDomain))
	if req.DoQTokenDomain != nil {
		tokenDomain = strings.ToLower(strings.TrimSpace(*req.DoQTokenDomain))
		if tokenDomain != "" {
			normalized, err := validACMEDomain(tokenDomain)
			if err != nil {
				ws.dnsMu.Unlock()
				httpx.WriteJSONError(w, http.StatusBadRequest, "doq_token_domain: "+err.Error())
				return
			}
			tokenDomain = normalized
		}
	}

	access := ws.loadAccessConfig()
	tokens := access.DoQTokens
	if req.DoQTokens != nil {
		normalized, problem := normalizeDoQTokens(*req.DoQTokens)
		if problem != "" {
			ws.dnsMu.Unlock()
			httpx.WriteJSONError(w, http.StatusBadRequest, problem)
			return
		}
		tokens = normalized
	}
	if auth == "sni-token" {
		if tokenDomain == "" {
			ws.dnsMu.Unlock()
			httpx.WriteJSONError(w, http.StatusBadRequest, "doq_token_domain is required when doq_auth is \"sni-token\"")
			return
		}
		if len(tokens) == 0 {
			ws.dnsMu.Unlock()
			httpx.WriteJSONError(w, http.StatusBadRequest, "at least one DoQ token is required when doq_auth is \"sni-token\"")
			return
		}
	}

	if req.DoQEnabled != nil {
		cfg.DoQEnabled = *req.DoQEnabled
	}
	if req.DoH3Enabled != nil {
		cfg.DoH3Enabled = *req.DoH3Enabled
	}
	cfg.DoQPort = port
	cfg.DoQAuth = auth
	cfg.DoQTokenDomain = tokenDomain
	err := ws.db.SetSetting("dns", cfg)
	if err != nil {
		*cfg = before
	}
	saved := *cfg
	ws.dnsMu.Unlock()
	if err != nil {
		httpx.WriteJSONError(w, http.StatusInternalServerError, "Could not persist the transport settings")
		return
	}

	if req.DoQTokens != nil {
		access.DoQTokens = tokens
		if err := ws.db.SetSetting("access", access); err != nil {
			httpx.WriteJSONError(w, http.StatusInternalServerError, "Could not persist the DoQ tokens")
			return
		}
	}

	if ws.transportApplier != nil {
		if err := ws.transportApplier(saved, access.DoQTokens); err != nil {
			httpx.WriteJSONError(w, http.StatusInternalServerError, "The transport settings were saved, but the listener could not start: "+err.Error())
			return
		}
	}
	ws.writeTransportSettings(w)
}
