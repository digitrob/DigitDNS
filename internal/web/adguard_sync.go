package web

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
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
	// Adblock names the filter rules this source imported into filtering
	// custom_rules, so synced_only/mirror can withdraw just its own entries.
	Adblock   []string `json:"adblock,omitempty"`
	UpdatedAt string   `json:"updated_at,omitempty"`
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
	// The review fields below are new in v5.11. They travel on preview and on
	// apply so the checksum can cover the whole review, not just the text.
	DomainActions   map[string]string `json:"domain_actions,omitempty"`
	AdblockDomains  []string          `json:"adblock_domains,omitempty"`
	RemovalMode     string            `json:"removal_mode,omitempty"`
	ConfirmMirror   *bool             `json:"confirm_mirror,omitempty"`
	AcknowledgeRisk *bool             `json:"acknowledge_risk,omitempty"`
}

type adguardApplyRequest struct {
	adguardPreviewRequest
	Checksum string   `json:"checksum"`
	Import   []string `json:"import"`
	Managed  []string `json:"managed"`
	// RemovalMode, ConfirmMirror and AcknowledgeRisk live on the embedded
	// preview request; duplicating their JSON tags here would make the apply
	// copy authoritative and silently disagree with the preview checksum.
}

type adguardSyncRow struct {
	Domain       string              `json:"domain"`
	Kind         adguard.PatternKind `json:"kind"`
	Bucket       string              `json:"bucket"`
	Group        string              `json:"group,omitempty"`
	Target       string              `json:"target,omitempty"`
	TargetIsSelf bool                `json:"target_is_self"`
	CoveredBy    string              `json:"covered_by,omitempty"`
	Warning      string              `json:"warning,omitempty"`
	Broad        bool                `json:"broad"`
	// InDigitDNS is set only for rows already covered by a DigitDNS route, so
	// clients can dim them. Conflict rows stay visible for review.
	InDigitDNS      bool     `json:"in_digitdns"`
	SuggestedAction string   `json:"suggested_action"`
	Variants        []string `json:"variants,omitempty"`
}

// adguardNotice is the operator-facing replacement for raw parser warnings.
// Warnings remain in the API for compatibility and risk checks, but the UI
// renders these short, grouped messages instead of a wall of domain lines.
type adguardNotice struct {
	Kind    string   `json:"kind"`
	Title   string   `json:"title"`
	Message string   `json:"message"`
	Targets []string `json:"targets,omitempty"`
	Domains []string `json:"domains,omitempty"`
}

func decodeAdGuardBody(body map[string]any, out any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, out)
}

const (
	adGuardActionProxy  = "proxy"
	adGuardActionDirect = "direct"
	adGuardActionBlock  = "block"
)

// normalizeAdGuardUpstream folds the UI's "this-server" shorthand into the
// stored/self form: an empty managed upstream means "route through DigitDNS".
func normalizeAdGuardUpstream(raw string) string {
	value := strings.TrimSpace(raw)
	if strings.EqualFold(value, "this-server") {
		return ""
	}
	return value
}

// normalizeAdGuardFilterDomain turns a source domain (including a wildcard) into
// the host used in an AdGuard-style custom filter line.
func normalizeAdGuardFilterDomain(raw string) string {
	value := strings.ToLower(strings.TrimSpace(raw))
	value = strings.TrimPrefix(value, "||")
	value = strings.TrimSuffix(value, "^")
	value = strings.TrimPrefix(value, "*.")
	return strings.TrimSuffix(value, ".")
}

// normalizeAdGuardPatternKey keeps the DigitDNS spelling (a leading "*." is
// meaningful) while making review keys comparable on both sides of a preview.
func normalizeAdGuardPatternKey(raw string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(raw)), ".")
}

