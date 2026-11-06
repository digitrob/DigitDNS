package web

import (
	"encoding/json"
	"net/http"

	"digitdns/internal/httpx"
)

// proxyLeakView is the Proxy Leak Protection card's state. The guard is
// recommended on; `enabled` is the saved policy and is applied live.
type proxyLeakView struct {
	Enabled     bool `json:"enabled"`
	Recommended bool `json:"recommended"`
}

// handleProxyLeakSettings reads or writes the global proxy leak guard. Unlike
// DNSSEC this policy is live: the resolver reads an atomic flag, so toggling it
// does not need a restart.
func (ws *WebServer) handleProxyLeakSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		ws.writeProxyLeak(w)
	case http.MethodPost:
		ws.saveProxyLeak(w, r)
	default:
		httpx.WriteMethodNotAllowed(w, "GET, POST")
	}
}

func (ws *WebServer) writeProxyLeak(w http.ResponseWriter) {
	ws.dnsMu.Lock()
	cfg := ws.mutableDNSLocked()
	enabled := cfg.ProxyLeakProtection
	ws.dnsMu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(proxyLeakView{
		Enabled:     enabled,
		Recommended: true,
	})
}

func (ws *WebServer) saveProxyLeak(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Enabled *bool `json:"enabled"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		httpx.WriteJSONError(w, http.StatusBadRequest, "Could not read the proxy leak settings: "+err.Error())
		return
	}
	if body.Enabled == nil {
		httpx.WriteJSONError(w, http.StatusBadRequest, "enabled is required")
		return
	}

	ws.dnsMu.Lock()
	cfg := ws.mutableDNSLocked()
	old := cfg.ProxyLeakProtection
	cfg.ProxyLeakProtection = *body.Enabled
	err := ws.db.SetSetting("dns", cfg)
	if err != nil {
		cfg.ProxyLeakProtection = old
	}
	ws.dnsMu.Unlock()
	if err != nil {
		httpx.WriteJSONError(w, http.StatusInternalServerError, "Could not persist the proxy leak settings")
		return
	}

	if ws.proxyLeakSetter != nil {
		ws.proxyLeakSetter(*body.Enabled)
	}
	ws.writeProxyLeak(w)
}
