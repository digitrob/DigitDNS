package web

import (
	"encoding/json"
	"log"
	"net/http"

	"digitdns/internal/database"
	"digitdns/internal/httpx"
)

// handleStatsSettings reads and writes the aggregate-store policy. The settings
// are deliberately restart-scoped: the store is opened once during boot with a
// fixed retention/top-N/anonymization configuration. Saving records the choice
// and tells the operator that a restart is needed rather than returning 200 for
// a value the running aggregator never saw.
func (ws *WebServer) handleStatsSettings(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	switch r.Method {
	case http.MethodGet, http.MethodHead:
		settings, err := ws.db.LoadStatsSettings()
		if err != nil {
			log.Printf("[Web] Could not read stats settings: %v", err)
			httpx.WriteJSONError(w, http.StatusInternalServerError, "Could not read stats settings")
			return
		}
		_ = json.NewEncoder(w).Encode(settings)
	case http.MethodPost:
		var req database.StatsSettings
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&req); err != nil {
			httpx.WriteJSONError(w, http.StatusBadRequest, "Invalid request payload")
			return
		}
		if err := req.Validate(); err != nil {
			httpx.WriteJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		req = req.Normalize()
		if err := ws.db.SaveStatsSettings(req); err != nil {
			log.Printf("[Web] Could not save stats settings: %v", err)
			httpx.WriteJSONError(w, http.StatusInternalServerError, "Could not save stats settings")
			return
		}
		log.Printf("[Web] Stats settings saved; restart required (enabled=%v retention=%dd top=%d/%d anonymize=%v excluded=%d)",
			req.Enabled, req.RetentionDays, req.TopDomains, req.TopClients, req.AnonymizeClients, len(req.ExcludeClients))
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success":          true,
			"restart_required": true,
			"settings":         req,
		})
	default:
		httpx.WriteMethodNotAllowed(w, "GET, HEAD, POST")
	}
}
