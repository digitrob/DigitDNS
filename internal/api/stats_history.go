package api

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"digitdns/internal/aggregate"
	"digitdns/internal/database"
)

// historicalStatsProblem maps the aggregate package's validation errors to a
// 400 and leaves storage failures as 500. The endpoints are read-only and share
// the v3 auth gate, so they can be reached by either a dashboard session or the
// master REST key.
func historicalStatsProblem(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, aggregate.ErrInvalidRange):
		writeProblemAt(w, r, http.StatusBadRequest, "invalid_request", "range must be 1h, 24h, 7d, 30d or 90d")
	case errors.Is(err, aggregate.ErrInvalidInterval):
		writeProblemAt(w, r, http.StatusBadRequest, "invalid_request", "interval is not valid for this range")
	case errors.Is(err, aggregate.ErrInvalidDimension):
		writeProblemAt(w, r, http.StatusBadRequest, "invalid_request", "dimension must be domain, blocked_domain, client, upstream, action, protocol, qtype, rcode or relay_country")
	case errors.Is(err, aggregate.ErrInvalidLimit):
		writeProblemAt(w, r, http.StatusBadRequest, "invalid_request", "limit must be between 1 and 100")
	default:
		writeProblemAt(w, r, http.StatusInternalServerError, "internal", err.Error())
	}
}

func statsWindow(r *http.Request) string {
	window := strings.TrimSpace(r.URL.Query().Get("range"))
	if window == "" {
		return aggregate.Range24h
	}
	return window
}

func (a *API) handleV3StatsSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeProblemAt(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "GET or HEAD only")
		return
	}
	agg := a.stats.Aggregate()
	if agg == nil {
		writeProblemAt(w, r, http.StatusServiceUnavailable, "stats_unavailable", "historical stats are not available on this instance")
		return
	}
	out, err := agg.Summary(statsWindow(r))
	if err != nil {
		historicalStatsProblem(w, r, err)
		return
	}
	_ = json.NewEncoder(w).Encode(out)
}

func (a *API) handleV3StatsSeries(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeProblemAt(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "GET or HEAD only")
		return
	}
	agg := a.stats.Aggregate()
	if agg == nil {
		writeProblemAt(w, r, http.StatusServiceUnavailable, "stats_unavailable", "historical stats are not available on this instance")
		return
	}
	interval := strings.TrimSpace(r.URL.Query().Get("interval"))
	out, err := agg.Series(statsWindow(r), interval)
	if err != nil {
		historicalStatsProblem(w, r, err)
		return
	}
	_ = json.NewEncoder(w).Encode(out)
}

func (a *API) handleV3StatsTop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeProblemAt(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "GET or HEAD only")
		return
	}
	agg := a.stats.Aggregate()
	if agg == nil {
		writeProblemAt(w, r, http.StatusServiceUnavailable, "stats_unavailable", "historical stats are not available on this instance")
		return
	}
	dimension := strings.TrimSpace(r.URL.Query().Get("dimension"))
	if dimension == "" {
		dimension = "domain"
	}
	limit := 10
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil {
			writeProblemAt(w, r, http.StatusBadRequest, "invalid_request", "limit must be an integer")
			return
		}
		limit = n
	}
	out, err := agg.Top(statsWindow(r), dimension, limit)
	if err != nil {
		historicalStatsProblem(w, r, err)
		return
	}
	_ = json.NewEncoder(w).Encode(a.withClientNames(out))
}

// withClientNames resolves an IP-keyed client row to the account's current name.
// The aggregate store keeps the address as its identity, so this also fixes rows
// written before names were available; a deleted or unknown client keeps its IP.
// Rows that share a name after resolution are merged and re-ranked.
func (a *API) withClientNames(top aggregate.Top) aggregate.Top {
	if top.Dimension != "client" || a.clients == nil || len(top.Items) == 0 {
		return top
	}
	merged := make(map[string]uint64, len(top.Items))
	for _, item := range top.Items {
		name := item.Value
		// Only addresses can resolve to a client record; names already stored in
		// newer buckets pass through untouched.
		if net.ParseIP(item.Value) != nil {
			if client, ok := a.clients.IsIPAllowed(item.Value); ok && client != nil {
				if trimmed := strings.TrimSpace(client.Name); trimmed != "" {
					name = trimmed
				}
			}
		}
		merged[name] += item.Count
	}
	items := make([]aggregate.TopItem, 0, len(merged))
	for value, count := range merged {
		items = append(items, aggregate.TopItem{Value: value, Count: count})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Count != items[j].Count {
			return items[i].Count > items[j].Count
		}
		return items[i].Value < items[j].Value
	})
	top.Items = items
	return top
}

