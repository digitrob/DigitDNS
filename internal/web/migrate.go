package web

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"strings"
	"time"

	"digitdns/internal/core/upstream"
	"digitdns/internal/database"
	"digitdns/internal/httpx"
	"digitdns/internal/migrate"
	"digitdns/internal/service"
)

const (
	migrateUploadMax   = 64 << 20
	migrateAnalysisTTL = 15 * time.Minute
	migrateMaxPending  = 64
	migrateSampleMax   = 20
)

// migrateAnalysis is the normalized, secret-free bundle a preview is looking at.
// The raw archive is never kept: a parser reads the source once, and the apply
// path only sees the allowlisted Bundle.
type migrateAnalysis struct {
	bundle  migrate.Bundle
	report  string
	expires time.Time
}

// migrateUndo is the snapshot an applied import can be rolled back to. It holds
// the rule policies, the filtering settings and the IDs of the objects the
// apply created; the objects themselves are deleted by ID, so an operator's own
// later edits to unrelated records are never touched.
type migrateUndo struct {
	expires   time.Time
	proxied   []string
	disabled  []string
	blocked   []string
	direct    []string
	records   map[string]string
	added     map[string]string
	filtering database.FilteringSettings
	clientIDs []string
	listIDs   []string
	upstreams []string
}

// migrateTrace records the live objects one apply created, so the undo token
// can remove exactly those and nothing else.
type migrateTrace struct {
	clientIDs []string
	listIDs   []string
	upstreams []string
}

type migratePreview struct {
	Token           string             `json:"token"`
	ExpiresAt       time.Time          `json:"expires_at"`
	Source          migrate.Source     `json:"source"`
	Stats           migrate.Stats      `json:"stats"`
	Warnings        []migrate.Issue    `json:"warnings,omitempty"`
	Skipped         []migrate.Issue    `json:"skipped,omitempty"`
	Filtering       *migrate.Filtering `json:"filtering,omitempty"`
	UpstreamsOptIn  bool               `json:"upstreams_opt_in"`
	ReportAvailable bool               `json:"report_available"`
	Sample          migrateSample      `json:"sample"`
}

type migrateSample struct {
	Proxied     []string `json:"proxied,omitempty"`
	Blocked     []string `json:"blocked,omitempty"`
	Direct      []string `json:"direct,omitempty"`
	Allowlist   []string `json:"allowlist,omitempty"`
	Records     []string `json:"records,omitempty"`
	Clients     []string `json:"clients,omitempty"`
	FilterLists []string `json:"filter_lists,omitempty"`
	CustomRules []string `json:"custom_rules,omitempty"`
	Rewrites    []string `json:"rewrites,omitempty"`
	Upstreams   []string `json:"upstreams,omitempty"`
}

