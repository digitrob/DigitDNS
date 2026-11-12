package web

import (
	"encoding/json"
	"net/http"
	"strings"

	"digitdns/internal/httpx"
)

// explainTarget is the relay decision for a proxied name: where it would exit
// and whether that exit is currently usable.
type explainTarget struct {
	Kind           string `json:"kind"`
	AgentID        string `json:"agent_id,omitempty"`
	Name           string `json:"name,omitempty"`
	Country        string `json:"country,omitempty"`
	EgressID       string `json:"egress_id,omitempty"`
	EgressKind     string `json:"egress_kind,omitempty"`
	Healthy        bool   `json:"healthy"`
	FailClosed     bool   `json:"fail_closed,omitempty"`
	FallbackReason string `json:"fallback_reason,omitempty"`
}

type explainResponse struct {
	Domain  string `json:"domain"`
	Client  string `json:"client_ip,omitempty"`
	Account string `json:"account,omitempty"`
	Engine  bool   `json:"engine"`

	Action  string `json:"action"`
	Rule    string `json:"rule"`
	Pattern string `json:"pattern,omitempty"`
	Source  string `json:"source,omitempty"`

	BlocklistSource   string `json:"blocklist_source,omitempty"`
	BlocklistPattern  string `json:"blocklist_pattern,omitempty"`
	AllowSource       string `json:"allow_source,omitempty"`
	AllowPattern      string `json:"allow_pattern,omitempty"`
	SuppressedSource  string `json:"suppressed_source,omitempty"`
	SuppressedPattern string `json:"suppressed_pattern,omitempty"`
	CustomRecord      string `json:"custom_record,omitempty"`

	Target *explainTarget `json:"target,omitempty"`
}

// handleExplain is the support tool behind "why did this name do that?".
// It follows the resolver's precedence and adds the detail the query log drops:
// the blocklist source, an allowlist exception that suppressed it, and the relay
// target that a proxied name would use for this client.
func (ws *WebServer) handleExplain(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		httpx.WriteMethodNotAllowed(w, "GET, HEAD")
		return
	}
	domain := strings.TrimSpace(r.URL.Query().Get("domain"))
	if domain == "" {
		httpx.WriteJSONError(w, http.StatusBadRequest, "domain is required")
		return
	}
	out := ws.explain(domain, strings.TrimSpace(r.URL.Query().Get("client")))
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// Explain exposes the decision to the v3 adapter.
func (ws *WebServer) Explain(domain, clientIP string) (map[string]any, error) {
	data, err := json.Marshal(ws.explain(domain, clientIP))
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// explain is the shared decision builder used by the dashboard handler and the
// v3 API, so Android and the panel can never disagree about a name.
func (ws *WebServer) explain(domain, clientIP string) explainResponse {
	out := explainResponse{Domain: domain, Client: clientIP, Engine: true}
	if ws.engineState != nil {
		out.Engine = ws.engineState()
	}
	if !out.Engine {
		out.Action = "DIRECT"
		out.Rule = "Engine off"
		out.Source = "engine"
	} else if ws.matcher != nil {
		exp := ws.matcher.Explain(domain)
		out.Action = exp.Action
		out.Rule = exp.Rule
		out.Pattern = exp.Pattern
		out.Source = exp.Source
		out.BlocklistSource = exp.BlocklistSource
		out.BlocklistPattern = exp.BlocklistPattern
		out.AllowSource = exp.AllowSource
		out.AllowPattern = exp.AllowPattern
		out.SuppressedSource = exp.SuppressedSource
		out.SuppressedPattern = exp.SuppressedPattern
		out.CustomRecord = exp.CustomRecord
	}

	if clientIP != "" && ws.clients != nil {
		if client, ok := ws.clients.IsIPAllowed(clientIP); ok && client != nil {
			out.Account = client.Name
		}
	}
	if ws.relayRouter != nil {
		target, _ := ws.relayRouter.LookupFor(domain, clientIP)
		out.Target = &explainTarget{
			Kind: target.Kind, AgentID: target.AgentID, Name: target.Name,
			Country: target.Country, EgressID: target.EgressID,
			EgressKind: target.EgressKind, Healthy: target.Healthy,
			FailClosed: target.FailClosed, FallbackReason: target.FallbackReason,
		}
	}

	return out
}
