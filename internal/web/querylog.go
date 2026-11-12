package web

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"digitdns/internal/httpx"
	"digitdns/internal/service"
)

func (ws *WebServer) handleQueryLogSettings(w http.ResponseWriter, r *http.Request) {
	if ws.queryLog == nil {
		httpx.WriteJSONError(w, http.StatusServiceUnavailable, "query log is unavailable")
		return
	}
	switch r.Method {
	case http.MethodGet, http.MethodHead:
		writeEgressJSON(w, http.StatusOK, map[string]any{"settings": ws.queryLog.Settings()})
	case http.MethodPost:
		var in service.QueryLogSettings
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192)).Decode(&in); err != nil {
			httpx.WriteJSONError(w, http.StatusBadRequest, "invalid JSON body")
			return
		}
		if err := ws.queryLog.SaveSettings(in); err != nil {
			httpx.WriteJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeEgressJSON(w, http.StatusOK, map[string]any{"settings": ws.queryLog.Settings()})
	default:
		httpx.WriteMethodNotAllowed(w, "GET, HEAD, POST")
	}
}

func queryLogFilter(r *http.Request) service.QueryLogFilter {
	f := service.QueryLogFilter{
		Client: strings.TrimSpace(r.URL.Query().Get("client")),
		Domain: strings.TrimSpace(r.URL.Query().Get("domain")),
		Action: strings.TrimSpace(r.URL.Query().Get("action")),
		Proto:  strings.TrimSpace(r.URL.Query().Get("proto")),
	}
	if raw := r.URL.Query().Get("from"); raw != "" {
		if t, err := time.Parse(time.RFC3339, raw); err == nil {
			f.From = t
		}
	}
	if raw := r.URL.Query().Get("to"); raw != "" {
		if t, err := time.Parse(time.RFC3339, raw); err == nil {
			f.To = t
		}
	}
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			f.Limit = n
		}
	}
	return f
}

func (ws *WebServer) handleQueryLog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		httpx.WriteMethodNotAllowed(w, "GET, HEAD")
		return
	}
	if ws.queryLog == nil {
		httpx.WriteJSONError(w, http.StatusServiceUnavailable, "query log is unavailable")
		return
	}
	items := ws.queryLog.Search(queryLogFilter(r))
	writeEgressJSON(w, http.StatusOK, map[string]any{"items": items, "count": len(items)})
}

func (ws *WebServer) handleQueryLogExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		httpx.WriteMethodNotAllowed(w, "GET, HEAD")
		return
	}
	if ws.queryLog == nil {
		httpx.WriteJSONError(w, http.StatusServiceUnavailable, "query log is unavailable")
		return
	}
	items := ws.queryLog.Search(queryLogFilter(r))
	format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
	stamp := time.Now().UTC().Format("20060102-150405")
	if format == "csv" {
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=digitdns-query-log-%s.csv", stamp))
		cw := csv.NewWriter(w)
		_ = cw.Write([]string{"time", "client_ip", "account", "domain", "qtype", "action", "rule", "pattern", "protocol", "rcode", "latency_ms", "cached", "egress"})
		for _, it := range items {
			_ = cw.Write([]string{
				it.Timestamp.UTC().Format(time.RFC3339), it.ClientIP, it.AccountName, it.Domain,
				fmt.Sprint(it.QType), it.Action, it.RuleMatched, it.MatchedPattern, it.Protocol,
				fmt.Sprint(it.Rcode), fmt.Sprintf("%.1f", it.LatencyMs), fmt.Sprint(it.Cached),
				it.RelayEgressName,
			})
		}
		cw.Flush()
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=digitdns-query-log-%s.json", stamp))
	_ = json.NewEncoder(w).Encode(items)
}

func (ws *WebServer) handleQueryLogClear(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.WriteMethodNotAllowed(w, "POST")
		return
	}
	if ws.queryLog == nil {
		httpx.WriteJSONError(w, http.StatusServiceUnavailable, "query log is unavailable")
		return
	}
	if err := ws.queryLog.Clear(); err != nil {
		httpx.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeEgressJSON(w, http.StatusOK, map[string]any{"ok": true})
}
