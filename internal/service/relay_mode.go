package service

import (
	"context"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"digitdns/internal/agent"
	"digitdns/internal/core/proxy"
	"digitdns/internal/database"
	"digitdns/internal/version"
)

// RelayModeManager lets a full DigitDNS instance act as a relay agent for one
// or more hubs. Each hub keeps its own credential, catalogue, counters and
// worker; the proxy selects the hub by the source client IP, so one hub's
// clients never inherit another hub's host allowlist.
type RelayModeManager struct {
	db               *database.DB
	proxy            *proxy.Server
	relayPort        int
	publicIPv6       func() string
	localHostAllowed func(string) bool

	mu            sync.Mutex
	globalEnabled bool
	hubs          map[string]*relayHub
	order         []string
	policy        *multiRelayPolicy
	cancel        context.CancelFunc
	wg            sync.WaitGroup
}

type relayHub struct {
	settings      database.RelayHubSettings
	online        bool
	lastHeartbeat time.Time
	lastError     string
	policy        *relayModePolicy
	cancel        context.CancelFunc
}

// RelayModeView is the safe, display-only shape the panel receives. The agent
// key is never included.
type RelayModeView struct {
	ID            string    `json:"id"`
	Name          string    `json:"name,omitempty"`
	Enabled       bool      `json:"enabled"`
	HubURL        string    `json:"hub_url"`
	AgentID       string    `json:"agent_id,omitempty"`
	AgentKeySet   bool      `json:"agent_key_set"`
	Country       string    `json:"country,omitempty"`
	IPv6Enabled   bool      `json:"ipv6_enabled"`
	Hosts         int       `json:"hosts"`
	Clients       int       `json:"clients"`
	Online        bool      `json:"online"`
	LastSync      time.Time `json:"last_sync,omitempty"`
	LastHeartbeat time.Time `json:"last_heartbeat,omitempty"`
	LastError     string    `json:"last_error,omitempty"`
	RelayBytesIn  uint64    `json:"relay_bytes_in"`
	RelayBytesOut uint64    `json:"relay_bytes_out"`
	RelayPort     int       `json:"relay_port"`
}

func NewRelayModeManager(
	db *database.DB,
	sni *proxy.Server,
	relayPort int,
	publicIPv6 func() string,
	localHostAllowed func(string) bool,
) *RelayModeManager {
	if relayPort == 0 {
		relayPort = 443
	}
	return &RelayModeManager{
		db: db, proxy: sni, relayPort: relayPort,
		publicIPv6: publicIPv6, localHostAllowed: localHostAllowed,
		hubs: map[string]*relayHub{}, globalEnabled: true,
	}
}

type relayModePolicy struct {
	hubID   string
	hosts   *agent.Allowlist
	clients *agent.ClientList
	in      atomic.Uint64
	out     atomic.Uint64
}

func (p *relayModePolicy) HubID() string { return p.hubID }
func (p *relayModePolicy) AllowsHost(host string) bool {
	return p != nil && p.hosts.Allows(host)
}
func (p *relayModePolicy) AllowsClient(ip net.IP) bool {
	return p != nil && p.clients.Allows(ip)
}
func (p *relayModePolicy) AddBytes(in, out uint64) {
	if p == nil {
		return
	}
	if in > 0 {
		p.in.Add(in)
	}
	if out > 0 {
		p.out.Add(out)
	}
}

type multiRelayPolicy struct {
	hubs      []*relayModePolicy
	byClient  map[string]*relayModePolicy
	ambiguous map[string]bool
}

func (p *multiRelayPolicy) Enabled() bool { return p != nil && len(p.hubs) > 0 }
func (p *multiRelayPolicy) ForClient(ip net.IP) (proxy.RelayHostPolicy, bool) {
	if p == nil || ip == nil {
		return nil, false
	}
	key := ip.String()
	if p.ambiguous[key] {
		return nil, false
	}
	hub := p.byClient[key]
	if hub == nil {
		return nil, false
	}
	return hub, true
}

// Compatibility helpers for the single-hub tests and diagnostics.
func (p *multiRelayPolicy) AllowsClient(ip net.IP) bool {
	_, ok := p.ForClient(ip)
	return ok
}
func (p *multiRelayPolicy) AllowsHost(host string) bool {
	for _, h := range p.hubs {
		if h.AllowsHost(host) {
			return true
		}
	}
	return false
}