func (ws *WebServer) handleMigrateAnalyze(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		httpx.WriteMethodNotAllowed(w, "POST")
		return
	}

	bundle, err := ws.readMigrateBundle(w, r)
	if err != nil {
		if errors.Is(err, migrate.ErrPiHoleV6NeedsBrowser) {
			httpx.WriteJSONError(w, http.StatusBadRequest, "This is a Pi-hole v6 Teleporter archive; the browser must read its gravity.db. Reload the panel and try again.")
			return
		}
		httpx.WriteJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	bundle = bundle.Normalize()
	bundle, _ = migrate.SplitOverCapCustomRules(bundle)
	if err := bundle.Validate(); err != nil {
		httpx.WriteJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	token, report, expires, err := ws.storeMigrateAnalysis(bundle)
	if err != nil {
		httpx.WriteJSONError(w, http.StatusInternalServerError, "Could not prepare the import preview")
		return
	}

	preview := migratePreview{
		Token:           token,
		ExpiresAt:       expires,
		Source:          bundle.Source,
		Stats:           bundle.Stats,
		Warnings:        bundle.Warnings,
		Skipped:         bundle.Skipped,
		Filtering:       bundle.Filtering,
		UpstreamsOptIn:  bundle.UpstreamsOptIn,
		ReportAvailable: strings.TrimSpace(report) != "",
		Sample:          sampleBundle(bundle),
	}
	_ = json.NewEncoder(w).Encode(preview)
}

func (ws *WebServer) readMigrateBundle(w http.ResponseWriter, r *http.Request) (migrate.Bundle, error) {
	contentType := strings.ToLower(r.Header.Get("Content-Type"))
	if strings.HasPrefix(contentType, "multipart/form-data") {
		r.Body = http.MaxBytesReader(w, r.Body, migrateUploadMax)
		if err := r.ParseMultipartForm(migrateUploadMax); err != nil {
			return migrate.Bundle{}, fmt.Errorf("could not read the upload: %w", err)
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			return migrate.Bundle{}, fmt.Errorf("choose a Pi-hole or AdGuard Home export")
		}
		defer file.Close()
		data, err := io.ReadAll(io.LimitReader(file, migrateUploadMax+1))
		if err != nil {
			return migrate.Bundle{}, fmt.Errorf("could not read the upload: %w", err)
		}
		if len(data) > migrateUploadMax {
			return migrate.Bundle{}, fmt.Errorf("the upload is larger than %d MB", migrateUploadMax>>20)
		}
		return parseMigrateUpload(header.Filename, data)
	}

	var req struct {
		Bundle     migrate.Bundle `json:"bundle"`
		PiHoleTOML string         `json:"pihole_toml"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, migrateUploadMax)).Decode(&req); err != nil {
		return migrate.Bundle{}, fmt.Errorf("invalid import payload: %w", err)
	}
	bundle := req.Bundle
	if strings.TrimSpace(req.PiHoleTOML) != "" {
		tomlBundle, err := migrate.ParsePiHoleTOML([]byte(req.PiHoleTOML))
		if err != nil {
			return migrate.Bundle{}, err
		}
		bundle = migrate.MergeBundles(bundle, tomlBundle)
	}
	if bundle.SchemaVersion == 0 && bundle.Source.Kind == "" {
		return migrate.Bundle{}, fmt.Errorf("the import payload has no bundle")
	}
	return bundle, nil
}

func parseMigrateUpload(name string, data []byte) (migrate.Bundle, error) {
	lower := strings.ToLower(strings.TrimSpace(name))
	switch {
	case strings.HasSuffix(lower, ".yaml"), strings.HasSuffix(lower, ".yml"):
		return migrate.ParseAdGuardYAML(data)
	case strings.HasSuffix(lower, ".toml"):
		return migrate.ParsePiHoleTOML(data)
	case strings.HasSuffix(lower, ".zip"):
		return migrate.ParsePiHoleV5ZIP(data)
	}
	if len(data) >= 2 && data[0] == 'P' && data[1] == 'K' {
		return migrate.ParsePiHoleV5ZIP(data)
	}
	if bytes.Contains(data, []byte("filters:")) || bytes.Contains(data, []byte("user_rules:")) {
		return migrate.ParseAdGuardYAML(data)
	}
	if bytes.Contains(data, []byte("[dns]")) || bytes.Contains(data, []byte("[webserver")) {
		return migrate.ParsePiHoleTOML(data)
	}
	return migrate.Bundle{}, fmt.Errorf("unsupported file type; use a Pi-hole Teleporter ZIP, an AdGuard Home YAML, or pihole.toml")
}

func newMigrateToken() (string, error) {
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(tokenBytes), nil
}

func (ws *WebServer) storeMigrateAnalysis(bundle migrate.Bundle) (string, string, time.Time, error) {
	token, err := newMigrateToken()
	if err != nil {
		return "", "", time.Time{}, err
	}
	expires := time.Now().Add(migrateAnalysisTTL)
	report := buildMigrateReport(bundle.Skipped)

	ws.migrateMu.Lock()
	defer ws.migrateMu.Unlock()
	if ws.migrateAnalyses == nil {
		ws.migrateAnalyses = make(map[string]*migrateAnalysis)
	}
	now := time.Now()
	for key, analysis := range ws.migrateAnalyses {
		if now.After(analysis.expires) {
			delete(ws.migrateAnalyses, key)
		}
	}
	if len(ws.migrateAnalyses) >= migrateMaxPending {
		for key := range ws.migrateAnalyses {
			delete(ws.migrateAnalyses, key)
			break
		}
	}
	ws.migrateAnalyses[token] = &migrateAnalysis{bundle: bundle, report: report, expires: expires}
	return token, report, expires, nil
}

func (ws *WebServer) takeMigrateAnalysis(token string) (*migrateAnalysis, bool) {
	ws.migrateMu.Lock()
	defer ws.migrateMu.Unlock()
	analysis, ok := ws.migrateAnalyses[token]
	if !ok {
		return nil, false
	}
	delete(ws.migrateAnalyses, token)
	if time.Now().After(analysis.expires) {
		return nil, false
	}
	return analysis, true
}

func (ws *WebServer) getMigrateAnalysis(token string) (*migrateAnalysis, bool) {
	ws.migrateMu.Lock()
	defer ws.migrateMu.Unlock()
	analysis, ok := ws.migrateAnalyses[token]
	if !ok || time.Now().After(analysis.expires) {
		return nil, false
	}
	return analysis, true
}

func (ws *WebServer) captureMigrateUndo() *migrateUndo {
	rules := ws.loadRulesConfig()
	undo := &migrateUndo{
		proxied:  toStringSlice(rules["custom_proxied"]),
		disabled: toStringSlice(rules["custom_proxied_disabled"]),
		blocked:  toStringSlice(rules["custom_blocked"]),
		direct:   toStringSlice(rules["custom_direct"]),
		records:  map[string]string{},
		added:    map[string]string{},
	}
	if records, ok := rules["custom_records"].(map[string]string); ok {
		for name, value := range records {
			undo.records[name] = value
		}
	}
	if added, ok := rules["custom_domain_added"].(map[string]string); ok {
		for name, stamp := range added {
			undo.added[name] = stamp
		}
	}
	if ws.filters != nil {
		undo.filtering = ws.filters.Settings()
	}
	return undo
}

func (ws *WebServer) storeMigrateUndo(undo *migrateUndo) (string, error) {
	token, err := newMigrateToken()
	if err != nil {
		return "", err
	}
	undo.expires = time.Now().Add(migrateAnalysisTTL)
	ws.migrateMu.Lock()
	defer ws.migrateMu.Unlock()
	if ws.migrateUndos == nil {
		ws.migrateUndos = make(map[string]*migrateUndo)
	}
	now := time.Now()
	for key, entry := range ws.migrateUndos {
		if now.After(entry.expires) {
			delete(ws.migrateUndos, key)
		}
	}
	if len(ws.migrateUndos) >= migrateMaxPending {
		for key := range ws.migrateUndos {
			delete(ws.migrateUndos, key)
			break
		}
	}
	ws.migrateUndos[token] = undo
	return token, nil
}

func (ws *WebServer) takeMigrateUndo(token string) (*migrateUndo, bool) {
	ws.migrateMu.Lock()
	defer ws.migrateMu.Unlock()
	undo, ok := ws.migrateUndos[token]
	if !ok {
		return nil, false
	}
	delete(ws.migrateUndos, token)
	if time.Now().After(undo.expires) {
		return nil, false
	}
	return undo, true
}

func (ws *WebServer) handleMigrateUndo(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		httpx.WriteMethodNotAllowed(w, "POST")
		return
	}
	var req struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		httpx.WriteJSONError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}
	undo, ok := ws.takeMigrateUndo(strings.TrimSpace(req.Token))
	if !ok {
		httpx.WriteJSONError(w, http.StatusGone, "This undo window has expired")
		return
	}
	if err := ws.restoreMigrateUndo(undo); err != nil {
		httpx.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success":           true,
		"removed_clients":   len(undo.clientIDs),
		"removed_lists":     len(undo.listIDs),
		"removed_upstreams": len(undo.upstreams),
	})
}

func (ws *WebServer) restoreMigrateUndo(undo *migrateUndo) error {
	saves := []database.Policy{
		{Key: "custom_proxied", Name: "Custom Proxied Domains", Category: "custom", Enabled: true, CustomDomains: undo.proxied},
		{Key: "custom_proxied_disabled", Name: "Disabled Proxied Domains", Category: "custom", Enabled: true, CustomDomains: undo.disabled},
		{Key: "custom_blocked", Name: "Custom Blocked Domains", Category: "custom", Enabled: true, CustomDomains: undo.blocked},
		{Key: "custom_direct", Name: "Custom Direct Domains", Category: "custom", Enabled: true, CustomDomains: undo.direct},
	}
	recordEntries := make([]string, 0, len(undo.records))
	for domain, value := range undo.records {
		recordEntries = append(recordEntries, domain+"="+value)
	}
	sort.Strings(recordEntries)
	saves = append(saves, database.Policy{Key: "custom_records", Name: "Custom A Records", Category: "custom", Enabled: true, CustomDomains: recordEntries})
	addedEntries := make([]string, 0, len(undo.added))
	for domain, stamp := range undo.added {
		addedEntries = append(addedEntries, domain+"="+stamp)
	}
	sort.Strings(addedEntries)
	saves = append(saves, database.Policy{Key: "custom_domain_added", Name: "Custom Domain Added Times", Category: "custom", Enabled: true, CustomDomains: addedEntries})
	for _, policy := range saves {
		if err := ws.db.SavePolicy(policy); err != nil {
			return fmt.Errorf("could not restore imported rules: %w", err)
		}
	}
	if ws.matcher != nil {
		ws.matcher.SetCustomRules(undo.proxied, undo.blocked, undo.direct, undo.records)
	}
	if ws.filters != nil {
		if err := ws.filters.SaveSettings(undo.filtering); err != nil {
			return fmt.Errorf("could not restore filtering settings: %w", err)
		}
		for _, id := range undo.listIDs {
			_ = ws.filters.DeleteList(id)
		}
	}
	if ws.clients != nil {
		for _, id := range undo.clientIDs {
			_ = ws.clients.DeleteClient(id)
		}
	}
	if len(undo.upstreams) > 0 {
		var list []string
		for _, addr := range undo.upstreams {
			next, _, status := ws.removeUpstream(addr)
			if status == 0 {
				list = next
			}
		}
		if len(list) > 0 && ws.upstreams != nil {
			ws.upstreams.SetUpstreams(list)
		}
	}
	return nil
}

func (ws *WebServer) handleMigrateReport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		httpx.WriteMethodNotAllowed(w, "GET, HEAD")
		return
	}
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	if token == "" {
		httpx.WriteJSONError(w, http.StatusBadRequest, "token is required")
		return
	}
	analysis, ok := ws.getMigrateAnalysis(token)
	if !ok {
		httpx.WriteJSONError(w, http.StatusNotFound, "This import preview has expired; analyze the file again")
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="digitdns-import-skipped-regex.txt"`)
	_, _ = w.Write([]byte(analysis.report))
}

func (ws *WebServer) handleMigrateApply(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		httpx.WriteMethodNotAllowed(w, "POST")
		return
	}
	var req struct {
		Token     string `json:"token"`
		Upstreams bool   `json:"upstreams"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		httpx.WriteJSONError(w, http.StatusBadRequest, "Invalid request payload")
		return
	}
	analysis, ok := ws.takeMigrateAnalysis(strings.TrimSpace(req.Token))
	if !ok {
		httpx.WriteJSONError(w, http.StatusGone, "This import preview has expired; analyze the file again")
		return
	}
	bundle := analysis.bundle
	bundle.UpstreamsOptIn = req.Upstreams
	result, undoToken, err := ws.applyMigration(bundle)
	if err != nil {
		httpx.WriteJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success":    true,
		"added":      result.Added,
		"skipped":    result.Skipped,
		"undo_token": undoToken,
	})
}

// applyMigration turns a previewed Bundle into live state. Rules and records
// are written first, because they are the part the operator can see and undo
// from the Domains tab; lists, clients and upstreams follow and report their
// own failures instead of rolling back a rule set that already landed.
func (ws *WebServer) applyMigration(bundle migrate.Bundle) (migrate.MergeResult, string, error) {
	target := ws.migrateTarget()
	undo := ws.captureMigrateUndo()
	trace := &migrateTrace{}
	result := migrate.Merge(target, bundle)
	if err := ws.saveMigrationRules(result.Target); err != nil {
		return result, "", fmt.Errorf("could not save the imported rules: %w", err)
	}
	if err := ws.applyMigrationFiltering(result.Target); err != nil {
		result.Skipped = append(result.Skipped, migrate.Issue{
			Code:   "filtering-settings-failed",
			Reason: err.Error(),
		})
	}
	ws.applyMigrationFilterLists(bundle, target, &result, trace)
	ws.applyMigrationClients(bundle, target, &result, trace)
	ws.applyMigrationUpstreams(bundle, target, &result, trace)
	// Rewrites were folded into result.Target.Records by Merge and are saved with
	// the rest of the custom answers; anything that did not land (a conflicting
	// answer for the same name) is already in result.Skipped.
	undo.clientIDs = trace.clientIDs
	undo.listIDs = trace.listIDs
	undo.upstreams = trace.upstreams
	undoToken, err := ws.storeMigrateUndo(undo)
	if err != nil {
		undoToken = ""
	}
	return result, undoToken, nil
}

func (ws *WebServer) migrateTarget() migrate.Target {
	target := migrate.NewTarget()
	rules := ws.loadRulesConfig()
	target.Proxied = toStringSlice(rules["custom_proxied"])
	target.Blocked = toStringSlice(rules["custom_blocked"])
	target.Direct = toStringSlice(rules["custom_direct"])
	if records, ok := rules["custom_records"].(map[string]string); ok {
		target.Records = make(map[string]string, len(records))
		for name, value := range records {
			target.Records[name] = value
		}
	}
	if ws.clients != nil {
		if clients, err := ws.clients.ListClients(); err == nil {
			for _, client := range clients {
				target.Clients = append(target.Clients, migrate.Client{
					Name:    client.Name,
					IDs:     append([]string(nil), client.AllowedIPs...),
					Enabled: client.Enabled,
				})
			}
		}
	}
	if ws.filters != nil {
		settings := ws.filters.Settings()
		target.Allowlist = append([]string(nil), settings.Allowlist...)
		target.Filtering = &migrate.Filtering{
			Enabled:             &settings.Enabled,
			BlockMode:           settings.BlockMode,
			BlockTTL:            &settings.BlockTTL,
			UpdateIntervalHours: &settings.UpdateIntervalHours,
			CustomRules:         settings.CustomRules,
		}
		if lists, err := ws.filters.Lists(); err == nil {
			for _, list := range lists {
				target.FilterLists = append(target.FilterLists, migrate.FilterList{
					Name:     list.Name,
					URL:      list.URL,
					Homepage: list.Homepage,
					License:  list.License,
					Enabled:  list.Enabled,
				})
			}
		}
	}
	ws.dnsMu.Lock()
	cfg := ws.mutableDNSLocked()
	target.Upstreams = append([]string(nil), cfg.Upstreams...)
	ws.dnsMu.Unlock()
	return target
}

func (ws *WebServer) saveMigrationRules(target migrate.Target) error {
	rules := ws.loadRulesConfig()
	disabled := toStringSlice(rules["custom_proxied_disabled"])
	added := make(map[string]string)
	if existing, ok := rules["custom_domain_added"].(map[string]string); ok {
		for domain, stamp := range existing {
			added[domain] = stamp
		}
	}
	all := make(map[string]bool)
	for _, domains := range [][]string{target.Proxied, disabled, target.Blocked, target.Direct} {
		for _, domain := range domains {
			all[domain] = true
		}
	}
	now := time.Now().UTC().Format(time.RFC3339)
	for domain := range all {
		if _, ok := added[domain]; !ok {
			added[domain] = now
		}
	}
	for domain := range added {
		if !all[domain] {
			delete(added, domain)
		}
	}

	saves := []database.Policy{
		{Key: "custom_proxied", Name: "Custom Proxied Domains", Category: "custom", Enabled: true, CustomDomains: target.Proxied},
		{Key: "custom_proxied_disabled", Name: "Disabled Proxied Domains", Category: "custom", Enabled: true, CustomDomains: disabled},
		{Key: "custom_blocked", Name: "Custom Blocked Domains", Category: "custom", Enabled: true, CustomDomains: target.Blocked},
		{Key: "custom_direct", Name: "Custom Direct Domains", Category: "custom", Enabled: true, CustomDomains: target.Direct},
	}
	recordEntries := make([]string, 0, len(target.Records))
	for domain, value := range target.Records {
		recordEntries = append(recordEntries, domain+"="+value)
	}
	sort.Strings(recordEntries)
	saves = append(saves, database.Policy{Key: "custom_records", Name: "Custom A Records", Category: "custom", Enabled: true, CustomDomains: recordEntries})
	addedEntries := make([]string, 0, len(added))
	for domain, stamp := range added {
		addedEntries = append(addedEntries, domain+"="+stamp)
	}
	sort.Strings(addedEntries)
	saves = append(saves, database.Policy{Key: "custom_domain_added", Name: "Custom Domain Added Times", Category: "custom", Enabled: true, CustomDomains: addedEntries})
	for _, policy := range saves {
		if err := ws.db.SavePolicy(policy); err != nil {
			return err
		}
	}
	if ws.matcher != nil {
		ws.matcher.SetCustomRules(target.Proxied, target.Blocked, target.Direct, target.Records)
	}
	allowed := map[string]bool{}
	for _, domain := range append(append([]string{}, target.Proxied...), disabled...) {
		if key, ok := normalizeProfileKey(domain); ok {
			allowed[key] = true
		}
	}
	if ws.pruneResolutionProfiles(allowed) {
		ws.applyResolutionPolicy()
		if ws.cache != nil {
			ws.cache.Flush()
		}
	}
	return nil
}

func (ws *WebServer) applyMigrationFiltering(target migrate.Target) error {
	if ws.filters == nil || target.Filtering == nil {
		return nil
	}
	next := ws.filters.Settings()
	if target.Filtering.Enabled != nil {
		next.Enabled = *target.Filtering.Enabled
	}
	if target.Filtering.BlockMode != "" {
		next.BlockMode = target.Filtering.BlockMode
	}
	if target.Filtering.BlockTTL != nil {
		next.BlockTTL = *target.Filtering.BlockTTL
	}
	if target.Filtering.UpdateIntervalHours != nil {
		next.UpdateIntervalHours = *target.Filtering.UpdateIntervalHours
	}
	next.Allowlist = append([]string(nil), target.Allowlist...)
	next.CustomRules = target.Filtering.CustomRules
	return ws.filters.SaveSettings(next)
}

func (ws *WebServer) applyMigrationFilterLists(bundle migrate.Bundle, target migrate.Target, result *migrate.MergeResult, trace *migrateTrace) {
	if len(bundle.FilterLists) == 0 {
		return
	}
	if ws.filters == nil {
		result.Added.FilterLists = 0
		result.Added.FilterRules = 0
		result.Skipped = append(result.Skipped, migrate.Issue{
			Code:   "filtering-unavailable",
			Reason: "the filtering engine is not available on this instance",
		})
		return
	}
	existing := make(map[string]struct{}, len(target.FilterLists))
	for _, list := range target.FilterLists {
		existing[webFilterListKey(list)] = struct{}{}
	}
	for _, list := range bundle.FilterLists {
		if _, ok := existing[webFilterListKey(list)]; ok {
			continue
		}
		added, err := ws.filters.AddListWithMeta(list.Name, list.URL, list.Content, list.Homepage, list.License, "")
		if err != nil {
			if result.Added.FilterLists > 0 {
				result.Added.FilterLists--
			}
			result.Skipped = append(result.Skipped, migrate.Issue{
				Code:   "filter-list-add-failed",
				Item:   list.Name,
				Reason: err.Error(),
			})
			continue
		}
		trace.listIDs = append(trace.listIDs, added.ID)
		if !list.Enabled {
			if err := ws.filters.SetListEnabled(added.ID, false); err != nil {
				result.Skipped = append(result.Skipped, migrate.Issue{
					Code:   "filter-list-disable-failed",
					Item:   list.Name,
					Reason: err.Error(),
				})
			}
		}
		existing[webFilterListKey(list)] = struct{}{}
	}
}

func (ws *WebServer) applyMigrationClients(bundle migrate.Bundle, target migrate.Target, result *migrate.MergeResult, trace *migrateTrace) {
	if len(bundle.Clients) == 0 || ws.clients == nil {
		return
	}
	existing := make(map[string]struct{}, len(target.Clients))
	for _, client := range target.Clients {
		existing[strings.ToLower(client.Name)] = struct{}{}
	}
	for _, client := range bundle.Clients {
		if _, ok := existing[strings.ToLower(client.Name)]; ok {
			continue
		}
		ips := validClientIPs(client.IDs)
		if len(ips) == 0 {
			if result.Added.Clients > 0 {
				result.Added.Clients--
			}
			result.Skipped = append(result.Skipped, migrate.Issue{
				Code:   "client-no-ip",
				Item:   client.Name,
				Reason: "client has no IP or CIDR identifier; MAC and hostname identifiers are not imported",
			})
			continue
		}
		created, err := ws.clients.ProvisionClient(service.CreateClientRequest{
			Name: client.Name,
			IPs:  ips,
			Note: client.Note,
		})
		if err != nil {
			if result.Added.Clients > 0 {
				result.Added.Clients--
			}
			result.Skipped = append(result.Skipped, migrate.Issue{
				Code:   "client-add-failed",
				Item:   client.Name,
				Reason: err.Error(),
			})
			continue
		}
		trace.clientIDs = append(trace.clientIDs, created.ID)
		if !client.Enabled {
			_, _ = ws.clients.ToggleClient(created.ID, false)
		}
		existing[strings.ToLower(client.Name)] = struct{}{}
	}
}

func (ws *WebServer) applyMigrationUpstreams(bundle migrate.Bundle, target migrate.Target, result *migrate.MergeResult, trace *migrateTrace) {
	if len(bundle.Upstreams) == 0 {
		return
	}
	if !bundle.UpstreamsOptIn {
		return
	}
	existing := make(map[string]struct{}, len(target.Upstreams))
	for _, addr := range target.Upstreams {
		existing[addr] = struct{}{}
	}
	var list []string
	for _, raw := range bundle.Upstreams {
		canonical, ok := upstream.NormalizeUpstream(raw)
		if !ok {
			if result.Added.Upstreams > 0 {
				result.Added.Upstreams--
			}
			result.Skipped = append(result.Skipped, migrate.Issue{
				Code:   "upstream-invalid",
				Item:   raw,
				Reason: "not a host, host:port, tls:// or https:// resolver",
			})
			continue
		}
		if _, ok := existing[canonical]; ok {
			continue
		}
		next, errMsg, status := ws.addUpstream(canonical)
		if status != 0 {
			if result.Added.Upstreams > 0 {
				result.Added.Upstreams--
			}
			result.Skipped = append(result.Skipped, migrate.Issue{
				Code:   "upstream-add-failed",
				Item:   canonical,
				Reason: errMsg,
			})
			continue
		}
		list = next
		existing[canonical] = struct{}{}
		trace.upstreams = append(trace.upstreams, canonical)
	}
	if len(list) > 0 && ws.upstreams != nil {
		ws.upstreams.SetUpstreams(list)
	}
}

func validClientIPs(ids []string) []string {
	out := make([]string, 0, len(ids))
	for _, raw := range ids {
		value := strings.TrimSpace(raw)
		if value == "" {
			continue
		}
		if strings.Contains(value, "/") {
			if _, network, err := net.ParseCIDR(value); err == nil {
				out = append(out, network.String())
			}
			continue
		}
		if ip := net.ParseIP(strings.Trim(value, "[]")); ip != nil {
			out = append(out, ip.String())
		}
	}
	return out
}

func webFilterListKey(list migrate.FilterList) string {
	if list.URL != "" {
		return "url:" + list.URL
	}
	return "name:" + strings.ToLower(list.Name)
}

func sampleBundle(bundle migrate.Bundle) migrateSample {
	sample := migrateSample{
		Proxied:   sampleStrings(bundle.Proxied),
		Blocked:   sampleStrings(bundle.Blocked),
		Direct:    sampleStrings(bundle.Direct),
		Allowlist: sampleStrings(bundle.Allowlist),
		Upstreams: sampleStrings(bundle.Upstreams),
	}
	for _, record := range bundle.Records {
		sample.Records = append(sample.Records, record.Name+" → "+record.Value)
	}
	for _, client := range bundle.Clients {
		sample.Clients = append(sample.Clients, client.Name+" ("+strings.Join(client.IDs, ", ")+")")
	}
	for _, list := range bundle.FilterLists {
		label := list.Name
		if list.URL != "" {
			label += " — " + list.URL
		} else {
			label += " (uploaded)"
		}
		sample.FilterLists = append(sample.FilterLists, label)
	}
	for _, rewrite := range bundle.Rewrites {
		sample.Rewrites = append(sample.Rewrites, rewrite.Domain+" → "+rewrite.Target)
	}
	if bundle.Filtering != nil {
		for _, line := range strings.Split(bundle.Filtering.CustomRules, "\n") {
			if trimmed := strings.TrimSpace(line); trimmed != "" {
				sample.CustomRules = append(sample.CustomRules, trimmed)
			}
		}
	}
	sample.Records = sampleStrings(sample.Records)
	sample.Clients = sampleStrings(sample.Clients)
	sample.FilterLists = sampleStrings(sample.FilterLists)
	sample.CustomRules = sampleStrings(sample.CustomRules)
	sample.Rewrites = sampleStrings(sample.Rewrites)
	return sample
}

func sampleStrings(in []string) []string {
	if len(in) > migrateSampleMax {
		return append([]string(nil), in[:migrateSampleMax]...)
	}
	return append([]string(nil), in...)
}

func buildMigrateReport(issues []migrate.Issue) string {
	var out strings.Builder
	for _, issue := range issues {
		if !strings.HasPrefix(issue.Code, "regex-") {
			continue
		}
		fmt.Fprintf(&out, "%s\t%s\t%s\n", issue.Code, issue.Item, issue.Reason)
	}
	return out.String()
}
