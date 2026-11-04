package service

import (
	"path/filepath"
	"testing"
	"time"

	"digitdns/internal/core/matcher"
	"digitdns/internal/crypto"
	"digitdns/internal/database"
)

func openFilterTestDB(t *testing.T) *database.DB {
	t.Helper()
	dir := t.TempDir()
	cipher, err := crypto.LoadOrGenerateMasterKey(filepath.Join(dir, "filter.key"))
	if err != nil {
		t.Fatalf("cipher: %v", err)
	}
	db, err := database.Open(filepath.Join(dir, "filter.db"), cipher)
	if err != nil {
		t.Fatalf("database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestFilterServiceCompilesAndAppliesPolicy(t *testing.T) {
	db := openFilterTestDB(t)
	m := matcher.NewMatcher()
	f := NewFilterService(db, m)

	list := database.FilterList{ID: "one", Name: "Test list", Enabled: true}
	if err := db.SaveFilterList(list); err != nil {
		t.Fatalf("save list: %v", err)
	}
	if err := db.SaveFilterSource("one", "||tracker.example^\n"); err != nil {
		t.Fatalf("save source: %v", err)
	}
	f.rebuild()

	if action, rule, _ := m.MatchDetail("tracker.example"); action != matcher.ActionBlock || rule != "Blocklist: Test list" {
		t.Fatalf("blocked = (%v, %q)", action, rule)
	}

	settings := database.DefaultFilteringSettings()
	settings.Allowlist = []string{"tracker.example"}
	if err := f.SaveSettings(settings); err != nil {
		t.Fatalf("save settings: %v", err)
	}
	f.rebuild()
	if action, _, _ := m.MatchDetail("tracker.example"); action != matcher.ActionDirect {
		t.Fatal("allowlist did not suppress the list")
	}

	settings.Enabled = false
	if err := f.SaveSettings(settings); err != nil {
		t.Fatalf("save disabled settings: %v", err)
	}
	f.rebuild()
	if action, _, _ := m.MatchDetail("tracker.example"); action != matcher.ActionDirect {
		t.Fatal("disabled filtering still blocked")
	}

	settings.Enabled = true
	settings.BlockMode = database.FilterBlockNXDOMAIN
	settings.BlockTTL = 42
	if err := f.SaveSettings(settings); err != nil {
		t.Fatalf("save nxdomain settings: %v", err)
	}
	mode, ttl := f.BlockPolicy()
	if mode != database.FilterBlockNXDOMAIN || ttl != 42 {
		t.Fatalf("block policy = %q/%d", mode, ttl)
	}
	// SaveSettings publishes the policy and rebuilds in the background; let that
	// goroutine finish before t.Cleanup closes the database.
	time.Sleep(50 * time.Millisecond)
}

func TestFilterServiceRejectsPrivateURLs(t *testing.T) {
	if err := validateListURL("https://127.0.0.1/list.txt"); err == nil {
		t.Fatal("private URL was accepted")
	}
	if err := validateListURL("http://example.com/list.txt"); err == nil {
		t.Fatal("plain HTTP URL was accepted")
	}
}
