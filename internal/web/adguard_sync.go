package web

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"time"

	"digitdns/internal/adguard"
	"digitdns/internal/database"
)

// AdGuard coverage sync keeps DigitDNS as the primary rule store. AdGuard is a
// source of domains to import and a place to export DigitDNS-only domains back
// to; nothing here rewrites a handwritten AdGuard config in place.
const (
	syncSourcesPolicyKey     = "sync_adguard_sources"
	syncProvenancePolicyKey  = "sync_adguard_provenance"
	adGuardFetchTimeout      = 8 * time.Second
	adGuardMaxSourceBytes    = 4 << 20
	adGuardRemovalKeep       = "keep"
	adGuardRemovalSyncedOnly = "synced_only"
	adGuardRemovalMirror     = "mirror"
)

type adguardSource struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	URL             string   `json:"url"`
	Text            string   `json:"text"`
	Enabled         bool     `json:"enabled"`
	LastChecksum    string   `json:"last_checksum,omitempty"`
	LastAppliedAt   string   `json:"last_applied_at,omitempty"`
	ManagedUpstream string   `json:"managed_upstream,omitempty"`
	ManagedPatterns []string `json:"managed_patterns,omitempty"`
	Imported        []string `json:"imported,omitempty"`
	UpdatedAt       string   `json:"updated_at,omitempty"`
}

type adguardProvenance struct {
	SourceID string `json:"source_id"`
	Pattern  string `json:"pattern"`
	Group    string `json:"group,omitempty"`
	Target   string `json:"target,omitempty"`
}

type adguardSyncState struct {
	sources    []adguardSource
	provenance []adguardProvenance
}

type adguardPreviewRequest struct {
	SourceID        string `json:"source_id"`
	Name            string `json:"name"`
	URL             string `json:"url"`
	Text            string `json:"text"`
	Enabled         *bool  `json:"enabled"`
	ManagedUpstream string `json:"managed_upstream"`
	// ManagedSelection narrows only the generated block. The default preview
	// carries none, matching the dashboard's unchecked DigitDNS-only proposal.
	ManagedSelection []string `json:"managed_selection"`
}

type adguardApplyRequest struct {
	adguardPreviewRequest
	Checksum        string   `json:"checksum"`
	Import          []string `json:"import"`
	Managed         []string `json:"managed"`
	RemovalMode     string   `json:"removal_mode"`
	ConfirmMirror   bool     `json:"confirm_mirror"`
	AcknowledgeRisk bool     `json:"acknowledge_risk"`
}

type adguardSyncRow struct {
	Domain       string             `json:"domain"`
	Kind         adguard.PatternKind `json:"kind"`
	Bucket       string             `json:"bucket"`
	Group        string             `json:"group,omitempty"`
	Target       string             `json:"target,omitempty"`
	TargetIsSelf bool               `json:"target_is_self"`
	CoveredBy    string             `json:"covered_by,omitempty"`
	Warning      string             `json:"warning,omitempty"`
}

func decodeAdGuardBody(body map[string]any, out any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, out)
}

func newAdGuardSourceID() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err == nil {
		return hex.EncodeToString(buf)
	}
	return fmt.Sprintf("%016x", time.Now().UnixNano())
}

func redactSourceURL(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.User == nil {
		return value
	}
	parsed.User = nil
	return parsed.String()
}

func (ws *WebServer) adGuardSelfHosts() []string {
	hosts := []string{"127.0.0.1", "localhost"}
	if ws.settings != nil {
		if ws.settings.PublicIP != "" {
			hosts = append(hosts, ws.settings.PublicIP)
		}
		if ws.settings.PublicIPv6 != "" {
			hosts = append(hosts, ws.settings.PublicIPv6)
		}
	}
	if ws.tlsSettings != nil {
		if h := ws.tlsSettings.Domain; h != "" {
			hosts = append(hosts, h)
		}
		if h := ws.tlsSettings.DoTDomain; h != "" {
			hosts = append(hosts, h)
		}
	}
	if ws.dnsCfg != nil {
		if h := ws.dnsCfg.DoQTokenDomain; h != "" {
			hosts = append(hosts, h)
		}
	}
	return hosts
}

