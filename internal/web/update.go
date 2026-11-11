package web

import (
	"context"
	"encoding/json"
	"errors"
	"log"
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

// updateCheckTTL bounds how long the dashboard may reuse a GitHub answer. The
// version on main changes at release time, so a minute is more than fresh
// enough, and it keeps every page load from spending a GitHub round trip.
const updateCheckTTL = 60 * time.Second

// handleUpdateCheck answers GET /api/update/check with the running version, the
// version published on the project's main branch, and whether an update is
// available. Read-only; safe to call on dashboard load.
//
// ?force=1 bypasses the cache: that is the manual "Check for updates" button,
// which must not repeat a stale failure a minute after the operator fixed it.
func (ws *WebServer) handleUpdateCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.WriteMethodNotAllowed(w, "GET")
		return
	}
	if ws.updater == nil {
		httpx.WriteJSONError(w, http.StatusServiceUnavailable, "the update service is unavailable")
		return
	}
	st, err := ws.updateCheck(r.Context(), r.URL.Query().Get("force") == "1")
	writeUpdateCheck(w, st, err)
}

// updateCheck runs (or reuses) one GitHub version check. The outbound request
// deliberately does not inherit the browser request's cancellation: a page
// reload, a closed tab or a reverse-proxy read timeout used to cancel the fetch
// and surface "context canceled" on the card. The check is bounded by its own
// 15 s deadline instead, and its result is cached for every later caller.
func (ws *WebServer) updateCheck(ctx context.Context, force bool) (selfupdate.Status, error) {
	ws.updateCheckMu.Lock()
	if !force && !ws.updateCheckAt.IsZero() && time.Since(ws.updateCheckAt) < updateCheckTTL {
		st, err := ws.updateCheckSt, ws.updateCheckErr
		ws.updateCheckMu.Unlock()
		return st, err
	}
	if ws.updateCheckBusy {
		done := ws.updateCheckDone
		ws.updateCheckMu.Unlock()
		select {
		case <-done:
			ws.updateCheckMu.Lock()
			st, err := ws.updateCheckSt, ws.updateCheckErr
			ws.updateCheckMu.Unlock()
			return st, err
		case <-ctx.Done():
			// The caller went away. Answer with the last completed check rather
			// than inventing an error the operator would see on the next load.
			ws.updateCheckMu.Lock()
			st, err := ws.updateCheckSt, ws.updateCheckErr
			ws.updateCheckMu.Unlock()
			return st, err
		}
	}
	ws.updateCheckBusy = true
	ws.updateCheckDone = make(chan struct{})
	done := ws.updateCheckDone
	ws.updateCheckMu.Unlock()

	checkCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
	check := ws.updateCheckFn
	if check == nil {
		check = ws.updater.Check
	}
	st, err := check(checkCtx)
	cancel()

	ws.updateCheckMu.Lock()
	ws.updateCheckAt = time.Now()
	ws.updateCheckSt = st
	ws.updateCheckErr = err
	ws.updateCheckBusy = false
	close(done)
	ws.updateCheckMu.Unlock()
	if err != nil {
		log.Printf("[Update] version check failed: %v", err)
	}
	return st, err
}

// writeUpdateCheck always serialises the full Status, including the container
// fields. A failed check used to omit them, which collapsed the Docker update
// method block until the next successful check.
func writeUpdateCheck(w http.ResponseWriter, st selfupdate.Status, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	payload := struct {
		selfupdate.Status
		Error string `json:"error,omitempty"`
	}{Status: st}
	if err != nil {
		payload.Error = updateCheckMessage(err)
	}
	_ = json.NewEncoder(w).Encode(payload)
}

// updateCheckMessage turns a transport failure into something an operator can
// act on. The raw error (including "context canceled") stays in the log.
func updateCheckMessage(err error) string {
	switch {
	case errors.Is(err, context.Canceled):
		return "The update check was interrupted. Try again."
	case errors.Is(err, context.DeadlineExceeded):
		return "Couldn't reach GitHub in time. Check the server's outbound connection and try again."
	default:
		return "Couldn't reach GitHub. Check the server's outbound connection and try again."
	}
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
