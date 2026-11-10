package web

import (
	"encoding/json"
	"log"
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
	DoH3Port       int      `json:"doh3_port"`
	// Actual listener state. A saved toggle is the desired state; these say
	// whether the bind succeeded, so a failed start is visible instead of
	// looking like a running endpoint.
	DoH3Listening bool `json:"doh3_listening"`
	DoQListening  bool `json:"doq_listening"`
	// IPAllowlist explains where DoQ's "IP allowlist" mode gets its sources.
	// Both lists are shared with the resolver: per-client AllowedIPs and the
	// server-wide resolver allowlist. AllowAll is the live access mode.
	IPAllowlist ipAllowlistStatus `json:"ip_allowlist"`
	// LastError is the persistent reason from the last failed apply. Cleared on
	// the next successful save so the card can explain a bind/cert failure long
	// after the toast has gone.
	LastError string `json:"last_error"`
}

// ipAllowlistStatus is the read-only description of the IP gate behind DoQ. It
// carries counts rather than the addresses themselves: the settings card only
// needs to say where the entries come from and whether the gate is actually
// restricting access.
type ipAllowlistStatus struct {
	AllowAll        bool `json:"allow_all"`
	ClientAccounts  int  `json:"client_accounts"`
	ClientEntries   int  `json:"client_entries"`
	ResolverEntries int  `json:"resolver_entries"`
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
		DoH3Port:       cfg.DoHPort,
	}
	ws.dnsMu.Unlock()

	if view.DoQPort == 0 {
		view.DoQPort = defaultDoQPort
	}
	if view.DoH3Port == 0 {
		view.DoH3Port = 8443
	}
	view.DoQAuth = normalizeDoQAuth(view.DoQAuth)
	access := ws.loadAccessConfig()
	view.DoQTokens = access.DoQTokens
	if view.DoQTokens == nil {
		view.DoQTokens = []string{}
	}
	accounts, entries, resolverEntries, allowAll := ws.clients.AllowlistStats()
	view.IPAllowlist = ipAllowlistStatus{
		AllowAll:        allowAll,
		ClientAccounts:  accounts,
		ClientEntries:   entries,
		ResolverEntries: resolverEntries,
	}
	var listenerErr string
	if ws.transportStatus != nil {
		view.DoH3Listening, view.DoQListening, listenerErr = ws.transportStatus()
	}
	ws.transportErrMu.Lock()
	view.LastError = ws.transportLastError
	ws.transportErrMu.Unlock()
	// A save failure is the more specific explanation; otherwise surface the
	// asynchronous listener failure (a Serve goroutine that died after the save
	// returned, or a listener that failed at boot).
	if view.LastError == "" {
		view.LastError = listenerErr
	}
	return view
}

func (ws *WebServer) setTransportLastError(msg string) {
	ws.transportErrMu.Lock()
	ws.transportLastError = msg
	ws.transportErrMu.Unlock()
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

	// A rapid switch toggle must not interleave two apply/persist transactions.
	ws.transportSaveMu.Lock()
	defer ws.transportSaveMu.Unlock()

	ws.dnsMu.Lock()
	cfg := ws.mutableDNSLocked()
	before := *cfg
	ws.dnsMu.Unlock()

	next := before

	auth := normalizeDoQAuth(before.DoQAuth)
	if req.DoQAuth != nil {
		auth = normalizeDoQAuth(*req.DoQAuth)
		if auth == "" {
			httpx.WriteJSONError(w, http.StatusBadRequest, "doq_auth must be \"ip\" or \"sni-token\"")
			return
		}
	}
	port := before.DoQPort
	if port == 0 {
		port = defaultDoQPort
	}
	if req.DoQPort != nil {
		port = *req.DoQPort
		if port == 0 {
			port = defaultDoQPort
		}
		if port < 1 || port > 65535 {
			httpx.WriteJSONError(w, http.StatusBadRequest, "doq_port must be between 1 and 65535")
			return
		}
	}

	tokenDomain := strings.ToLower(strings.TrimSpace(before.DoQTokenDomain))
	if req.DoQTokenDomain != nil {
		tokenDomain = strings.ToLower(strings.TrimSpace(*req.DoQTokenDomain))
		if tokenDomain != "" {
			normalized, err := validACMEDomain(tokenDomain)
			if err != nil {
				httpx.WriteJSONError(w, http.StatusBadRequest, "doq_token_domain: "+err.Error())
				return
			}
			tokenDomain = normalized
		}
	}

	access := ws.loadAccessConfig()
	beforeTokens := access.DoQTokens
	tokens := beforeTokens
	if req.DoQTokens != nil {
		normalized, problem := normalizeDoQTokens(*req.DoQTokens)
		if problem != "" {
			httpx.WriteJSONError(w, http.StatusBadRequest, problem)
			return
		}
		tokens = normalized
	}
	if auth == "sni-token" {
		if tokenDomain == "" {
			httpx.WriteJSONError(w, http.StatusBadRequest, "doq_token_domain is required when doq_auth is \"sni-token\"")
			return
		}
		if len(tokens) == 0 {
			httpx.WriteJSONError(w, http.StatusBadRequest, "at least one DoQ token is required when doq_auth is \"sni-token\"")
			return
		}
	}

	if req.DoQEnabled != nil {
		next.DoQEnabled = *req.DoQEnabled
	}
	if req.DoH3Enabled != nil {
		next.DoH3Enabled = *req.DoH3Enabled
	}
	next.DoQPort = port
	next.DoQAuth = auth
	next.DoQTokenDomain = tokenDomain

	rollback := func(reason string) {
		ws.setTransportLastError(reason)
		log.Printf("[Transports] %s", reason)
		if ws.transportApplier != nil {
			if err := ws.transportApplier(before, beforeTokens); err != nil {
				log.Printf("[Transports] rollback failed: %v", err)
			}
		}
	}

	if ws.transportApplier != nil {
		if err := ws.transportApplier(next, tokens); err != nil {
			rollback("apply failed: " + err.Error())
			httpx.WriteJSONError(w, http.StatusInternalServerError, "The transport settings were not applied: "+err.Error())
			return
		}
	}

	if req.DoQTokens != nil {
		nextAccess := access
		nextAccess.DoQTokens = tokens
		if err := ws.db.SetSetting("access", nextAccess); err != nil {
			rollback("could not persist the DoQ tokens: " + err.Error())
			httpx.WriteJSONError(w, http.StatusInternalServerError, "Could not persist the DoQ tokens")
			return
		}
	}

	ws.dnsMu.Lock()
	current := ws.mutableDNSLocked()
	current.DoQEnabled = next.DoQEnabled
	current.DoH3Enabled = next.DoH3Enabled
	current.DoQPort = next.DoQPort
	current.DoQAuth = next.DoQAuth
	current.DoQTokenDomain = next.DoQTokenDomain
	err := ws.db.SetSetting("dns", current)
	ws.dnsMu.Unlock()
	if err != nil {
		ws.dnsMu.Lock()
		current.DoQEnabled = before.DoQEnabled
		current.DoH3Enabled = before.DoH3Enabled
		current.DoQPort = before.DoQPort
		current.DoQAuth = before.DoQAuth
		current.DoQTokenDomain = before.DoQTokenDomain
		ws.dnsMu.Unlock()
		if req.DoQTokens != nil {
			_ = ws.db.SetSetting("access", access)
		}
		rollback("could not persist the transport settings: " + err.Error())
		httpx.WriteJSONError(w, http.StatusInternalServerError, "Could not persist the transport settings")
		return
	}

	ws.setTransportLastError("")
	ws.writeTransportSettings(w)
}
