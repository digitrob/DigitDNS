package web

import (
	"encoding/json"
	"net/http"
	"testing"

	"digitdns/internal/database"
)

func TestStreamRecentReturnsTheRing(t *testing.T) {
	ws, _, cleanup := setupTestWebServer(t)
	defer cleanup()

	ws.stats.PushQueryLog(database.QueryLogItem{
		Domain: "recent.example",
		Action: "DIRECT",
	})
	rec := doJSON(t, ws.handleStreamRecent, http.MethodGet, "/api/stream/recent", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
	var body struct {
		Queries []database.QueryLogItem `json:"queries"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Queries) != 1 || body.Queries[0].Domain != "recent.example" {
		t.Fatalf("queries = %+v", body.Queries)
	}
}