func (ws *WebServer) loadAdGuardSyncState() (adguardSyncState, error) {
	state := adguardSyncState{sources: []adguardSource{}, provenance: []adguardProvenance{}}
	srcPolicy, err := ws.db.GetPolicy(syncSourcesPolicyKey)
	if err != nil {
		return state, err
	}
	if srcPolicy != nil {
		state.sources = make([]adguardSource, 0, len(srcPolicy.CustomDomains))
		for _, raw := range srcPolicy.CustomDomains {
			var src adguardSource
			if err := json.Unmarshal([]byte(raw), &src); err == nil && src.ID != "" {
				state.sources = append(state.sources, src)
			}
		}
	}
	provPolicy, err := ws.db.GetPolicy(syncProvenancePolicyKey)
	if err != nil {
		return state, err
	}
	if provPolicy != nil {
		state.provenance = make([]adguardProvenance, 0, len(provPolicy.CustomDomains))
		for _, raw := range provPolicy.CustomDomains {
			var entry adguardProvenance
			if err := json.Unmarshal([]byte(raw), &entry); err == nil && entry.Pattern != "" {
				state.provenance = append(state.provenance, entry)
			}
		}
	}
	return state, nil
}

func (ws *WebServer) saveAdGuardSyncState(state adguardSyncState) error {
	sourceLines := make([]string, 0, len(state.sources))
	for _, src := range state.sources {
		raw, err := json.Marshal(src)
		if err != nil {
			return err
		}
		sourceLines = append(sourceLines, string(raw))
	}
	provLines := make([]string, 0, len(state.provenance))
	for _, entry := range state.provenance {
		raw, err := json.Marshal(entry)
		if err != nil {
			return err
		}
		provLines = append(provLines, string(raw))
	}
	return ws.db.SavePolicies([]database.Policy{
		{
			Key:           syncSourcesPolicyKey,
			Name:          "AdGuard Sync Sources",
			Category:      "sync",
			Enabled:       true,
			CustomDomains: sourceLines,
		},
		{
			Key:           syncProvenancePolicyKey,
			Name:          "AdGuard Sync Provenance",
			Category:      "sync",
			Enabled:       true,
			CustomDomains: provLines,
		},
	})
}

func findAdGuardSource(sources []adguardSource, id string) (adguardSource, int, bool) {
	for i, src := range sources {
		if src.ID == id {
			return src, i, true
		}
	}
	return adguardSource{}, -1, false
}

func (ws *WebServer) adGuardSyncSources() ([]adguardSource, error) {
	state, err := ws.loadAdGuardSyncState()
	if err != nil {
		return nil, err
	}
	return state.sources, nil
}

// adGuardSyncSaveSource creates or updates a source. A URL containing userinfo
// is stored without it: policies are deliberately unsealed, so a credential in
// a source URL would otherwise be readable from a copied data.db.
func (ws *WebServer) adGuardSyncSaveSource(body map[string]any) (adguardSource, error) {
	var req adguardPreviewRequest
	if err := decodeAdGuardBody(body, &req); err != nil {
		return adguardSource{}, err
	}
	ws.adguardSyncMu.Lock()
	defer ws.adguardSyncMu.Unlock()

	state, err := ws.loadAdGuardSyncState()
	if err != nil {
		return adguardSource{}, err
	}
	src, index, found := findAdGuardSource(state.sources, req.SourceID)
	if !found {
		src = adguardSource{ID: newAdGuardSourceID(), Enabled: true}
	}
	if strings.TrimSpace(req.Name) != "" {
		src.Name = strings.TrimSpace(req.Name)
	} else if src.Name == "" {
		src.Name = "AdGuard Home"
	}
	if strings.TrimSpace(req.URL) != "" || req.SourceID == "" {
		src.URL = redactSourceURL(req.URL)
	}
	if strings.TrimSpace(req.Text) != "" || req.SourceID == "" {
		src.Text = req.Text
	}
	if req.Enabled != nil {
		src.Enabled = *req.Enabled
	}
	if strings.TrimSpace(req.ManagedUpstream) != "" {
		src.ManagedUpstream = strings.TrimSpace(req.ManagedUpstream)
	}
	if src.Text == "" && src.URL == "" {
		return adguardSource{}, errors.New("paste an AdGuard config or provide a source URL")
	}
	src.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if src.ManagedPatterns == nil {
		src.ManagedPatterns = []string{}
	}
	if src.Imported == nil {
		src.Imported = []string{}
	}
	if found {
		state.sources[index] = src
	} else {
		state.sources = append(state.sources, src)
	}
	if err := ws.saveAdGuardSyncState(state); err != nil {
		return adguardSource{}, err
	}
	return src, nil
}

