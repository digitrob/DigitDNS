package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"digitdns/internal/database"
	"digitdns/internal/httpx"
	"digitdns/internal/service"
)

// SetRelayModeManager installs the full-instance relay manager. Nil in the
// lighter test harnesses, where the endpoint reports the persisted record.
func (ws *WebServer) SetRelayModeManager(m *service.RelayModeManager) {
	ws.relayMode = m
}

type relayModeRequest struct {
	ID       *string `json:"id"`
	Name     *string `json:"name"`
	Enabled  *bool   `json:"enabled"`
	HubURL   *string `json:"hub_url"`
	AgentKey *string `json:"agent_key"`
	Token    *string `json:"token"`
}

func (ws *WebServer) loadRelayHubs() ([]database.RelayHubSettings, error) {
	var hubs []database.RelayHubSettings
	if err := ws.db.GetSetting("relay_hubs", &hubs); err == nil && len(hubs) > 0 {
		return hubs, nil
	}
	var old database.RelayModeSettings
	if err := ws.db.GetSetting("relay_mode", &old); err == nil && old.HubURL != "" {
		hubs = []database.RelayHubSettings{{
			ID: "hub-1", Name: "Hub", Enabled: old.Enabled, HubURL: old.HubURL,
			AgentKey: old.AgentKey, AgentID: old.AgentID, Country: old.Country,
			Hosts: old.Hosts, Clients: old.Clients, IPv6Enabled: old.IPv6Enabled,
			CatalogueETag: old.CatalogueETag, LastSync: old.LastSync, LastError: old.LastError,
		}}
	}
	return hubs, nil
}

func (ws *WebServer) relayModeViews() []service.RelayModeView {
	if ws.relayMode != nil {
		if views := ws.relayMode.Statuses(); len(views) > 0 {
			return views
		}
	}
	hubs, _ := ws.loadRelayHubs()
	out := make([]service.RelayModeView, 0, len(hubs))
	for _, h := range hubs {
		out = append(out, service.RelayModeView{
			ID: h.ID, Name: h.Name, Enabled: h.Enabled, HubURL: h.HubURL,
			AgentID: h.AgentID, AgentKeySet: h.AgentKey != "", Country: h.Country,
			IPv6Enabled: h.IPv6Enabled, Hosts: len(h.Hosts), Clients: len(h.Clients),
			LastSync: h.LastSync, LastHeartbeat: h.LastHeartbeat, LastError: h.LastError,
			RelayBytesIn: h.RelayBytesIn, RelayBytesOut: h.RelayBytesOut, RelayPort: 443,
		})
	}
	return out
}

func (ws *WebServer) handleRelayMode(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	switch r.Method {
	case http.MethodGet, http.MethodHead:
		views := ws.relayModeViews()
		global := true
		if ws.relayMode != nil {
			global = ws.relayMode.GlobalEnabled()
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"global_enabled": global,
			"relay_hubs":     views,
			// Kept for older clients that still read the single-hub key.
			"relay_mode": firstRelayView(views),
		})
	case http.MethodPost, http.MethodPut:
		var req relayModeRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&req); err != nil {
			httpx.WriteJSONError(w, http.StatusBadRequest, "the request body could not be read as JSON")
			return
		}
		view, err := ws.saveRelayHub(r.Context(), req)
		if err != nil {
			httpx.WriteJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		views := ws.relayModeViews()
		global := true
		if ws.relayMode != nil {
			global = ws.relayMode.GlobalEnabled()
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"global_enabled": global,
			"relay_hub":      view, "relay_hubs": views, "relay_mode": firstRelayView(views),
		})
	default:
		httpx.WriteMethodNotAllowed(w, "GET, HEAD, POST, PUT")
	}
}

func firstRelayView(views []service.RelayModeView) service.RelayModeView {
	if len(views) == 0 {
		return service.RelayModeView{RelayPort: 443}
	}
	return views[0]
}

// handleRelayModeByID serves /api/relay-mode/{id} and /{id}/test.
func (ws *WebServer) handleRelayModeByID(w http.ResponseWriter, r *http.Request) {
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/relay-mode/"), "/")
	if rest == "" {
		ws.handleRelayMode(w, r)
		return
	}
	id, action, _ := strings.Cut(rest, "/")
	if action == "test" {
		if r.Method != http.MethodPost {
			httpx.WriteMethodNotAllowed(w, "POST")
			return
		}
		ws.testRelayHub(w, r, id)
		return
	}
	if action != "" {
		httpx.WriteJSONError(w, http.StatusNotFound, "unknown relay mode action")
		return
	}
	switch r.Method {
	case http.MethodDelete:
		if err := ws.deleteRelayHub(id); err != nil {
			httpx.WriteJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"relay_hubs": ws.relayModeViews()})
	default:
		httpx.WriteMethodNotAllowed(w, "DELETE")
	}
}

