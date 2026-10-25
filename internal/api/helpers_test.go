package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"digitdns/internal/core/cache"
	"digitdns/internal/core/matcher"
	"digitdns/internal/core/upstream"
	"digitdns/internal/crypto"
	"digitdns/internal/database"
	"digitdns/internal/service"
)

func setupTestAPI(t *testing.T) (*API, *database.DB, *database.ServerSettings, func()) {
	// Per-test directory rather than fixed names in the package directory. Every test
	// in this package calls this helper, so shared filenames meant each run reused one
	// database file: a test that crashed before its cleanup left test_api.db next to the
	// source, and the next run opened that leftover instead of a fresh database. It is
	// also what a future t.Parallel() would deadlock on, since bbolt takes an exclusive
	// flock on the file. t.TempDir() removes the directory after the test's own cleanups
	// have run, so the handles below are closed first — which is what Windows requires.
	dir := t.TempDir()
	tmpDB := filepath.Join(dir, "api.db")
	tmpKey := filepath.Join(dir, "api.key")

	cipher, err := crypto.LoadOrGenerateMasterKey(tmpKey)
	if err != nil {
		t.Fatalf("failed to create cipher: %v", err)
	}

	db, err := database.Open(tmpDB, cipher)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}

	c := cache.NewCache(1000, 60, 3600)
	m := matcher.NewMatcher()
	u := upstream.NewUpstreamPool([]string{"1.1.1.1:53"}, 2*time.Second, true, "")

	clients := service.NewClientService(db, true)
	stats := service.NewStatsService(db, nil, nil)

	settings := &database.ServerSettings{
		PublicIP:      "127.0.0.1",
		BindHost:      "127.0.0.1",
		WebPort:       8080,
		AdminUsername: "admin",
		// The REST API authenticates with APIKey only; this field is here to make
		// the record realistic, never to be verified against.
		AdminPassword: "unused-by-the-rest-api",
		APIKey:        "digitdns_live_testkey123",
		APIBind:       "127.0.0.1",
	}
	tlsSettings := &database.TLSSettings{}

	apiInst := NewAPI(db, clients, stats, c, m, u, settings, tlsSettings, nil)

	cleanup := func() {
		// Both of these run a ticker goroutine that outlives the test unless it is
		// stopped, and this helper is called once per test in the package. The files
		// themselves are t.TempDir()'s to remove.
		stats.Close()
		c.Close()
		_ = db.Close()
	}

	return apiInst, db, settings, cleanup
}

// setAPIBind rewrites the exposure rule the way the Settings page does.
func setAPIBind(t *testing.T, settings *database.ServerSettings, bind string) {
	t.Helper()
	if err := settings.UpdateAndPersist(func(m *database.MutableSettings) { m.APIBind = bind }, nil); err != nil {
		t.Fatalf("could not set APIBind to %q: %v", bind, err)
	}
}

const testKey = "digitdns_live_testkey123"

// authedRequest builds a loopback request carrying the test master key.
func authedRequest(method, path string, body []byte) *http.Request {
	var req *http.Request
	if body == nil {
		req = httptest.NewRequest(method, path, nil)
	} else {
		req = httptest.NewRequest(method, path, bytes.NewReader(body))
	}
	req.Header.Set("X-API-Key", testKey)
	req.RemoteAddr = "127.0.0.1:12345"
	return req
}
