package web

import (
	"context"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"digitdns/internal/adguard"
	"digitdns/internal/service"
)

const adGuardSyncTestConfig = `# Streaming
[/bbc.co.uk/*.bbci.co.uk/chatgpt.com/blocked.example/]https://norwich-vps.duckdns.org:8443/dns-query
# Regional
[/kommersant.ru/]77.88.8.8
`

func saveAdGuardTestRules(t *testing.T, ws *WebServer, body string) {
	t.Helper()
	req := httptest.NewRequest("POST", "/api/config/rules", strings.NewReader(body))
	rec := httptest.NewRecorder()
	ws.handleConfigRules(rec, req)
	if rec.Code >= 400 {
		t.Fatalf("rules save = %d — %s", rec.Code, rec.Body.String())
	}
}

func adGuardSyncRowBuckets(rows any) map[string]string {
	out := map[string]string{}
	for _, row := range rows.([]adguardSyncRow) {
		out[row.Domain] = row.Bucket
	}
	return out
}

func adGuardSyncProxied(t *testing.T, ws *WebServer) []string {
	t.Helper()
	return toStringSlice(ws.loadRulesConfig()["custom_proxied"])
}

func adGuardSyncList(t *testing.T, ws *WebServer, key string) []string {
	t.Helper()
	return toStringSlice(ws.loadRulesConfig()[key])
}

// adGuardSyncTestServer attaches the real filtering service so adblock writes
// exercise SaveSettings and custom_rules rather than the nil-filter no-op.
func adGuardSyncTestServer(t *testing.T) (*WebServer, func()) {
	t.Helper()
	ws, db, cleanup := setupTestWebServer(t)
	filters := service.NewFilterService(db, ws.matcher)
	filters.Start()
	ws.SetFilterService(filters)
	return ws, func() {
		filters.Close()
		cleanup()
	}
}

func adGuardNoticeKinds(notices []adguardNotice) []string {
	kinds := make([]string, 0, len(notices))
	for _, notice := range notices {
		kinds = append(kinds, notice.Kind)
	}
	return kinds
}