// adGuardScopeTargets mirrors the Domains list quick-action menu: exact, the host
// wildcard and the parent wildcard. A wildcard already named by the AdGuard list
// is a complete scope, so it is kept verbatim; deriving more patterns from it
// would stack a wildcard on top of a wildcard (or broaden the listed rule).
func adGuardScopeTargets(domain string) []string {
	raw := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(domain)), ".")
	if raw == "" {
		return nil
	}
	if strings.HasPrefix(raw, "*.") {
		return []string{raw}
	}
	base := raw
	labels := strings.Split(base, ".")
	targets := []string{base}
	if len(labels) >= 2 {
		targets = append(targets, "*."+base)
	}
	if len(labels) >= 3 {
		targets = append(targets, "*."+strings.Join(labels[1:], "."))
	}
	return targets
}

func canonicalAdGuardDomainActions(in map[string]string) (map[string]string, error) {
	out := make(map[string]string, len(in))
	for rawKey, rawAction := range in {
		key := normalizeAdGuardPatternKey(rawKey)
		action := strings.TrimSpace(rawAction)
		switch action {
		case adGuardActionProxy, adGuardActionDirect, adGuardActionBlock:
		default:
			return nil, fmt.Errorf("invalid domain action %q for %q; use proxy, direct or block", rawAction, rawKey)
		}
		if key == "" {
			return nil, errors.New("domain action has an empty domain")
		}
		out[key] = action
	}
	return out, nil
}

func canonicalAdGuardAdblockDomains(in []string) []string {
	out := make([]string, 0, len(in))
	seen := make(map[string]bool, len(in))
	for _, raw := range in {
		domain := normalizeAdGuardFilterDomain(raw)
		if domain == "" || seen[domain] {
			continue
		}
		seen[domain] = true
		out = append(out, domain)
	}
	sort.Strings(out)
	return out
}

// adGuardHasReview reports whether the request carries v5.11 review choices.
// A v5.10 preview sends only text/upstream/selection, so its checksum must stay
// the byte-exact hash of the text.
func adGuardHasReview(req adguardPreviewRequest) bool {
	return len(req.DomainActions) > 0 || len(req.AdblockDomains) > 0 || req.RemovalMode != "" ||
		req.ConfirmMirror != nil || req.AcknowledgeRisk != nil
}

type adGuardReviewPayload struct {
	Text            string            `json:"text"`
	ManagedUpstream string            `json:"managed_upstream"`
	RemovalMode     string            `json:"removal_mode"`
	ConfirmMirror   bool              `json:"confirm_mirror"`
	AcknowledgeRisk bool              `json:"acknowledge_risk"`
	DomainActions   map[string]string `json:"domain_actions"`
	AdblockDomains  []string          `json:"adblock_domains"`
}

