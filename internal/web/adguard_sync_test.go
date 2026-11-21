package web

import (
	"context"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
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
	if !ok || len(notices) != 2 {
		t.Fatalf("notices = %#v, want upstream + broad", preview["notices"])
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
