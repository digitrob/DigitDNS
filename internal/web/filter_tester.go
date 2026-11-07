package web

import (
	"encoding/json"
	"net/http"
	"strings"

	"digitdns/internal/httpx"
)

// RuleTestResult is the answer to "what would happen to this name?".
//
// It reports the same three values the query log records: the action, the rule
// label that decided it, and the pattern that matched (the exact host or
// "*.base"). That is enough to tell an exact rule from a suffix rule without
// reading the compiled index, and it is the difference between an operator
// fixing a rule in a minute and opening a support thread.
type RuleTestResult struct {
	Domain  string `json:"domain"`
	Action  string `json:"action"`
	Rule    string `json:"rule"`
	Pattern string `json:"pattern"`
}

// handleFilterTest answers GET /api/filter/test?domain=… from the live matcher.
// It is read-only: no rule is added, removed or recompiled.
func (ws *WebServer) handleFilterTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		httpx.WriteMethodNotAllowed(w, "GET")
		return
	}
	if ws.matcher == nil {
		httpx.WriteJSONError(w, http.StatusServiceUnavailable, "the filter matcher is unavailable")
		return
	}
	domain := strings.TrimSpace(r.URL.Query().Get("domain"))
	if domain == "" {
		httpx.WriteJSONError(w, http.StatusBadRequest, "domain is required")
		return
	}
	// A DNS name cannot exceed 253 characters in presentation form. Refusing an
	// oversized value here keeps the matcher's label walk bounded by the same
	// limit the resolver enforces.
	if len(domain) > 253 {
		httpx.WriteJSONError(w, http.StatusBadRequest, "domain is longer than 253 characters")
		return
	}

	action, rule, pattern := ws.matcher.MatchDetail(domain)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(RuleTestResult{
		Domain:  domain,
		Action:  action.String(),
		Rule:    rule,
		Pattern: pattern,
	})
}
