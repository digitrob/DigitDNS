package web

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"digitdns/internal/database"
	"digitdns/internal/httpx"
	"digitdns/internal/selfupdate"
)

// SetUpdater wires the self-update engine (v2.6). Called from main once the
// binary and data paths are known; when it is nil the update endpoints answer
// 503 and the dashboard simply never shows an update badge.
func (ws *WebServer) SetUpdater(u *selfupdate.Updater) {
	ws.updater = u
	// Apply the saved container update method at startup. A systemd install
	// ignores it in SetContainerUpdateMode.
	if u != nil && ws.db != nil {
		u.SetContainerUpdateMode(ws.db.GetUpdateSettings().ContainerMode)
	}
}

// handleUpdateMode answers POST /api/update/mode with {"mode":"image"|"binary"}.
//
// It exists only for container installs: the choice decides whether the panel
// refuses the in-app swap (image, the default) or allows it as an explicitly
// temporary patch. A systemd install always uses the signed binary path, so the
// endpoint cannot change anything there.
func (ws *WebServer) handleUpdateMode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.WriteMethodNotAllowed(w, "POST")
		return
	}
	if ws.updater == nil {
		httpx.WriteJSONError(w, http.StatusServiceUnavailable, "the update service is unavailable")
		return
	}
	if !ws.updater.InContainer() {
		httpx.WriteJSONError(w, http.StatusBadRequest, "the container update method only applies to container installs")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
	var req struct {
		Mode string `json:"mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	mode := strings.ToLower(strings.TrimSpace(req.Mode))
	if mode != "image" && mode != "binary" {
		httpx.WriteJSONError(w, http.StatusBadRequest, `mode must be "image" or "binary"`)
		return
	}
	if ws.db != nil {
		if err := ws.db.SaveUpdateSettings(database.UpdateSettings{ContainerMode: mode}); err != nil {
			httpx.WriteJSONError(w, http.StatusInternalServerError, "could not save the update method")
			return
		}
	}
	ws.updater.SetContainerUpdateMode(mode)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"mode":      ws.updater.ContainerUpdateMode(),
		"container": ws.updater.InContainer(),
	})
}

// handleUpdateCheck answers GET /api/update/check with the running version, the
// version published on the project's main branch, and whether an update is
// available. Read-only; safe to call on dashboard load.
func (ws *WebServer) handleUpdateCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.WriteMethodNotAllowed(w, "GET")
		return
	}
	if ws.updater == nil {
		httpx.WriteJSONError(w, http.StatusServiceUnavailable, "the update service is unavailable")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()
	st, err := ws.updater.Check(ctx)
	if err != nil {
		// A failed check is not fatal — report it with the fields we do know so
		// the dashboard can show "couldn't check" rather than break.
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"current":          st.Current,
			"update_available": false,
			"supported":        st.Supported,
			"error":            err.Error(),
		})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(st)
}

// handleUpdateApply answers POST /api/update/apply by starting the update in the
// background. It returns immediately; the dashboard polls /api/update/status.
// The daemon restarts itself onto the new binary once the swap succeeds.
func (ws *WebServer) handleUpdateApply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.WriteMethodNotAllowed(w, "POST")
		return
	}
	if ws.updater == nil {
		httpx.WriteJSONError(w, http.StatusServiceUnavailable, "the update service is unavailable")
		return
	}
	if err := ws.updater.StartApply(); err != nil {
		httpx.WriteJSONError(w, http.StatusConflict, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{"started": true})
}

// handleUpdateStatus answers GET /api/update/status with the live progress of an
// in-flight apply, for the progress modal to poll.
func (ws *WebServer) handleUpdateStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.WriteMethodNotAllowed(w, "GET")
		return
	}
	if ws.updater == nil {
		httpx.WriteJSONError(w, http.StatusServiceUnavailable, "the update service is unavailable")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(ws.updater.Progress())
}
