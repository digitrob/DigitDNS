package web

import (
	"bytes"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

// createAgent exercises the panel flow: one POST returns the record and the
// clear key once. The key is the credential the agent starts with.
func createAgent(t *testing.T, ws *WebServer, country, name string) (string, string) {
	t.Helper()
	rec := doJSON(t, ws.handleAgents, http.MethodPost, "/api/agents",
		`{"country":"`+country+`","name":"`+name+`"}`, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create agent: %d %s", rec.Code, rec.Body.String())
	}
	var created struct {
		Key   string `json:"key"`
		Agent struct {
			ID string `json:"id"`
		} `json:"agent"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	if created.Key == "" || created.Agent.ID == "" {
		t.Fatalf("create response missing key or id: %s", rec.Body.String())
	}
	return created.Agent.ID, created.Key
}

func TestAgentCreateHeartbeatCatalogueLoop(t *testing.T) {
	ws := newAgentWebServer(t)
	agentID, key := createAgent(t, ws, "DE", "DE-1")
	auth := map[string]string{agentKeyHeader: key}

	rec := doJSON(t, ws.handleAgentHeartbeat, http.MethodPost, "/agent/heartbeat",
		`{"version":"1.0.1","relay_port":443,"bytes_in":10,"bytes_out":20}`, auth)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("heartbeat: %d %s", rec.Code, rec.Body.String())
	}
	agent, err := ws.db.GetAgent(agentID)
	if err != nil {
		t.Fatal(err)
	}
	if agent.PublicIP != "203.0.113.9" || agent.LastSeen.IsZero() {
		t.Fatalf("heartbeat did not record address/liveness: %+v", agent)
	}

	rec = doJSON(t, ws.handleAgentCatalogue, http.MethodGet, "/agent/catalogue", "", auth)
	if rec.Code != http.StatusOK {
		t.Fatalf("catalogue: %d %s", rec.Code, rec.Body.String())
	}
	var cat agentCatalogue
	if err := json.Unmarshal(rec.Body.Bytes(), &cat); err != nil {
		t.Fatalf("decode catalogue: %v", err)
	}
	if cat.Country != "DE" || cat.AgentID != agentID {
		t.Fatalf("catalogue identifies the wrong agent: %+v", cat)
	}

	rec = doJSON(t, ws.handleAgentCatalogue, http.MethodGet, "/agent/catalogue", "", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("catalogue without a key must be refused, got %d", rec.Code)
	}
}

func TestAgentConnectionTestRecordsPing(t *testing.T) {
	ws := newAgentWebServer(t)
	agentID, _ := createAgent(t, ws, "DE", "DE-1")

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	port, _ := strconv.Atoi(strings.Split(ln.Addr().String(), ":")[1])
	if err := ws.db.TouchAgent(agentID, "1.0.0", "127.0.0.1", port, 0, 0); err != nil {
		t.Fatal(err)
	}

	rec := doJSON(t, ws.handleAgentByID, http.MethodPost, "/api/agents/"+agentID+"/test", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("test: %d %s", rec.Code, rec.Body.String())
	}
	var out struct {
		OK     bool    `json:"ok"`
		PingMs float64 `json:"ping_ms"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if !out.OK || out.PingMs <= 0 {
		t.Fatalf("test result = %+v", out)
	}
	agent, _ := ws.db.GetAgent(agentID)
	if agent.PingAt.IsZero() || agent.PingMs <= 0 {
		t.Fatalf("ping not stored: %+v", agent)
	}
}

func TestAgentKeyCannotBeForged(t *testing.T) {
	ws := newAgentWebServer(t)
	agentID, _ := createAgent(t, ws, "DE", "DE-1")

	rec := doJSON(t, ws.handleAgentCatalogue, http.MethodGet, "/agent/catalogue", "",
		map[string]string{agentKeyHeader: agentID + ":not-the-secret"})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("a forged key must be refused, got %d", rec.Code)
	}
}

func TestAgentCatalogueOnlyCarriesAssignedDomains(t *testing.T) {
	ws := newAgentWebServer(t)
	deID, deKey := createAgent(t, ws, "DE", "DE-1")
	usID, usKey := createAgent(t, ws, "US", "US-1")

	rules := `{
		"custom_proxied": ["direct.example", "service.example"],
		"services": {"tv": "TV"},
		"domain_services": {"service.example": "tv"},
		"domain_agents": {"direct.example": "` + deID + `"},
		"service_agents": {"tv": "` + deID + `"}
	}`
	if rec := doJSON(t, ws.handleConfigRules, http.MethodPost, "/api/config/rules", rules, nil); rec.Code != http.StatusOK {
		t.Fatalf("save rules: %d %s", rec.Code, rec.Body.String())
	}

	rec := doJSON(t, ws.handleAgentCatalogue, http.MethodGet, "/agent/catalogue", "",
		map[string]string{agentKeyHeader: deKey})
	var cat agentCatalogue
	_ = json.Unmarshal(rec.Body.Bytes(), &cat)
	joined := strings.Join(cat.Hosts, ",")
	for _, want := range []string{"direct.example", "*.direct.example", "service.example", "*.service.example"} {
		if !strings.Contains(joined, want) {
			t.Errorf("catalogue hosts %q missing %q", joined, want)
		}
	}

	rec = doJSON(t, ws.handleAgentCatalogue, http.MethodGet, "/agent/catalogue", "",
		map[string]string{agentKeyHeader: usKey})
	var usCat agentCatalogue
	_ = json.Unmarshal(rec.Body.Bytes(), &usCat)
	if len(usCat.Hosts) != 0 {
		t.Errorf("unassigned agent %s hosts = %v, want none", usID, usCat.Hosts)
	}
}

func TestAgentEditAndRotateKey(t *testing.T) {
	ws := newAgentWebServer(t)
	agentID, oldKey := createAgent(t, ws, "DE", "DE-1")

	rec := doJSON(t, ws.handleAgentByID, http.MethodGet, "/api/agents/"+agentID+"/key", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("copy key: %d %s", rec.Code, rec.Body.String())
	}
	var copied struct {
		Key string `json:"key"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &copied)
	if copied.Key != oldKey {
		t.Fatalf("copied key = %q, want %q", copied.Key, oldKey)
	}

	rec = doJSON(t, ws.handleAgentByID, http.MethodPut, "/api/agents/"+agentID,
		`{"name":"Frankfurt 2","country":"US","public_ip":"203.0.113.20"}`, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("edit: %d %s", rec.Code, rec.Body.String())
	}
	agent, err := ws.db.GetAgent(agentID)
	if err != nil {
		t.Fatal(err)
	}
	if agent.Name != "Frankfurt 2" || agent.Country != "US" || agent.PublicIP != "203.0.113.20" {
		t.Fatalf("agent after edit = %+v", agent)
	}

	rec = doJSON(t, ws.handleAgentByID, http.MethodPost, "/api/agents/"+agentID+"/rotate-key", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("rotate: %d %s", rec.Code, rec.Body.String())
	}
	var rotated struct {
		Key string `json:"key"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &rotated)
	if rotated.Key == "" || rotated.Key == oldKey {
		t.Fatalf("rotate returned %q, old %q", rotated.Key, oldKey)
	}
	rec = doJSON(t, ws.handleAgentByID, http.MethodGet, "/api/agents/"+agentID+"/key", "", nil)
	var copiedAfter struct {
		Key string `json:"key"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &copiedAfter)
	if copiedAfter.Key != rotated.Key {
		t.Fatalf("copied key after rotate = %q, want %q", copiedAfter.Key, rotated.Key)
	}

	// The old key is dead; the new one authenticates.
	rec = doJSON(t, ws.handleAgentCatalogue, http.MethodGet, "/agent/catalogue", "",
		map[string]string{agentKeyHeader: oldKey})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("old key still works: %d", rec.Code)
	}
	rec = doJSON(t, ws.handleAgentCatalogue, http.MethodGet, "/agent/catalogue", "",
		map[string]string{agentKeyHeader: rotated.Key})
	if rec.Code != http.StatusOK {
		t.Fatalf("new key rejected: %d %s", rec.Code, rec.Body.String())
	}
}

func TestLegacyEnrolTokenStillWorks(t *testing.T) {
	ws := newAgentWebServer(t)
	token, _, err := ws.db.CreateEnrolmentToken("DE", "Legacy")
	if err != nil {
		t.Fatal(err)
	}
	rec := doJSON(t, ws.handleAgentEnrol, http.MethodPost, "/agent/enrol",
		`{"token":"`+token+`","version":"1.0.0"}`, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("enrol: %d %s", rec.Code, rec.Body.String())
	}
	var enrolled struct {
		Key string `json:"key"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &enrolled)
	if enrolled.Key == "" {
		t.Fatal("enrol returned no key")
	}
}

// newAgentWebServer reuses the package harness, which already opens a real
// database and a real client service - the two things the agent routes need.
func newAgentWebServer(t *testing.T) *WebServer {
	t.Helper()
	ws, _, cleanup := setupTestWebServer(t)
	t.Cleanup(cleanup)
	return ws
}

func doJSON(t *testing.T, h http.HandlerFunc, method, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, path, nil)
	} else {
		r = httptest.NewRequest(method, path, bytes.NewBufferString(body))
	}
	r.RemoteAddr = "203.0.113.9:4242"
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h(rec, r)
	return rec
}
