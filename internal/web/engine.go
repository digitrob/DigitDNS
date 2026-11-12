package web

import (
	"encoding/json"
	"net/http"

	"digitdns/internal/httpx"
)

// SetEngineSetter installs the live policy-engine switch. Nil in harnesses,
// where a save is persistence-only.
func (ws *WebServer) SetEngineSetter(fn func(bool)) {
	ws.engineSetter = fn
}

// SetEngineState installs the live state reader. Nil falls back to the stored
// DNS settings record.
func (ws *WebServer) SetEngineState(fn func() bool) {
	ws.engineState = fn
}

func (ws *WebServer) engineEnabled() bool {
	if ws.engineState != nil {
		return ws.engineState()
	}
	var cfg struct {
		EngineDisabled bool `json:"engine_disabled"`
	}
	_ = ws.db.GetSetting("dns", &cfg)
	return !cfg.EngineDisabled
}

// handleEngine is the panel's live engine switch. GET reports the state; POST
// accepts {"enabled": bool}, persists it and applies it without a restart.
func (ws *WebServer) handleEngine(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	switch r.Method {
	case http.MethodGet, http.MethodHead:
		_ = json.NewEncoder(w).Encode(map[string]any{"enabled": ws.engineEnabled()})
	case http.MethodPost, http.MethodPut:
		var req struct {
			Enabled *bool `json:"enabled"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil || req.Enabled == nil {
			httpx.WriteJSONError(w, http.StatusBadRequest, "enabled (boolean) is required")
			return
		}
		enabled := *req.Enabled
		ws.dnsMu.Lock()
		cfg := ws.mutableDNSLocked()
		cfg.EngineDisabled = !enabled
		err := ws.db.SetSetting("dns", cfg)
		ws.dnsMu.Unlock()
		if err != nil {
			httpx.WriteJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if ws.engineSetter != nil {
			ws.engineSetter(enabled)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"enabled": enabled})
	default:
		httpx.WriteMethodNotAllowed(w, "GET, HEAD, POST, PUT")
	}
}
