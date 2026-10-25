package web

import (
	"encoding/json"
	"net/http"

	"digitdns/internal/httpx"
)

// handleStreamClear empties the live log on the server as well as in the browser
// that pressed the button. Clearing only the client buffer was never a clear: the
// daemon's ring still held the last hundred queries, so the next SSE reconnect
// (a token refresh, a network blip, a tab waking from sleep) replayed them and the
// table refilled with the entries the operator had just dismissed.
//
// The handler is deliberately small and destructive only of telemetry: no query
// history is persisted anywhere, so this frees memory and resets the view without
// touching configuration, subscribers or the DNS cache.
func (ws *WebServer) handleStreamClear(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.WriteMethodNotAllowed(w, "POST")
		return
	}

	ws.stats.ClearRecentLogs()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]bool{"cleared": true})
}