func (ws *WebServer) adGuardSyncDeleteSource(id string) (map[string]any, error) {
	ws.adguardSyncMu.Lock()
	defer ws.adguardSyncMu.Unlock()
	state, err := ws.loadAdGuardSyncState()
	if err != nil {
		return nil, err
	}
	_, index, found := findAdGuardSource(state.sources, id)
	if !found {
		return nil, adGuardSyncNotFound("AdGuard sync source")
	}
	state.sources = append(state.sources[:index], state.sources[index+1:]...)
	kept := state.provenance[:0]
	for _, entry := range state.provenance {
		if entry.SourceID != id {
			kept = append(kept, entry)
		}
	}
	state.provenance = kept
	if err := ws.saveAdGuardSyncState(state); err != nil {
		return nil, err
	}
	// Imported domains are deliberately left in the rule store: deleting a
	// source record is not a request to stop routing its domains.
	return map[string]any{"success": true, "id": id, "rules_kept": true}, nil
}

var errAdGuardSyncNotFound = errors.New("AdGuard sync source not found")

func adGuardSyncNotFound(what string) error { return fmt.Errorf("%w: %s", errAdGuardSyncNotFound, what) }

func fetchAdGuardSource(ctx context.Context, rawURL string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", errors.New("source URL must be an absolute http(s) URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", errors.New("source URL must use http or https")
	}
	ctx, cancel := context.WithTimeout(ctx, adGuardFetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return "", errors.New("could not build the source request")
	}
	req.Header.Set("User-Agent", "DigitDNS-AdGuard-Sync/1.0")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", errors.New("could not fetch the source URL")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("source URL returned HTTP %d", resp.StatusCode)
	}
	limited := io.LimitReader(resp.Body, adGuardMaxSourceBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return "", errors.New("could not read the source response")
	}
	if len(data) > adGuardMaxSourceBytes {
		return "", fmt.Errorf("source is larger than the %d-byte limit", adGuardMaxSourceBytes)
	}
	return string(data), nil
}

// adGuardPreviewText resolves a preview's source text. A failed refresh never
// replaces the last good copy: it falls back to the stored text and says so.
func (ws *WebServer) adGuardPreviewText(req *adguardPreviewRequest) (string, []string, error) {
	warnings := []string{}
	// Keep the exact bytes: the checksum returned with a preview has to match
	// the text sent back with apply, including its trailing newline.
	text := req.Text
	if strings.TrimSpace(text) == "" && strings.TrimSpace(req.URL) != "" {
		fetched, err := fetchAdGuardSource(context.Background(), req.URL)
		if err != nil {
			if req.SourceID == "" {
				return "", warnings, err
			}
			state, loadErr := ws.loadAdGuardSyncState()
			if loadErr != nil {
				return "", warnings, loadErr
			}
			src, _, ok := findAdGuardSource(state.sources, req.SourceID)
			if !ok || strings.TrimSpace(src.Text) == "" {
				return "", warnings, err
			}
			text = src.Text
			warnings = append(warnings, "Fetch failed; previewing the last good source: "+err.Error())
		} else {
			text = fetched
		}
	}
	if text == "" && req.SourceID != "" {
		state, err := ws.loadAdGuardSyncState()
		if err != nil {
			return "", warnings, err
		}
		src, _, ok := findAdGuardSource(state.sources, req.SourceID)
		if ok {
			text = src.Text
			if strings.TrimSpace(req.URL) == "" {
				req.URL = src.URL
			}
			if strings.TrimSpace(req.Name) == "" {
				req.Name = src.Name
			}
			if strings.TrimSpace(req.ManagedUpstream) == "" {
				req.ManagedUpstream = src.ManagedUpstream
			}
		}
	}
	if strings.TrimSpace(text) == "" {
		return "", warnings, errors.New("paste an AdGuard config or provide a source URL")
	}
	return text, warnings, nil
}

func patternsFromDomains(domains []string) []adguard.Pattern {
	out := make([]adguard.Pattern, 0, len(domains))
	for _, domain := range domains {
		if pattern, ok := adguard.NormalizeToken(domain); ok {
			out = append(out, pattern)
		}
	}
	return out
}

