package web

import (
	"encoding/json"
	"net/http"
	"strings"

	"digitdns/internal/database"
	"digitdns/internal/filterpresets"
	"digitdns/internal/httpx"
)

func (ws *WebServer) filteringUnavailable(w http.ResponseWriter) bool {
	if ws.filters == nil {
		httpx.WriteJSONError(w, http.StatusServiceUnavailable, "Filtering engine is not available on this instance")
		return true
	}
	return false
}

// handleFiltering returns the policy, list metadata and compiled rule count.
func (ws *WebServer) handleFiltering(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		httpx.WriteMethodNotAllowed(w, "GET, HEAD")
		return
	}
	if ws.filteringUnavailable(w) {
		return
	}
	ws.filters.EnsurePresetCounts()
	status, err := ws.filters.Status()
	if err != nil {
		httpx.WriteJSONError(w, http.StatusInternalServerError, "Could not read filtering status")
		return
	}
	_ = json.NewEncoder(w).Encode(status)
}

func (ws *WebServer) handleFilteringSettings(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		httpx.WriteMethodNotAllowed(w, "POST")
		return
	}
	if ws.filteringUnavailable(w) {
		return
	}
	var req database.FilteringSettings
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		httpx.WriteJSONError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}
	if err := ws.filters.SaveSettings(req); err != nil {
		httpx.WriteJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	status, err := ws.filters.Status()
	if err != nil {
		httpx.WriteJSONError(w, http.StatusInternalServerError, "Settings saved, but status could not be read")
		return
	}
	_ = json.NewEncoder(w).Encode(status)
}

func (ws *WebServer) handleFilteringRefresh(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		httpx.WriteMethodNotAllowed(w, "POST")
		return
	}
	if ws.filteringUnavailable(w) {
		return
	}
	ws.filters.RefreshAll()
	_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
}

// handleFilterLists adds a URL subscription or an uploaded list body.
func (ws *WebServer) handleFilterLists(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		httpx.WriteMethodNotAllowed(w, "POST")
		return
	}
	if ws.filteringUnavailable(w) {
		return
	}
	var req struct {
		Name     string `json:"name"`
		URL      string `json:"url"`
		Homepage string `json:"homepage"`
		Content  string `json:"content"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 20<<20)).Decode(&req); err != nil {
		httpx.WriteJSONError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}
	list, err := ws.filters.AddListWithMeta(req.Name, req.URL, req.Content, req.Homepage, "", "")
	if err != nil {
		httpx.WriteJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	_ = json.NewEncoder(w).Encode(list)
}

// handleFilteringPresets returns the curated catalogue, and accepts a batch of
// preset IDs to install. The catalogue is static; only the raw list URLs are
// fetched by the updater.
func (ws *WebServer) handleFilteringPresets(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if ws.filteringUnavailable(w) {
		return
	}
	switch r.Method {
	case http.MethodGet, http.MethodHead:
		_ = json.NewEncoder(w).Encode(map[string]any{"presets": filterpresets.All()})
	case http.MethodPost:
		var req struct {
			IDs []string `json:"ids"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
			httpx.WriteJSONError(w, http.StatusBadRequest, "Invalid request payload")
			return
		}
		if len(req.IDs) == 0 {
			httpx.WriteJSONError(w, http.StatusBadRequest, "ids is required")
			return
		}
		if len(req.IDs) > 100 {
			httpx.WriteJSONError(w, http.StatusBadRequest, "too many presets in one request")
			return
		}
		_ = json.NewEncoder(w).Encode(ws.filters.AddPresets(req.IDs))
	default:
		httpx.WriteMethodNotAllowed(w, "GET, HEAD, POST")
	}
}

func (ws *WebServer) handleFilterListItem(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if ws.filteringUnavailable(w) {
		return
	}
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/filtering/lists/"), "/")
	if rest == "" {
		httpx.WriteJSONError(w, http.StatusBadRequest, "Missing filter list id")
		return
	}
	parts := strings.Split(rest, "/")
	id := parts[0]
	if id == "" || len(id) > 64 || strings.ContainsAny(id, " /\\") {
		httpx.WriteJSONError(w, http.StatusBadRequest, "Invalid filter list id")
		return
	}
	if len(parts) == 2 && parts[1] == "update" {
		if r.Method != http.MethodPost {
			httpx.WriteMethodNotAllowed(w, "POST")
			return
		}
		ws.filters.UpdateList(id)
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
		return
	}
	if len(parts) != 1 {
		httpx.WriteJSONError(w, http.StatusNotFound, "Unknown filter list action")
		return
	}
	switch r.Method {
	case http.MethodDelete:
		if err := ws.filters.DeleteList(id); err != nil {
			httpx.WriteJSONError(w, http.StatusInternalServerError, "Could not delete filter list")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	case http.MethodPatch, http.MethodPost:
		var req struct {
			Enabled *bool `json:"enabled"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
			httpx.WriteJSONError(w, http.StatusBadRequest, "Invalid request payload")
			return
		}
		if req.Enabled == nil {
			httpx.WriteJSONError(w, http.StatusBadRequest, "enabled is required")
			return
		}
		if err := ws.filters.SetListEnabled(id, *req.Enabled); err != nil {
			httpx.WriteJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
	default:
		httpx.WriteMethodNotAllowed(w, "PATCH, POST, DELETE")
	}
}