func (ws *WebServer) handleRelayModeGlobal(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.WriteMethodNotAllowed(w, "POST")
		return
	}
	if ws.relayMode == nil {
		httpx.WriteJSONError(w, http.StatusServiceUnavailable, "relay mode is unavailable")
		return
	}
	var req struct {
		Enabled *bool `json:"enabled"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil || req.Enabled == nil {
		httpx.WriteJSONError(w, http.StatusBadRequest, "enabled is required")
		return
	}
	if err := ws.relayMode.SetGlobalEnabled(*req.Enabled); err != nil {
		httpx.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"global_enabled": ws.relayMode.GlobalEnabled(),
		"relay_hubs":     ws.relayModeViews(),
	})
}

func (ws *WebServer) testRelayHub(w http.ResponseWriter, r *http.Request, id string) {
	if ws.relayMode == nil {
		httpx.WriteJSONError(w, http.StatusServiceUnavailable, "relay mode is unavailable")
		return
	}
	latency, err := ws.relayMode.TestHub(r.Context(), id)
	if err != nil {
		writeEgressJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error(), "latency_ms": 0})
		return
	}
	writeEgressJSON(w, http.StatusOK, map[string]any{
		"ok": true, "latency_ms": float64(latency.Microseconds()) / 1000.0,
	})
}

func (ws *WebServer) handleRelayModeTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.WriteMethodNotAllowed(w, "POST")
		return
	}
	id := strings.TrimSpace(r.URL.Query().Get("id"))
	if id == "" {
		views := ws.relayModeViews()
		if len(views) > 0 {
			id = views[0].ID
		}
	}
	ws.testRelayHub(w, r, id)
}

func (ws *WebServer) deleteRelayHub(id string) error {
	hubs, err := ws.loadRelayHubs()
	if err != nil {
		return err
	}
	next := hubs[:0]
	found := false
	for _, h := range hubs {
		if h.ID == id {
			found = true
			continue
		}
		next = append(next, h)
	}
	if !found {
		return fmt.Errorf("no relay hub with id %q", id)
	}
	if err := ws.db.SetSetting("relay_hubs", next); err != nil {
		return err
	}
	if ws.relayMode != nil {
		return ws.relayMode.Reload()
	}
	return nil
}

func (ws *WebServer) saveRelayHub(ctx context.Context, req relayModeRequest) (service.RelayModeView, error) {
	hubs, err := ws.loadRelayHubs()
	if err != nil {
		return service.RelayModeView{}, err
	}
	idx := -1
	if req.ID != nil && strings.TrimSpace(*req.ID) != "" {
		for i := range hubs {
			if hubs[i].ID == strings.TrimSpace(*req.ID) {
				idx = i
				break
			}
		}
		if idx < 0 {
			return service.RelayModeView{}, fmt.Errorf("no relay hub with id %q", *req.ID)
		}
	}
	if idx < 0 {
		hubs = append(hubs, database.RelayHubSettings{
			ID: fmt.Sprintf("hub-%d", time.Now().UnixNano()), Enabled: true,
		})
		idx = len(hubs) - 1
	}
	h := &hubs[idx]
	if req.Name != nil {
		h.Name = strings.TrimSpace(*req.Name)
	}
	if req.Enabled != nil {
		h.Enabled = *req.Enabled
	}
	if req.HubURL != nil {
		h.HubURL = strings.TrimRight(strings.TrimSpace(*req.HubURL), "/")
	}
	if h.HubURL != "" {
		parsed, err := url.Parse(h.HubURL)
		if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
			return service.RelayModeView{}, errors.New("hub_url must be an http or https base URL")
		}
	}
	if req.AgentKey != nil && strings.TrimSpace(*req.AgentKey) != "" {
		key := strings.TrimSpace(*req.AgentKey)
		id, secret, ok := strings.Cut(key, ":")
		if !ok || id == "" || secret == "" {
			return service.RelayModeView{}, errors.New("agent_key must be the id:secret value from the hub")
		}
		h.AgentKey = key
		h.AgentID = id
	}
	if req.Token != nil && strings.TrimSpace(*req.Token) != "" && h.AgentKey == "" {
		if h.HubURL == "" {
			return service.RelayModeView{}, errors.New("hub_url is required before enrolment")
		}
		if ws.relayMode == nil {
			return service.RelayModeView{}, errors.New("relay mode is unavailable in this build")
		}
		enrolCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		enrolment, err := ws.relayMode.Enroll(enrolCtx, h.HubURL, strings.TrimSpace(*req.Token))
		if err != nil {
			return service.RelayModeView{}, err
		}
		h.AgentKey = enrolment.Key
		h.AgentID = enrolment.AgentID
		h.Country = enrolment.Country
	}
	if h.Enabled && (h.HubURL == "" || h.AgentKey == "") {
		return service.RelayModeView{}, errors.New("hub_url and an agent key or enrolment token are required")
	}
	h.LastError = ""
	if err := ws.db.SetSetting("relay_hubs", hubs); err != nil {
		return service.RelayModeView{}, err
	}
	// Keep the legacy single-hub record in step for older clients and for the
	// existing test contract. The list remains the source of truth.
	_ = ws.db.SetSetting("relay_mode", database.RelayModeSettings{
		Enabled: h.Enabled, HubURL: h.HubURL, AgentKey: h.AgentKey, AgentID: h.AgentID,
		Country: h.Country, Hosts: h.Hosts, Clients: h.Clients, IPv6Enabled: h.IPv6Enabled,
		CatalogueETag: h.CatalogueETag, LastSync: h.LastSync, LastError: h.LastError,
	})
	if ws.relayMode != nil {
		if err := ws.relayMode.Reload(); err != nil {
			return service.RelayModeView{}, err
		}
		for _, v := range ws.relayMode.Statuses() {
			if v.ID == h.ID {
				return v, nil
			}
		}
	}
	return service.RelayModeView{
		ID: h.ID, Name: h.Name, Enabled: h.Enabled, HubURL: h.HubURL,
		AgentID: h.AgentID, AgentKeySet: h.AgentKey != "", Country: h.Country,
		Hosts: len(h.Hosts), Clients: len(h.Clients), RelayPort: 443,
	}, nil
}
