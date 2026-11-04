package api

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestV3_FilteringRoutes(t *testing.T) {
	apiInst, _, _, cleanup := setupTestAPI(t)
	defer cleanup()
	apiInst.SetV3Backend(&fakeV3Backend{})
	mux := http.NewServeMux()
	apiInst.RegisterRoutes(mux)
	const key = "digitdns_live_testkey123"

	w := v3Request(mux, http.MethodGet, "/api/v3/filtering", key, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("filtering status = %d, want 200: %s", w.Code, w.Body.String())
	}
	var status map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &status)
	if status["total_domains"] != float64(3) {
		t.Fatalf("status body = %v", status)
	}

	if w := v3Request(mux, http.MethodPost, "/api/v3/filtering/settings", key, map[string]any{"enabled": true}); w.Code != http.StatusOK {
		t.Fatalf("save settings = %d, want 200: %s", w.Code, w.Body.String())
	}
	if w := v3Request(mux, http.MethodPut, "/api/v3/filtering/settings", key, map[string]any{"enabled": true}); w.Code != http.StatusOK {
		t.Fatalf("put settings = %d, want 200: %s", w.Code, w.Body.String())
	}
	if w := v3Request(mux, http.MethodPost, "/api/v3/filtering/refresh", key, nil); w.Code != http.StatusOK {
		t.Fatalf("refresh = %d, want 200: %s", w.Code, w.Body.String())
	}
	if w := v3Request(mux, http.MethodGet, "/api/v3/filtering/presets", key, nil); w.Code != http.StatusOK {
		t.Fatalf("get presets = %d, want 200: %s", w.Code, w.Body.String())
	}
	if w := v3Request(mux, http.MethodPost, "/api/v3/filtering/presets", key, map[string]any{"ids": []string{"hagezi-pro"}}); w.Code != http.StatusOK {
		t.Fatalf("add presets = %d, want 200: %s", w.Code, w.Body.String())
	}
	if w := v3Request(mux, http.MethodPost, "/api/v3/filtering/lists", key, map[string]any{"name": "Custom", "url": "https://example.com/list.txt"}); w.Code != http.StatusOK {
		t.Fatalf("add list = %d, want 200: %s", w.Code, w.Body.String())
	}

	for _, tc := range []struct {
		method string
		path   string
		body   any
	}{
		{http.MethodPatch, "/api/v3/filtering/lists/abc", map[string]any{"enabled": false}},
		{http.MethodPost, "/api/v3/filtering/lists/abc", map[string]any{"enabled": true}},
		{http.MethodDelete, "/api/v3/filtering/lists/abc", nil},
		{http.MethodPost, "/api/v3/filtering/lists/abc/update", nil},
	} {
		if w := v3Request(mux, tc.method, tc.path, key, tc.body); w.Code != http.StatusOK {
			t.Fatalf("%s %s = %d, want 200: %s", tc.method, tc.path, w.Code, w.Body.String())
		}
	}

	if w := v3Request(mux, http.MethodPost, "/api/v3/filtering/lists/abc/reorder", key, nil); w.Code != http.StatusNotFound {
		t.Fatalf("unknown item action = %d, want 404", w.Code)
	}
	if w := v3Request(mux, http.MethodGet, "/api/v3/filtering/lists/abc", key, nil); w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET list item = %d, want 405", w.Code)
	}
	if w := v3Request(mux, http.MethodGet, "/api/v3/filtering", "", nil); w.Code != http.StatusUnauthorized {
		t.Fatalf("filtering without credential = %d, want 401", w.Code)
	}
}
