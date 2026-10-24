package geoip

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// fixture is a 1.5 KB MaxMind database generated for these tests: 203.0.113.0/24
// is NL, 198.51.100.0/24 is US, 2001:db8:beef::/48 is DE, build date 22.10.2026.
const fixture = "testdata/country.mmdb"

func loadFixture(t *testing.T, dir string) *Service {
	t.Helper()
	body, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, FileName), body, 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	svc := New(dir, "")
	if err := svc.Load(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	return svc
}

func TestCountryLookup(t *testing.T) {
	svc := loadFixture(t, t.TempDir())

	cases := map[string]string{
		"203.0.113.5":      "NL",
		"198.51.100.200":   "US",
		"2001:db8:beef::1": "DE",
		"8.8.8.8":          "", // not in the fixture
	}
	for addr, want := range cases {
		if got := svc.Country(net.ParseIP(addr)); got != want {
			t.Errorf("Country(%s) = %q, want %q", addr, got, want)
		}
	}
	if got := svc.Country(nil); got != "" {
		t.Errorf("Country(nil) = %q, want empty", got)
	}
}

func TestStatusReportsLoadedDatabase(t *testing.T) {
	svc := loadFixture(t, t.TempDir())

	status := svc.Status()
	if !status.Available {
		t.Fatal("Status.Available = false for a loaded database")
	}
	if status.DatabaseDate != "22.10.2026" {
		t.Errorf("DatabaseDate = %q, want 22.10.2026", status.DatabaseDate)
	}
	if status.UpdatedAt == "" {
		t.Error("UpdatedAt is empty for a loaded database")
	}
	if status.Path == "" {
		t.Error("Path is empty")
	}
}

func TestMissingDatabaseIsNotAnError(t *testing.T) {
	svc := New(t.TempDir(), "")
	if err := svc.Load(); err != nil {
		t.Fatalf("Load with no file: %v", err)
	}
	if status := svc.Status(); status.Available {
		t.Error("Available = true with no file")
	}
	if got := svc.Country(net.ParseIP("203.0.113.5")); got != "" {
		t.Errorf("Country without a database = %q, want empty", got)
	}
}

func TestUpdateDownloadsAndSwaps(t *testing.T) {
	body, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	const etag = `"fixture-v1"`
	var conditionalHits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") == etag {
			conditionalHits++
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", etag)
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	dir := t.TempDir()
	svc := New(dir, srv.URL)

	if err := svc.Update(context.Background()); err != nil {
		t.Fatalf("first Update: %v", err)
	}
	if got := svc.Country(net.ParseIP("203.0.113.5")); got != "NL" {
		t.Fatalf("Country after update = %q, want NL", got)
	}
	if _, err := os.Stat(filepath.Join(dir, FileName)); err != nil {
		t.Fatalf("database was not written: %v", err)
	}
	if got := svc.Status().DatabaseDate; got != "22.10.2026" {
		t.Errorf("DatabaseDate after update = %q", got)
	}

	// The next call carries the ETag and the server answers 304; nothing changes.
	if err := svc.Update(context.Background()); err != nil {
		t.Fatalf("second Update: %v", err)
	}
	if conditionalHits != 1 {
		t.Errorf("conditional requests = %d, want 1", conditionalHits)
	}
	if got := svc.Country(net.ParseIP("203.0.113.5")); got != "NL" {
		t.Errorf("Country after 304 = %q, want NL", got)
	}
}

func TestUpdateRejectsGarbageAndKeepsOldDatabase(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html><body>captive portal</body></html>"))
	}))
	defer srv.Close()

	dir := t.TempDir()
	svc := loadFixture(t, dir)
	svc.source = srv.URL

	if err := svc.Update(context.Background()); err == nil {
		t.Fatal("Update accepted an HTML page as a database")
	}
	if got := svc.Country(net.ParseIP("203.0.113.5")); got != "NL" {
		t.Errorf("Country after a failed update = %q, want the previous NL", got)
	}
	if status := svc.Status(); status.Error == "" {
		t.Error("Status.Error is empty after a failed update")
	}
}

func TestUpdateWithoutSourceFails(t *testing.T) {
	svc := New(t.TempDir(), "")
	if err := svc.Update(context.Background()); err == nil {
		t.Fatal("Update with no source succeeded")
	}
}

// The reader is swapped while lookups run, which is what the daily updater does
// in production. Nothing may panic and every answer must be one of the two
// databases.
func TestConcurrentLookupDuringSwap(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := os.ReadFile(fixture)
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	svc := loadFixture(t, t.TempDir())
	svc.source = srv.URL
	ip := net.ParseIP("203.0.113.5")

	var wg sync.WaitGroup
	stop := make(chan struct{})
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					if got := svc.Country(ip); got != "NL" && got != "" {
						t.Errorf("Country during swap = %q", got)
						return
					}
				}
			}
		}()
	}
	for i := 0; i < 5; i++ {
		if err := svc.Update(context.Background()); err != nil {
			t.Errorf("Update %d: %v", i, err)
		}
	}
	close(stop)
	wg.Wait()
}

func TestStale(t *testing.T) {
	svc := New(t.TempDir(), "x")
	if !svc.stale() {
		t.Error("a service with no database should be stale")
	}
	svc = loadFixture(t, t.TempDir())
	// The fixture file was written now, so it is fresh.
	if svc.stale() {
		t.Error("a just-written database should not be stale")
	}
	svc.updatedAt.Store(time.Now().Add(-48 * time.Hour).Unix())
	if !svc.stale() {
		t.Error("a two-day-old database should be stale")
	}
}

func TestStatusErrorIsClearedBySuccess(t *testing.T) {
	svc := New(t.TempDir(), "http://127.0.0.1:1/unreachable")
	if err := svc.Update(context.Background()); err == nil {
		t.Fatal("Update to a closed port succeeded")
	}
	if !strings.Contains(svc.Status().Error, "download") {
		t.Errorf("Status.Error = %q, want the download failure", svc.Status().Error)
	}
}
