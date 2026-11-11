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
	"digitdns/internal/netutil"
	"digitdns/internal/service"
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

// agentView is the panel shape of an agent: the stored record plus the two
// fields that are derived from time. Computing them here keeps the dashboard's
// online threshold in step with the DNS router's.
type agentView struct {
	database.Agent
	Online         bool  `json:"online"`
	SyncAgeSeconds int64 `json:"sync_age_seconds"`
}

func (ws *WebServer) agentView(a database.Agent) agentView {
	view := agentView{Agent: a}
	view.Online = a.Enabled && !a.LastSeen.IsZero() && time.Since(a.LastSeen) <= service.AgentOnlineFor
	if !a.CatalogueSyncedAt.IsZero() {
		view.SyncAgeSeconds = int64(time.Since(a.CatalogueSyncedAt).Seconds())
		if view.SyncAgeSeconds < 0 {
			view.SyncAgeSeconds = 0
		}
	}
	return view
}

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
		views := make([]agentView, len(agents))
		for i, a := range agents {
			views[i] = ws.agentView(a)
		}
		body, err := json.Marshal(map[string]any{"agents": views})
		if err != nil {
			httpx.WriteJSONError(w, http.StatusInternalServerError, "could not encode agents")
			return
		}
		httpx.WriteJSONBytes(w, r, http.StatusOK, body, "application/json")

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
			Name        string  `json:"name"`
			Country     string  `json:"country"`
			PublicIP    string  `json:"public_ip"`
			PublicIPv6  *string `json:"public_ipv6"`
			IPv6Enabled *bool   `json:"ipv6_enabled"`
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
		current, err := ws.db.GetAgent(id)
		if err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, database.ErrAgentNotFound) {
				status = http.StatusNotFound
			}
			httpx.WriteJSONError(w, status, err.Error())
			return
		}
		publicIPv6 := current.PublicIPv6
		if req.PublicIPv6 != nil {
			publicIPv6 = strings.TrimSpace(*req.PublicIPv6)
		}
		if publicIPv6 != "" {
			ip, ok := netutil.ParsePublicIPv6(publicIPv6)
			if !ok {
				httpx.WriteJSONError(w, http.StatusBadRequest, "public_ipv6 must be a global IPv6 address")
				return
			}
			publicIPv6 = ip.String()
		}
		ipv6Enabled := current.IPv6Enabled
		if req.IPv6Enabled != nil {
			ipv6Enabled = *req.IPv6Enabled
		}
		if err := ws.db.UpdateAgentDetails(id, req.Name, country, req.PublicIP, publicIPv6, ipv6Enabled); err != nil {
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
	// IPv6Enabled is the effective policy: the per-agent opt-in AND the global
	// Proxy IPv6 master switch. The agent uses it to rebind IPv4-only and to
	// stop detecting/reporting IPv6.
	IPv6Enabled bool `json:"ipv6_enabled"`
}

// agentIPv6Effective combines the per-agent opt-in with the global master
// switch. The agent is told the effective value, so turning the global switch
// off stops every agent's IPv6 listener rather than only hiding DNS answers.
func (ws *WebServer) agentIPv6Effective(agent database.Agent) bool {
	if !agent.IPv6Enabled {
		return false
	}
	if ws.settings == nil {
		return true
	}
	return ws.settings.Snapshot().ProxyIPv6Enabled
}

