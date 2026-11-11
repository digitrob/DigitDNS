package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"digitdns/internal/selfupdate"
)

// TestUpdateCheckDetachesFromTheBrowserRequest is the fix for the card's
// "reach GitHub: ... context canceled": a page reload used to cancel the
// outbound fetch through r.Context(). The check now runs on a detached context
// with its own deadline, and a failure still carries the container fields so
// the Docker update-method block cannot collapse.
func TestUpdateCheckDetachesFromTheBrowserRequest(t *testing.T) {
	t.Setenv("DIGITDNS_CONTAINER", "1")
	ws, _, cleanup := setupTestWebServer(t)
	defer cleanup()
	ws.SetUpdater(selfupdate.New("digitdns", nil, t.TempDir()))

	var calls atomic.Int32
	ws.updateCheckFn = func(ctx context.Context) (selfupdate.Status, error) {
		calls.Add(1)
		if err := ctx.Err(); err != nil {
			t.Errorf("the outbound check inherited the browser cancellation: %v", err)
		}
		return selfupdate.Status{
			Current:    "5.5.3",
			Container:  true,
			UpdateMode: "image",
		}, context.Canceled
	}

	handler := ws.buildAdminHandler()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest(http.MethodGet, "/api/update/check", nil).WithContext(ctx)
	req.Header.Set("Authorization", "Bearer "+ws.sessions.Create())
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("update check = %d, want 200: %s", w.Code, w.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["container"] != true {
		t.Errorf("container = %v, want true — a failed check must not collapse the Docker block", body["container"])
	}
	if body["update_mode"] != "image" {
		t.Errorf("update_mode = %v, want image", body["update_mode"])
	}
	msg, _ := body["error"].(string)
	if strings.Contains(strings.ToLower(msg), "context canceled") {
		t.Errorf("error leaked the raw cancellation: %q", msg)
	}
	if msg == "" {
		t.Error("a failed check should carry a human-readable error")
	}
	if calls.Load() != 1 {
		t.Errorf("check calls = %d, want 1", calls.Load())
	}
}

// TestUpdateCheckCachesAndForceBypasses: the dashboard asks on every boot, and
// the manual button must not repeat a cached failure.
func TestUpdateCheckCachesAndForceBypasses(t *testing.T) {
	ws, _, cleanup := setupTestWebServer(t)
	defer cleanup()
	ws.SetUpdater(selfupdate.New("digitdns", nil, t.TempDir()))

	var calls atomic.Int32
	ws.updateCheckFn = func(context.Context) (selfupdate.Status, error) {
		calls.Add(1)
		return selfupdate.Status{Current: "5.5.3"}, nil
	}

	handler := ws.buildAdminHandler()
	get := func(path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer "+ws.sessions.Create())
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		return w
	}

	if w := get("/api/update/check"); w.Code != http.StatusOK {
		t.Fatalf("first check = %d", w.Code)
	}
	if w := get("/api/update/check"); w.Code != http.StatusOK {
		t.Fatalf("cached check = %d", w.Code)
	}
	if calls.Load() != 1 {
		t.Errorf("check calls = %d after two unforced checks, want 1", calls.Load())
	}
	if w := get("/api/update/check?force=1"); w.Code != http.StatusOK {
		t.Fatalf("forced check = %d", w.Code)
	}
	if calls.Load() != 2 {
		t.Errorf("check calls = %d after a forced check, want 2", calls.Load())
	}
}