func TestAdGuardSyncPreviewBuckets(t *testing.T) {
	ws, _, cleanup := setupTestWebServer(t)
	defer cleanup()
	if err := ws.saveProxiedRules([]string{"already.example", "chatgpt.com", "intranet.example"}); err != nil {
		t.Fatal(err)
	}
	// A blocked row becomes a conflict, never a silent override.
	saveAdGuardTestRules(t, ws, `{"custom_blocked":["blocked.example"]}`)

	preview, err := ws.adGuardSyncPreview(map[string]any{
		"text":    adGuardSyncTestConfig,
		"enabled": true,
	})
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	buckets := adGuardSyncRowBuckets(preview["rows"])
	for domain, want := range map[string]string{
		"bbc.co.uk":     "adguardOnly",
		"*.bbci.co.uk":  "adguardOnly",
		"chatgpt.com":   "covered",
		"kommersant.ru": "adguardOnly",
	} {
		if got := buckets[domain]; got != want {
			t.Errorf("bucket[%s] = %q, want %q", domain, got, want)
		}
	}
	counts := preview["counts"].(map[string]int)
	if counts["adguardOnly"] != 3 || counts["covered"] != 1 {
		t.Errorf("counts = %#v", counts)
	}
	warnings := strings.Join(preview["warnings"].([]string), "\n")
	if !strings.Contains(warnings, "77.88.8.8") {
		t.Errorf("non-DigitDNS target was not reported: %s", warnings)
	}
	if block := preview["generated_block"].(string); block != "" {
		t.Errorf("unselected managed proposals produced a block:\n%s", block)
	}
	selected, err := ws.adGuardSyncPreview(map[string]any{
		"text":              adGuardSyncTestConfig,
		"managed_selection": []string{"already.example"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if block := selected["generated_block"].(string); !strings.Contains(block, "[/already.example]") {
		t.Errorf("selected managed block = %q", block)
	}
}

func TestAdGuardSyncApplyProvenanceRemovalAndExport(t *testing.T) {
	ws, _, cleanup := setupTestWebServer(t)
	defer cleanup()
	if err := ws.saveProxiedRules([]string{"already.example"}); err != nil {
		t.Fatal(err)
	}

	preview, err := ws.adGuardSyncPreview(map[string]any{"text": adGuardSyncTestConfig})
	if err != nil {
		t.Fatal(err)
	}
	body := map[string]any{
		"text":             adGuardSyncTestConfig,
		"checksum":         preview["checksum"],
		"name":             "Norwich",
		"import":           []string{"bbc.co.uk", "*.bbci.co.uk", "kommersant.ru"},
		"managed":          []string{"already.example"},
		"removal_mode":     "keep",
		"acknowledge_risk": true,
	}
	applied, err := ws.adGuardSyncApply(httptest.NewRequest("POST", "/api/v3/sync/adguard/apply", nil), body)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	source := applied["source"].(adguardSource)
	if source.ID == "" || source.Name != "Norwich" {
		t.Fatalf("source = %#v", source)
	}
	for _, domain := range []string{"bbc.co.uk", "*.bbci.co.uk", "kommersant.ru", "already.example"} {
		if !slices.Contains(adGuardSyncProxied(t, ws), domain) {
			t.Errorf("after apply, missing %s in %v", domain, adGuardSyncProxied(t, ws))
		}
	}
	if !slices.Contains(source.Imported, "bbc.co.uk") {
		t.Errorf("provenance = %v", source.Imported)
	}

	export, err := ws.adGuardSyncExport(source.ID)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	block := export["block"].(string)
	for _, want := range []string{
		"# BEGIN DIGITDNS SYNC — managed",
		"[/already.example]https://norwich-vps.duckdns.org:8443/dns-query",
		"# END DIGITDNS SYNC — managed",
	} {
		if !strings.Contains(block, want) {
			t.Errorf("export missing %q:\n%s", want, block)
		}
	}

	// A second preview sees the imported domains as covered.
	preview2, err := ws.adGuardSyncPreview(map[string]any{"source_id": source.ID})
	if err != nil {
		t.Fatal(err)
	}
	if got := adGuardSyncRowBuckets(preview2["rows"])["bbc.co.uk"]; got != "covered" {
		t.Errorf("second preview bucket = %q, want covered", got)
	}

	// Dropping a domain from the source and applying synced_only removes only
	// the row this source previously owned.
	shorter := "# Streaming\n[/bbc.co.uk/]https://norwich-vps.duckdns.org:8443/dns-query\n"
	preview3, err := ws.adGuardSyncPreview(map[string]any{"source_id": source.ID, "text": shorter})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ws.adGuardSyncApply(httptest.NewRequest("POST", "/api/v3/sync/adguard/apply", nil), map[string]any{
		"source_id":        source.ID,
		"text":             shorter,
		"checksum":         preview3["checksum"],
		"removal_mode":     "synced_only",
		"acknowledge_risk": true,
	}); err != nil {
		t.Fatalf("second apply: %v", err)
	}
	got := adGuardSyncProxied(t, ws)
	for _, removed := range []string{"*.bbci.co.uk", "kommersant.ru"} {
		if slices.Contains(got, removed) {
			t.Errorf("synced_only left %s in %v", removed, got)
		}
	}
	for _, kept := range []string{"bbc.co.uk", "already.example"} {
		if !slices.Contains(got, kept) {
			t.Errorf("synced_only removed unrelated %s from %v", kept, got)
		}
	}
}

func TestAdGuardSyncRejectsStaleChecksum(t *testing.T) {
	ws, _, cleanup := setupTestWebServer(t)
	defer cleanup()
	_, err := ws.adGuardSyncApply(httptest.NewRequest("POST", "/api/v3/sync/adguard/apply", nil), map[string]any{
		"text":     adGuardSyncTestConfig,
		"checksum": "not-the-checksum",
	})
	if err == nil || !strings.Contains(err.Error(), "preview checksum") {
		t.Fatalf("stale apply error = %v", err)
	}
}

func TestAdGuardSyncMirrorRequiresConfirmationAndRemovesUnnamedRules(t *testing.T) {
	ws, _, cleanup := setupTestWebServer(t)
	defer cleanup()
	if err := ws.saveProxiedRules([]string{"manual.example"}); err != nil {
		t.Fatal(err)
	}
	const config = "[/source.example/]http://127.0.0.1:8443/dns-query\n"
	preview, err := ws.adGuardSyncPreview(map[string]any{"text": config})
	if err != nil {
		t.Fatal(err)
	}
	base := map[string]any{
		"text":         config,
		"checksum":     preview["checksum"],
		"import":       []string{"source.example"},
		"removal_mode": "mirror",
	}
	if _, err := ws.adGuardSyncApply(httptest.NewRequest("POST", "/api/v3/sync/adguard/apply", nil), base); err == nil || !strings.Contains(err.Error(), "confirmation") {
		t.Fatalf("unconfirmed mirror error = %v", err)
	}
	base["confirm_mirror"] = true
	if _, err := ws.adGuardSyncApply(httptest.NewRequest("POST", "/api/v3/sync/adguard/apply", nil), base); err != nil {
		t.Fatalf("confirmed mirror: %v", err)
	}
	got := adGuardSyncProxied(t, ws)
	if !slices.Equal(got, []string{"source.example"}) {
		t.Fatalf("mirror left custom_proxied = %v, want [source.example]", got)
	}
}

func TestAdGuardSyncConflictMustBeAcknowledged(t *testing.T) {
	ws, _, cleanup := setupTestWebServer(t)
	defer cleanup()
	saveAdGuardTestRules(t, ws, `{"custom_blocked":["blocked.example"]}`)
	const config = "[/blocked.example/]http://127.0.0.1:8443/dns-query\n"
	preview, err := ws.adGuardSyncPreview(map[string]any{"text": config})
	if err != nil {
		t.Fatal(err)
	}
	if counts := preview["counts"].(map[string]int); counts["conflict"] != 1 {
		t.Fatalf("counts = %#v", counts)
	}
	body := map[string]any{"text": config, "checksum": preview["checksum"]}
	if _, err := ws.adGuardSyncApply(httptest.NewRequest("POST", "/api/v3/sync/adguard/apply", nil), body); err == nil || !strings.Contains(err.Error(), "acknowledge") {
		t.Fatalf("unacknowledged conflict error = %v", err)
	}
	body["acknowledge_risk"] = true
	if _, err := ws.adGuardSyncApply(httptest.NewRequest("POST", "/api/v3/sync/adguard/apply", nil), body); err != nil {
		t.Fatalf("acknowledged conflict: %v", err)
	}
}

func TestAdGuardSyncPreviewFetchFailureKeepsLastGoodText(t *testing.T) {
	ws, _, cleanup := setupTestWebServer(t)
	defer cleanup()
	const lastGood = "[/source.example/]http://127.0.0.1:8443/dns-query\n"
	src, err := ws.adGuardSyncSaveSource(map[string]any{
		"name": "Broken URL",
		"url":  "http://127.0.0.1:1/adguard.txt",
		"text": lastGood,
	})
	if err != nil {
		t.Fatal(err)
	}
	preview, err := ws.adGuardSyncPreview(map[string]any{
		"source_id": src.ID,
		"url":       src.URL,
	})
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if preview["text"] != lastGood {
		t.Errorf("preview used %q, want last good text", preview["text"])
	}
	warnings := strings.Join(preview["warnings"].([]string), "\n")
	if !strings.Contains(warnings, "Fetch failed") {
		t.Errorf("warnings = %q", warnings)
	}
}

func TestAdGuardSyncSourceURLLosesCredentials(t *testing.T) {
	ws, _, cleanup := setupTestWebServer(t)
	defer cleanup()
	src, err := ws.adGuardSyncSaveSource(map[string]any{
		"name": "Private",
		"url":  "https://admin:secret@example.test/adguard.txt",
		"text": "[/example.com/]https://norwich-vps.duckdns.org:8443/dns-query",
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(src.URL, "secret") || strings.Contains(src.URL, "admin@") {
		t.Errorf("stored URL still carries credentials: %q", src.URL)
	}
}

func TestAdGuardSyncRefreshKeepsLastGoodSourceOnFetchFailure(t *testing.T) {
	ws, _, cleanup := setupTestWebServer(t)
	defer cleanup()
	src, err := ws.adGuardSyncSaveSource(map[string]any{
		"name": "Broken URL",
		"url":  "http://127.0.0.1:1/adguard.txt",
		"text": adGuardSyncTestConfig,
	})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := ws.adGuardSyncRefreshSource(
		httptest.NewRequest("POST", "/api/v3/sync/adguard/sources/"+src.ID+"/refresh", nil).WithContext(context.Background()),
		src.ID,
	)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	warn := resp["warning"].(string)
	if !strings.Contains(warn, "Fetch failed") {
		t.Errorf("warning = %q", warn)
	}
	after, _, ok := findAdGuardSource(ws.sourcesForTest(t), src.ID)
	if !ok || after.Text != adGuardSyncTestConfig {
		t.Errorf("last good source was replaced: %#v", after)
	}
}

func (ws *WebServer) sourcesForTest(t *testing.T) []adguardSource {
	t.Helper()
	sources, err := ws.adGuardSyncSources()
	if err != nil {
		t.Fatal(err)
	}
	return sources
}

func TestAdGuardSyncPreviewGroupsWarningsIntoNotices(t *testing.T) {
	ws, _, cleanup := setupTestWebServer(t)
	defer cleanup()
	const config = `[/www.google.com/kommersant.ru/]77.88.8.8
[/bbc.co.uk/]37.230.192.51
`
	preview, err := ws.adGuardSyncPreview(map[string]any{"text": config})
	if err != nil {
		t.Fatalf("preview: %v", err)
	}

	notices, ok := preview["notices"].([]adguardNotice)
	if !ok || len(notices) != 3 {
		t.Fatalf("notices = %#v, want upstream + broad + unchanged", preview["notices"])
	}
	upstream := notices[0]
	if upstream.Kind != "upstream" ||
		!strings.Contains(upstream.Message, "77.88.8.8") ||
		!strings.Contains(upstream.Message, "37.230.192.51") ||
		!strings.Contains(upstream.Message, "not this DigitDNS server") {
		t.Errorf("upstream notice = %#v", upstream)
	}
	broad := notices[1]
	if broad.Kind != "broad" || !strings.Contains(broad.Message, "www.google.com") {
		t.Errorf("broad notice = %#v", broad)
	}
	if got := preview["broad_count"].(int); got != 1 {
		t.Errorf("broad_count = %d, want 1", got)
	}
	foreign := preview["foreign_targets"].([]string)
	if !slices.Equal(foreign, []string{"37.230.192.51", "77.88.8.8"}) {
		t.Errorf("foreign_targets = %v", foreign)
	}
	rows := preview["rows"].([]adguardSyncRow)
	for _, row := range rows {
		if row.Domain == "www.google.com" && !row.Broad {
			t.Error("broad row was not flagged")
		}
	}
}

func TestAdGuardSyncApplyWritesPerDomainRouteActions(t *testing.T) {
	ws, _, cleanup := setupTestWebServer(t)
	defer cleanup()
	const config = "[/proxy.example/direct.example/block.example/]http://127.0.0.1:8443/dns-query\n"
	actions := map[string]string{
		"proxy.example":  "proxy",
		"direct.example": "direct",
		"block.example":  "block",
	}
	preview, err := ws.adGuardSyncPreview(map[string]any{"text": config, "domain_actions": actions})
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	applied, err := ws.adGuardSyncApply(httptest.NewRequest("POST", "/api/v3/sync/adguard/apply", nil), map[string]any{
		"text":           config,
		"checksum":       preview["checksum"],
		"domain_actions": actions,
		"import":         []string{"proxy.example", "direct.example", "block.example"},
	})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if got := applied["route_changes"]; got != 3 {
		t.Errorf("route_changes = %v, want 3", got)
	}
	for key, want := range map[string]string{
		"custom_proxied": "proxy.example",
		"custom_direct":  "direct.example",
		"custom_blocked": "block.example",
	} {
		if got := adGuardSyncList(t, ws, key); !slices.Equal(got, []string{want}) {
			t.Errorf("%s = %v, want [%s]", key, got, want)
		}
	}
	source := applied["source"].(adguardSource)
	for _, domain := range []string{"proxy.example", "direct.example", "block.example"} {
		if !slices.Contains(source.Imported, domain) {
			t.Errorf("provenance = %v, missing %s", source.Imported, domain)
		}
	}
}

func TestAdGuardScopeTargetsKeepsListedWildcard(t *testing.T) {
	got := adGuardScopeTargets("*.New.Test.")
	want := []string{"*.new.test"}
	if !slices.Equal(got, want) {
		t.Fatalf("adGuardScopeTargets(*.New.Test.) = %v, want %v", got, want)
	}
}

func TestAdGuardSyncApplyAcceptsWildcardScopeTargets(t *testing.T) {
	ws, _, cleanup := setupTestWebServer(t)
	defer cleanup()
	const config = "[/new.example/]http://127.0.0.1:8443/dns-query\n"
	actions := map[string]string{
		"new.example":   "block",
		"*.new.example": "block",
	}
	preview, err := ws.adGuardSyncPreview(map[string]any{"text": config, "domain_actions": actions})
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if _, err := ws.adGuardSyncApply(httptest.NewRequest("POST", "/api/v3/sync/adguard/apply", nil), map[string]any{
		"text":           config,
		"checksum":       preview["checksum"],
		"name":           "Wildcard scope",
		"domain_actions": actions,
		"import":         []string{"new.example", "*.new.example"},
	}); err != nil {
		t.Fatalf("apply: %v", err)
	}
	blocked := adGuardSyncList(t, ws, "custom_blocked")
	if len(blocked) != 2 || !slices.Contains(blocked, "new.example") || !slices.Contains(blocked, "*.new.example") {
		t.Errorf("custom_blocked = %v, want the domain and its wildcard scope", blocked)
	}
	source := ws.sourcesForTest(t)[0]
	for _, domain := range []string{"new.example", "*.new.example"} {
		if !slices.Contains(source.Imported, domain) {
			t.Errorf("provenance = %v, missing %s", source.Imported, domain)
		}
	}
}

func TestAdGuardSyncApplyEditsReviewedCoveredRoute(t *testing.T) {
	ws, _, cleanup := setupTestWebServer(t)
	defer cleanup()
	if err := ws.saveProxiedRules([]string{"covered.example"}); err != nil {
		t.Fatal(err)
	}
	const config = "[/covered.example/]http://127.0.0.1:8443/dns-query\n"
	actions := map[string]string{"covered.example": "direct"}
	preview, err := ws.adGuardSyncPreview(map[string]any{"text": config, "domain_actions": actions})
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if buckets := adGuardSyncRowBuckets(preview["rows"]); buckets["covered.example"] != "covered" {
		t.Fatalf("covered.example bucket = %q, want covered", buckets["covered.example"])
	}
	result, err := ws.adGuardSyncApply(httptest.NewRequest("POST", "/api/v3/sync/adguard/apply", nil), map[string]any{
		"text":           config,
		"checksum":       preview["checksum"],
		"name":           "Route edit",
		"domain_actions": actions,
		"import":         []string{"covered.example"},
	})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if got := adGuardSyncList(t, ws, "custom_direct"); !slices.Equal(got, []string{"covered.example"}) {
		t.Errorf("custom_direct = %v, want [covered.example]", got)
	}
	if got := adGuardSyncList(t, ws, "custom_proxied"); len(got) != 0 {
		t.Errorf("custom_proxied = %v, want empty", got)
	}
	applied, _ := result["source"].(adguardSource)
	if slices.Contains(applied.Imported, "covered.example") {
		t.Errorf("provenance = %v, manual covered rule must stay unowned", applied.Imported)
	}
}

func TestAdGuardSyncApplyAdblockCustomRulesNoDuplicates(t *testing.T) {
	ws, cleanup := adGuardSyncTestServer(t)
	defer cleanup()
	const config = "[/proxy.example/]http://127.0.0.1:8443/dns-query\n"
	adblock := []string{"*.ads.example", "ads.example", "tracker.example"}
	preview, err := ws.adGuardSyncPreview(map[string]any{"text": config, "adblock_domains": adblock})
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	body := map[string]any{
		"text":            config,
		"checksum":        preview["checksum"],
		"adblock_domains": adblock,
	}
	countLines := func(want string) int {
		count := 0
		for _, line := range strings.Split(ws.filters.Settings().CustomRules, "\n") {
			if strings.TrimSpace(line) == want {
				count++
			}
		}
		return count
	}
	applied, err := ws.adGuardSyncApply(httptest.NewRequest("POST", "/api/v3/sync/adguard/apply", nil), body)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if got := applied["adblock_added"]; got != 2 {
		t.Errorf("adblock_added = %v, want 2 (wildcard and plain dedupe to one line)", got)
	}
	for _, line := range []string{"||ads.example^", "||tracker.example^"} {
		if got := countLines(line); got != 1 {
			t.Errorf("%q appears %d times in custom_rules: %q", line, got, ws.filters.Settings().CustomRules)
		}
	}
	source := applied["source"].(adguardSource)
	if !slices.Equal(source.Adblock, []string{"ads.example", "tracker.example"}) {
		t.Errorf("source adblock = %v", source.Adblock)
	}

	repeat, err := ws.adGuardSyncApply(httptest.NewRequest("POST", "/api/v3/sync/adguard/apply", nil), body)
	if err != nil {
		t.Fatalf("repeat apply: %v", err)
	}
	if got := repeat["adblock_added"]; got != 0 {
		t.Errorf("repeat adblock_added = %v, want 0", got)
	}
	for _, line := range []string{"||ads.example^", "||tracker.example^"} {
		if got := countLines(line); got != 1 {
			t.Errorf("repeat left %q %d times", line, got)
		}
	}
}

func TestAdGuardSyncPreviewNoticesAndRowMetadata(t *testing.T) {
	ws, _, cleanup := setupTestWebServer(t)
	defer cleanup()
	if err := ws.saveProxiedRules([]string{"covered.example"}); err != nil {
		t.Fatal(err)
	}
	saveAdGuardTestRules(t, ws, `{"custom_blocked":["conflict.example"]}`)
	const config = "[/covered.example/new.example/conflict.example/]http://127.0.0.1:8443/dns-query\n"
	preview, err := ws.adGuardSyncPreview(map[string]any{"text": config})
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	kinds := adGuardNoticeKinds(preview["notices"].([]adguardNotice))
	for _, want := range []string{"hidden", "unchanged"} {
		if !slices.Contains(kinds, want) {
			t.Errorf("notice kinds = %v, missing %s", kinds, want)
		}
	}
	for _, notice := range preview["notices"].([]adguardNotice) {
		if notice.Kind == "unchanged" && !strings.Contains(notice.Title, "will not change") {
			t.Errorf("unchanged notice = %#v", notice)
		}
	}
	rows := map[string]adguardSyncRow{}
	for _, row := range preview["rows"].([]adguardSyncRow) {
		rows[row.Domain] = row
	}
	covered := rows["covered.example"]
	if !covered.InDigitDNS || covered.SuggestedAction != "proxy" {
		t.Errorf("covered row = %#v", covered)
	}
	if !slices.Contains(covered.Variants, "new.example") || slices.Contains(covered.Variants, "covered.example") {
		t.Errorf("covered variants = %v", covered.Variants)
	}
	conflict := rows["conflict.example"]
	if conflict.InDigitDNS {
		t.Errorf("conflict row was marked hidden: %#v", conflict)
	}
	if conflict.SuggestedAction != "block" {
		t.Errorf("conflict suggested action = %q, want block", conflict.SuggestedAction)
	}
	fresh := rows["new.example"]
	if fresh.InDigitDNS || fresh.SuggestedAction != "proxy" {
		t.Errorf("fresh row = %#v", fresh)
	}
	if !slices.Contains(fresh.Variants, "covered.example") {
		t.Errorf("fresh variants = %v", fresh.Variants)
	}
	if len(fresh.Variants) > 8 {
		t.Errorf("too many variants: %v", fresh.Variants)
	}
}

func TestAdGuardSyncReviewChecksumBindsAllFields(t *testing.T) {
	ws, _, cleanup := setupTestWebServer(t)
	defer cleanup()
	const config = "[/change.example/]http://127.0.0.1:8443/dns-query\n"
	base := map[string]any{
		"text":             config,
		"domain_actions":   map[string]string{"change.example": "proxy"},
		"adblock_domains":  []string{"ads.example"},
		"removal_mode":     "synced_only",
		"managed_upstream": "this-server",
	}
	preview, err := ws.adGuardSyncPreview(base)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	checksum := preview["checksum"].(string)
	if checksum == adguard.Checksum(config) {
		t.Fatal("review fields did not change the checksum")
	}
	bodyFor := func(overrides map[string]any) map[string]any {
		body := make(map[string]any, len(base)+3)
		for key, value := range base {
			body[key] = value
		}
		for key, value := range overrides {
			body[key] = value
		}
		body["checksum"] = checksum
		body["import"] = []string{"change.example"}
		return body
	}
	for _, tc := range []struct {
		name  string
		field string
		value any
	}{
		{"action", "domain_actions", map[string]string{"change.example": "direct"}},
		{"adblock", "adblock_domains", []string{"other.example"}},
		{"removal", "removal_mode", "keep"},
		{"upstream", "managed_upstream", "8.8.8.8"},
	} {
		_, err := ws.adGuardSyncApply(httptest.NewRequest("POST", "/api/v3/sync/adguard/apply", nil),
			bodyFor(map[string]any{tc.field: tc.value}))
		if err == nil || !strings.Contains(err.Error(), "preview checksum") {
			t.Errorf("%s change: apply error = %v", tc.name, err)
		}
	}
	if _, err := ws.adGuardSyncApply(httptest.NewRequest("POST", "/api/v3/sync/adguard/apply", nil), bodyFor(nil)); err != nil {
		t.Fatalf("unchanged review: %v", err)
	}

	legacy, err := ws.adGuardSyncPreview(map[string]any{"text": config})
	if err != nil {
		t.Fatal(err)
	}
	if got := legacy["checksum"]; got != adguard.Checksum(config) {
		t.Errorf("legacy checksum = %v, want %s", got, adguard.Checksum(config))
	}
	if _, err := ws.adGuardSyncApply(httptest.NewRequest("POST", "/api/v3/sync/adguard/apply", nil), map[string]any{
		"text":             config,
		"checksum":         legacy["checksum"],
		"removal_mode":     "keep",
		"acknowledge_risk": true,
	}); err != nil {
		t.Fatalf("legacy apply: %v", err)
	}
}

func TestAdGuardSyncThisServerAliasNormalization(t *testing.T) {
	ws, _, cleanup := setupTestWebServer(t)
	defer cleanup()
	const config = "[/alias.example/]https://external.example.test/dns-query\n"
	review := map[string]string{"alias.example": "proxy"}
	aliased, err := ws.adGuardSyncPreview(map[string]any{
		"text": config, "domain_actions": review, "managed_upstream": "  this-server  ",
	})
	if err != nil {
		t.Fatal(err)
	}
	empty, err := ws.adGuardSyncPreview(map[string]any{
		"text": config, "domain_actions": review, "managed_upstream": "",
	})
	if err != nil {
		t.Fatal(err)
	}
	if aliased["checksum"] != empty["checksum"] || aliased["upstream"] != empty["upstream"] {
		t.Errorf("alias preview = %v, empty preview = %v", aliased["checksum"], empty["checksum"])
	}
	src, err := ws.adGuardSyncSaveSource(map[string]any{
		"name": "Alias", "text": config, "managed_upstream": " THIS-SERVER ",
	})
	if err != nil {
		t.Fatal(err)
	}
	if src.ManagedUpstream != "" {
		t.Errorf("saved upstream = %q, want empty for this-server", src.ManagedUpstream)
	}
}

func TestAdGuardSyncSyncedOnlyRemovesDirectAndBlockedRoutes(t *testing.T) {
	ws, _, cleanup := setupTestWebServer(t)
	defer cleanup()
	const full = "[/proxy.example/direct.example/block.example/]http://127.0.0.1:8443/dns-query\n"
	actions := map[string]string{
		"proxy.example":  "proxy",
		"direct.example": "direct",
		"block.example":  "block",
	}
	preview, err := ws.adGuardSyncPreview(map[string]any{"text": full, "domain_actions": actions})
	if err != nil {
		t.Fatal(err)
	}
	applied, err := ws.adGuardSyncApply(httptest.NewRequest("POST", "/api/v3/sync/adguard/apply", nil), map[string]any{
		"text": full, "checksum": preview["checksum"], "domain_actions": actions,
		"import": []string{"proxy.example", "direct.example", "block.example"},
	})
	if err != nil {
		t.Fatal(err)
	}
	source := applied["source"].(adguardSource)

	const shorter = "[/proxy.example/]http://127.0.0.1:8443/dns-query\n"
	removal := map[string]any{
		"domain_actions": map[string]string{"proxy.example": "proxy"},
		"removal_mode":   "synced_only",
	}
	preview2, err := ws.adGuardSyncPreview(map[string]any{
		"source_id": source.ID, "text": shorter,
		"domain_actions": removal["domain_actions"], "removal_mode": removal["removal_mode"],
	})
	if err != nil {
		t.Fatal(err)
	}
	body := map[string]any{
		"source_id": source.ID, "text": shorter, "checksum": preview2["checksum"],
		"domain_actions": removal["domain_actions"], "removal_mode": removal["removal_mode"],
	}
	second, err := ws.adGuardSyncApply(httptest.NewRequest("POST", "/api/v3/sync/adguard/apply", nil), body)
	if err != nil {
		t.Fatalf("synced_only apply: %v", err)
	}
	if got := second["removed"]; got != 2 {
		t.Errorf("removed = %v, want 2", got)
	}
	if got := adGuardSyncList(t, ws, "custom_proxied"); !slices.Equal(got, []string{"proxy.example"}) {
		t.Errorf("custom_proxied = %v", got)
	}
	for _, key := range []string{"custom_blocked", "custom_direct"} {
		if got := adGuardSyncList(t, ws, key); len(got) != 0 {
			t.Errorf("%s = %v, want empty", key, got)
		}
	}
}

func TestAdGuardSyncSaveSourceDeduplicatesIdenticalText(t *testing.T) {
	ws, _, cleanup := setupTestWebServer(t)
	defer cleanup()
	const text = "[/dedupe.example/]http://127.0.0.1:8443/dns-query\n"
	first, err := ws.adGuardSyncSaveSource(map[string]any{"name": "First", "text": text})
	if err != nil {
		t.Fatal(err)
	}
	state, err := ws.loadAdGuardSyncState()
	if err != nil {
		t.Fatal(err)
	}
	state.provenance = append(state.provenance, adguardProvenance{
		SourceID: first.ID, Pattern: "dedupe.example", Group: "seed",
	})
	if err := ws.saveAdGuardSyncState(state); err != nil {
		t.Fatal(err)
	}

	second, err := ws.adGuardSyncSaveSource(map[string]any{"name": "Renamed", "text": text})
	if err != nil {
		t.Fatal(err)
	}
	if second.ID != first.ID {
		t.Errorf("duplicate source id = %q, want %q", second.ID, first.ID)
	}
	if got := adguard.Checksum(second.Text); got != adguard.Checksum(text) {
		t.Errorf("reused text checksum = %s", got)
	}
	if second.Name != "Renamed" {
		t.Errorf("reused source name = %q, want Renamed", second.Name)
	}
	if sources := ws.sourcesForTest(t); len(sources) != 1 {
		t.Fatalf("sources = %d, want 1: %#v", len(sources), sources)
	}
	after, err := ws.loadAdGuardSyncState()
	if err != nil {
		t.Fatal(err)
	}
	if len(after.provenance) != 1 || after.provenance[0].SourceID != first.ID {
		t.Errorf("provenance = %#v, want entry for %q", after.provenance, first.ID)
	}

	const edited = "[/dedupe.example/other.example/]http://127.0.0.1:8443/dns-query\n"
	third, err := ws.adGuardSyncSaveSource(map[string]any{"source_id": first.ID, "text": edited})
	if err != nil {
		t.Fatal(err)
	}
	if third.ID != first.ID {
		t.Errorf("explicit edit changed id to %q", third.ID)
	}
	if sources := ws.sourcesForTest(t); len(sources) != 1 {
		t.Fatalf("sources after edit = %d, want 1", len(sources))
	}
}
