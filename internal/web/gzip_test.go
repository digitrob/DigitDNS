package web

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func gzipTestHandler(body, contentType string, status int, contentLength string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if contentType != "" {
			w.Header().Set("Content-Type", contentType)
		}
		if contentLength != "" {
			w.Header().Set("Content-Length", contentLength)
		}
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}
}

func TestGzipMiddlewareCompressesAPIJSON(t *testing.T) {
	ws, _, cleanup := setupTestWebServer(t)
	defer cleanup()
	body := `{"data":"` + strings.Repeat("x", 2048) + `"}`
	handler := ws.requireAuthWithMasterKey(gzipTestHandler(body, "application/json", http.StatusOK, ""))

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Authorization", "Bearer "+ws.settings.GetAPIKey())
	req.Header.Set("Accept-Encoding", "gzip, deflate, br")
	rec := httptest.NewRecorder()
	handler(rec, req)

	if got := rec.Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("Content-Encoding = %q, want gzip", got)
	}
	if !strings.Contains(rec.Header().Get("Vary"), "Accept-Encoding") {
		t.Errorf("Vary = %q, want Accept-Encoding", rec.Header().Get("Vary"))
	}
	zr, err := gzip.NewReader(rec.Body)
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	decoded, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("read gzip body: %v", err)
	}
	if string(decoded) != body {
		t.Errorf("decoded body does not match the handler output")
	}
}

func TestGzipMiddlewareHonoursOptOutAndExistingEncoding(t *testing.T) {
	ws, _, cleanup := setupTestWebServer(t)
	defer cleanup()
	body := strings.Repeat("y", 2048)

	t.Run("no Accept-Encoding", func(t *testing.T) {
		handler := ws.requireAuthWithMasterKey(gzipTestHandler(body, "application/json", http.StatusOK, ""))
		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		req.Header.Set("Authorization", "Bearer "+ws.settings.GetAPIKey())
		rec := httptest.NewRecorder()
		handler(rec, req)
		if got := rec.Header().Get("Content-Encoding"); got != "" {
			t.Errorf("Content-Encoding = %q, want none", got)
		}
	})

	t.Run("q=0", func(t *testing.T) {
		handler := ws.requireAuthWithMasterKey(gzipTestHandler(body, "application/json", http.StatusOK, ""))
		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		req.Header.Set("Authorization", "Bearer "+ws.settings.GetAPIKey())
		req.Header.Set("Accept-Encoding", "gzip;q=0")
		rec := httptest.NewRecorder()
		handler(rec, req)
		if got := rec.Header().Get("Content-Encoding"); got != "" {
			t.Errorf("Content-Encoding = %q, want none", got)
		}
	})

	t.Run("already encoded", func(t *testing.T) {
		handler := ws.requireAuthWithMasterKey(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Content-Encoding", "br")
			_, _ = io.WriteString(w, body)
		})
		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		req.Header.Set("Authorization", "Bearer "+ws.settings.GetAPIKey())
		req.Header.Set("Accept-Encoding", "gzip")
		rec := httptest.NewRecorder()
		handler(rec, req)
		if got := rec.Header().Get("Content-Encoding"); got != "br" {
			t.Errorf("Content-Encoding = %q, want the handler's br", got)
		}
	})

	t.Run("event stream", func(t *testing.T) {
		handler := ws.requireAuthWithMasterKey(gzipTestHandler(body, "text/event-stream", http.StatusOK, ""))
		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		req.Header.Set("Authorization", "Bearer "+ws.settings.GetAPIKey())
		req.Header.Set("Accept-Encoding", "gzip")
		rec := httptest.NewRecorder()
		handler(rec, req)
		if got := rec.Header().Get("Content-Encoding"); got != "" {
			t.Errorf("Content-Encoding = %q, want none for SSE", got)
		}
	})

	t.Run("small known body", func(t *testing.T) {
		handler := ws.requireAuthWithMasterKey(gzipTestHandler("short", "application/json", http.StatusOK, "5"))
		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		req.Header.Set("Authorization", "Bearer "+ws.settings.GetAPIKey())
		req.Header.Set("Accept-Encoding", "gzip")
		rec := httptest.NewRecorder()
		handler(rec, req)
		if got := rec.Header().Get("Content-Encoding"); got != "" {
			t.Errorf("Content-Encoding = %q, want none for a small body", got)
		}
	})

	t.Run("small unknown body", func(t *testing.T) {
		handler := ws.requireAuthWithMasterKey(gzipTestHandler("short", "application/json", http.StatusOK, ""))
		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		req.Header.Set("Authorization", "Bearer "+ws.settings.GetAPIKey())
		req.Header.Set("Accept-Encoding", "gzip")
		rec := httptest.NewRecorder()
		handler(rec, req)
		if got := rec.Header().Get("Content-Encoding"); got != "" {
			t.Errorf("Content-Encoding = %q, want none for a small body", got)
		}
		if got := rec.Body.String(); got != "short" {
			t.Errorf("body = %q, want short", got)
		}
		if got := rec.Header().Get("Content-Length"); got != "5" {
			t.Errorf("Content-Length = %q, want 5", got)
		}
	})
}
