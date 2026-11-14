package web

import (
	"compress/gzip"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

// gzipMinAPIResponse is the point at which compressing an API response pays for
// its own header and trailer. The JSON endpoints here are normally well above
// it; a short error body or the empty client list is not.
const gzipMinAPIResponse = 512

// The wrapper and its gzip.Writer are created on every API response. The
// dashboard polls several endpoints a second, so pooling both removes two
// allocations per request without changing the buffering behaviour.
var gzipResponsePool = sync.Pool{
	New: func() any { return &gzipResponseWriter{} },
}

var gzipWriterPool = sync.Pool{
	New: func() any { return gzip.NewWriter(io.Discard) },
}

// withGzip negotiates gzip for an authenticated API handler. Static assets have
// their own pre-compressed representation and are not routed through here; the
// stats cache and httpx.WriteJSONBytes already set Content-Encoding, which the
// writer below treats as "leave it alone" rather than compressing twice.
func (ws *WebServer) withGzip(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// HEAD carries no body, Range is not an API shape, and the SSE routes
		// are deliberately excluded before they reach this wrapper.
		if r.Method == http.MethodHead || r.Header.Get("Range") != "" || !acceptsEncoding(r, "gzip") {
			next(w, r)
			return
		}
		gw := gzipResponsePool.Get().(*gzipResponseWriter)
		gw.ResponseWriter = w
		gw.buf = gw.buf[:0]
		gw.status = 0
		gw.wrote = false
		gw.decided = false
		gw.compress = false
		gw.gz = nil
		defer func() {
			_ = gw.Close()
			if cap(gw.buf) > 64<<10 {
				gw.buf = nil
			}
			gw.ResponseWriter = nil
			gw.gz = nil
			gzipResponsePool.Put(gw)
		}()
		next(gw, r)
	}
}

// gzipResponseWriter buffers only the first gzipMinAPIResponse bytes. That is
// what lets it answer the one question the handler cannot: whether the body is
// large enough to be worth encoding. A 60-byte JSON array must stay identity;
// a 20 KB filter catalogue should not. Once the threshold is crossed it streams
// through gzip, so a large response is never held in memory.
type gzipResponseWriter struct {
	http.ResponseWriter
	buf      []byte
	status   int
	wrote    bool // WriteHeader was called by the handler
	decided  bool
	compress bool
	gz       *gzip.Writer
}

func (w *gzipResponseWriter) WriteHeader(status int) {
	if w.wrote {
		return
	}
	w.wrote = true
	w.status = status
}

func (w *gzipResponseWriter) Write(p []byte) (int, error) {
	if !w.wrote {
		w.WriteHeader(http.StatusOK)
	}
	if w.decided {
		if w.gz != nil {
			return w.gz.Write(p)
		}
		return w.ResponseWriter.Write(p)
	}

	w.buf = append(w.buf, p...)
	if len(w.buf) >= gzipMinAPIResponse {
		w.decide()
	}
	return len(p), nil
}

func (w *gzipResponseWriter) Flush() {
	if !w.decided {
		w.decide()
	}
	if w.gz != nil {
		_ = w.gz.Flush()
	}
	_ = http.NewResponseController(w.ResponseWriter).Flush()
}

// Unwrap lets http.ResponseController find the underlying writer's Flusher,
// Hijacker or deadline methods through this wrapper.
func (w *gzipResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func (w *gzipResponseWriter) Close() error {
	if !w.decided {
		w.decide()
	}
	if w.gz == nil {
		return nil
	}
	err := w.gz.Close()
	gzipWriterPool.Put(w.gz)
	w.gz = nil
	return err
}

// decide commits the status, headers and buffered prefix exactly once.
func (w *gzipResponseWriter) decide() {
	if w.decided {
		return
	}
	w.decided = true
	if !w.wrote {
		w.status = http.StatusOK
	}

	h := w.Header()
	_, alreadyEncoded := h["Content-Encoding"]
	small := len(w.buf) < gzipMinAPIResponse
	if raw := h.Get("Content-Length"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n < gzipMinAPIResponse {
			small = true
		}
	}
	w.compress = w.status != http.StatusNoContent &&
		w.status != http.StatusNotModified &&
		!alreadyEncoded &&
		!small &&
		compressibleAPIType(h.Get("Content-Type"))

	if w.compress {
		// The compressed body has a different length; leaving the identity
		// Content-Length in place would make every client truncate it.
		h.Del("Content-Length")
		addVary(h, "Accept-Encoding")
		h.Set("Content-Encoding", "gzip")
		gz := gzipWriterPool.Get().(*gzip.Writer)
		gz.Reset(w.ResponseWriter)
		w.gz = gz
	} else if h.Get("Content-Length") == "" {
		h.Set("Content-Length", strconv.Itoa(len(w.buf)))
	}

	w.ResponseWriter.WriteHeader(w.status)
	if len(w.buf) > 0 {
		if w.gz != nil {
			_, _ = w.gz.Write(w.buf)
		} else {
			_, _ = w.ResponseWriter.Write(w.buf)
		}
	}
	w.buf = nil
}

// compressibleAPIType is deliberately narrow: this wrapper is attached to API
// routes, not to downloads, images or already-compressed payloads. An absent
// Content-Type is treated as compressible because the JSON handlers here set it
// immediately after the first write; text/event-stream is the one streaming type
// that must never be buffered or encoded.
func compressibleAPIType(contentType string) bool {
	ct := strings.ToLower(strings.TrimSpace(contentType))
	if i := strings.IndexByte(ct, ';'); i >= 0 {
		ct = strings.TrimSpace(ct[:i])
	}
	switch ct {
	case "", "application/json", "application/xml", "application/javascript",
		"text/plain", "text/html", "text/css", "text/xml", "image/svg+xml":
		return true
	case "text/event-stream":
		return false
	}
	return strings.HasPrefix(ct, "text/")
}

func addVary(h http.Header, value string) {
	for _, existing := range h.Values("Vary") {
		for _, part := range strings.Split(existing, ",") {
			if strings.EqualFold(strings.TrimSpace(part), value) {
				return
			}
		}
	}
	h.Add("Vary", value)
}
