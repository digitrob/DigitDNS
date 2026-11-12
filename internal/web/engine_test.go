package web

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"digitdns/internal/database"
	"digitdns/internal/logbuffer"
)

func TestEngineSwitchPersistsAndApplies(t *testing.T) {
	ws := newAgentWebServer(t)
	applied := true
	ws.SetEngineSetter(func(enabled bool) { applied = enabled })
	ws.SetEngineState(func() bool { return applied })

	rec := doJSON(t, ws.handleEngine, http.MethodGet, "/api/engine", "", nil)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"enabled":true`) {
		t.Fatalf("initial engine = %d %s", rec.Code, rec.Body.String())
	}

	rec = doJSON(t, ws.handleEngine, http.MethodPost, "/api/engine", `{"enabled":false}`, nil)
	if rec.Code != http.StatusOK || applied {
		t.Fatalf("engine off = %d applied=%v body=%s", rec.Code, applied, rec.Body.String())
	}
	var cfg database.DNSSettings
	if err := ws.db.GetSetting("dns", &cfg); err != nil {
		t.Fatal(err)
	}
	if !cfg.EngineDisabled {
		t.Fatal("engine off was not persisted")
	}

	rec = doJSON(t, ws.handleEngine, http.MethodPost, "/api/engine", `{"enabled":true}`, nil)
	if rec.Code != http.StatusOK || !applied {
		t.Fatalf("engine on = %d applied=%v body=%s", rec.Code, applied, rec.Body.String())
	}
}

func TestEngineSwitchRejectsMissingBoolean(t *testing.T) {
	ws := newAgentWebServer(t)
	rec := doJSON(t, ws.handleEngine, http.MethodPost, "/api/engine", `{}`, nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("missing enabled = %d, want 400", rec.Code)
	}
}

func TestSystemLogEndpointReturnsBufferTail(t *testing.T) {
	ws := newAgentWebServer(t)
	buf := logbuffer.New(10, 1<<20)
	_, _ = buf.Write([]byte("line one\nline two\n"))
	ws.SetSystemLog(buf)

	rec := doJSON(t, ws.handleSystemLog, http.MethodGet, "/api/system-log?limit=1", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("system log = %d %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Lines []string `json:"lines"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Lines) != 1 || body.Lines[0] != "line two" {
		t.Fatalf("lines = %v, want the newest line", body.Lines)
	}

	rec = doJSON(t, ws.handleSystemLogExport, http.MethodGet, "/api/system-log/export", "", nil)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "line one") {
		t.Fatalf("export = %d %q", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Header().Get("Content-Disposition"), "attachment") {
		t.Fatal("export is not an attachment")
	}
}
