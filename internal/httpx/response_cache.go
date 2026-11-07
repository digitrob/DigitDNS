package httpx

import (
	"bytes"
	"compress/gzip"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// gzipMinBytes is the point at which compression pays for itself. The cached
// stats document is ~6 KB and the history document ~5 KB, so both are well over
// the line; a small error body is not.
const gzipMinBytes = 512

// CachedResponse is one encoded JSON document. GzipBody is the same document
// compressed once when the entry is stored, so a cache hit costs no compression
// work even when several dashboards poll at different times.
type CachedResponse struct {
	Body        []byte
	GzipBody    []byte
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
	entry := CachedResponse{
		Body:        copied,
		ContentType: contentType,
		Expires:     now.Add(c.ttl),
	}
	if len(copied) >= gzipMinBytes {
		entry.GzipBody = gzipBytes(copied)
	}
	c.entries[key] = entry
}

func gzipBytes(body []byte) []byte {
	var buf bytes.Buffer
	zw, err := gzip.NewWriterLevel(&buf, gzip.BestSpeed)
	if err != nil {
		return nil
	}
	if _, err := zw.Write(body); err != nil {
		return nil
	}
	if err := zw.Close(); err != nil {
		return nil
	}
	return buf.Bytes()
}

func acceptsGzip(header string) bool {
	for _, part := range strings.Split(header, ",") {
		fields := strings.Split(part, ";")
		if !strings.EqualFold(strings.TrimSpace(fields[0]), "gzip") {
			continue
		}
		q := 1.0
		for _, param := range fields[1:] {
			param = strings.TrimSpace(param)
			if strings.HasPrefix(param, "q=") {
				if v, err := strconv.ParseFloat(strings.TrimPrefix(param, "q="), 64); err == nil {
					q = v
				}
			}
		}
		if q > 0 {
			return true
		}
	}
	return false
}

// WriteCached serves an encoded response, honouring HEAD without writing a body
// and negotiating gzip when the cached entry has a compressed representation.
func WriteCached(w http.ResponseWriter, r *http.Request, entry CachedResponse) {
	body := entry.Body
	if entry.GzipBody != nil {
		// The same URL has two representations; shared caches must key on the
		// request's encoding.
		w.Header().Set("Vary", "Accept-Encoding")
		if acceptsGzip(r.Header.Get("Accept-Encoding")) {
			body = entry.GzipBody
			w.Header().Set("Content-Encoding", "gzip")
		}
	}
	if entry.ContentType != "" {
		w.Header().Set("Content-Type", entry.ContentType)
	}
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}
	_, _ = w.Write(body)
}

// WriteJSONBytes writes one encoded JSON document with the same gzip negotiation
// as the cached endpoints. The caller owns the ETag/304 logic.
func WriteJSONBytes(w http.ResponseWriter, r *http.Request, status int, body []byte, contentType string) {
	if len(body) >= gzipMinBytes {
		w.Header().Set("Vary", "Accept-Encoding")
		if acceptsGzip(r.Header.Get("Accept-Encoding")) {
			if zipped := gzipBytes(body); zipped != nil {
				body = zipped
				w.Header().Set("Content-Encoding", "gzip")
			}
		}
	}
	if contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(status)
	if r.Method == http.MethodHead {
		return
	}
	_, _ = w.Write(body)
}