var historyExportDimensions = []string{
	"domain", "blocked_domain", "client", "upstream", "protocol", "relay_country",
}

// handleV3StatsExport downloads the selected range as a CSV series or a JSON
// document containing the summary, series and ranked tables. It uses the same
// v3 auth gate as the read endpoints; the browser downloads it with fetch so the
// Authorization header is present.
func (a *API) handleV3StatsExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeProblemAt(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "GET or HEAD only")
		return
	}
	agg := a.stats.Aggregate()
	if agg == nil {
		writeProblemAt(w, r, http.StatusServiceUnavailable, "stats_unavailable", "historical stats are not available on this instance")
		return
	}
	window := statsWindow(r)
	format := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("format")))
	if format == "" {
		format = "csv"
	}
	if format != "csv" && format != "json" {
		writeProblemAt(w, r, http.StatusBadRequest, "invalid_request", "format must be csv or json")
		return
	}
	summary, err := agg.Summary(window)
	if err != nil {
		historicalStatsProblem(w, r, err)
		return
	}
	series, err := agg.Series(window, "")
	if err != nil {
		historicalStatsProblem(w, r, err)
		return
	}
	tops := make(map[string]aggregate.Top, len(historyExportDimensions))
	for _, dimension := range historyExportDimensions {
		top, err := agg.Top(window, dimension, 100)
		if err != nil {
			historicalStatsProblem(w, r, err)
			return
		}
		tops[dimension] = a.withClientNames(top)
	}
	if format == "json" {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", `attachment; filename="digitdns-stats-`+window+`.json"`)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"range": window, "summary": summary, "series": series, "tops": tops,
		})
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="digitdns-stats-`+window+`.csv"`)
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"time", "queries", "blocked", "proxied", "cached", "failed", "latency_ms"})
	for _, point := range series.Points {
		_ = cw.Write([]string{
			point.Start.UTC().Format("2006-01-02T15:04:05Z"),
			strconv.FormatUint(point.Queries, 10),
			strconv.FormatUint(point.Blocked, 10),
			strconv.FormatUint(point.Proxied, 10),
			strconv.FormatUint(point.Cached, 10),
			strconv.FormatUint(point.Failed, 10),
			strconv.FormatFloat(point.LatencyMs, 'f', 3, 64),
		})
	}
	cw.Flush()
}

// handleV3StatsSettings exposes the aggregate-store policy over v3. The
// dashboard's /api/stats/settings is session-only, so a master-key client (and
// the Android app) needs this v3 twin. The values are restart-scoped: saving
// records the choice and asks the caller to restart, exactly like the panel.
func (a *API) handleV3StatsSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet, http.MethodHead:
		settings, err := a.db.LoadStatsSettings()
		if err != nil {
			writeProblemAt(w, r, http.StatusInternalServerError, "internal", "could not read stats settings")
			return
		}
		_ = json.NewEncoder(w).Encode(settings)
	case http.MethodPost:
		var req database.StatsSettings
		if !decodeStrict(w, r, &req) {
			return
		}
		if err := req.Validate(); err != nil {
			writeProblemAt(w, r, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
		req = req.Normalize()
		if err := a.db.SaveStatsSettings(req); err != nil {
			writeProblemAt(w, r, http.StatusInternalServerError, "internal", "could not save stats settings")
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success":          true,
			"restart_required": true,
			"settings":         req,
		})
	default:
		writeProblemAt(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "GET, HEAD or POST")
	}
}
