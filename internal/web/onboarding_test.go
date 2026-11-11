package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOnboardingReportsLiveChecklist(t *testing.T) {
	ws, _, cleanup := setupTestWebServer(t)
	defer cleanup()
	h := ws.buildAdminHandler()
	tok := bearerFor(t, h)

	req := httptest.NewRequest(http.MethodGet, "/api/onboarding", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /api/onboarding = %d: %s", w.Code, w.Body.String())
	}
	var body struct {
		Steps []struct {
			ID   string `json:"id"`
			Done bool   `json:"done"`
		} `json:"steps"`
		Total   int  `json:"total"`
		AllDone bool `json:"all_done"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode onboarding: %v", err)
	}
	if body.Total != len(body.Steps) || body.Total == 0 {
		t.Fatalf("total=%d steps=%d", body.Total, len(body.Steps))
	}
	if body.AllDone {
		t.Error("a fresh test install should not report the checklist complete")
	}
	seen := map[string]bool{}
	for _, step := range body.Steps {
		if step.ID == "" {
			t.Error("a checklist step has no id")
		}
		seen[step.ID] = true
	}
	for _, want := range []string{"password", "2fa", "tls", "dns", "upstreams", "client"} {
		if !seen[want] {
			t.Errorf("checklist is missing step %q", want)
		}
	}
}
