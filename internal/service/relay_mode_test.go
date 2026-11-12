package service

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"digitdns/internal/core/proxy"
	"digitdns/internal/database"
)

func TestRelayModeManagerSyncsAndHeartbeats(t *testing.T) {
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/agent/catalogue":
			if r.URL.Query().Get("wait") != "" {
				time.Sleep(40 * time.Millisecond)
			}
			w.Header().Set("ETag", `"v1"`)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"country":      "DE",
				"hosts":        []string{"example.com"},
				"clients":      []string{"127.0.0.1"},
				"ipv6_enabled": false,
			})
		case "/agent/heartbeat":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ipv6_enabled":    false,
				"config_revision": `"v1"`,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer hub.Close()

	db := egressTestDB(t)
	settings := database.RelayModeSettings{
		Enabled:  true,
		HubURL:   hub.URL,
		AgentKey: "agt_uk:secret",
		AgentID:  "agt_uk",
	}
	if err := db.SetSetting("relay_mode", settings); err != nil {
		t.Fatal(err)
	}

	sni := proxy.NewServer(database.SNIProxySettings{}, "127.0.0.1", "", nil)
	manager := NewRelayModeManager(db, sni, 443, func() string { return "" }, nil)
	if err := manager.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer manager.Stop()

	deadline := time.Now().Add(3 * time.Second)
	for {
		view := manager.Status()
		if view.Online && view.Hosts == 1 && view.Clients == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("relay mode did not sync/heartbeat: %+v", view)
		}
		time.Sleep(10 * time.Millisecond)
	}

	manager.mu.Lock()
	policy := manager.policy
	manager.mu.Unlock()
	if policy == nil || !policy.AllowsHost("example.com") || !policy.AllowsClient(net.ParseIP("127.0.0.1")) {
		t.Fatal("the synced catalogue was not applied to the proxy policy")
	}

	latency, err := manager.Test(context.Background())
	if err != nil {
		t.Fatalf("relay mode test: %v", err)
	}
	if latency <= 0 {
		t.Fatalf("relay mode test latency = %v, want a measured duration", latency)
	}
}
