package web

import "testing"

// allow_all lives under its own key now, written by /api/access/mode. The older
// "access" record can still contain a copy, and letting that copy win made the
// dashboard show the wrong mode (the banner stayed on after switching to
// whitelist, or disappeared after switching back).
func TestLoadAccessConfigUsesLiveAllowAll(t *testing.T) {
	ws, db, cleanup := setupTestWebServer(t)
	defer cleanup()

	if err := db.SetSetting("access", map[string]any{
		"allow_all":   true,
		"doh_tokens":  []string{"keep-me"},
		"doq_tokens":  []string{"doq"},
		"allowed_ips": []string{},
	}); err != nil {
		t.Fatalf("seed access: %v", err)
	}
	if err := ws.clients.SetAllowAll(false); err != nil {
		t.Fatalf("SetAllowAll(false): %v", err)
	}

	got := ws.loadAccessConfig()
	if got.AllowAll {
		t.Error("loadAccessConfig returned the stale access.allow_all=true")
	}
	if len(got.DoHTokens) != 1 || got.DoHTokens[0] != "keep-me" {
		t.Errorf("DoH tokens = %#v, want [keep-me]", got.DoHTokens)
	}
	if len(got.DoQTokens) != 1 || got.DoQTokens[0] != "doq" {
		t.Errorf("DoQ tokens = %#v, want [doq]", got.DoQTokens)
	}

	if err := ws.clients.SetAllowAll(true); err != nil {
		t.Fatalf("SetAllowAll(true): %v", err)
	}
	if !ws.loadAccessConfig().AllowAll {
		t.Error("loadAccessConfig did not follow the live allow_all=true")
	}
}
