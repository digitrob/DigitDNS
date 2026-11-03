package web

import (
	"encoding/json"
	"net/http"
	"strings"

	"digitdns/internal/database"
	"digitdns/internal/httpx"
)

// dashboardLayoutPolicyKey is the custom policy that stores the operator's
// dashboard arrangement. One "layout=<json>" entry keeps the schema free of a
// new table; the value is small and belongs to the install, not to a browser.
const dashboardLayoutPolicyKey = "dashboard_layout"

// loadDashboardLayout returns the saved arrangement, or an empty object when
// none has been stored (or the stored value is unreadable — the dashboard then
// falls back to its built-in order rather than refusing to render).
func (ws *WebServer) loadDashboardLayout() map[string]any {
	policies, err := ws.db.ListPolicies()
	if err != nil {
		return map[string]any{}
	}
	for _, p := range policies {
		if p.Key != dashboardLayoutPolicyKey {
			continue
		}
		for _, entry := range p.CustomDomains {
			key, value, ok := strings.Cut(entry, "=")
			if !ok || strings.TrimSpace(key) != "layout" {
				continue
			}
			var layout map[string]any
			if json.Unmarshal([]byte(value), &layout) == nil {
				return layout
			}
		}
	}
	return map[string]any{}
}

// handleDashboardLayout saves the arrangement for every browser on this
// install. Only the shape of the document is checked here; the client owns the
// meaning of its card ids and clamps the numbers when applying them.
func (ws *WebServer) handleDashboardLayout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.WriteMethodNotAllowed(w, "POST")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	var req struct {
		Layout json.RawMessage `json:"layout"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteJSONError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}

	layout := map[string]any{}
	if len(req.Layout) > 0 && string(req.Layout) != "null" {
		if err := json.Unmarshal(req.Layout, &layout); err != nil {
			httpx.WriteJSONError(w, http.StatusBadRequest, "Layout must be a JSON object")
			return
		}
	}
	encoded, err := json.Marshal(layout)
	if err != nil {
		httpx.WriteJSONError(w, http.StatusBadRequest, "Layout must be a JSON object")
		return
	}

	if err := ws.db.SavePolicy(database.Policy{
		Key:           dashboardLayoutPolicyKey,
		Name:          "Dashboard Layout",
		Category:      "custom",
		Enabled:       true,
		CustomDomains: []string{"layout=" + string(encoded)},
	}); err != nil {
		httpx.WriteJSONError(w, http.StatusInternalServerError, "Could not save the dashboard layout")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"success": true})
}
