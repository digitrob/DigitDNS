package httpx

import (
	"net/http"
	"net/http/httptest"
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
