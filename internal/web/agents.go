package web

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"digitdns/internal/database"
	"digitdns/internal/httpx"
)

// Geo agent endpoints.
//
// Two audiences, deliberately kept apart:
//
//	/api/agents…   the panel. Admin session only.
//	/agent/…       the agents themselves. Key auth, never an admin session.
//
// The split matters: an admin session must not be able to impersonate an agent,
// and an agent key must not reach a panel route. They share no middleware.

const agentKeyHeader = "X-Agent-Key"

// handleAgents is the panel collection endpoint: GET lists, POST mints a
// one-time enrolment token.
func (ws *WebServer) handleAgents(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		agents, err := ws.db.ListAgents()
		if err != nil {
			httpx.WriteJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if agents == nil {
			agents = []database.Agent{}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"agents": agents})

	case http.MethodPost:
		var req struct {
			Country string `json:"country"`
			Name    string `json:"name"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBody)).Decode(&req); err != nil {
			httpx.WriteJSONError(w, http.StatusBadRequest, "the request body could not be read as JSON")
			return
		}
		if strings.TrimSpace(req.Country) == "" {
			httpx.WriteJSONError(w, http.StatusBadRequest, "a country is required")
			return
		}
		agent, key, err := ws.db.CreateAgent(req.Country, req.Name)
		if err != nil {
			httpx.WriteJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		ws.refreshRelayAgents()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		// The clear key is returned here and can be copied again through the
		// authenticated /key action. The database keeps an encrypted copy; list
		// responses never carry it.
		_ = json.NewEncoder(w).Encode(map[string]any{
			"agent": agent,
			"key":   key,
		})

	default:
		httpx.WriteMethodNotAllowed(w, "GET, POST")
	}
}

// handleAgentByID is the panel item endpoint. The path is /api/agents/<id> and
// an optional action suffix: /api/agents/<id>/disable, /enable.
func (ws *WebServer) handleAgentByID(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/agents/")
	parts := strings.SplitN(strings.Trim(rest, "/"), "/", 2)
	id := parts[0]
	action := ""
	if len(parts) == 2 {
		action = parts[1]
	}
	if id == "" {
		httpx.WriteJSONError(w, http.StatusBadRequest, "an agent id is required")
		return
	}

	switch r.Method {
	case http.MethodGet:
		if action != "key" {
			httpx.WriteJSONError(w, http.StatusBadRequest, "expected /key")
			return
		}
		key, err := ws.db.AgentKey(id)
		if err != nil {
			status := http.StatusInternalServerError
			switch {
			case errors.Is(err, database.ErrAgentNotFound):
				status = http.StatusNotFound
			case errors.Is(err, database.ErrAgentKeyUnavailable):
				status = http.StatusConflict
			}
			httpx.WriteJSONError(w, status, err.Error())
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"key": key})

	case http.MethodDelete:
		if action != "" {
			httpx.WriteJSONError(w, http.StatusBadRequest, "unexpected action "+action)
			return
		}
		if err := ws.db.DeleteAgent(id); err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, database.ErrAgentNotFound) {
				status = http.StatusNotFound
			}
			httpx.WriteJSONError(w, status, err.Error())
			return
		}
		ws.pruneAgentAssignments(id)
		ws.refreshRelayAgents()
		w.WriteHeader(http.StatusNoContent)

	case http.MethodPost:
		switch action {
		case "enable", "disable":
			if err := ws.db.SetAgentEnabled(id, action == "enable"); err != nil {
				status := http.StatusInternalServerError
				if errors.Is(err, database.ErrAgentNotFound) {
					status = http.StatusNotFound
				}
				httpx.WriteJSONError(w, status, err.Error())
				return
			}
			ws.refreshRelayAgents()
			w.WriteHeader(http.StatusNoContent)
		case "rotate-key":
			key, err := ws.db.RotateAgentKey(id)
			if err != nil {
				status := http.StatusInternalServerError
				if errors.Is(err, database.ErrAgentNotFound) {
					status = http.StatusNotFound
				}
				httpx.WriteJSONError(w, status, err.Error())
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"key": key})
		case "test":
			ws.handleAgentTest(w, id)
		default:
			httpx.WriteJSONError(w, http.StatusBadRequest, "expected /enable, /disable, /rotate-key or /test")
		}

	case http.MethodPut:
		if action != "" {
			httpx.WriteJSONError(w, http.StatusBadRequest, "unexpected action "+action)
			return
		}
		var req struct {
			Name     string `json:"name"`
			Country  string `json:"country"`
			PublicIP string `json:"public_ip"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBody)).Decode(&req); err != nil {
			httpx.WriteJSONError(w, http.StatusBadRequest, "the request body could not be read as JSON")
			return
		}
		country := strings.ToUpper(strings.TrimSpace(req.Country))
		if len(country) != 2 {
			httpx.WriteJSONError(w, http.StatusBadRequest, "a two-letter country code is required")
			return
		}
		if err := ws.db.UpdateAgentDetails(id, req.Name, country, req.PublicIP); err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, database.ErrAgentNotFound) {
				status = http.StatusNotFound
			}
			httpx.WriteJSONError(w, status, err.Error())
			return
		}
		ws.refreshRelayAgents()
		agent, err := ws.db.GetAgent(id)
		if err != nil {
			httpx.WriteJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"agent": agent})

	default:
		httpx.WriteMethodNotAllowed(w, "DELETE, GET, POST, PUT")
	}
}

