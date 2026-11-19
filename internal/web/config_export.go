package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"digitdns/internal/database"
	"digitdns/internal/httpx"
	"digitdns/internal/version"
)

// handleConfigExport writes a redacted, human-readable snapshot of the settings
// an operator would want before a large change. It is deliberately an allowlist,
// not a dump: the master API key, admin password verifier, admin path, DoH/DoQ
// tokens and any TLS private key material are absent by construction.
func (ws *WebServer) handleConfigExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		httpx.WriteMethodNotAllowed(w, "GET, HEAD")
		return
	}
	server := ws.settings.Snapshot()
	ws.dnsMu.Lock()
	dns := *ws.mutableDNSLocked()
	ws.dnsMu.Unlock()
	access := ws.loadAccessConfig()

	filtering := database.DefaultFilteringSettings()
	filterLists := []database.FilterList{}
	if ws.filters != nil {
		filtering = ws.filters.Settings()
		if lists, err := ws.filters.Lists(); err == nil {
			filterLists = lists
		}
	}

	export := map[string]any{
		"exported_at": time.Now().UTC().Format(time.RFC3339),
		"version":     version.Get(),
		"server": map[string]any{
			"public_ip":            server.PublicIP,
			"public_ipv6":          server.PublicIPv6,
			"proxy_ipv6_enabled":   server.ProxyIPv6Enabled,
			"bind_host":            server.BindHost,
			"web_port":             server.WebPort,
			"admin_username":       server.AdminUsername,
			"api_bind":             server.APIBind,
			"api_bind_list":        server.APIBindList,
			"session_idle_minutes": server.SessionIdleMinutes,
		},
		"dns": dns,
		"tls": ws.tlsSettings.Snapshot(),
		"access": map[string]any{
			"allow_all":   access.AllowAll,
			"allowed_ips": access.AllowedIPs,
		},
		"rules":        ws.loadRulesConfig(),
		"filtering":    filtering,
		"filter_lists": filterLists,
		"subscription": ws.subSettings.Snapshot(),
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", fmt.Sprintf(
		`attachment; filename="digitdns-config-%s.json"`,
		time.Now().UTC().Format("20060102-150405"),
	))
	_ = json.NewEncoder(w).Encode(export)
}
