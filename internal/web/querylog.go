package web

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"digitdns/internal/database"
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
		writeEgressJSON(w, http.StatusOK, map[string]any{
			"settings": ws.queryLog.Settings(),
			"usage":    ws.queryLog.Usage(),
		})
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
		writeEgressJSON(w, http.StatusOK, map[string]any{
			"settings": ws.queryLog.Settings(),
			"usage":    ws.queryLog.Usage(),
		})
	default:
		httpx.WriteMethodNotAllowed(w, "GET, HEAD, POST")
	}
}

var queryLogQTypeNames = map[string]uint16{
	"A": 1, "NS": 2, "CNAME": 5, "SOA": 6, "PTR": 12, "MX": 15, "TXT": 16,
	"AAAA": 28, "SRV": 33, "DS": 43, "RRSIG": 46, "DNSKEY": 48, "SVCB": 64,
	"HTTPS": 65, "ANY": 255,
}

var queryLogRcodeNames = map[string]int{
	"NOERROR": 0, "FORMERR": 1, "SERVFAIL": 2, "NXDOMAIN": 3, "NOTIMP": 4, "REFUSED": 5,
}

func queryLogQTypeName(qtype uint16) string {
	for name, value := range queryLogQTypeNames {
		if value == qtype {
			return name
		}
	}
	return fmt.Sprint(qtype)
}

func queryLogRcodeName(rcode int) string {
	for name, value := range queryLogRcodeNames {
		if value == rcode {
			return name
		}
	}
	return fmt.Sprint(rcode)
}

// queryLogQTypes accepts either wire numbers (28) or names (AAAA), comma separated,
// so a URL written by hand and a URL written by the Android app both work.
func queryLogQTypes(raw string) []uint16 {
	var out []uint16
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if n, err := strconv.ParseUint(part, 10, 16); err == nil {
			out = append(out, uint16(n))
			continue
		}
		if n, ok := queryLogQTypeNames[strings.ToUpper(part)]; ok {
			out = append(out, n)
		}
	}
	return out
}

func queryLogRcodes(raw string) []int {
	var out []int
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if n, err := strconv.Atoi(part); err == nil {
			out = append(out, n)
			continue
		}
		if n, ok := queryLogRcodeNames[strings.ToUpper(part)]; ok {
			out = append(out, n)
		}
	}
	return out
}

func queryLogCached(raw string) *bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes":
		v := true
		return &v
	case "0", "false", "no":
		v := false
		return &v
	default:
		return nil
	}
}

func queryLogFilter(r *http.Request) service.QueryLogFilter {
	q := r.URL.Query()
	f := service.QueryLogFilter{
		Client: strings.TrimSpace(q.Get("client")),
		Domain: strings.TrimSpace(q.Get("domain")),
		Action: strings.TrimSpace(q.Get("action")),
		Proto:  strings.TrimSpace(q.Get("proto")),
		QTypes: queryLogQTypes(q.Get("qtype")),
		Rcodes: queryLogRcodes(q.Get("rcode")),
		Cached: queryLogCached(q.Get("cached")),
		Before: strings.TrimSpace(q.Get("before")),
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
	items := ws.queryLog.SearchPage(queryLogFilter(r))
	writeEgressJSON(w, http.StatusOK, map[string]any{
		"items":       items.Items,
		"count":       len(items.Items),
		"next_cursor": items.NextCursor,
		"has_more":    items.HasMore,
	})
}

// queryLogAnswerCell renders the answer section for the CSV export. One cell keeps
// the CSV rectangular while preserving wire order and per-record TTL.
func queryLogAnswerCell(it database.QueryLogItem) string {
	parts := make([]string, 0, len(it.Answers))
	for _, a := range it.Answers {
		parts = append(parts, fmt.Sprintf("%s=%s (ttl=%d)", a.Type, a.Value, a.TTL))
	}
	if len(parts) == 0 && len(it.ResolvedIPs) > 0 {
		for _, ip := range it.ResolvedIPs {
			parts = append(parts, "A="+ip)
		}
	}
	return strings.Join(parts, "; ")
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
	filter := queryLogFilter(r)
	format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
	stamp := time.Now().UTC().Format("20060102-150405")
	// A trailer reports truncation without buffering the whole export: the header
	// can only be written once the walk finishes, and holding a week of rows just
	// to set one header is the cost the old 1000-row cap was avoiding.
	w.Header().Set("Trailer", "X-Query-Log-Truncated")

	if format == "csv" {
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=digitdns-query-log-%s.csv", stamp))
		cw := csv.NewWriter(w)
		_ = cw.Write([]string{
			"time", "client_ip", "account", "domain", "qtype", "qtype_name", "action",
			"rule", "pattern", "protocol", "rcode", "rcode_name", "latency_ms", "cached",
			"upstream", "resolved_ips", "answers", "egress",
		})
		_, truncated, err := ws.queryLog.Export(filter, func(it database.QueryLogItem) error {
			return cw.Write([]string{
				it.Timestamp.UTC().Format(time.RFC3339), it.ClientIP, it.AccountName, it.Domain,
				fmt.Sprint(it.QType), queryLogQTypeName(it.QType), it.Action, it.RuleMatched,
				it.MatchedPattern, it.Protocol, fmt.Sprint(it.Rcode), queryLogRcodeName(it.Rcode),
				fmt.Sprintf("%.3f", it.LatencyMs), fmt.Sprint(it.Cached), it.Upstream,
				strings.Join(it.ResolvedIPs, "; "), queryLogAnswerCell(it), it.RelayEgressName,
			})
		})
		cw.Flush()
		if err == nil && truncated {
			w.Header().Set("X-Query-Log-Truncated", "true")
		}
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=digitdns-query-log-%s.json", stamp))
	_, _ = w.Write([]byte("["))
	first := true
	_, truncated, err := ws.queryLog.Export(filter, func(it database.QueryLogItem) error {
		data, marshalErr := json.Marshal(it)
		if marshalErr != nil {
			return marshalErr
		}
		if !first {
			if _, err := w.Write([]byte(",")); err != nil {
				return err
			}
		}
		first = false
		_, err := w.Write(data)
		return err
	})
	_, _ = w.Write([]byte("]"))
	if err == nil && truncated {
		w.Header().Set("X-Query-Log-Truncated", "true")
	}
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