// candidatePatterns collapses one source's overlapping rules. A plain rule and
// its `*.child` form both cover the same children under DigitDNS semantics, so
// only the broader one is proposed for import; the narrower one is shown as
// already covered by it.
func candidatePatterns(parsed []adguard.Pattern) ([]adguard.Pattern, map[string]string) {
	candidates := make([]adguard.Pattern, 0, len(parsed))
	coveredBy := make(map[string]string, len(parsed))
	for _, pattern := range parsed {
		key := pattern.String()
		absorbed := false
		for _, existing := range candidates {
			if adguard.Covers(existing, pattern) {
				coveredBy[key] = existing.String()
				absorbed = true
				break
			}
		}
		if absorbed {
			continue
		}
		kept := candidates[:0]
		for _, existing := range candidates {
			if adguard.Covers(pattern, existing) {
				coveredBy[existing.String()] = key
			} else {
				kept = append(kept, existing)
			}
		}
		candidates = append(kept, pattern)
	}
	return candidates, coveredBy
}

func rowForPattern(pattern adguard.Pattern, bucket string) adguardSyncRow {
	row := adguardSyncRow{
		Domain:       pattern.String(),
		Kind:         pattern.Kind,
		Bucket:       bucket,
		Group:        pattern.Group,
		Target:       pattern.Target,
		TargetIsSelf: pattern.TargetIsSelf,
	}
	if !pattern.TargetIsSelf && pattern.Target != "" {
		row.Warning = "Target is not this DigitDNS server; importing the domain will not change AdGuard routing."
	}
	return row
}

func (ws *WebServer) adGuardPreview(req adguardPreviewRequest) (map[string]any, error) {
	text, warnings, err := ws.adGuardPreviewText(&req)
	if err != nil {
		return nil, err
	}
	parsed := adguard.Parse(text, ws.adGuardSelfHosts())
	warnings = append(warnings, parsed.Warnings...)

	rules := ws.loadRulesConfig()
	proxyDomains := toStringSlice(rules["custom_proxied"])
	blockedDomains := toStringSlice(rules["custom_blocked"])
	directDomains := toStringSlice(rules["custom_direct"])
	disabledDomains := toStringSlice(rules["custom_proxied_disabled"])
	proxied := patternsFromDomains(proxyDomains)
	nonProxy := append(patternsFromDomains(blockedDomains), patternsFromDomains(directDomains)...)
	nonProxy = append(nonProxy, patternsFromDomains(disabledDomains)...)

	candidates, coveredBy := candidatePatterns(parsed.Patterns)
	rows := make([]adguardSyncRow, 0, len(parsed.Patterns))
	counts := map[string]int{"covered": 0, "adguardOnly": 0, "digitdnsOnly": 0, "conflict": 0}
	for _, pattern := range parsed.Patterns {
		bucket := "adguardOnly"
		if covered, ok := coveredBy[pattern.String()]; ok {
			bucket = "covered"
			row := rowForPattern(pattern, bucket)
			row.CoveredBy = covered
			rows = append(rows, row)
			counts[bucket]++
			continue
		}
		if conflict, ok := adguard.CoveredByAny(pattern, nonProxy); ok {
			bucket = "conflict"
			row := rowForPattern(pattern, bucket)
			row.CoveredBy = conflict.String()
			rows = append(rows, row)
			counts[bucket]++
			continue
		}
		if existing, ok := adguard.CoveredByAny(pattern, proxied); ok {
			bucket = "covered"
			row := rowForPattern(pattern, bucket)
			row.CoveredBy = existing.String()
			rows = append(rows, row)
			counts[bucket]++
			continue
		}
		rows = append(rows, rowForPattern(pattern, bucket))
		counts[bucket]++
	}

	managed := make([]adguardSyncRow, 0)
	for _, existing := range proxied {
		if _, ok := adguard.CoveredByAny(existing, parsed.Patterns); ok {
			continue
		}
		row := rowForPattern(existing, "digitdnsOnly")
		row.Target = req.ManagedUpstream
		managed = append(managed, row)
		counts["digitdnsOnly"]++
	}
	sort.Slice(managed, func(i, j int) bool { return managed[i].Domain < managed[j].Domain })

	upstream := strings.TrimSpace(req.ManagedUpstream)
	if upstream == "" {
		for _, group := range parsed.Groups {
			if group.TargetIsSelf {
				upstream = group.Target
				break
			}
		}
		if upstream == "" && len(parsed.Groups) > 0 {
			upstream = parsed.Groups[0].Target
		}
	}
	managedAllowed := make(map[string]bool, len(managed))
	for _, row := range managed {
		managedAllowed[row.Domain] = true
	}
	for _, domain := range req.ManagedSelection {
		if !managedAllowed[domain] {
			return nil, fmt.Errorf("managed selection %q is no longer DigitDNS-only", domain)
		}
	}
	// No selection means no generated block: the panel's DigitDNS-only proposal
	// starts unchecked, and an export must never include a domain the operator
	// did not choose.
	managedPatterns := make([]adguard.Pattern, 0, len(req.ManagedSelection))
	for _, domain := range req.ManagedSelection {
		if pattern, ok := adguard.NormalizeToken(domain); ok {
			managedPatterns = append(managedPatterns, pattern)
		}
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = "AdGuard Home"
	}
	return map[string]any{
		"source_id":       req.SourceID,
		"name":            name,
		"url":             redactSourceURL(req.URL),
		"enabled":         req.Enabled == nil || *req.Enabled,
		"text":            text,
		"checksum":        adguard.Checksum(text),
		"upstream":        upstream,
		"groups":          parsed.Groups,
		"rows":            rows,
		"managed":         managed,
		"counts":          counts,
		"warnings":        warnings,
		"generated_block": adguard.GenerateManagedBlock(managedPatterns, upstream),
		"candidates":      len(candidates),
	}, nil
}

