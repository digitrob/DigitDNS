package web

import (
	"encoding/json"
	"net/http"
	"strings"

	"digitdns/internal/httpx"
)

// onboardingStep is one first-run item. Tab is the dashboard tab the button
// switches to, so the checklist is a set of actions rather than a scoreboard.
type onboardingStep struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Done        bool   `json:"done"`
	Tab         string `json:"tab"`
	Route       string `json:"route"`
}

// handleOnboarding answers GET /api/onboarding with the live first-run state.
// Every field is derived from the running configuration, not from a stored
// "onboarding done" flag, so the checklist cannot go stale after a restore.
func (ws *WebServer) handleOnboarding(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		httpx.WriteMethodNotAllowed(w, "GET, HEAD")
		return
	}
	transports := ws.transportSettingsView()
	upstreamCount := 0
	if ws.upstreams != nil {
		upstreamCount = len(ws.upstreams.GetUpstreams())
	}
	clientCount := 0
	if ws.clients != nil {
		if list, err := ws.clients.ListClients(); err == nil {
			clientCount = len(list)
		}
	}
	domain := ""
	if ws.tlsSettings != nil {
		domain = strings.TrimSpace(ws.tlsSettings.GetDomain())
	}
	twoFA := ws.authSettings != nil && ws.authSettings.TOTPEnabledNow()
	steps := []onboardingStep{
		{
			ID: "password", Title: "Replace the default admin password",
			Description: "The panel still reports the stored password as weak or default.",
			Done:        !ws.adminPasswordNeedsAttention(), Tab: "rules", Route: "/settings",
		},
		{
			ID: "2fa", Title: "Enable two-factor authentication",
			Description: "A TOTP app protects the panel even if the password leaks.",
			Done:        twoFA, Tab: "rules", Route: "/settings",
		},
		{
			ID: "tls", Title: "Set the panel domain and certificate",
			Description: "The dashboard and client transports should serve a trusted certificate.",
			Done:        domain != "", Tab: "rules", Route: "/settings",
		},
		{
			ID: "dns", Title: "Confirm a DNS listener is up",
			Description: "Plain DNS, DoT, DoH or DoQ must be listening before clients can resolve.",
			Done:        transports.PlainDNSListening || transports.DoTListening || transports.DoHListening,
			Tab:         "rules", Route: "/settings",
		},
		{
			ID: "upstreams", Title: "Add at least one upstream resolver",
			Description: "The resolver pool is empty, so nothing can be answered.",
			Done:        upstreamCount > 0, Tab: "rules", Route: "/settings",
		},
		{
			ID: "client", Title: "Create the first client",
			Description: "A client is how a device or subscriber gets a policy and a quota.",
			Done:        clientCount > 0, Tab: "clients", Route: "/clients",
		},
	}
	done := 0
	for _, s := range steps {
		if s.Done {
			done++
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"steps":     steps,
		"completed": done,
		"total":     len(steps),
		"all_done":  done == len(steps),
	})
}
