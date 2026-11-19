package web

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"digitdns/internal/core/matcher"
	"digitdns/internal/migrate"
	"digitdns/internal/service"
)

func migrateTestServer(t *testing.T) (*WebServer, func()) {
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

func postMigrateJSON(t *testing.T, ws *WebServer, path string, payload any) *httptest.ResponseRecorder {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	switch path {
	case "/api/migrate/analyze":
		ws.handleMigrateAnalyze(rec, req)
	case "/api/migrate/apply":
		ws.handleMigrateApply(rec, req)
	case "/api/migrate/undo":
		ws.handleMigrateUndo(rec, req)
	default:
		t.Fatalf("unknown path %s", path)
	}
	return rec
}

func TestMigrateAnalyzeAdGuardUpload(t *testing.T) {
	ws, cleanup := migrateTestServer(t)
	defer cleanup()

	data, err := os.ReadFile(filepath.Join("..", "migrate", "testdata", "adguard-home.yaml"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "adguard-home.yaml")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write(data)
	_ = writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/migrate/analyze", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	ws.handleMigrateAnalyze(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var preview migratePreview
	if err := json.Unmarshal(rec.Body.Bytes(), &preview); err != nil {
		t.Fatalf("decode preview: %v", err)
	}
	if preview.Token == "" {
		t.Fatal("preview has no token")
	}
	if preview.Source.Kind != migrate.SourceAdGuardHome {
		t.Errorf("source = %q, want adguard-home", preview.Source.Kind)
	}
	if preview.Stats.FilterLists != 2 || preview.Stats.Clients != 2 || preview.Stats.Records != 1 {
		t.Errorf("stats = %+v, want 2 lists, 2 clients, 1 record", preview.Stats)
	}
	if len(preview.Warnings) == 0 {
		t.Error("preview has no warnings for the skipped sections")
	}
	if preview.Stats.Proxied != 0 {
		t.Errorf("Proxied = %d, want 0 for this fixture", preview.Stats.Proxied)
	}
}

func TestMigrateAnalyzeAndApplyBundle(t *testing.T) {
	ws, cleanup := migrateTestServer(t)
	defer cleanup()

	bundle := migrate.Bundle{
		SchemaVersion: migrate.SchemaVersion,
		Source:        migrate.Source{Kind: migrate.SourceAdGuardHome},
		Proxied:       []string{"imported.example"},
		Blocked:       []string{"blocked-imported.example"},
		Direct:        []string{"direct-imported.example"},
		Allowlist:     []string{"allowed-imported.example"},
		Records: []migrate.Record{
			{Name: "pin-imported.example", Type: "A", Value: "192.0.2.77"},
			{Name: "v6-imported.example", Type: "AAAA", Value: "2001:db8::77"},
		},
		Rewrites: []migrate.Rewrite{
			{Domain: "cname-imported.example", Target: "target-imported.example"},
		},
		Clients: []migrate.Client{
			{Name: "imported-laptop", IDs: []string{"192.0.2.88"}, Enabled: true},
		},
		Filtering: &migrate.Filtering{
			BlockMode:   "nxdomain",
			BlockTTL:    intPtr(20),
			CustomRules: "||custom-imported.example^\n",
		},
	}
	rec := postMigrateJSON(t, ws, "/api/migrate/analyze", map[string]any{"bundle": bundle})
	if rec.Code != http.StatusOK {
		t.Fatalf("analyze status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var preview migratePreview
	if err := json.Unmarshal(rec.Body.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	if preview.Stats.Proxied != 1 || preview.Stats.Clients != 1 {
		t.Fatalf("preview stats = %+v", preview.Stats)
	}

	rec = postMigrateJSON(t, ws, "/api/migrate/apply", map[string]any{"token": preview.Token})
	if rec.Code != http.StatusOK {
		t.Fatalf("apply status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var applied struct {
		Success bool          `json:"success"`
		Added   migrate.Stats `json:"added"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &applied); err != nil {
		t.Fatal(err)
	}
	if !applied.Success {
		t.Fatal("apply did not report success")
	}
	if applied.Added.Proxied != 1 || applied.Added.Blocked != 1 || applied.Added.Direct != 1 || applied.Added.Records != 2 || applied.Added.Rewrites != 1 || applied.Added.Clients != 1 {
		t.Errorf("added = %+v, want one of each and both records plus one rewrite", applied.Added)
	}

	rules := ws.loadRulesConfig()
	if !containsString(rules["custom_proxied"], "imported.example") {
		t.Errorf("custom_proxied = %v", rules["custom_proxied"])
	}
	records, _ := rules["custom_records"].(map[string]string)
	if records["pin-imported.example"] != "192.0.2.77" {
		t.Errorf("custom_records = %v", records)
	}
	if records["v6-imported.example"] != "2001:db8::77" {
		t.Errorf("AAAA record missing from custom_records: %v", records)
	}
	if records["cname-imported.example"] != "target-imported.example" {
		t.Errorf("CNAME rewrite missing from custom_records: %v", records)
	}
	clients, err := ws.clients.ListClients()
	if err != nil {
		t.Fatal(err)
	}
	if len(clients) != 1 || clients[0].Name != "imported-laptop" {
		t.Errorf("clients = %+v, want imported-laptop", clients)
	}
	settings := ws.filters.Settings()
	if settings.BlockMode != "nxdomain" || settings.BlockTTL != 20 {
		t.Errorf("filtering = %+v, want nxdomain/20", settings)
	}
	if !containsString(settings.Allowlist, "allowed-imported.example") {
		t.Errorf("allowlist = %v", settings.Allowlist)
	}
	if !strings.Contains(settings.CustomRules, "||custom-imported.example^") {
		t.Errorf("custom rules = %q", settings.CustomRules)
	}
	if action, _ := ws.matcher.Match("imported.example"); action != matcher.ActionProxy {
		t.Errorf("matcher action = %v, want PROXY", action)
	}
}

func TestMigrateAnalyzeRejectsV6Zip(t *testing.T) {
	ws, cleanup := migrateTestServer(t)
	defer cleanup()

	var body bytes.Buffer
	zw := zip.NewWriter(&body)
	file, err := zw.Create("etc/pihole/gravity.db")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = file.Write([]byte("not a real database"))
	_ = zw.Close()

	var upload bytes.Buffer
	writer := multipart.NewWriter(&upload)
	part, err := writer.CreateFormFile("file", "teleporter.zip")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write(body.Bytes())
	_ = writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/migrate/analyze", &upload)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()
	ws.handleMigrateAnalyze(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "browser") {
		t.Errorf("body = %s, want the browser-parsing message", rec.Body.String())
	}
}

func TestMigrateRegexReport(t *testing.T) {
	ws, cleanup := migrateTestServer(t)
	defer cleanup()

	var rules strings.Builder
	for i := 0; i < 65; i++ {
		fmt.Fprintf(&rules, "/^ad[0-9]+\\.example%d$/\n", i)
	}
	bundle := migrate.Bundle{
		SchemaVersion: migrate.SchemaVersion,
		Source:        migrate.Source{Kind: migrate.SourceAdGuardHome},
		Filtering:     &migrate.Filtering{CustomRules: rules.String()},
	}
	rec := postMigrateJSON(t, ws, "/api/migrate/analyze", map[string]any{"bundle": bundle})
	if rec.Code != http.StatusOK {
		t.Fatalf("analyze status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var preview migratePreview
	if err := json.Unmarshal(rec.Body.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	if !preview.ReportAvailable {
		t.Fatal("preview says no report is available")
	}
	if preview.Stats.Skipped == 0 {
		t.Fatal("preview did not report the over-cap regex")
	}

	req := httptest.NewRequest(http.MethodGet, "/api/migrate/report?token="+preview.Token, nil)
	report := httptest.NewRecorder()
	ws.handleMigrateReport(report, req)
	if report.Code != http.StatusOK {
		t.Fatalf("report status = %d", report.Code)
	}
	if !strings.Contains(report.Body.String(), "regex-over-cap") {
		t.Errorf("report = %q, want the over-cap code", report.Body.String())
	}
}

func TestMigrateApplyTokenIsSingleUse(t *testing.T) {
	ws, cleanup := migrateTestServer(t)
	defer cleanup()

	bundle := migrate.Bundle{
		SchemaVersion: migrate.SchemaVersion,
		Source:        migrate.Source{Kind: migrate.SourceAdGuardHome},
		Proxied:       []string{"once.example"},
	}
	rec := postMigrateJSON(t, ws, "/api/migrate/analyze", map[string]any{"bundle": bundle})
	if rec.Code != http.StatusOK {
		t.Fatalf("analyze status = %d", rec.Code)
	}
	var preview migratePreview
	if err := json.Unmarshal(rec.Body.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	rec = postMigrateJSON(t, ws, "/api/migrate/apply", map[string]any{"token": preview.Token})
	if rec.Code != http.StatusOK {
		t.Fatalf("first apply status = %d, body = %s", rec.Code, rec.Body.String())
	}
	rec = postMigrateJSON(t, ws, "/api/migrate/apply", map[string]any{"token": preview.Token})
	if rec.Code != http.StatusGone {
		t.Fatalf("second apply status = %d, want 410", rec.Code)
	}
}

func TestMigrateUndoRestoresRulesAndClients(t *testing.T) {
	ws, cleanup := migrateTestServer(t)
	defer cleanup()

	bundle := migrate.Bundle{
		SchemaVersion: migrate.SchemaVersion,
		Source:        migrate.Source{Kind: migrate.SourceAdGuardHome},
		Proxied:       []string{"undo.example"},
		Clients: []migrate.Client{
			{Name: "undo-client", IDs: []string{"192.0.2.99"}, Enabled: true},
		},
	}
	rec := postMigrateJSON(t, ws, "/api/migrate/analyze", map[string]any{"bundle": bundle})
	if rec.Code != http.StatusOK {
		t.Fatalf("analyze status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var preview migratePreview
	if err := json.Unmarshal(rec.Body.Bytes(), &preview); err != nil {
		t.Fatal(err)
	}
	rec = postMigrateJSON(t, ws, "/api/migrate/apply", map[string]any{"token": preview.Token})
	if rec.Code != http.StatusOK {
		t.Fatalf("apply status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var applied struct {
		UndoToken string `json:"undo_token"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &applied); err != nil {
		t.Fatal(err)
	}
	if applied.UndoToken == "" {
		t.Fatal("apply returned no undo token")
	}
	if !containsString(ws.loadRulesConfig()["custom_proxied"], "undo.example") {
		t.Fatal("imported rule is missing before undo")
	}
	if clients, _ := ws.clients.ListClients(); len(clients) != 1 {
		t.Fatalf("clients before undo = %d, want 1", len(clients))
	}

	rec = postMigrateJSON(t, ws, "/api/migrate/undo", map[string]any{"token": applied.UndoToken})
	if rec.Code != http.StatusOK {
		t.Fatalf("undo status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if containsString(ws.loadRulesConfig()["custom_proxied"], "undo.example") {
		t.Error("imported rule survived undo")
	}
	if clients, _ := ws.clients.ListClients(); len(clients) != 0 {
		t.Errorf("clients after undo = %d, want 0", len(clients))
	}

	rec = postMigrateJSON(t, ws, "/api/migrate/undo", map[string]any{"token": applied.UndoToken})
	if rec.Code != http.StatusGone {
		t.Fatalf("second undo status = %d, want 410", rec.Code)
	}
}

func containsString(value any, want string) bool {
	list, _ := value.([]string)
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}

func intPtr(value int) *int { return &value }
