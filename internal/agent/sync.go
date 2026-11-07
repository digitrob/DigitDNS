package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// Errs returned by the sync client. The caller distinguishes "the hub said no"
// from "the hub is unreachable", because the first needs an operator and the
// second needs patience.
var (
	ErrHubRejected = fmt.Errorf("hub rejected the credential")
	ErrHubNoConfig = fmt.Errorf("hub returned no catalogue")
)

// Hub is the control-plane client. It deliberately keeps no timer of its own:
// the caller decides when to sync, so a test can drive it directly.
type Hub struct {
	BaseURL string
	HTTP    *http.Client
}

// NewHub builds a client with a bounded timeout. The timeout matters more than
// it looks: the agent must not hang behind an unresponsive control plane while
// it already has a cached catalogue it could be serving from.
func NewHub(baseURL string) *Hub {
	return &Hub{
		BaseURL: baseURL,
		HTTP:    &http.Client{Timeout: 15 * time.Second},
	}
}

// Enrolment is what the hub returns once, at first contact.
type Enrolment struct {
	AgentID string `json:"-"`
	Key     string `json:"key"`
	Country string `json:"-"`
}

// Enrol exchanges a one-time token for a long-lived key.
func (h *Hub) Enrol(token, version string) (Enrolment, error) {
	var out struct {
		Key   string `json:"key"`
		Agent struct {
			ID      string `json:"id"`
			Country string `json:"country"`
		} `json:"agent"`
	}
	body := map[string]string{"token": token, "version": version}
	if err := h.post("/agent/enrol", "", body, &out); err != nil {
		return Enrolment{}, err
	}
	if out.Key == "" {
		return Enrolment{}, ErrHubRejected
	}
	return Enrolment{AgentID: out.Agent.ID, Key: out.Key, Country: out.Agent.Country}, nil
}

// Catalogue is the allowlist pair the hub computes for this agent.
type Catalogue struct {
	Country string   `json:"country"`
	Hosts   []string `json:"hosts"`
	Clients []string `json:"clients"`
	Synced  string   `json:"synced_at"`
	// Pointer so an older hub that does not send the field cannot force an
	// upgraded agent to disable IPv6 merely by omitting it.
	IPv6Enabled *bool `json:"ipv6_enabled"`
}

// HeartbeatState is the hub's reply to a heartbeat: the effective IPv6 policy
// for this agent, plus the current config revision so an agent whose long-poll
// dropped can notice it is behind without waiting for the next full sync.
type HeartbeatState struct {
	IPv6Enabled    *bool  `json:"ipv6_enabled"`
	ConfigRevision string `json:"config_revision"`
}

// Fetch pulls the allowlists. A 401 is reported as ErrHubRejected so the caller
// can stop retrying with a key that will never work. It is the unconditional
// form used by tests and old callers; the agent loop uses FetchConditional.
func (h *Hub) Fetch(key string) (Catalogue, error) {
	cat, _, _, err := h.FetchConditional(context.Background(), key, "", 0)
	return cat, err
}

// FetchConditional performs a catalogue request with an ETag and an optional
// long-poll wait. It returns the catalogue, the response ETag, and whether the
// hub answered 304 (in which case the caller's cached copy is still current).
//
// A wait longer than zero needs its own client timeout: the shared client's 15 s
// deadline is shorter than the poll itself.
func (h *Hub) FetchConditional(ctx context.Context, key, etag string, wait time.Duration) (Catalogue, string, bool, error) {
	path := "/agent/catalogue"
	if wait > 0 {
		path += fmt.Sprintf("?wait=%d", int(wait/time.Second))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.BaseURL+path, nil)
	if err != nil {
		return Catalogue{}, "", false, err
	}
	req.Header.Set("X-Agent-Key", key)
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}

	client := h.HTTP
	if wait > 0 {
		clone := *h.HTTP
		clone.Timeout = wait + 10*time.Second
		client = &clone
	}
	resp, err := client.Do(req)
	if err != nil {
		return Catalogue{}, "", false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified {
		_, _ = io.Copy(io.Discard, resp.Body)
		if next := resp.Header.Get("ETag"); next != "" {
			etag = next
		}
		return Catalogue{}, etag, true, nil
	}
	if resp.StatusCode == http.StatusUnauthorized {
		_, _ = io.Copy(io.Discard, resp.Body)
		return Catalogue{}, "", false, ErrHubRejected
	}
	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return Catalogue{}, "", false, fmt.Errorf("hub %s: %s", req.URL.Path, bytes.TrimSpace(msg))
	}
	var out Catalogue
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return Catalogue{}, "", false, err
	}
	if out.Hosts == nil {
		out.Hosts = []string{}
	}
	if out.Clients == nil {
		out.Clients = []string{}
	}
	return out, resp.Header.Get("ETag"), false, nil
}

