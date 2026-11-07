package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"digitdns/internal/selfupdate"
)

// The container update method is saved and applied live, and a fresh updater
// picks the saved value up at startup.
func TestUpdateModePersistsAndApplies(t *testing.T) {
	t.Setenv("DIGITDNS_CONTAINER", "1")
	ws, _, cleanup := setupTestWebServer(t)
	defer cleanup()
	ws.SetUpdater(selfupdate.New("digitdns", nil, t.TempDir()))
	handler := ws.buildAdminHandler()

	send := func(method, body string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, "/api/update/mode", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+ws.sessions.Create())
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		return w
	}

	if w := send(http.MethodPost, `{"mode":"binary"}`); w.Code != http.StatusOK {
		t.Fatalf("set binary = %d: %s", w.Code, w.Body.String())
	}
	if got := ws.updater.ContainerUpdateMode(); got != "binary" {
		t.Fatalf("live mode = %q, want binary", got)
	}
	if got := ws.db.GetUpdateSettings().ContainerMode; got != "binary" {
		t.Fatalf("stored mode = %q, want binary", got)
	}

	// A replacement updater (the next process) reads the saved setting.
	fresh := selfupdate.New("digitdns", nil, t.TempDir())
	ws.SetUpdater(fresh)
	if got := fresh.ContainerUpdateMode(); got != "binary" {
		t.Fatalf("startup mode = %q, want binary", got)
	}

	if w := send(http.MethodPost, `{"mode":"sideways"}`); w.Code != http.StatusBadRequest {
		t.Fatalf("invalid mode = %d, want 400", w.Code)
	}
	if w := send(http.MethodGet, ""); w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET mode = %d, want 405", w.Code)
	}
}
