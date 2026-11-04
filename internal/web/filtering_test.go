package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFilteringAPIUnavailableWithoutService(t *testing.T) {
	ws, _, cleanup := setupTestWebServer(t)
	defer cleanup()

	w := httptest.NewRecorder()
	ws.handleFiltering(w, httptest.NewRequest(http.MethodGet, "/api/filtering", nil))
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("filtering without service = %d, want 503", w.Code)
	}
}