// adGuardReviewChecksum binds the operator's review choices to the source text.
// json.Marshal sorts map keys; the domain list is sorted during normalization.
func adGuardReviewChecksum(req adguardPreviewRequest) string {
	actions, _ := canonicalAdGuardDomainActions(req.DomainActions)
	mode := strings.TrimSpace(req.RemovalMode)
	if mode == "" {
		mode = adGuardRemovalKeep
	}
	payload := adGuardReviewPayload{
		Text:            req.Text,
		ManagedUpstream: normalizeAdGuardUpstream(req.ManagedUpstream),
		RemovalMode:     mode,
		ConfirmMirror:   req.ConfirmMirror != nil && *req.ConfirmMirror,
		AcknowledgeRisk: req.AcknowledgeRisk != nil && *req.AcknowledgeRisk,
		DomainActions:   actions,
		AdblockDomains:  canonicalAdGuardAdblockDomains(req.AdblockDomains),
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func adGuardRequestChecksum(req adguardPreviewRequest) string {
	if !adGuardHasReview(req) {
		return adguard.Checksum(req.Text)
	}
	return adGuardReviewChecksum(req)
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
		// A second card with the same pasted config is almost always a double
		// click, not a second source. Reuse the existing record (and its ID,
		// provenance and imported list) instead of splitting ownership. An
		// explicit source_id still edits exactly that source.
		if strings.TrimSpace(req.Text) != "" {
			want := adguard.Checksum(req.Text)
			for i, candidate := range state.sources {
				if adguard.Checksum(candidate.Text) == want {
					src, index, found = candidate, i, true
					break
				}
			}
		}
	}
	if !found {
		src = adguardSource{ID: newAdGuardSourceID(), Enabled: true}
	}
	if strings.TrimSpace(req.Name) != "" {
		src.Name = strings.TrimSpace(req.Name)
	} else if src.Name == "" {
		src.Name = "AdGuard Home"
	}
	if strings.TrimSpace(req.URL) != "" || !found {
		src.URL = redactSourceURL(req.URL)
	}
	if strings.TrimSpace(req.Text) != "" || !found {
		src.Text = req.Text
	}
	if req.Enabled != nil {
		src.Enabled = *req.Enabled
	}
	if upstream := normalizeAdGuardUpstream(req.ManagedUpstream); upstream != "" {
		src.ManagedUpstream = upstream
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

func adGuardSyncNotFound(what string) error {
	return fmt.Errorf("%w: %s", errAdGuardSyncNotFound, what)
}

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
				req.ManagedUpstream = normalizeAdGuardUpstream(src.ManagedUpstream)
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
		Broad:        pattern.Broad,
	}
	if !pattern.TargetIsSelf && pattern.Target != "" {
		row.Warning = "Target is not this DigitDNS server; importing the domain will not change AdGuard routing."
	}
	return row
}

// adGuardGroupKey ties a row to the parsed upstream block it came from.
func adGuardGroupKey(group, target string) string {
	return group + "\x00" + target
}

func adGuardVariantsByGroup(groups []adguard.Group) map[string][]string {
	out := make(map[string][]string, len(groups))
	for _, group := range groups {
		seen := make(map[string]bool, len(group.Patterns))
		variants := make([]string, 0, len(group.Patterns))
		for _, pattern := range group.Patterns {
			domain := pattern.String()
			if seen[domain] {
				continue
			}
			seen[domain] = true
			variants = append(variants, domain)
		}
		out[adGuardGroupKey(group.Label, group.Target)] = variants
	}
	return out
}

func adGuardVariantsForRow(row adguardSyncRow, byGroup map[string][]string) []string {
	candidates := byGroup[adGuardGroupKey(row.Group, row.Target)]
	out := make([]string, 0, len(candidates))
	seen := map[string]bool{row.Domain: true}
	for _, domain := range candidates {
		if seen[domain] {
			continue
		}
		seen[domain] = true
		out = append(out, domain)
	}
	sort.Strings(out)
	if len(out) > 8 {
		out = out[:8]
	}
	return out
}

// adGuardRouteAction reports the DigitDNS action for an existing rule. Block and
// direct win over a same-shaped proxy entry because handleConfigRules keeps a
// domain in exactly one writeable list.
func adGuardRouteAction(coveredBy string, proxied, blocked, direct, disabled []string) string {
	lists := []struct {
		domains []string
		action  string
	}{
		{blocked, adGuardActionBlock},
		{direct, adGuardActionDirect},
		{proxied, adGuardActionProxy},
		{disabled, adGuardActionProxy},
	}
	for _, list := range lists {
		if stringSliceContains(list.domains, coveredBy) {
			return list.action
		}
	}
	pattern, ok := adguard.NormalizeToken(coveredBy)
	if !ok || pattern.Value == "" {
		return ""
	}
	for _, list := range lists {
		for _, domain := range list.domains {
			existing, ok := adguard.NormalizeToken(domain)
			if ok && adguard.Covers(existing, pattern) {
				return list.action
			}
		}
	}
	return ""
}

func adGuardAnnotateRow(row *adguardSyncRow, variants map[string][]string, proxied, blocked, direct, disabled []string) {
	switch row.Bucket {
	case "covered", "conflict":
		// Conflicts deliberately stay visible: in_digitdns is the hint clients
		// use to dim/hide rows, and an operator must still be able to review
		// a rule that disagrees with DigitDNS.
		row.InDigitDNS = row.Bucket == "covered"
		row.SuggestedAction = adGuardRouteAction(row.CoveredBy, proxied, blocked, direct, disabled)
	default:
		row.SuggestedAction = adGuardActionProxy
	}
	if row.SuggestedAction == "" {
		row.SuggestedAction = adGuardActionProxy
	}
	row.Variants = adGuardVariantsForRow(*row, variants)
}

func appendUnique(list []string, value string) []string {
	if value == "" || stringSliceContains(list, value) {
		return list
	}
	return append(list, value)
}

// adGuardPreviewNotices groups the two noisy parser warnings into one message
// each. The old one-line-per-domain strings stay in `warnings` for API clients
// that still read them, but the dashboard and app show these instead.
func adGuardPreviewNotices(groups []adguard.Group, patterns []adguard.Pattern, conflicts, covered int) ([]adguardNotice, []string, int) {
	foreign := make([]string, 0)
	for _, group := range groups {
		if group.TargetIsSelf {
			continue
		}
		foreign = appendUnique(foreign, adguard.TargetHost(group.Target))
	}
	sort.Strings(foreign)

	broad := make([]string, 0)
	for _, pattern := range patterns {
		if pattern.Broad {
			broad = appendUnique(broad, pattern.String())
		}
	}
	sort.Strings(broad)

	notices := make([]adguardNotice, 0, 5)
	if len(foreign) > 0 {
		notices = append(notices, adguardNotice{
			Kind:    "upstream",
			Title:   "External upstream targets",
			Message: "These upstream targets are not this DigitDNS server: " + strings.Join(foreign, ", ") + ". Importing copies their domains into DigitDNS; AdGuard will keep routing them there until you change its upstream settings.",
			Targets: foreign,
		})
	}
	if len(broad) > 0 {
		examples := broad
		if len(examples) > 4 {
			examples = examples[:4]
		}
		notices = append(notices, adguardNotice{
			Kind:    "broad",
			Title:   "High-traffic domains",
			Message: fmt.Sprintf("%d high-traffic domains are included (for example: %s). They can route a large share of ordinary traffic, so review them before applying.", len(broad), strings.Join(examples, ", ")),
			Domains: examples,
		})
	}
	if conflicts > 0 {
		notices = append(notices, adguardNotice{
			Kind:    "conflict",
			Title:   "Rules worth a second look",
			Message: fmt.Sprintf("%d domains already have a different DigitDNS action. They stay in your list to review; nothing is overwritten without your choice.", conflicts),
		})
	}
	if covered > 0 {
		notices = append(notices, adguardNotice{
			Kind:    "hidden",
			Title:   "Already in DigitDNS",
			Message: fmt.Sprintf("%d domains are already covered by your DigitDNS rules. They are dimmed in the list and keep their current action.", covered),
		})
	}
	notices = append(notices, adguardNotice{
		Kind:    "unchanged",
		Title:   "What will not change",
		Message: "Your existing DigitDNS routes, disabled proxy entries and AdGuard upstream settings stay as they are. Only domains you select are added.",
	})
	return notices, foreign, len(broad)
}

func (ws *WebServer) adGuardPreview(req adguardPreviewRequest) (map[string]any, error) {
	if _, err := canonicalAdGuardDomainActions(req.DomainActions); err != nil {
		return nil, err
	}
	req.ManagedUpstream = normalizeAdGuardUpstream(req.ManagedUpstream)
	text, warnings, err := ws.adGuardPreviewText(&req)
	if err != nil {
		return nil, err
	}
	// The checksum covers the resolved text: a source_id-only preview still
	// returns the text the client must send back with its review decisions.
	req.Text = text
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
	variantsByGroup := adGuardVariantsByGroup(parsed.Groups)
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
	for i := range rows {
		adGuardAnnotateRow(&rows[i], variantsByGroup, proxyDomains, blockedDomains, directDomains, disabledDomains)
	}

	managed := make([]adguardSyncRow, 0)
	for _, existing := range proxied {
		if _, ok := adguard.CoveredByAny(existing, parsed.Patterns); ok {
			continue
		}
		row := rowForPattern(existing, "digitdnsOnly")
		row.Target = req.ManagedUpstream
		row.InDigitDNS = true
		row.SuggestedAction = adGuardActionProxy
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
	notices, foreignTargets, broadCount := adGuardPreviewNotices(parsed.Groups, parsed.Patterns, counts["conflict"], counts["covered"])
	routeActions := make(map[string]string, len(rows))
	for _, row := range rows {
		routeActions[row.Domain] = row.SuggestedAction
	}
	return map[string]any{
		"source_id":       req.SourceID,
		"name":            name,
		"url":             redactSourceURL(req.URL),
		"enabled":         req.Enabled == nil || *req.Enabled,
		"text":            text,
		"checksum":        adGuardRequestChecksum(req),
		"upstream":        upstream,
		"groups":          parsed.Groups,
		"rows":            rows,
		"managed":         managed,
		"route_actions":   routeActions,
		"counts":          counts,
		"warnings":        warnings,
		"notices":         notices,
		"broad_count":     broadCount,
		"foreign_targets": foreignTargets,
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

type adGuardRouteLists struct {
	proxied  []string
	blocked  []string
	direct   []string
	disabled []string
}

func (ws *WebServer) adGuardCurrentRoutes() adGuardRouteLists {
	rules := ws.loadRulesConfig()
	return adGuardRouteLists{
		proxied:  toStringSlice(rules["custom_proxied"]),
		blocked:  toStringSlice(rules["custom_blocked"]),
		direct:   toStringSlice(rules["custom_direct"]),
		disabled: toStringSlice(rules["custom_proxied_disabled"]),
	}
}

func (lists adGuardRouteLists) contains(domain string) bool {
	return stringSliceContains(lists.proxied, domain) ||
		stringSliceContains(lists.blocked, domain) ||
		stringSliceContains(lists.direct, domain) ||
		stringSliceContains(lists.disabled, domain)
}

func (lists adGuardRouteLists) clone() adGuardRouteLists {
	return adGuardRouteLists{
		proxied:  append([]string{}, lists.proxied...),
		blocked:  append([]string{}, lists.blocked...),
		direct:   append([]string{}, lists.direct...),
		disabled: append([]string{}, lists.disabled...),
	}
}

func removeAdGuardDomain(domains []string, domain string) []string {
	kept := domains[:0]
	for _, value := range domains {
		if value != domain {
			kept = append(kept, value)
		}
	}
	return kept
}

// saveAdGuardRouteLists sends all four writeable/display route lists through
// the dashboard handler in one call, keeping its matcher, profile and relay
// side effects instead of writing the policies directly.
func (ws *WebServer) saveAdGuardRouteLists(lists adGuardRouteLists) error {
	payload, err := json.Marshal(map[string]any{
		"custom_proxied":          lists.proxied,
		"custom_blocked":          lists.blocked,
		"custom_direct":           lists.direct,
		"custom_proxied_disabled": lists.disabled,
	})
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

// adGuardApplyFiltering adds the selected adblock domains to the shared
// filtering custom_rules (no separate list or hive). Lines this source owned
// are withdrawn only in synced_only/mirror modes. It reports how many lines
// were added plus the domains still tracked for this source.
func (ws *WebServer) adGuardApplyFiltering(selected, previous []string, removePrevious bool) (int, []string, error) {
	tracked := make([]string, 0, len(selected)+len(previous))
	trackedSeen := make(map[string]bool, len(selected)+len(previous))
	track := func(domain string) {
		if domain == "" || trackedSeen[domain] {
			return
		}
		trackedSeen[domain] = true
		tracked = append(tracked, domain)
	}
	for _, raw := range selected {
		track(normalizeAdGuardFilterDomain(raw))
	}
	if ws.filters == nil {
		for _, raw := range previous {
			track(normalizeAdGuardFilterDomain(raw))
		}
		sort.Strings(tracked)
		return 0, tracked, nil
	}

	settings := ws.filters.Settings()
	lines := make([]string, 0, len(strings.Split(settings.CustomRules, "\n"))+len(selected))
	present := make(map[string]bool)
	for _, raw := range strings.Split(settings.CustomRules, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		lines = append(lines, line)
		if strings.HasPrefix(line, "||") && strings.HasSuffix(line, "^") {
			present[normalizeAdGuardFilterDomain(line[2:len(line)-1])] = true
		}
	}
	changed := false
	if removePrevious {
		for _, raw := range previous {
			delete(present, normalizeAdGuardFilterDomain(raw))
		}
		kept := lines[:0]
		for _, line := range lines {
			domain := ""
			if strings.HasPrefix(line, "||") && strings.HasSuffix(line, "^") {
				domain = normalizeAdGuardFilterDomain(line[2 : len(line)-1])
			}
			if domain != "" && stringSliceContains(previous, domain) {
				changed = true
				continue
			}
			kept = append(kept, line)
		}
		lines = kept
	}
	added := 0
	for _, raw := range selected {
		domain := normalizeAdGuardFilterDomain(raw)
		if domain == "" || present[domain] {
			continue
		}
		lines = append(lines, "||"+domain+"^")
		present[domain] = true
		added++
	}
	// Keep tracking only rules that are still present: withdrawn lines must not
	// linger on the source and be "removed" again on every later apply.
	for _, raw := range previous {
		domain := normalizeAdGuardFilterDomain(raw)
		if domain != "" && present[domain] {
			track(domain)
		}
	}
	sort.Strings(tracked)
	if !changed && added == 0 {
		return 0, tracked, nil
	}
	settings.CustomRules = strings.Join(lines, "\n")
	if err := ws.filters.SaveSettings(settings); err != nil {
		return 0, tracked, err
	}
	return added, tracked, nil
}

func (ws *WebServer) adGuardSyncApply(r *http.Request, body map[string]any) (map[string]any, error) {
	var req adguardApplyRequest
	if err := decodeAdGuardBody(body, &req); err != nil {
		return nil, err
	}
	req.ManagedUpstream = normalizeAdGuardUpstream(req.ManagedUpstream)
	actions, err := canonicalAdGuardDomainActions(req.DomainActions)
	if err != nil {
		return nil, err
	}
	if req.Checksum == "" {
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
	confirmMirror := req.ConfirmMirror != nil && *req.ConfirmMirror
	acknowledgeRisk := req.AcknowledgeRisk != nil && *req.AcknowledgeRisk
	if req.RemovalMode == adGuardRemovalMirror && !confirmMirror {
		return nil, errors.New("mirror mode requires explicit confirmation")
	}

	ws.adguardSyncMu.Lock()
	defer ws.adguardSyncMu.Unlock()

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
	// A source_id-only apply reads the last good text/upstream without a network
	// fetch, so the checksum is built from the same bytes the preview returned.
	// When text was supplied, preview kept an empty upstream too, so leave it
	// alone or the two checksums would disagree.
	if strings.TrimSpace(req.Text) == "" {
		req.Text = src.Text
		if strings.TrimSpace(req.ManagedUpstream) == "" {
			req.ManagedUpstream = normalizeAdGuardUpstream(src.ManagedUpstream)
		}
	}
	// A v5.10 client sends a legacy text-only checksum with apply-time review
	// flags; accepting that literal keeps old clients working while a v5.11
	// checksum still binds every review field.
	if req.Checksum != adGuardRequestChecksum(req.adguardPreviewRequest) && req.Checksum != adguard.Checksum(req.Text) {
		return nil, errors.New("preview checksum does not match the source text; preview again")
	}

	preview, err := ws.adGuardPreview(req.adguardPreviewRequest)
	if err != nil {
		return nil, err
	}
	counts, _ := preview["counts"].(map[string]int)
	conflicts := counts["conflict"]
	broadCount, _ := preview["broad_count"].(int)
	if (conflicts > 0 || broadCount > 0) && !acknowledgeRisk {
		return nil, errors.New("preview has conflicts or broad domains; acknowledge the risks before applying")
	}

	allowed := map[string]bool{}
	reviewed := map[string]bool{}
	rows, _ := preview["rows"].([]adguardSyncRow)
	for _, row := range rows {
		if row.Bucket == "adguardOnly" {
			allowed[row.Domain] = true
		}
		// Covered and conflict rows can be reviewed explicitly: their route chip
		// opens the same menu, and a chosen action rides in Import so Apply writes
		// it. Rows outside this preview are still never selected.
		if row.Bucket == "adguardOnly" || row.Bucket == "covered" || row.Bucket == "conflict" {
			reviewed[row.Domain] = true
		}
	}
	managedAllowed := map[string]bool{}
	managedRows, _ := preview["managed"].([]adguardSyncRow)
	for _, row := range managedRows {
		managedAllowed[row.Domain] = true
	}
	// Re-applying a domain this source already owns is idempotent, even though
	// the fresh preview now calls it covered. Manual rules stay out of `allowed`.
	ownedBefore := map[string]bool{}
	for _, domain := range src.Imported {
		ownedBefore[domain] = true
	}
	for _, entry := range state.provenance {
		if entry.SourceID == src.ID {
			ownedBefore[entry.Pattern] = true
		}
	}
	routes := ws.adGuardCurrentRoutes()
	// A wildcard scope from the Domains list writes extra patterns. Accept them
	// only when their origin is a reviewed row; an extra pattern already in
	// DigitDNS must be explicitly requested by the operator before it is rewritten.
	scopeOrigins := make(map[string]bool, len(reviewed)+len(ownedBefore))
	for domain := range reviewed {
		scopeOrigins[domain] = true
	}
	for domain := range ownedBefore {
		if routes.contains(domain) {
			scopeOrigins[domain] = true
		}
	}
	requestedSet := make(map[string]bool, len(req.Import))
	for _, raw := range req.Import {
		requestedSet[normalizeAdGuardPatternKey(raw)] = true
	}
	requested := make([]string, 0, len(req.Import))
	for _, raw := range req.Import {
		domain := normalizeAdGuardPatternKey(raw)
		validScope := false
		for origin := range scopeOrigins {
			for _, target := range adGuardScopeTargets(origin) {
				if target == domain {
					validScope = true
					break
				}
			}
			if validScope {
				break
			}
		}
		if !validScope {
			return nil, fmt.Errorf("selection %q is no longer an uncovered AdGuard domain; preview again", raw)
		}
		if !allowed[domain] && !(ownedBefore[domain] && routes.contains(domain)) &&
			!requestedSet[domain] && routes.contains(domain) {
			return nil, fmt.Errorf("wildcard target %q is already in DigitDNS; choose another scope", raw)
		}
		requested = append(requested, domain)
	}
	for _, domain := range req.Managed {
		if !managedAllowed[domain] {
			return nil, fmt.Errorf("managed proposal %q is no longer DigitDNS-only", domain)
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

	oldRoutes := routes.clone()
	selected := make(map[string]string, len(requested))
	for _, domain := range requested {
		action := actions[domain]
		if action == "" {
			action = adGuardActionProxy
		}
		selected[domain] = action
	}
	// A selected domain is written exactly once: clear it from every writeable
	// list first, then add it to the list its reviewed action asks for. The
	// disabled proxy list is display-only and is preserved untouched.
	for domain := range selected {
		routes.proxied = removeAdGuardDomain(routes.proxied, domain)
		routes.blocked = removeAdGuardDomain(routes.blocked, domain)
		routes.direct = removeAdGuardDomain(routes.direct, domain)
	}
	for domain, action := range selected {
		switch action {
		case adGuardActionDirect:
			routes.direct = append(routes.direct, domain)
		case adGuardActionBlock:
			routes.blocked = append(routes.blocked, domain)
		default:
			routes.proxied = append(routes.proxied, domain)
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
		// Mirror makes this source authoritative for the writeable route lists:
		// anything it does not name (or cover) goes, unless another source owns
		// it. It is the destructive mode the operator explicitly confirms.
		otherSources := make(map[string]bool)
		for _, entry := range state.provenance {
			if entry.SourceID != src.ID {
				otherSources[entry.Pattern] = true
			}
		}
		for _, domain := range append(append(append([]string{}, routes.proxied...), routes.blocked...), routes.direct...) {
			if remove[domain] || otherSources[domain] || parsedKeys[domain] || selected[domain] != "" {
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
		for _, list := range []*[]string{&routes.proxied, &routes.blocked, &routes.direct} {
			kept := (*list)[:0]
			for _, domain := range *list {
				if !remove[domain] {
					kept = append(kept, domain)
				}
			}
			*list = kept
		}
	}
	sort.Strings(routes.proxied)
	sort.Strings(routes.blocked)
	sort.Strings(routes.direct)

	if err := ws.saveAdGuardRouteLists(routes); err != nil {
		return nil, err
	}
	var restoreFiltering func()
	if ws.filters != nil {
		oldSettings := ws.filters.Settings()
		restoreFiltering = func() { _ = ws.filters.SaveSettings(oldSettings) }
	}
	adblockAdded, trackedAdblock, err := ws.adGuardApplyFiltering(
		canonicalAdGuardAdblockDomains(req.AdblockDomains), src.Adblock, req.RemovalMode != adGuardRemovalKeep)
	if err != nil {
		_ = ws.saveAdGuardRouteLists(oldRoutes)
		return nil, err
	}
	src.Adblock = trackedAdblock
	// Provenance means "this source put the rule there", not "the source also
	// happens to cover it". A domain that was already in DigitDNS and shown as
	// covered must stay a manual rule; otherwise a later synced_only removal
	// could delete an operator's own entry.
	inRoutes := map[string]bool{}
	for _, domain := range routes.proxied {
		inRoutes[domain] = true
	}
	for _, domain := range routes.blocked {
		inRoutes[domain] = true
	}
	for _, domain := range routes.direct {
		inRoutes[domain] = true
	}
	owned := make(map[string]adguardProvenance)
	for _, entry := range state.provenance {
		if entry.SourceID == src.ID && parsedKeys[entry.Pattern] && inRoutes[entry.Pattern] {
			owned[entry.Pattern] = entry
		}
	}
	sourceTargets := make(map[string]bool, len(allowed))
	for domain := range allowed {
		for _, target := range adGuardScopeTargets(domain) {
			sourceTargets[target] = true
		}
	}
	for domain := range selected {
		if !inRoutes[domain] {
			continue
		}
		// An explicit review of a covered manual row changes its route but must
		// not turn it into source-owned state; otherwise a later synced-only
		// removal could delete an operator's own rule. Wildcard scopes staged on
		// a new import remain source-owned.
		if !sourceTargets[domain] && !ownedBefore[domain] {
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
		// Rules are already live; put the old lists and filter lines back rather
		// than leaving provenance describing a set of domains that never landed.
		_ = ws.saveAdGuardRouteLists(oldRoutes)
		if restoreFiltering != nil {
			restoreFiltering()
		}
		return nil, err
	}

	return map[string]any{
		"success":         true,
		"source":          src,
		"counts":          countsOfRows(rows),
		"imported":        len(req.Import),
		"route_changes":   len(selected),
		"adblock_added":   adblockAdded,
		"removed":         len(remove),
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
