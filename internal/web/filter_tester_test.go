package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFilterTestReportsActionAndPattern(t *testing.T) {
	ws, _, cleanup := setupTestWebServer(t)
	defer cleanup()

	// One exact block and one wildcard block exercise the two shapes the tester
	// has to distinguish for an operator.
	ws.matcher.SetCustomRules(nil, []string{"ads.example.com", "*.tracker.example"}, nil, nil)
	handler := ws.buildAdminHandler()

	get := func(domain string) (int, RuleTestResult) {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/api/filter/test?domain="+domain, nil)
		req.Header.Set("Authorization", "Bearer "+ws.sessions.Create())
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		var out RuleTestResult
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		return w.Code, out
	}

	code, exact := get("ads.example.com")
	if code != http.StatusOK {
		t.Fatalf("exact lookup = %d, want 200", code)
	}
	if exact.Action != "BLOCK" || exact.Pattern != "ads.example.com" || exact.Rule != "Custom Block" {
		t.Fatalf("exact result = %+v, want BLOCK / Custom Block / exact host", exact)
	}

	code, wildcard := get("pixel.tracker.example")
	if code != http.StatusOK || wildcard.Action != "BLOCK" || wildcard.Pattern != "*.tracker.example" {
		t.Fatalf("wildcard result = %d %+v, want BLOCK matched by *.tracker.example", code, wildcard)
	}

	code, direct := get("example.org")
	if code != http.StatusOK || direct.Action != "DIRECT" || direct.Pattern != "" {
		t.Fatalf("unmatched result = %d %+v, want DIRECT with no pattern", code, direct)
	}
}

func TestFilterTestRejectsEmptyDomain(t *testing.T) {
	ws, _, cleanup := setupTestWebServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/filter/test", nil)
	req.Header.Set("Authorization", "Bearer "+ws.sessions.Create())
	w := httptest.NewRecorder()
	ws.buildAdminHandler().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("empty domain = %d, want 400", w.Code)
	}
}
