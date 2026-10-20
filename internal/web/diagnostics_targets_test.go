package web

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"digitdns/internal/database"
)

// The Gaming Diagnostic Suite used to dial eight hardcoded game endpoints. v3.1.0
// made the list wholly operator-owned, so /api/diagnostics/targets is the CRUD
// surface: GET the list, POST add/edit, DELETE remove. Every write answers with
// the full, server-normalised list so the panel renders what was stored rather
// than what it sent.
func TestDiagnosticTargetsCRUD(t *testing.T) {
	ws, h, tok, cleanup := authedServer(t)
	defer cleanup()

	// Empty to begin with.
	body := decodeBody(t, authedGet(t, h, "/api/diagnostics/targets", tok))
	if targets, _ := body["targets"].([]any); len(targets) != 0 {
		t.Fatalf("fresh target list = %v, want empty", body["targets"])
	}

	// Add one: the bare host must come back widened to host:port.
	w := postJSON(t, h, "/api/diagnostics/targets", `{"name":"Home server","host":"game.example.com"}`, tok)
	if w.Code != http.StatusOK {
		t.Fatalf("add target = %d, want 200 — %s", w.Code, w.Body.String())
	}
	body = decodeBody(t, w)
	targets, _ := body["targets"].([]any)
	if len(targets) != 1 {
		t.Fatalf("targets after add = %v, want one entry", body["targets"])
	}
	row, _ := targets[0].(map[string]any)
	if row["name"] != "Home server" || row["host"] != "game.example.com:443" {
		t.Fatalf("stored target = %v, want name Home server / host game.example.com:443", row)
	}

	// Persisted, not just in memory.
	var stored database.DiagnosticSettings
	if err := ws.db.GetSetting("diagnostics", &stored); err != nil {
		t.Fatalf("reading persisted diagnostics: %v", err)
	}
	if got := stored.GetTargets(); len(got) != 1 || got[0].Host != "game.example.com:443" {
		t.Fatalf("persisted targets = %v, want one normalised entry", got)
	}

	// A second, distinct target appends.
	w = postJSON(t, h, "/api/diagnostics/targets", `{"name":"Voice","host":"203.0.113.9:27015"}`, tok)
	if w.Code != http.StatusOK {
		t.Fatalf("second add = %d, want 200 — %s", w.Code, w.Body.String())
	}
	body = decodeBody(t, w)
	if targets, _ = body["targets"].([]any); len(targets) != 2 {
		t.Fatalf("targets after second add = %v, want two", body["targets"])
	}

	// Re-adding the same host is an edit, not a duplicate.
	w = postJSON(t, h, "/api/diagnostics/targets", `{"name":"Home server (renamed)","host":"game.example.com"}`, tok)
	if w.Code != http.StatusOK {
		t.Fatalf("edit target = %d, want 200 — %s", w.Code, w.Body.String())
	}
	body = decodeBody(t, w)
	if targets, _ = body["targets"].([]any); len(targets) != 2 {
		t.Fatalf("targets after an edit = %v, want still two", body["targets"])
	}
	row, _ = targets[0].(map[string]any)
	if row["name"] != "Home server (renamed)" {
		t.Fatalf("edit did not rename the entry: %v", row)
	}

	// Delete by the stored (normalised) host, which is what the panel sends.
	req := authedRequest(t, h, http.MethodDelete, "/api/diagnostics/targets?host=game.example.com%3A443", tok)
	if req.Code != http.StatusOK {
		t.Fatalf("delete target = %d, want 200 — %s", req.Code, req.Body.String())
	}
	body = decodeBody(t, req)
	if targets, _ = body["targets"].([]any); len(targets) != 1 {
		t.Fatalf("targets after delete = %v, want one", body["targets"])
	}

	// Deleting an unknown host is a 404, not a silent success.
	if req := authedRequest(t, h, http.MethodDelete, "/api/diagnostics/targets?host=unknown.example.com", tok); req.Code != http.StatusNotFound {
		t.Fatalf("delete unknown host = %d, want 404", req.Code)
	}
}

