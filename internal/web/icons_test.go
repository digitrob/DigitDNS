package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// iconSearchServer points iconifyBase at a fake repository for the duration of
// one test. The cache is cleared too: tests in this package run sequentially,
// but a query string reused across tests must not leak a result.
func iconSearchServer(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(handler)
	old := iconifyBase
	iconifyBase = srv.URL
	clearIconSearchCache()
	t.Cleanup(func() {
		iconifyBase = old
		clearIconSearchCache()
		srv.Close()
	})
	return srv
}

func iconSearchGet(t *testing.T, query string) (int, []string) {
	t.Helper()
	rec := httptest.NewRecorder()
	(&WebServer{}).handleIconSearch(rec, httptest.NewRequest(http.MethodGet, "/api/icons/search?q="+query, nil))
	var body struct {
		Icons []string `json:"icons"`
	}
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode: %v (body %q)", err, rec.Body.String())
		}
	}
	return rec.Code, body.Icons
}

func TestIconSearchFiltersAndDedupes(t *testing.T) {
	iconSearchServer(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("prefixes"); got != "logos,flat-color-icons,twemoji,mdi,simple-icons" {
			t.Errorf("prefixes = %q", got)
		}
		switch r.URL.Query().Get("query") {
		case "tv":
			_, _ = w.Write([]byte(`{"icons":["mdi:tv","NOT AN ID","mdi:tv","../evil:x"]}`))
		case "television":
			_, _ = w.Write([]byte(`{"icons":["twemoji:television","simple-icons:netflix","logos:netflix","flat-color-icons:film"]}`))
		default:
			t.Errorf("unexpected upstream query %q", r.URL.Query().Get("query"))
		}
	})

	code, icons := iconSearchGet(t, "tv")
	if code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	// Colourful sets first (logos, flat colour, twemoji), then monochrome.
	want := []string{
		"logos:netflix",
		"flat-color-icons:film",
		"twemoji:television",
		"mdi:tv",
		"simple-icons:netflix",
	}
	if strings.Join(icons, ",") != strings.Join(want, ",") {
		t.Errorf("icons = %v, want %v", icons, want)
	}
}

func TestIconSearchShortQuerySkipsUpstream(t *testing.T) {
	var calls int32
	iconSearchServer(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		_, _ = w.Write([]byte(`{"icons":["mdi:tv"]}`))
	})

	code, icons := iconSearchGet(t, "t")
	if code != http.StatusOK || len(icons) != 0 {
		t.Fatalf("status=%d icons=%v", code, icons)
	}
	if atomic.LoadInt32(&calls) != 0 {
		t.Fatal("a one-character query reached the upstream")
	}
}

func TestIconSearchCachesPerQuery(t *testing.T) {
	var calls int32
	iconSearchServer(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		_, _ = w.Write([]byte(`{"icons":["mdi:tv"]}`))
	})

	iconSearchGet(t, "tv")
	iconSearchGet(t, "tv")
	// Two upstream terms on the first request (tv + television), none on the cached
	// second one.
	if got := atomic.LoadInt32(&calls); got != 2 {
		t.Fatalf("upstream calls = %d, want 2", got)
	}
}

func TestIconSearchReportsUpstreamFailure(t *testing.T) {
	iconSearchServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusInternalServerError)
	})

	if code, _ := iconSearchGet(t, "tv"); code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", code)
	}
}

func TestIconSearchRejectsWrites(t *testing.T) {
	rec := httptest.NewRecorder()
	(&WebServer{}).handleIconSearch(rec, httptest.NewRequest(http.MethodPost, "/api/icons/search?q=tv", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
}
