package web

import (
	"bytes"
	"testing"
)

// The TCP healthcheck connects and closes without a ClientHello, so an HTTPS
// panel logs one handshake error per probe from loopback. That line is our own
// liveness check, not a failure; a remote scanner's handshake error must still
// reach the log.
func TestQuietLoopbackTLSWriterDropsProbeNoise(t *testing.T) {
	var out bytes.Buffer
	w := quietLoopbackTLSWriter{next: &out}

	for _, probe := range [][]byte{
		[]byte("2026/11/16 08:00:00 http: TLS handshake error from 127.0.0.1:54321: EOF\n"),
		[]byte("2026/11/16 08:00:00 http: TLS handshake error from [::1]:54321: EOF\n"),
	} {
		if n, err := w.Write(probe); err != nil || n != len(probe) {
			t.Fatalf("Write(loopback probe) = (%d, %v), want (%d, nil)", n, err, len(probe))
		}
	}
	if out.Len() != 0 {
		t.Fatalf("loopback probe noise reached the log: %q", out.String())
	}

	remote := []byte("2026/11/16 08:00:00 http: TLS handshake error from 203.0.113.9:1234: EOF\n")
	if n, err := w.Write(remote); err != nil || n != len(remote) {
		t.Fatalf("Write(remote) = (%d, %v), want (%d, nil)", n, err, len(remote))
	}
	if !bytes.Equal(out.Bytes(), remote) {
		t.Fatalf("remote handshake error was filtered: %q", out.String())
	}
}