func TestDiagnosticTargetsRejectsBadInput(t *testing.T) {
	_, h, tok, cleanup := authedServer(t)
	defer cleanup()

	cases := []string{
		`{"name":"","host":"game.example.com"}`,
		`{"name":"   ","host":"game.example.com"}`,
		`{"name":"x","host":""}`,
		`{"name":"x","host":"https://game.example.com"}`,
		`{"name":"x","host":"game.example.com/path"}`,
		`{"name":"x","host":"game.example.com:70000"}`,
		`{"name":"x","host":"game example.com"}`,
		`{bad json`,
	}
	for _, b := range cases {
		if w := postJSON(t, h, "/api/diagnostics/targets", b, tok); w.Code != http.StatusBadRequest {
			t.Errorf("%s = %d, want 400 — %s", b, w.Code, w.Body.String())
		}
	}
}

func TestDiagnosticTargetsRejectsOtherMethods(t *testing.T) {
	_, h, tok, cleanup := authedServer(t)
	defer cleanup()

	for _, method := range []string{http.MethodPut, http.MethodPatch} {
		if w := authedRequest(t, h, method, "/api/diagnostics/targets", tok); w.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s = %d, want 405", method, w.Code)
		}
	}
}

// The run dials the configured list, not a built-in preset. With one unreachable
// target the report must be total=1, reachable=0 — never the old hardcoded eight.
func TestDiagnosticsRunUsesConfiguredTargets(t *testing.T) {
	ws, h, tok, cleanup := authedServer(t)
	defer cleanup()

	// A closed port on loopback: connection refused is deterministic and instant,
	// so the run does not depend on the internet or a timeout.
	if err := ws.db.SetSetting("diagnostics", &database.DiagnosticSettings{
		Targets: []database.DiagnosticTarget{{Name: "Closed", Host: "127.0.0.1:1"}},
	}); err != nil {
		t.Fatalf("seeding targets: %v", err)
	}
	ws.diagSettings = nil // force a reload of the seeded record

	w := postJSON(t, h, "/api/diagnostics/run", `{}`, tok)
	if w.Code != http.StatusOK {
		t.Fatalf("run = %d, want 200 — %s", w.Code, w.Body.String())
	}
	var report struct {
		Total     int `json:"total"`
		Reachable int `json:"reachable"`
		Results   []struct {
			Name   string `json:"name"`
			Target string `json:"target"`
		} `json:"results"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &report); err != nil {
		t.Fatalf("decoding run: %v", err)
	}
	if report.Total != 1 {
		t.Fatalf("total = %d, want 1 (the configured list, not the old eight presets)", report.Total)
	}
	if len(report.Results) != 1 || report.Results[0].Name != "Closed" {
		t.Fatalf("results = %+v, want the single configured target", report.Results)
	}
	if report.Reachable != 0 {
		t.Fatalf("reachable = %d, want 0 for a closed port", report.Reachable)
	}
}

// An empty list runs cleanly and reports nothing measured rather than inventing a
// target or dividing by zero.
func TestDiagnosticsRunWithNoTargets(t *testing.T) {
	_, h, tok, cleanup := authedServer(t)
	defer cleanup()

	w := postJSON(t, h, "/api/diagnostics/run", `{}`, tok)
	if w.Code != http.StatusOK {
		t.Fatalf("run with no targets = %d, want 200 — %s", w.Code, w.Body.String())
	}
	var report struct {
		Total   int `json:"total"`
		Results []any
		Score   int `json:"overall_score"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &report); err != nil {
		t.Fatalf("decoding run: %v", err)
	}
	if report.Total != 0 || len(report.Results) != 0 {
		t.Fatalf("empty run reported %d targets / %d results, want none", report.Total, len(report.Results))
	}
}

// The panel must draw the editable list and post additions; a regression that
// drops the wiring would leave the modal showing results for a stale list.
func TestDiagnosticsPanelWiresTargetEditor(t *testing.T) {
	app := readAsset(t, "js/app.js")
	html := readAsset(t, "index.html")

	for _, id := range []string{"diag-target-form", "diag-target-name", "diag-target-host", "diag-targets-list"} {
		if !strings.Contains(html, `id="`+id+`"`) {
			t.Errorf("index.html is missing #%s — the target editor cannot render", id)
		}
	}
	for _, want := range []struct{ src, why string }{
		{"function loadDiagTargets", "the list has to be loaded from the server"},
		{"function addDiagTarget", "the add form has to submit somewhere"},
		{"function removeDiagTarget", "each row's remove button has to reach the server"},
		{"/api/diagnostics/targets", "the CRUD endpoint has to be called"},
	} {
		if !strings.Contains(app, want.src) {
			t.Errorf("js/app.js is missing %q — %s", want.src, want.why)
		}
	}
}
