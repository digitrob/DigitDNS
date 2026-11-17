package web

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
)

// The Force response must carry the post-benchmark resolver numbers. The panel
// used to repaint from its previous /api/stats payload, which is cached for a
// second, so the row the server had just selected kept showing the latency it
// had before the benchmark — and the new choice could look slower than the old
// one. A plain GET keeps the smaller payload.
func TestAdaptiveRunResponseCarriesUpstreamStats(t *testing.T) {
	ws, _, cleanup := setupTestWebServer(t)
	defer cleanup()

	read := func(withStats bool) resolutionView {
		t.Helper()
		rec := httptest.NewRecorder()
		ws.writeResolution(rec, withStats)
		var view resolutionView
		if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil {
			t.Fatalf("decode resolution view: %v", err)
		}
		return view
	}

	if got := read(false); len(got.UpstreamStats) != 0 {
		t.Fatalf("settings read carried %d upstream stats, want none", len(got.UpstreamStats))
	}
	view := read(true)
	if len(view.UpstreamStats) != 1 {
		t.Fatalf("force response carried %d upstream stats, want 1", len(view.UpstreamStats))
	}
	if view.UpstreamStats[0].Address != "1.1.1.1:53" {
		t.Fatalf("force response stats = %+v, want the configured resolver", view.UpstreamStats)
	}
}