// agentCatalogue is what an agent syncs: the host allowlist and the client
// allowlist for its country, both computed here so the agent never interprets a
// rule.
type agentCatalogue struct {
	AgentID  string   `json:"agent_id"`
	Country  string   `json:"country"`
	Hosts    []string `json:"hosts"`
	Clients  []string `json:"clients"`
	SyncedAt string   `json:"synced_at"`
}

// agentFromRequest authenticates an agent by its key.
func (ws *WebServer) agentFromRequest(r *http.Request) (database.Agent, bool) {
	raw := r.Header.Get(agentKeyHeader)
	if raw == "" {
		// Also accept a bearer token so the agent can keep one header for
		// everything if that is simpler on its side.
		if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
			raw = strings.TrimPrefix(h, "Bearer ")
		}
	}
	id, secret, ok := strings.Cut(raw, ":")
	if !ok || id == "" || secret == "" {
		return database.Agent{}, false
	}
	agent, err := ws.db.AuthenticateAgent(id, secret)
	if err != nil {
		return database.Agent{}, false
	}
	return agent, true
}

// handleAgentEnrol consumes a one-time token and issues the agent its key.
func (ws *WebServer) handleAgentEnrol(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.WriteMethodNotAllowed(w, "POST")
		return
	}
	var req struct {
		Token   string `json:"token"`
		Version string `json:"version"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBody)).Decode(&req); err != nil {
		httpx.WriteJSONError(w, http.StatusBadRequest, "the request body could not be read as JSON")
		return
	}

	// The agent's public address is what it arrives from, not what it claims:
	// a box cannot be talked into believing it is somewhere else.
	ip := clientIP(r)
	agent, secret, err := ws.db.EnrolAgent(strings.TrimSpace(req.Token), ip, req.Version)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, database.ErrTokenInvalid) {
			status = http.StatusUnauthorized
		}
		httpx.WriteJSONError(w, status, err.Error())
		return
	}

	ws.refreshRelayAgents()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"agent": agent,
		// The key is shown once. Only its hash is stored.
		"key":    agent.ID + ":" + secret,
		"issued": agent.CreatedAt,
	})
}

// handleAgentHeartbeat records liveness and traffic totals.
func (ws *WebServer) handleAgentHeartbeat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.WriteMethodNotAllowed(w, "POST")
		return
	}
	agent, ok := ws.agentFromRequest(r)
	if !ok {
		httpx.WriteJSONError(w, http.StatusUnauthorized, "agent key is not valid")
		return
	}

	var req struct {
		Version   string `json:"version"`
		RelayPort int    `json:"relay_port"`
		BytesIn   uint64 `json:"bytes_in"`
		BytesOut  uint64 `json:"bytes_out"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBody)).Decode(&req); err != nil {
		httpx.WriteJSONError(w, http.StatusBadRequest, "the request body could not be read as JSON")
		return
	}
	if err := ws.db.TouchAgent(agent.ID, req.Version, clientIP(r), req.RelayPort, req.BytesIn, req.BytesOut); err != nil {
		httpx.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	ws.refreshRelayAgents()
	w.WriteHeader(http.StatusNoContent)
}

// handleAgentCatalogue returns the allowlists for this agent: the active proxied
// domains assigned directly or through a service default, plus every client the
// hub would admit. An agent with no assignments receives an empty host list and
// refuses everything, which is the safe default.
func (ws *WebServer) handleAgentCatalogue(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.WriteMethodNotAllowed(w, "GET")
		return
	}
	agent, ok := ws.agentFromRequest(r)
	if !ok {
		httpx.WriteJSONError(w, http.StatusUnauthorized, "agent key is not valid")
		return
	}

	rules := ws.loadRulesConfig()
	domainAgents, _ := rules["domain_agents"].(map[string]string)
	serviceAgents, _ := rules["service_agents"].(map[string]string)
	domainServices, _ := rules["domain_services"].(map[string]string)
	activeProxied := map[string]bool{}
	for _, d := range toStringSlice(rules["custom_proxied"]) {
		activeProxied[strings.TrimSuffix(strings.ToLower(strings.TrimSpace(d)), ".")] = true
	}

	assigned := map[string]string{} // domain pattern -> assignment source key
	for pattern, id := range domainAgents {
		if id == agent.ID {
			assigned[normalizeAgentPattern(pattern)] = pattern
		}
	}
	for serviceID, id := range serviceAgents {
		if id != agent.ID {
			continue
		}
		for d, sid := range domainServices {
			if sid != serviceID {
				continue
			}
			d = normalizeAgentPattern(d)
			if _, overridden := domainAgents[d]; overridden {
				continue
			}
			assigned[d] = serviceID
		}
	}

	seen := map[string]struct{}{}
	hosts := make([]string, 0, len(assigned)*2)
	add := func(h string) {
		h = normalizeAgentPattern(h)
		if h == "" {
			return
		}
		if _, dup := seen[h]; dup {
			return
		}
		seen[h] = struct{}{}
		hosts = append(hosts, h)
	}
	for pattern := range assigned {
		if !activeProxied[strings.TrimPrefix(pattern, "*.")] {
			continue
		}
		add(pattern)
		// A plain rule covers its subdomains in the hub matcher; the agent's
		// allowlist is exact-or-wildcard, so spell the implicit wildcard out.
		if !strings.HasPrefix(pattern, "*.") {
			add("*." + pattern)
		}
	}
	sort.Strings(hosts)

	clients, err := ws.panelClientIPs()
	if err != nil {
		httpx.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(agentCatalogue{
		AgentID:  agent.ID,
		Country:  agent.Country,
		Hosts:    hosts,
		Clients:  clients,
		SyncedAt: time.Now().UTC().Format(time.RFC3339),
	})
}