func (m *RelayModeManager) loadHubs() ([]database.RelayHubSettings, error) {
	if m == nil || m.db == nil {
		return nil, nil
	}
	var hubs []database.RelayHubSettings
	if err := m.db.GetSetting("relay_hubs", &hubs); err == nil && len(hubs) > 0 {
		return hubs, nil
	}
	var old database.RelayModeSettings
	if err := m.db.GetSetting("relay_mode", &old); err == nil && old.HubURL != "" {
		hubs = []database.RelayHubSettings{{
			ID: "hub-1", Name: "Hub", Enabled: old.Enabled, HubURL: old.HubURL,
			AgentKey: old.AgentKey, AgentID: old.AgentID, Country: old.Country,
			Hosts: old.Hosts, Clients: old.Clients, IPv6Enabled: old.IPv6Enabled,
			CatalogueETag: old.CatalogueETag, LastSync: old.LastSync, LastError: old.LastError,
		}}
	}
	return hubs, nil
}

func (m *RelayModeManager) saveHubs(hubs []database.RelayHubSettings) error {
	if m == nil || m.db == nil {
		return nil
	}
	return m.db.SetSetting("relay_hubs", hubs)
}

type relayModeGlobalSetting struct {
	Enabled *bool `json:"enabled"`
}

// GlobalEnabled is the master switch. Missing records read as enabled so an
// upgrade does not silently stop relays that were already configured.
func (m *RelayModeManager) GlobalEnabled() bool {
	if m == nil || m.db == nil {
		return true
	}
	var v relayModeGlobalSetting
	if err := m.db.GetSetting("relay_mode_global", &v); err != nil || v.Enabled == nil {
		return true
	}
	return *v.Enabled
}

func (m *RelayModeManager) SetGlobalEnabled(enabled bool) error {
	if m == nil || m.db == nil {
		return nil
	}
	if err := m.db.SetSetting("relay_mode_global", relayModeGlobalSetting{Enabled: &enabled}); err != nil {
		return err
	}
	return m.Reload()
}

// Start loads the persisted hubs and starts one sync/heartbeat pair per enabled
// hub. It is safe to call again after Stop.
func (m *RelayModeManager) Start(ctx context.Context) error {
	if m == nil {
		return nil
	}
	hubs, err := m.loadHubs()
	if err != nil {
		return err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	runCtx, cancel := context.WithCancel(ctx)
	global := m.GlobalEnabled()

	m.mu.Lock()
	m.cancel = cancel
	m.globalEnabled = global
	m.hubs = map[string]*relayHub{}
	m.order = nil
	for _, h := range hubs {
		if h.ID == "" {
			h.ID = fmt.Sprintf("hub-%d", time.Now().UnixNano())
		}
		if h.Name == "" {
			h.Name = h.AgentID
			if h.Name == "" {
				h.Name = h.HubURL
			}
		}
		m.hubs[h.ID] = &relayHub{settings: h, lastError: h.LastError}
		m.order = append(m.order, h.ID)
	}
	m.rebuildPolicyLocked()
	if global {
		for _, id := range m.order {
			h := m.hubs[id]
			if h.settings.Enabled && h.settings.HubURL != "" && h.settings.AgentKey != "" {
				m.startHubLocked(runCtx, id)
			}
		}
	}
	m.mu.Unlock()
	return nil
}

func (m *RelayModeManager) startHubLocked(ctx context.Context, id string) {
	h := m.hubs[id]
	hub := agent.NewHub(h.settings.HubURL)
	hubCtx, cancel := context.WithCancel(ctx)
	h.cancel = cancel
	key := h.settings.AgentKey
	m.wg.Add(2)
	go func() {
		defer m.wg.Done()
		m.syncLoop(hubCtx, id, hub, key)
	}()
	go func() {
		defer m.wg.Done()
		m.heartbeatLoop(hubCtx, id, hub, key)
	}()
	log.Printf("[Relay Mode] serving hub %s as %s", h.settings.HubURL, h.settings.AgentID)
}

// Stop cancels every hub worker and removes the relay policy, leaving local
// proxying exactly as it was.
func (m *RelayModeManager) Stop() {
	if m == nil {
		return
	}
	m.mu.Lock()
	cancel := m.cancel
	m.cancel = nil
	for _, h := range m.hubs {
		if h.cancel != nil {
			h.cancel()
		}
	}
	m.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	m.wg.Wait()
	m.mu.Lock()
	m.policy = nil
	m.mu.Unlock()
	if m.proxy != nil {
		m.proxy.SetRelayPolicy(nil, nil)
	}
}

func (m *RelayModeManager) Reload() error {
	m.Stop()
	return m.Start(context.Background())
}

// rebuildPolicyLocked compiles every hub's cached catalogue into one policy.
// A client IP that appears in two hubs is ambiguous and is refused rather than
// guessed, because the wrong choice would expose another hub's host list.
func (m *RelayModeManager) rebuildPolicyLocked() {
	if !m.globalEnabled {
		m.policy = nil
		if m.proxy != nil {
			m.proxy.SetRelayPolicy(nil, m.localHostAllowed)
		}
		return
	}
	var policies []*relayModePolicy
	byClient := map[string]*relayModePolicy{}
	ambiguous := map[string]bool{}
	for _, id := range m.order {
		h := m.hubs[id]
		if !h.settings.Enabled || len(h.settings.Clients) == 0 {
			h.policy = nil
			continue
		}
		p := &relayModePolicy{
			hubID: id, hosts: agent.NewAllowlist(h.settings.Hosts),
			clients: agent.NewClientList(h.settings.Clients),
		}
		if h.policy != nil {
			p.in.Store(h.policy.in.Load())
			p.out.Store(h.policy.out.Load())
		}
		h.policy = p
		for _, raw := range h.settings.Clients {
			ip := net.ParseIP(strings.TrimSpace(raw))
			if ip == nil {
				continue
			}
			key := ip.String()
			if prev, ok := byClient[key]; ok && prev != p {
				ambiguous[key] = true
				continue
			}
			byClient[key] = p
		}
		policies = append(policies, p)
	}
	for key := range ambiguous {
		delete(byClient, key)
	}
	m.policy = &multiRelayPolicy{hubs: policies, byClient: byClient, ambiguous: ambiguous}
	if m.proxy == nil {
		return
	}
	if len(policies) == 0 {
		m.proxy.SetRelayPolicy(nil, m.localHostAllowed)
	} else {
		m.proxy.SetRelayPolicy(m.policy, m.localHostAllowed)
	}
}

func (m *RelayModeManager) syncLoop(ctx context.Context, id string, hub *agent.Hub, key string) {
	wait := time.Duration(0)
	for {
		if ctx.Err() != nil {
			return
		}
		m.mu.Lock()
		h := m.hubs[id]
		etag := ""
		if h != nil {
			etag = h.settings.CatalogueETag
		}
		m.mu.Unlock()
		cat, newETag, notModified, err := hub.FetchConditional(ctx, key, etag, wait)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			m.setHubError(id, err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(10 * time.Second):
			}
			wait = 0
			continue
		}
		wait = 25 * time.Second
		if notModified {
			continue
		}
		m.applyCatalogue(id, cat, newETag)
	}
}

