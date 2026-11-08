package web

import (
	"strings"
	"testing"

	"digitdns/internal/version"
)

// The control-plane counter must survive a restart, and the ETag must still
// change when the build changes. Otherwise a restart either herds every agent
// into a full fetch (unpersisted counter) or hides an upgrade (counter alone).
func TestAgentRevisionPersistsAndCarriesBuildHash(t *testing.T) {
	ws, db, cleanup := setupTestWebServer(t)
	defer cleanup()

	if got := loadAgentRevision(db); got != 0 {
		t.Fatalf("fresh install revision = %d, want 0", got)
	}

	ws.agentRev.Store(41)
	ws.agentConfigChanged()
	if got := loadAgentRevision(db); got != 42 {
		t.Fatalf("persisted revision = %d, want 42", got)
	}

	tag := ws.agentRevision()
	if !strings.HasPrefix(tag, `"v42`) {
		t.Fatalf("ETag = %q, want the persisted counter", tag)
	}
	if build := version.Get().Hash; build != "" && !strings.Contains(tag, build) {
		t.Fatalf("ETag = %q, want the build hash %q", tag, build)
	}
}

// The rules and client list are identical for every agent, so one revision must
// read them once. A later revision must not reuse the previous snapshot.
func TestCatalogueInputsAreCachedPerRevision(t *testing.T) {
	ws, _, cleanup := setupTestWebServer(t)
	defer cleanup()

	first, _, err := ws.catalogueInputs()
	if err != nil {
		t.Fatalf("catalogueInputs: %v", err)
	}
	first["__cache_marker__"] = true

	second, _, err := ws.catalogueInputs()
	if err != nil {
		t.Fatalf("catalogueInputs (cached): %v", err)
	}
	if _, ok := second["__cache_marker__"]; !ok {
		t.Fatal("same revision re-read the rules instead of using the cache")
	}

	ws.agentConfigChanged()
	third, _, err := ws.catalogueInputs()
	if err != nil {
		t.Fatalf("catalogueInputs (new revision): %v", err)
	}
	if _, ok := third["__cache_marker__"]; ok {
		t.Fatal("a new revision reused the previous rules snapshot")
	}
}
