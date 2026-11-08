package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The scrape surface has to be reachable by a Prometheus bearer token (the
// master REST key) and by a live dashboard session, and by nothing else.
func TestMetricsRequiresAuthAndExposesPrometheusText(t *testing.T) {
	ws, _, cleanup := setupTestWebServer(t)
	defer cleanup()

	handler := ws.buildAdminHandler()

	unauth := httptest.NewRecorder()
	handler.ServeHTTP(unauth, httptest.NewRequest(http.MethodGet, "/api/metrics", nil))
	if unauth.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated /api/metrics = %d, want 401", unauth.Code)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/metrics", nil)
	req.Header.Set("Authorization", "Bearer "+ws.settings.GetAPIKey())
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("/api/metrics = %d, want 200 (body %q)", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain; version=0.0.4") {
		t.Errorf("Content-Type = %q, want the Prometheus text format", ct)
	}
	body := w.Body.String()
	for _, want := range []string{
		"# TYPE digitdns_build_info gauge",
		"digitdns_build_info{",
		"# TYPE digitdns_queries_total counter",
		"digitdns_cache_hit_ratio",
		"# TYPE digitdns_latency_ms summary",
		"digitdns_latency_ms{quantile=\"0.99\"",
		"digitdns_agent_catalogue_requests_total",
		"digitdns_agent_catalogue_builds_total",
		"digitdns_agents_online",
		"digitdns_agent_revision",
		"digitdns_relays_refused_actionable_total",
		"digitdns_relays_flood_total",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("metrics body is missing %q", want)
		}
	}
	// Cardinality guard: a scrape must not enumerate client addresses or names.
	for _, forbidden := range []string{"client_ip=", "domain=", "account="} {
		if strings.Contains(body, forbidden) {
			t.Errorf("metrics body carries unbounded label %q", forbidden)
		}
	}
}

func TestMetricsRejectsWrites(t *testing.T) {
	ws, _, cleanup := setupTestWebServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodPost, "/api/metrics", nil)
	req.Header.Set("Authorization", "Bearer "+ws.settings.GetAPIKey())
	w := httptest.NewRecorder()
	ws.buildAdminHandler().ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST /api/metrics = %d, want 405", w.Code)
	}
}