// Heartbeat reports liveness, traffic totals and the agent's public IPv6, and
// returns the hub's effective IPv6 policy. An empty publicIPv6 tells the hub
// nothing new; it must not erase an operator's manual value.
func (h *Hub) Heartbeat(key, version string, relayPort int, in, out uint64, publicIPv6, catalogueRevision string) (HeartbeatState, error) {
	body := map[string]any{
		"version":            version,
		"relay_port":         relayPort,
		"bytes_in":           in,
		"bytes_out":          out,
		"public_ipv6":        publicIPv6,
		"catalogue_revision": catalogueRevision,
	}
	var state HeartbeatState
	if err := h.post("/agent/heartbeat", key, body, &state); err != nil {
		return HeartbeatState{}, err
	}
	return state, nil
}

func (h *Hub) get(path, key string, out any) error {
	req, err := http.NewRequest(http.MethodGet, h.BaseURL+path, nil)
	if err != nil {
		return err
	}
	return h.do(req, key, out)
}

func (h *Hub) post(path, key string, body, out any) error {
	encoded, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, h.BaseURL+path, bytes.NewReader(encoded))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return h.do(req, key, out)
}

func (h *Hub) do(req *http.Request, key string, out any) error {
	if key != "" {
		req.Header.Set("X-Agent-Key", key)
	}
	resp, err := h.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		_, _ = io.Copy(io.Discard, resp.Body)
		return ErrHubRejected
	}
	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("hub %s: %s", req.URL.Path, bytes.TrimSpace(msg))
	}
	if out == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// State is the on-disk identity. It exists so a restart does not need the
// enrolment token again - which by then is spent - and so the cached catalogue
// survives a hub outage. Losing this file costs a re-enrol, nothing more.
type State struct {
	AgentID  string    `json:"agent_id"`
	Key      string    `json:"key"`
	Country  string    `json:"country"`
	Hosts    []string  `json:"hosts"`
	Clients  []string  `json:"clients"`
	SyncedAt time.Time `json:"synced_at"`
	// CatalogueETag is the hub revision this catalogue was fetched at. A restart
	// can revalidate instead of downloading the same allowlists again.
	CatalogueETag string `json:"catalogue_etag,omitempty"`
	// IPv6Enabled caches the last effective hub policy. A hub outage must not
	// make an agent that the operator switched off start listening on IPv6
	// again; a zero value from an old state file therefore means IPv4-only.
	IPv6Enabled bool `json:"ipv6_enabled"`
}

// LoadState reads the cached identity, returning a zero State when the file is
// absent. An absent file is not an error: it is a first run.
func LoadState(path string) (State, error) {
	var st State
	if path == "" {
		return st, nil
	}
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return st, nil
	}
	if err != nil {
		return st, err
	}
	if err := json.Unmarshal(raw, &st); err != nil {
		return State{}, fmt.Errorf("state file %s is not valid JSON: %w", path, err)
	}
	return st, nil
}

// SaveState writes the identity atomically, so a crash mid-write cannot leave a
// truncated file that would force a re-enrol.
func SaveState(path string, st State) error {
	if path == "" {
		return nil
	}
	encoded, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(encoded, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
