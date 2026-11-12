package web

import (
	"context"
	"encoding/json"
	"errors"
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
	Enabled  *bool   `json:"enabled"`
	HubURL   *string `json:"hub_url"`
	AgentKey *string `json:"agent_key"`
	Token    *string `json:"token"`
}

func (ws *WebServer) relayModeView() service.RelayModeView {
	if ws.relayMode != nil {
		return ws.relayMode.Status()
	}
	var settings database.RelayModeSettings
	_ = ws.db.GetSetting("relay_mode", &settings)
	return service.RelayModeView{
		Enabled: settings.Enabled, HubURL: settings.HubURL,
		AgentID: settings.AgentID, AgentKeySet: settings.AgentKey != "",
		Country: settings.Country, IPv6Enabled: settings.IPv6Enabled,
		Hosts: len(settings.Hosts), Clients: len(settings.Clients),
		LastSync: settings.LastSync, LastError: settings.LastError, RelayPort: 443,
	}
}

func (ws *WebServer) handleRelayMode(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	switch r.Method {
	case http.MethodGet, http.MethodHead:
		_ = json.NewEncoder(w).Encode(map[string]any{"relay_mode": ws.relayModeView()})
	case http.MethodPost, http.MethodPut:
		var req relayModeRequest
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&req); err != nil {
			httpx.WriteJSONError(w, http.StatusBadRequest, "the request body could not be read as JSON")
			return
		}
		view, err := ws.saveRelayMode(r.Context(), req)
		if err != nil {
			httpx.WriteJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"relay_mode": view})
	default:
		httpx.WriteMethodNotAllowed(w, "GET, HEAD, POST, PUT")
	}
}

func (ws *WebServer) handleRelayModeTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.WriteMethodNotAllowed(w, "POST")
		return
	}
	if ws.relayMode == nil {
		httpx.WriteJSONError(w, http.StatusServiceUnavailable, "relay mode is unavailable")
		return
	}
	latency, err := ws.relayMode.Test(r.Context())
	if err != nil {
		writeEgressJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error(), "latency_ms": 0})
		return
	}
	writeEgressJSON(w, http.StatusOK, map[string]any{
		"ok": true, "latency_ms": float64(latency.Microseconds()) / 1000.0,
	})
}

func (ws *WebServer) saveRelayMode(ctx context.Context, req relayModeRequest) (service.RelayModeView, error) {
	var settings database.RelayModeSettings
	if err := ws.db.GetSetting("relay_mode", &settings); err != nil {
		return service.RelayModeView{}, err
	}
	if req.Enabled != nil {
		settings.Enabled = *req.Enabled
	}
	if req.HubURL != nil {
		settings.HubURL = strings.TrimRight(strings.TrimSpace(*req.HubURL), "/")
	}
	if settings.HubURL != "" {
		parsed, err := url.Parse(settings.HubURL)
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
		settings.AgentKey = key
		settings.AgentID = id
	}
	// A token is only useful on first contact. Exchange it now so a restart
	// before the first sync still has the long-lived key.
	if req.Token != nil && strings.TrimSpace(*req.Token) != "" && settings.AgentKey == "" {
		if settings.HubURL == "" {
			return service.RelayModeView{}, errors.New("hub_url is required before enrolment")
		}
		enrolCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		if ws.relayMode == nil {
			return service.RelayModeView{}, errors.New("relay mode is unavailable in this build")
		}
		enrolment, err := ws.relayMode.Enroll(enrolCtx, settings.HubURL, strings.TrimSpace(*req.Token))
		if err != nil {
			return service.RelayModeView{}, err
		}
		settings.AgentKey = enrolment.Key
		settings.AgentID = enrolment.AgentID
		settings.Country = enrolment.Country
	}
	if settings.Enabled && (settings.HubURL == "" || settings.AgentKey == "") {
		return service.RelayModeView{}, errors.New("hub_url and an agent key or enrolment token are required")
	}
	// A save clears the previous runtime error; the next sync/heartbeat will
	// write a fresh one if it still applies.
	settings.LastError = ""
	if err := ws.db.SetSetting("relay_mode", settings); err != nil {
		return service.RelayModeView{}, err
	}
	if ws.relayMode != nil {
		if err := ws.relayMode.Reload(); err != nil {
			return service.RelayModeView{}, err
		}
	}
	return ws.relayModeView(), nil
}
