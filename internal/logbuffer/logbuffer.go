// Package logbuffer keeps a bounded, in-process tail of the daemon's log so the
// dashboard and Android app can show it without shelling out to journalctl or
// docker. It is a diagnostic convenience, not durable storage: the authoritative
// log remains stderr/journald.
package logbuffer

import (
	"strings"
	"sync"
)

// Buffer is a concurrency-safe ring of complete log lines. Writes never block on
// readers and never grow without bound.
type Buffer struct {
	mu       sync.Mutex
	lines    []string
	pending  string
	maxLines int
	maxBytes int
	bytes    int
}

// New returns a buffer capped by both line count and approximate byte size. The
// byte cap is checked by dropping the oldest lines, so one pathological line
// cannot evict the whole history.
func New(maxLines, maxBytes int) *Buffer {
	if maxLines <= 0 {
		maxLines = 2000
	}
	if maxBytes <= 0 {
		maxBytes = 1 << 20
	}
	return &Buffer{maxLines: maxLines, maxBytes: maxBytes}
}

// Write implements io.Writer. Standard log.Logger emits one record per Write,
// but the pending field also handles a writer that splits a line across calls.
func (b *Buffer) Write(p []byte) (int, error) {
	if b == nil {
		return len(p), nil
	}
	text := b.pending + string(p)
	b.pending = ""
	parts := strings.Split(text, "\n")
	if !strings.HasSuffix(text, "\n") {
		b.pending = parts[len(parts)-1]
		parts = parts[:len(parts)-1]
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, line := range parts {
		b.appendLocked(strings.TrimSuffix(line, "\r"))
	}
	return len(p), nil
}

func (b *Buffer) appendLocked(line string) {
	// A single multi-megabyte line is more likely a bug than useful history.
	const maxLine = 8 << 10
	if len(line) > maxLine {
		line = line[:maxLine] + "…"
	}
	if line == "" {
		return
	}
	b.lines = append(b.lines, line)
	b.bytes += len(line) + 1
	for len(b.lines) > b.maxLines || b.bytes > b.maxBytes {
		b.bytes -= len(b.lines[0]) + 1
		b.lines = b.lines[1:]
	}
}

// Lines returns up to limit of the newest lines in chronological order. A
// non-positive limit returns everything the buffer holds.
func (b *Buffer) Lines(limit int) []string {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	start := 0
	if limit > 0 && len(b.lines) > limit {
		start = len(b.lines) - limit
	}
	out := make([]string, len(b.lines)-start)
	copy(out, b.lines[start:])
	return out
}

// String returns the whole buffer as one text block, for copy/export.
func (b *Buffer) String() string {
	return strings.Join(b.Lines(0), "\n")
}