// handleAgentTest performs the operator-facing connection test: a TCP connect
// to the agent's advertised relay port. That is a real network round trip, not a
// heartbeat-age guess. The measured latency is stored so the card can show it
// until the next test.
func (ws *WebServer) handleAgentTest(w http.ResponseWriter, id string) {
	agent, err := ws.db.GetAgent(id)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, database.ErrAgentNotFound) {
			status = http.StatusNotFound
		}
		httpx.WriteJSONError(w, status, err.Error())
		return
	}
	if agent.PublicIP == "" {
		httpx.WriteJSONError(w, http.StatusConflict, "the agent has not reported a public IPv4 address yet")
		return
	}
	port := agent.RelayPort
	if port == 0 {
		port = 443
	}
	start := time.Now()
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(agent.PublicIP, strconv.Itoa(port)), 3*time.Second)
	ms := float64(time.Since(start).Microseconds()) / 1000.0
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":        false,
			"error":     err.Error(),
			"ping_ms":   ms,
			"address":   net.JoinHostPort(agent.PublicIP, strconv.Itoa(port)),
			"last_seen": agent.LastSeen,
		})
		return
	}
	_ = conn.Close()
	if err := ws.db.RecordAgentPing(id, ms); err != nil {
		httpx.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	agent, _ = ws.db.GetAgent(id)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":        true,
		"ping_ms":   ms,
		"address":   net.JoinHostPort(agent.PublicIP, strconv.Itoa(port)),
		"agent":     agent,
		"last_seen": agent.LastSeen,
	})
}

// pruneAgentAssignments removes a deleted agent from both assignment maps, so
// the next DNS query falls back to the local relay instead of naming a record
// that no longer exists.
func (ws *WebServer) pruneAgentAssignments(agentID string) {
	rules := ws.loadRulesConfig()
	domainAgents, _ := rules["domain_agents"].(map[string]string)
	serviceAgents, _ := rules["service_agents"].(map[string]string)
	domainServices, _ := rules["domain_services"].(map[string]string)
	for d, id := range domainAgents {
		if id == agentID {
			delete(domainAgents, d)
		}
	}
	for id, aid := range serviceAgents {
		if aid == agentID {
			delete(serviceAgents, id)
		}
	}
	writeMap := func(key, name string, m map[string]string) error {
		entries := make([]string, 0, len(m))
		for k, v := range m {
			entries = append(entries, k+"="+v)
		}
		return ws.db.SavePolicy(database.Policy{
			Key: key, Name: name, Category: "custom", Enabled: true, CustomDomains: entries,
		})
	}
	_ = writeMap("domain_agents", "Domain Relay Agents", domainAgents)
	_ = writeMap("service_agents", "Service Relay Agents", serviceAgents)
	ws.refreshRelayAssignments(domainAgents, serviceAgents, domainServices)
}

// panelClientIPs collects every registered subscriber IP. Without this the agent
// would relay for anyone who learned its address, because a client that bypasses
// DNS never meets the hub's own checks.
func (ws *WebServer) panelClientIPs() ([]string, error) {
	// "any" is the agent's explicit open mode. It mirrors allow_all, so an
	// install that admits everyone does not accidentally make every geo agent
	// refuse them.
	if ws.clients.IsAllowAll() {
		return []string{"any"}, nil
	}
	clients, err := ws.clients.ListClients()
	if err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	out := []string{}
	add := func(ip string) {
		ip = strings.TrimSpace(ip)
		if ip == "" {
			return
		}
		if _, dup := seen[ip]; dup {
			return
		}
		seen[ip] = struct{}{}
		out = append(out, ip)
	}
	for _, c := range clients {
		for _, ip := range c.AllowedIPs {
			add(ip)
		}
	}
	// Server-wide resolver CIDRs are allowed by the hub even when they do not
	// belong to a subscriber account; the agent must make the same decision.
	for _, ip := range ws.clients.ResolverAllowCIDRs() {
		add(ip)
	}
	return out, nil
}

func normalizeAgentPattern(d string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(d)), ".")
}

// clientIP returns the address a request arrived from, ignoring forwarding
// headers: an agent's identity must be the address it actually connected from,
// or a caller could claim to be anywhere.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
