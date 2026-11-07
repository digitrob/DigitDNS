package httpx

import (
	"net/http"
	"strconv"
	"sync"
	"time"
)

// CachedResponse is one encoded JSON document.
type CachedResponse struct {
	Body        []byte
	ContentType string
	Expires     time.Time
}

// ResponseCache is a tiny TTL cache for encoded API responses. It exists for
// endpoints that several dashboards poll at once: the work is identical for
// every caller, so one encode per TTL is enough. It is deliberately bounded —
// without a cap, a query string chosen by the caller would be an unbounded map.
type ResponseCache struct {
	mu      sync.Mutex
	entries map[string]CachedResponse
	ttl     time.Duration
	max     int
}

// NewResponseCache builds a cache holding at most max entries for ttl.
func NewResponseCache(ttl time.Duration, max int) *ResponseCache {
	if ttl <= 0 {
		ttl = time.Second
	}
	if max <= 0 {
		max = 16
	}
	return &ResponseCache{
		entries: make(map[string]CachedResponse, max),
		ttl:     ttl,
		max:     max,
	}
}

// Get returns a live entry. An expired entry is removed so it cannot occupy a
// slot until the next sweep.
func (c *ResponseCache) Get(key string) (CachedResponse, bool) {
	if c == nil {
		return CachedResponse{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[key]
	if !ok {
		return CachedResponse{}, false
	}
	if time.Now().After(entry.Expires) {
		delete(c.entries, key)
		return CachedResponse{}, false
	}
	return entry, true
}

// Set stores an encoded body. When the cache is full it drops expired entries
// first, then the oldest one; the cache is a performance aid, never a source of
// truth, so evicting early is always safe.
func (c *ResponseCache) Set(key string, body []byte, contentType string) {
	if c == nil {
		return
	}
	now := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.entries) >= c.max {
		for k, entry := range c.entries {
			if now.After(entry.Expires) {
				delete(c.entries, k)
			}
		}
		if len(c.entries) >= c.max {
			var oldestKey string
			var oldest time.Time
			for k, entry := range c.entries {
				if oldestKey == "" || entry.Expires.Before(oldest) {
					oldestKey, oldest = k, entry.Expires
				}
			}
			delete(c.entries, oldestKey)
		}
	}
	// Copy the body: the caller may reuse its buffer, and a caller mutating the
	// slice after Set would corrupt every later response.
	copied := make([]byte, len(body))
	copy(copied, body)
	c.entries[key] = CachedResponse{
		Body:        copied,
		ContentType: contentType,
		Expires:     now.Add(c.ttl),
	}
}

// WriteCached serves an encoded response, honouring HEAD without writing a body.
func WriteCached(w http.ResponseWriter, r *http.Request, entry CachedResponse) {
	if entry.ContentType != "" {
		w.Header().Set("Content-Type", entry.ContentType)
	}
	w.Header().Set("Content-Length", strconv.Itoa(len(entry.Body)))
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}
	_, _ = w.Write(entry.Body)
}
