package httpx

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

func TestResponseCacheServesUntilExpiry(t *testing.T) {
	c := NewResponseCache(25*time.Millisecond, 2)
	c.Set("k", []byte(`{"ok":true}`), "application/json")

	entry, ok := c.Get("k")
	if !ok || string(entry.Body) != `{"ok":true}` || entry.ContentType != "application/json" {
		t.Fatalf("Get() = %+v, %v", entry, ok)
	}

	time.Sleep(40 * time.Millisecond)
	if _, ok := c.Get("k"); ok {
		t.Fatal("an expired entry was served")
	}
}

// The cache must own its bytes: a caller that reuses its encoding buffer after
// Set would otherwise corrupt every later response.
func TestResponseCacheCopiesBody(t *testing.T) {
	c := NewResponseCache(time.Second, 2)
	body := []byte(`{"first":true}`)
	c.Set("k", body, "application/json")
	copy(body, []byte(`{"second":false}`))

	entry, ok := c.Get("k")
	if !ok || string(entry.Body) != `{"first":true}` {
		t.Fatalf("cached body mutated with the caller's buffer: %q", entry.Body)
	}
}

func TestResponseCacheEvictsWhenFull(t *testing.T) {
	c := NewResponseCache(time.Minute, 2)
	c.Set("a", []byte("a"), "text/plain")
	c.Set("b", []byte("b"), "text/plain")
	c.Set("c", []byte("c"), "text/plain")

	if len(c.entries) > 2 {
		t.Fatalf("cache holds %d entries, want at most 2", len(c.entries))
	}
	if _, ok := c.Get("c"); !ok {
		t.Fatal("the newest entry was evicted")
	}
}

func TestWriteCachedHonoursHead(t *testing.T) {
	c := NewResponseCache(time.Second, 2)
	c.Set("k", []byte(`{"ok":true}`), "application/json")
	entry, _ := c.Get("k")

	w := httptest.NewRecorder()
	WriteCached(w, httptest.NewRequest(http.MethodHead, "/api/stats", nil), entry)
	if w.Body.Len() != 0 {
		t.Fatalf("HEAD wrote %d body bytes", w.Body.Len())
	}
	if got := w.Header().Get("Content-Length"); got != "11" {
		t.Fatalf("Content-Length = %q, want 11", got)
	}

	w = httptest.NewRecorder()
	WriteCached(w, httptest.NewRequest(http.MethodGet, "/api/stats", nil), entry)
	if w.Body.String() != `{"ok":true}` {
		t.Fatalf("GET body = %q", w.Body.String())
	}
}

func TestNilResponseCacheIsSafe(t *testing.T) {
	var c *ResponseCache
	if _, ok := c.Get("k"); ok {
		t.Fatal("nil cache served an entry")
	}
	c.Set("k", []byte("x"), "text/plain") // must not panic
}

func TestWriteJSONBytesNegotiatesGzip(t *testing.T) {
	body := bytes.Repeat([]byte(`{"host":"api.example"}`), 100)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/agent/catalogue", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	WriteJSONBytes(w, req, http.StatusOK, body, "application/json")

	if enc := w.Header().Get("Content-Encoding"); enc != "gzip" {
		t.Fatalf("Content-Encoding = %q, want gzip", enc)
	}
	if got := w.Header().Get("Vary"); got != "Accept-Encoding" {
		t.Fatalf("Vary = %q", got)
	}
	zr, err := gzip.NewReader(w.Body)
	if err != nil {
		t.Fatal(err)
	}
	plain, err := io.ReadAll(zr)
	if err != nil {
		t.Fatal(err)
	}
	_ = zr.Close()
	if !bytes.Equal(plain, body) {
		t.Fatal("gzip body did not round-trip")
	}
}

func TestWriteCachedNegotiatesGzip(t *testing.T) {
	body := bytes.Repeat([]byte(`{"field":"value"}`), 100)
	c := NewResponseCache(time.Second, 2)
	c.Set("k", body, "application/json")
	entry, ok := c.Get("k")
	if !ok || entry.GzipBody == nil {
		t.Fatal("large entry was not pre-compressed")
	}

	// A client that does not accept gzip gets the identity body.
	w := httptest.NewRecorder()
	WriteCached(w, httptest.NewRequest(http.MethodGet, "/api/stats", nil), entry)
	if enc := w.Header().Get("Content-Encoding"); enc != "" {
		t.Fatalf("identity client got Content-Encoding %q", enc)
	}
	if !bytes.Equal(w.Body.Bytes(), body) {
		t.Fatal("identity body was altered")
	}

	// A gzip client gets the compressed representation, and the headers describe
	// the bytes actually sent.
	w = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/stats", nil)
	req.Header.Set("Accept-Encoding", "br, gzip;q=0.9")
	WriteCached(w, req, entry)
	if enc := w.Header().Get("Content-Encoding"); enc != "gzip" {
		t.Fatalf("Content-Encoding = %q, want gzip", enc)
	}
	if got := w.Header().Get("Vary"); got != "Accept-Encoding" {
		t.Fatalf("Vary = %q", got)
	}
	zr, err := gzip.NewReader(w.Body)
	if err != nil {
		t.Fatalf("gzip reader: %v", err)
	}
	plain, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("decompress: %v", err)
	}
	_ = zr.Close()
	if !bytes.Equal(plain, body) {
		t.Fatal("gzip body did not round-trip")
	}

	// HEAD keeps the representation headers and writes no body.
	w = httptest.NewRecorder()
	head := httptest.NewRequest(http.MethodHead, "/api/stats", nil)
	head.Header.Set("Accept-Encoding", "gzip")
	WriteCached(w, head, entry)
	if enc := w.Header().Get("Content-Encoding"); enc != "gzip" {
		t.Fatalf("HEAD Content-Encoding = %q, want gzip", enc)
	}
	if w.Body.Len() != 0 {
		t.Fatalf("HEAD wrote %d bytes", w.Body.Len())
	}
	if got := w.Header().Get("Content-Length"); got != strconv.Itoa(len(entry.GzipBody)) {
		t.Fatalf("HEAD Content-Length = %q, want %d", got, len(entry.GzipBody))
	}
}
