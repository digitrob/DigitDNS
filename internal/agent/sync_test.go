package agent

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func TestHubEnrolFetchHeartbeat(t *testing.T) {
	var gotKey string
	mux := http.NewServeMux()

	mux.HandleFunc("/agent/enrol", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["token"] != "agt_good" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"bad token"}`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"key":   "id01:secret",
			"agent": map[string]any{"id": "id01", "country": "DE"},
		})
	})
	mux.HandleFunc("/agent/catalogue", func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("X-Agent-Key")
		if gotKey != "id01:secret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"country":      "DE",
			"hosts":        []string{"api.ard.de"},
			"clients":      []string{"203.0.113.5"},
			"ipv6_enabled": true,
		})
	})
	mux.HandleFunc("/agent/heartbeat", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Agent-Key") != "id01:secret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if body["public_ipv6"] != "2001:db8::5" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ipv6_enabled": true})
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	hub := NewHub(srv.URL)

	if _, err := hub.Enrol("agt_bad", "1.0.0"); !errors.Is(err, ErrHubRejected) {
		t.Fatalf("bad token should be rejected, got %v", err)
	}

	enr, err := hub.Enrol("agt_good", "1.0.0")
	if err != nil {
		t.Fatalf("enrol: %v", err)
	}
	if enr.AgentID != "id01" || enr.Country != "DE" || enr.Key != "id01:secret" {
		t.Fatalf("unexpected enrolment: %+v", enr)
	}

	cat, err := hub.Fetch(enr.Key)
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if len(cat.Hosts) != 1 || cat.Hosts[0] != "api.ard.de" {
		t.Fatalf("unexpected hosts: %+v", cat.Hosts)
	}
	if gotKey != "id01:secret" {
		t.Fatalf("key was not sent: %q", gotKey)
	}

	hb, err := hub.Heartbeat(enr.Key, "1.0.0", 443, 1, 2, "2001:db8::5", "")
	if err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	if hb.IPv6Enabled == nil || !*hb.IPv6Enabled {
		t.Fatalf("heartbeat IPv6 policy = %v, want true", hb.IPv6Enabled)
	}
	if _, err := hub.Heartbeat("id01:wrong", "1.0.0", 443, 1, 2, "2001:db8::5", ""); !errors.Is(err, ErrHubRejected) {
		t.Fatalf("wrong key should be rejected, got %v", err)
	}

	if _, err := hub.Fetch("id01:wrong"); !errors.Is(err, ErrHubRejected) {
		t.Fatalf("wrong key should be rejected on fetch, got %v", err)
	}
}

func TestStateRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "agent.json")

	// A missing file is a first run, not an error.
	st, err := LoadState(path)
	if err != nil {
		t.Fatalf("missing state should not error: %v", err)
	}
	if st.Key != "" {
		t.Fatal("missing state should be empty")
	}

	want := State{AgentID: "id01", Key: "id01:secret", Country: "DE", Hosts: []string{"a.example"}, IPv6Enabled: true}
	if err := SaveState(path, want); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := LoadState(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.Key != want.Key || got.Country != want.Country || len(got.Hosts) != 1 || got.IPv6Enabled != want.IPv6Enabled {
		t.Fatalf("state did not round-trip: %+v", got)
	}
}

func TestFetchConditionalUsesETagAndWait(t *testing.T) {
	var waitSeen string
	mux := http.NewServeMux()
	mux.HandleFunc("/agent/catalogue", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Agent-Key") != "id01:secret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		waitSeen = r.URL.Query().Get("wait")
		if r.Header.Get("If-None-Match") == `"rev1"` {
			w.Header().Set("ETag", `"rev1"`)
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"rev1"`)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"country": "DE",
			"hosts":   []string{"api.ard.de"},
			"clients": []string{"any"},
		})
	})
	mux.HandleFunc("/agent/heartbeat", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"ipv6_enabled":    true,
			"config_revision": `"rev1"`,
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	hub := NewHub(srv.URL)

	cat, etag, notModified, err := hub.FetchConditional(context.Background(), "id01:secret", "", 0)
	if err != nil || notModified || etag != `"rev1"` || len(cat.Hosts) != 1 {
		t.Fatalf("initial fetch = %+v, etag=%q, notModified=%v, err=%v", cat, etag, notModified, err)
	}

	_, etag, notModified, err = hub.FetchConditional(context.Background(), "id01:secret", etag, time.Second)
	if err != nil || !notModified || etag != `"rev1"` {
		t.Fatalf("conditional fetch = etag=%q, notModified=%v, err=%v", etag, notModified, err)
	}
	if waitSeen != "1" {
		t.Fatalf("wait query = %q, want 1", waitSeen)
	}

	hb, err := hub.Heartbeat("id01:secret", "1.0.0", 443, 0, 0, "", etag)
	if err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	if hb.ConfigRevision != `"rev1"` {
		t.Fatalf("config revision = %q", hb.ConfigRevision)
	}
}
