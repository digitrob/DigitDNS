package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOpenAPIV2Reachable(t *testing.T) {
	a, _, _, cleanup := setupTestAPI(t)
	defer cleanup()
	mux := http.NewServeMux()
	a.RegisterRoutes(mux)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v2/openapi.json", nil)
	r.RemoteAddr = "127.0.0.1:1"
	mux.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("got %d body=%s", w.Code, w.Body.String())
	}
}
