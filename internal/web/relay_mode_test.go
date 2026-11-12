package web

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"digitdns/internal/database"
)

func TestRelayModeSaveMasksAgentKey(t *testing.T) {
	ws := newAgentWebServer(t)
	rec := doJSON(t, ws.handleRelayMode, http.MethodPost, "/api/relay-mode",
		`{"enabled":true,"hub_url":"https://hub.example:8080","agent_key":"agt_uk:secret"}`, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("save relay mode = %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "secret") {
		t.Fatalf("relay mode response leaked the agent key: %s", rec.Body.String())
	}
	var body struct {
		RelayMode struct {
			Enabled     bool   `json:"enabled"`
			HubURL      string `json:"hub_url"`
			AgentID     string `json:"agent_id"`
			AgentKeySet bool   `json:"agent_key_set"`
		} `json:"relay_mode"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !body.RelayMode.Enabled || body.RelayMode.HubURL != "https://hub.example:8080" ||
		body.RelayMode.AgentID != "agt_uk" || !body.RelayMode.AgentKeySet {
		t.Fatalf("view = %+v", body.RelayMode)
	}

	var stored database.RelayModeSettings
	if err := ws.db.GetSetting("relay_mode", &stored); err != nil {
		t.Fatal(err)
	}
	if stored.AgentKey != "agt_uk:secret" {
		t.Fatal("the key was not persisted")
	}

	rec = doJSON(t, ws.handleRelayMode, http.MethodGet, "/api/relay-mode", "", nil)
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "secret") {
		t.Fatalf("get relay mode = %d %s", rec.Code, rec.Body.String())
	}
}

func TestRelayModeValidation(t *testing.T) {
	ws := newAgentWebServer(t)
	for _, body := range []string{
		`{"enabled":true,"hub_url":"ftp://hub.example","agent_key":"agt:secret"}`,
		`{"enabled":true,"hub_url":"https://hub.example"}`,
		`{"enabled":true,"hub_url":"https://hub.example","agent_key":"not-a-key"}`,
	} {
		rec := doJSON(t, ws.handleRelayMode, http.MethodPost, "/api/relay-mode", body, nil)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("body %s = %d, want 400 (%s)", body, rec.Code, rec.Body.String())
		}
	}
}