func (ws *WebServer) adGuardSyncPreview(body map[string]any) (map[string]any, error) {
	var req adguardPreviewRequest
	if err := decodeAdGuardBody(body, &req); err != nil {
		return nil, err
	}
	return ws.adGuardPreview(req)
}

func stringSliceContains(list []string, want string) bool {
	for _, value := range list {
		if value == want {
			return true
		}
	}
	return false
}

// saveProxiedRules reuses the dashboard's own rules handler so a sync apply
// persists, reloads the matcher and refreshes relay assignments exactly as a
// manual save does. No second copy of those side effects lives here.
func (ws *WebServer) saveProxiedRules(domains []string) error {
	payload, err := json.Marshal(map[string]any{"custom_proxied": domains})
	if err != nil {
		return err
	}
	req := httptest.NewRequest(http.MethodPost, "/api/config/rules", bytes.NewReader(payload))
	rec := httptest.NewRecorder()
	ws.handleConfigRules(rec, req)
	if rec.Code < 400 {
		return nil
	}
	var problem struct {
		Error string `json:"error"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &problem)
	if problem.Error != "" {
		return errors.New(problem.Error)
	}
	return fmt.Errorf("rules save failed with HTTP %d", rec.Code)
}

func (ws *WebServer) adGuardSyncApply(r *http.Request, body map[string]any) (map[string]any, error) {
	var req adguardApplyRequest
	if err := decodeAdGuardBody(body, &req); err != nil {
		return nil, err
	}
	if req.Checksum == "" || req.Checksum != adguard.Checksum(req.Text) {
		return nil, errors.New("preview checksum does not match the source text; preview again")
	}
	if req.RemovalMode == "" {
		req.RemovalMode = adGuardRemovalKeep
	}
	switch req.RemovalMode {
	case adGuardRemovalKeep, adGuardRemovalSyncedOnly, adGuardRemovalMirror:
	default:
		return nil, errors.New("unknown removal mode")
	}
	if req.RemovalMode == adGuardRemovalMirror && !req.ConfirmMirror {
		return nil, errors.New("mirror mode requires explicit confirmation")
	}

	ws.adguardSyncMu.Lock()
	defer ws.adguardSyncMu.Unlock()

	preview, err := ws.adGuardPreview(req.adguardPreviewRequest)
	if err != nil {
		return nil, err
	}
	warnings, _ := preview["warnings"].([]string)
	counts, _ := preview["counts"].(map[string]int)
	conflicts := counts["conflict"]
	hasBroad := false
	for _, warning := range warnings {
		if strings.Contains(warning, "broad domain") {
			hasBroad = true
		}
	}
	if (conflicts > 0 || hasBroad) && !req.AcknowledgeRisk {
		return nil, errors.New("preview has conflicts or broad domains; acknowledge the risks before applying")
	}

	allowed := map[string]bool{}
	rows, _ := preview["rows"].([]adguardSyncRow)
	for _, row := range rows {
		if row.Bucket == "adguardOnly" {
			allowed[row.Domain] = true
		}
	}
	managedAllowed := map[string]bool{}
	managedRows, _ := preview["managed"].([]adguardSyncRow)
	for _, row := range managedRows {
		managedAllowed[row.Domain] = true
	}
	for _, domain := range req.Import {
		if !allowed[domain] {
			return nil, fmt.Errorf("selection %q is no longer an uncovered AdGuard domain; preview again", domain)
		}
	}
	for _, domain := range req.Managed {
		if !managedAllowed[domain] {
			return nil, fmt.Errorf("managed proposal %q is no longer DigitDNS-only", domain)
		}
	}

	state, err := ws.loadAdGuardSyncState()
	if err != nil {
		return nil, err
	}
	src, index, found := findAdGuardSource(state.sources, req.SourceID)
	if !found {
		src = adguardSource{
			ID:              newAdGuardSourceID(),
			Enabled:         true,
			ManagedPatterns: []string{},
			Imported:        []string{},
		}
	}
	if strings.TrimSpace(req.Name) != "" {
		src.Name = strings.TrimSpace(req.Name)
	} else if src.Name == "" {
		src.Name = "AdGuard Home"
	}
	if strings.TrimSpace(req.URL) != "" || !found {
		src.URL = redactSourceURL(req.URL)
	}
	src.Text = req.Text
	if req.Enabled != nil {
		src.Enabled = *req.Enabled
	}
	upstream, _ := preview["upstream"].(string)
	if strings.TrimSpace(req.ManagedUpstream) != "" {
		upstream = strings.TrimSpace(req.ManagedUpstream)
	}
	src.ManagedUpstream = upstream
	src.ManagedPatterns = append([]string{}, req.Managed...)
	src.LastChecksum = req.Checksum
	src.LastAppliedAt = time.Now().UTC().Format(time.RFC3339)
	src.UpdatedAt = src.LastAppliedAt

	rules := ws.loadRulesConfig()
	oldProxied := toStringSlice(rules["custom_proxied"])
	newProxied := append([]string{}, oldProxied...)
	existing := make(map[string]bool, len(newProxied))
	for _, domain := range newProxied {
		existing[domain] = true
	}
	for _, domain := range req.Import {
		if !existing[domain] {
			newProxied = append(newProxied, domain)
			existing[domain] = true
		}
	}

	parsed := parsedRows(preview)
	parsedKeys := make(map[string]bool, len(parsed))
	parsedByKey := make(map[string]adguard.Pattern, len(parsed))
	for _, pattern := range parsed {
		key := pattern.String()
		parsedKeys[key] = true
		parsedByKey[key] = pattern
	}
	remove := make(map[string]bool)
	if req.RemovalMode != adGuardRemovalKeep {
		for _, entry := range state.provenance {
			if entry.SourceID == src.ID && !parsedKeys[entry.Pattern] {
				remove[entry.Pattern] = true
			}
		}
	}
	if req.RemovalMode == adGuardRemovalMirror {
		// Mirror makes this source authoritative for custom_proxied: anything it
		// does not name (or cover) is removed unless another source owns it. It
		// is the destructive mode the UI makes the operator acknowledge.
		otherSources := make(map[string]bool)
		for _, entry := range state.provenance {
			if entry.SourceID != src.ID {
				otherSources[entry.Pattern] = true
			}
		}
		for _, domain := range newProxied {
			if otherSources[domain] || parsedKeys[domain] || stringSliceContains(req.Import, domain) {
				continue
			}
			pattern, ok := adguard.NormalizeToken(domain)
			if !ok {
				continue
			}
			if _, covered := adguard.CoveredByAny(pattern, parsed); !covered {
				remove[domain] = true
			}
		}
	}
	if len(remove) > 0 {
		kept := newProxied[:0]
		for _, domain := range newProxied {
			if !remove[domain] {
				kept = append(kept, domain)
			}
		}
		newProxied = kept
	}
	sort.Strings(newProxied)

	if err := ws.saveProxiedRules(newProxied); err != nil {
		return nil, err
	}
	// Provenance means "this source put the rule there", not "the source also
	// happens to cover it". A domain that was already in DigitDNS and shown as
	// covered must stay a manual rule; otherwise a later synced_only removal
	// could delete an operator's own entry.
	owned := make(map[string]adguardProvenance)
	for _, entry := range state.provenance {
		if entry.SourceID == src.ID && parsedKeys[entry.Pattern] && stringSliceContains(newProxied, entry.Pattern) {
			owned[entry.Pattern] = entry
		}
	}
	for _, domain := range req.Import {
		if !stringSliceContains(newProxied, domain) {
			continue
		}
		entry := adguardProvenance{SourceID: src.ID, Pattern: domain}
		if pattern, ok := parsedByKey[domain]; ok {
			entry.Group, entry.Target = pattern.Group, pattern.Target
		}
		owned[domain] = entry
	}
	src.Imported = make([]string, 0, len(owned))
	for domain := range owned {
		src.Imported = append(src.Imported, domain)
	}
	sort.Strings(src.Imported)

	if found {
		state.sources[index] = src
	} else {
		state.sources = append(state.sources, src)
	}
	keptProv := make([]adguardProvenance, 0, len(state.provenance))
	for _, entry := range state.provenance {
		if entry.SourceID == src.ID {
			continue
		}
		keptProv = append(keptProv, entry)
	}
	for _, domain := range src.Imported {
		keptProv = append(keptProv, owned[domain])
	}
	state.provenance = keptProv
	if err := ws.saveAdGuardSyncState(state); err != nil {
		// Rule state is already live; put the old list back rather than leaving
		// provenance describing a set of domains that was never applied.
		_ = ws.saveProxiedRules(oldProxied)
		return nil, err
	}

	return map[string]any{
		"success":       true,
		"source":        src,
		"counts":        countsOfRows(rows),
		"imported":      len(req.Import),
		"removed":       len(remove),
		"generated_block": adguard.GenerateManagedBlock(patternsFromStrings(req.Managed), upstream),
	}, nil
}

func parsedRows(preview map[string]any) []adguard.Pattern {
	rows, _ := preview["rows"].([]adguardSyncRow)
	out := make([]adguard.Pattern, 0, len(rows))
	for _, row := range rows {
		pattern, ok := adguard.NormalizeToken(row.Domain)
		if !ok {
			continue
		}
		pattern.Group = row.Group
		pattern.Target = row.Target
		pattern.TargetIsSelf = row.TargetIsSelf
		out = append(out, pattern)
	}
	return out
}

func patternsFromStrings(domains []string) []adguard.Pattern {
	out := make([]adguard.Pattern, 0, len(domains))
	for _, domain := range domains {
		if pattern, ok := adguard.NormalizeToken(domain); ok {
			out = append(out, pattern)
		}
	}
	return out
}

func countsOfRows(rows []adguardSyncRow) map[string]int {
	counts := map[string]int{"covered": 0, "adguardOnly": 0, "conflict": 0}
	for _, row := range rows {
		counts[row.Bucket]++
	}
	return counts
}

func (ws *WebServer) adGuardSyncRefreshSource(r *http.Request, id string) (map[string]any, error) {
	ws.adguardSyncMu.Lock()
	defer ws.adguardSyncMu.Unlock()
	state, err := ws.loadAdGuardSyncState()
	if err != nil {
		return nil, err
	}
	src, index, found := findAdGuardSource(state.sources, id)
	if !found {
		return nil, adGuardSyncNotFound("AdGuard sync source")
	}
	if strings.TrimSpace(src.URL) == "" {
		return map[string]any{"source": src, "warning": "This source has no URL; paste updated text to refresh it."}, nil
	}
	text, err := fetchAdGuardSource(r.Context(), src.URL)
	if err != nil {
		return map[string]any{"source": src, "warning": "Fetch failed; the last good source was kept. " + err.Error()}, nil
	}
	src.Text = text
	src.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	state.sources[index] = src
	if err := ws.saveAdGuardSyncState(state); err != nil {
		return nil, err
	}
	return map[string]any{"source": src, "warning": ""}, nil
}

func (ws *WebServer) adGuardSyncExport(id string) (map[string]any, error) {
	state, err := ws.loadAdGuardSyncState()
	if err != nil {
		return nil, err
	}
	src, _, found := findAdGuardSource(state.sources, id)
	if !found {
		return nil, adGuardSyncNotFound("AdGuard sync source")
	}
	patterns := patternsFromStrings(src.ManagedPatterns)
	return map[string]any{
		"source_id": src.ID,
		"name":      src.Name,
		"upstream":  src.ManagedUpstream,
		"block":     adguard.GenerateManagedBlock(patterns, src.ManagedUpstream),
		"filename":  "digitdns-managed-adguard.txt",
	}, nil
}