func (m *RelayModeManager) applyCatalogue(id string, cat agent.Catalogue, etag string) {
	m.mu.Lock()
	h := m.hubs[id]
	if h == nil || !h.settings.Enabled {
		m.mu.Unlock()
		return
	}
	h.settings.Hosts = append([]string(nil), cat.Hosts...)
	h.settings.Clients = append([]string(nil), cat.Clients...)
	h.settings.Country = cat.Country
	h.settings.CatalogueETag = etag
	h.settings.LastSync = time.Now().UTC()
	h.settings.LastError = ""
	if cat.IPv6Enabled != nil {
		h.settings.IPv6Enabled = *cat.IPv6Enabled
	}
	settings := h.settings
	m.rebuildPolicyLocked()
	m.mu.Unlock()
	if err := m.saveHubs(m.snapshotHubs()); err != nil {
		log.Printf("[Relay Mode] could not persist hub %s catalogue: %v", id, err)
	}
	log.Printf("[Relay Mode] hub %s catalogue synced: %d hosts, %d clients", id, len(settings.Hosts), len(settings.Clients))
}

func (m *RelayModeManager) heartbeatLoop(ctx context.Context, id string, hub *agent.Hub, key string) {
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		m.mu.Lock()
		h := m.hubs[id]
		if h == nil {
			m.mu.Unlock()
			return
		}
		settings := h.settings
		var in, out uint64
		if h.policy != nil {
			in, out = h.policy.in.Load(), h.policy.out.Load()
		}
		m.mu.Unlock()
		ipv6 := ""
		if m.publicIPv6 != nil {
			ipv6 = m.publicIPv6()
		}
		_, err := hub.Heartbeat(key, version.Display(), m.relayPort, in, out, ipv6, settings.CatalogueETag)
		m.mu.Lock()
		h = m.hubs[id]
		if h != nil {
			if err != nil {
				h.online = false
				h.lastError = err.Error()
			} else {
				h.online = true
				h.lastHeartbeat = time.Now().UTC()
				h.lastError = ""
			}
			h.settings.LastHeartbeat = h.lastHeartbeat
			h.settings.LastError = h.lastError
			h.settings.RelayBytesIn, h.settings.RelayBytesOut = in, out
		}
		m.mu.Unlock()
		if err != nil {
			m.persistHubError(id, err)
		} else {
			_ = m.saveHubs(m.snapshotHubs())
		}
		timer.Reset(30 * time.Second)
	}
}