// agentCatalogueCacheKey scopes an encoded catalogue to the revision and the
// per-agent fields that change its body. The revision covers the global rules,
// clients and assignments; country and the effective IPv6 flag cover the rest.
func (ws *WebServer) agentCatalogueCacheKey(agent database.Agent, revision string) string {
	return revision + "|" + agent.ID + "|" + agent.Country + "|" +
		strconv.FormatBool(agent.IPv6Enabled) + "|" + strconv.FormatBool(ws.agentIPv6Effective(agent))
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
		Version           string `json:"version"`
		RelayPort         int    `json:"relay_port"`
		BytesIn           uint64 `json:"bytes_in"`
		BytesOut          uint64 `json:"bytes_out"`
		PublicIPv6        string `json:"public_ipv6"`
		CatalogueRevision string `json:"catalogue_revision"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBody)).Decode(&req); err != nil {
		httpx.WriteJSONError(w, http.StatusBadRequest, "the request body could not be read as JSON")
		return
	}
	// The reported value wins; the source address of the heartbeat is the
	// fallback when the agent itself could not detect an address.
	publicIPv6 := strings.TrimSpace(req.PublicIPv6)
	if publicIPv6 == "" {
		publicIPv6 = clientIP(r)
	}
	updated, err := ws.db.TouchAgent(agent.ID, req.Version, clientIP(r), publicIPv6, req.RelayPort, req.BytesIn, req.BytesOut, req.CatalogueRevision)
	if err != nil {
		httpx.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Update just this agent in the live router. TouchAgent returns the record it
	// wrote, so the handler no longer needs a second read; the next DNS answer
	// still sees this agent's new address and liveness.
	if ws.relayRouter != nil {
		ws.relayRouter.UpdateAgent(updated)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ipv6_enabled":    ws.agentIPv6Effective(agent),
		"config_revision": ws.agentRevision(),
	})
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
	ws.agentCatRequests.Add(1)

	etag := ws.agentRevision()
	ifNone := strings.TrimSpace(r.Header.Get("If-None-Match"))
	wait := 0
	if ifNone != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("wait"))); err == nil {
			wait = n
		}
		if wait > 25 {
			wait = 25
		}
		if wait < 0 {
			wait = 0
		}
	}
	deadline := time.Now().Add(time.Duration(wait) * time.Second)

	for {
		if ifNone != etag {
			// Re-read the agent: its country or IPv6 policy may have changed
			// while this request was parked in the long-poll.
			fresh, err := ws.db.GetAgent(agent.ID)
			if err != nil {
				httpx.WriteJSONError(w, http.StatusUnauthorized, "agent key is not valid")
				return
			}
			cacheKey := ws.agentCatalogueCacheKey(fresh, etag)
			ws.agentCatBodyMu.Lock()
			body := ws.agentCatBodies[cacheKey]
			ws.agentCatBodyMu.Unlock()
			if body == nil {
				cat, err := ws.buildAgentCatalogue(fresh)
				if err != nil {
					httpx.WriteJSONError(w, http.StatusInternalServerError, err.Error())
					return
				}
				body, err = json.Marshal(cat)
				if err != nil {
					httpx.WriteJSONError(w, http.StatusInternalServerError, "could not encode catalogue")
					return
				}
				ws.agentCatBodyMu.Lock()
				if ws.agentCatBodies == nil {
					ws.agentCatBodies = make(map[string][]byte)
				}
				if len(ws.agentCatBodies) > 512 {
					ws.agentCatBodies = make(map[string][]byte)
				}
				ws.agentCatBodies[cacheKey] = body
				ws.agentCatBodyMu.Unlock()
			}
			w.Header().Set("ETag", etag)
			w.Header().Set("Cache-Control", "no-cache")
			httpx.WriteJSONBytes(w, r, http.StatusOK, body, "application/json")
			return
		}

		remaining := time.Until(deadline)
		if wait == 0 || remaining <= 0 {
			ws.agentCatNotMod.Add(1)
			w.Header().Set("ETag", etag)
			w.Header().Set("Cache-Control", "no-cache")
			w.WriteHeader(http.StatusNotModified)
			return
		}
		// A 10-second safety tick means a missed notification delays a change by
		// seconds rather than until the next scheduled sync. The revision check
		// itself is a single atomic load.
		tick := remaining
		if tick > 10*time.Second {
			tick = 10 * time.Second
		}
		ws.agentCatParked.Add(1)
		ws.agentNotify.wait(r.Context(), tick)
		ws.agentCatParked.Add(-1)
		if r.Context().Err() != nil {
			return
		}
		etag = ws.agentRevision()
	}
}

// buildAgentCatalogue computes the allowlists and policy for one agent. It is
// deliberately separate from the HTTP handler so the ETag/304 fast path can
// avoid this work entirely.
func (ws *WebServer) buildAgentCatalogue(agent database.Agent) (agentCatalogue, error) {
	ws.agentCatBuilds.Add(1)
	rules, clients, err := ws.catalogueInputs()
	if err != nil {
		return agentCatalogue{}, err
	}
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

	return agentCatalogue{
		AgentID:     agent.ID,
		Country:     agent.Country,
		Hosts:       hosts,
		Clients:     clients,
		SyncedAt:    time.Now().UTC().Format(time.RFC3339),
		IPv6Enabled: ws.agentIPv6Effective(agent),
	}, nil
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
	candidates := make([]string, 0, 2)
	if agent.PublicIP != "" {
		candidates = append(candidates, agent.PublicIP)
	}
	if agent.PublicIPv6 != "" {
		candidates = append(candidates, agent.PublicIPv6)
	}
	if len(candidates) == 0 {
		httpx.WriteJSONError(w, http.StatusConflict, "the agent has not reported a public address yet")
		return
	}
	port := agent.RelayPort
	if port == 0 {
		port = 443
	}
	var (
		used    string
		ms      float64
		lastErr error
	)
	for _, host := range candidates {
		start := time.Now()
		conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, strconv.Itoa(port)), 3*time.Second)
		if err != nil {
			lastErr = err
			continue
		}
		_ = conn.Close()
		used = host
		ms = float64(time.Since(start).Microseconds()) / 1000.0
		break
	}
	if used == "" {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ok":        false,
			"error":     lastErr.Error(),
			"ping_ms":   ms,
			"address":   net.JoinHostPort(candidates[0], strconv.Itoa(port)),
			"last_seen": agent.LastSeen,
		})
		return
	}
	if err := ws.db.RecordAgentPing(id, ms); err != nil {
		httpx.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	agent, _ = ws.db.GetAgent(id)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":        true,
		"ping_ms":   ms,
		"address":   net.JoinHostPort(used, strconv.Itoa(port)),
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
