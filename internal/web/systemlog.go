package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"digitdns/internal/httpx"
	"digitdns/internal/logbuffer"
)

// SetSystemLog attaches the in-process log tail. Nil makes the endpoint answer
// an empty list rather than fail: a harness without the buffer should not break
// the settings page.
func (ws *WebServer) SetSystemLog(buf *logbuffer.Buffer) {
	ws.systemLog = buf
}

func (ws *WebServer) handleSystemLog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		httpx.WriteMethodNotAllowed(w, "GET, HEAD")
		return
	}
	limit := 1000
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			if n > 5000 {
				n = 5000
			}
			limit = n
		}
	}
	lines := []string{}
	if ws.systemLog != nil {
		lines = ws.systemLog.Lines(limit)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"lines": lines})
}

func (ws *WebServer) handleSystemLogExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		httpx.WriteMethodNotAllowed(w, "GET, HEAD")
		return
	}
	body := ""
	if ws.systemLog != nil {
		body = ws.systemLog.String()
	}
	name := fmt.Sprintf("digitdns-system-%s.log", time.Now().UTC().Format("20060102-150405"))
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	_, _ = w.Write([]byte(body))
}