func (m *RelayModeManager) snapshotHubs() []database.RelayHubSettings {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]database.RelayHubSettings, 0, len(m.order))
	for _, id := range m.order {
		if h := m.hubs[id]; h != nil {
			out = append(out, h.settings)
		}
	}
	return out
}

func (m *RelayModeManager) setHubError(id string, err error) {
	if err == nil {
		return
	}
	m.mu.Lock()
	if h := m.hubs[id]; h != nil {
		h.online = false
		h.lastError = err.Error()
		h.settings.LastError = err.Error()
	}
	m.mu.Unlock()
	m.persistHubError(id, err)
}

func (m *RelayModeManager) persistHubError(id string, err error) {
	if err == nil {
		return
	}
	_ = m.saveHubs(m.snapshotHubs())
}

// Statuses returns every hub view, in creation order.
func (m *RelayModeManager) Statuses() []RelayModeView {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]RelayModeView, 0, len(m.order))
	for _, id := range m.order {
		h := m.hubs[id]
		if h == nil {
			continue
		}
		var in, outBytes uint64
		if h.policy != nil {
			in, outBytes = h.policy.in.Load(), h.policy.out.Load()
		}
		out = append(out, RelayModeView{
			ID: id, Name: h.settings.Name, Enabled: h.settings.Enabled,
			HubURL: h.settings.HubURL, AgentID: h.settings.AgentID,
			AgentKeySet: h.settings.AgentKey != "", Country: h.settings.Country,
			IPv6Enabled: h.settings.IPv6Enabled, Hosts: len(h.settings.Hosts),
			Clients: len(h.settings.Clients), Online: h.online,
			LastSync: h.settings.LastSync, LastHeartbeat: h.lastHeartbeat,
			LastError: h.lastError, RelayBytesIn: in, RelayBytesOut: outBytes,
			RelayPort: m.relayPort,
		})
	}
	return out
}

// Status keeps the single-hub contract for older callers and the test suite.
func (m *RelayModeManager) Status() RelayModeView {
	views := m.Statuses()
	if len(views) == 0 {
		return RelayModeView{RelayPort: 443}
	}
	return views[0]
}

// Test performs a real catalogue request against one hub.
func (m *RelayModeManager) Test(ctx context.Context) (time.Duration, error) {
	views := m.Statuses()
	if len(views) == 0 {
		return 0, fmt.Errorf("relay mode is not enabled")
	}
	return m.TestHub(ctx, views[0].ID)
}

func (m *RelayModeManager) TestHub(ctx context.Context, id string) (time.Duration, error) {
	if m == nil {
		return 0, fmt.Errorf("relay mode is unavailable")
	}
	m.mu.Lock()
	h := m.hubs[id]
	var settings database.RelayHubSettings
	if h != nil {
		settings = h.settings
	}
	m.mu.Unlock()
	if settings.HubURL == "" || settings.AgentKey == "" {
		return 0, fmt.Errorf("relay mode is not enabled")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	start := time.Now()
	if _, _, _, err := agent.NewHub(settings.HubURL).FetchConditional(ctx, settings.AgentKey, "", 0); err != nil {
		return 0, err
	}
	return time.Since(start), nil
}

// Enroll exchanges a one-time hub token for an agent key.
func (m *RelayModeManager) Enroll(ctx context.Context, hubURL, token string) (agent.Enrolment, error) {
	hubURL = strings.TrimRight(strings.TrimSpace(hubURL), "/")
	if hubURL == "" {
		return agent.Enrolment{}, fmt.Errorf("hub URL is required")
	}
	if strings.TrimSpace(token) == "" {
		return agent.Enrolment{}, fmt.Errorf("enrolment token is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	type result struct {
		enrolment agent.Enrolment
		err       error
	}
	ch := make(chan result, 1)
	go func() {
		enrolment, err := agent.NewHub(hubURL).Enrol(token, version.Display())
		ch <- result{enrolment, err}
	}()
	select {
	case <-ctx.Done():
		return agent.Enrolment{}, ctx.Err()
	case res := <-ch:
		return res.enrolment, res.err
	}
}
