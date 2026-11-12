package service

import (
	"context"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"digitdns/internal/agent"
	"digitdns/internal/core/proxy"
	"digitdns/internal/database"
	"digitdns/internal/version"
)

// RelayModeManager lets a full DigitDNS instance act as a relay agent for
// another DigitDNS hub. It reuses the existing agent protocol, but applies the
// catalogue to the instance's own SNI proxy so local proxying and relay traffic
// share the same 80/443 listeners.
type RelayModeManager struct {
	db               *database.DB
	proxy            *proxy.Server
	relayPort        int
	publicIPv6       func() string
	localHostAllowed func(string) bool

	mu            sync.Mutex
	settings      database.RelayModeSettings
	online        bool
	lastHeartbeat time.Time
	lastError     string
	policy        *relayModePolicy
	cancel        context.CancelFunc
	wg            sync.WaitGroup
}

// RelayModeView is the safe, display-only shape the panel receives. The agent
// key is never included.
type RelayModeView struct {
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
	}
}

type relayModePolicy struct {
	hosts   *agent.Allowlist
	clients *agent.ClientList
}

func (p *relayModePolicy) Enabled() bool { return p != nil }
func (p *relayModePolicy) AllowsClient(ip net.IP) bool {
	return p != nil && p.clients.Allows(ip)
}
func (p *relayModePolicy) AllowsHost(host string) bool {
	return p != nil && p.hosts.Allows(host)
}

func (m *RelayModeManager) loadSettings() (database.RelayModeSettings, error) {
	var settings database.RelayModeSettings
	if m == nil || m.db == nil {
		return settings, nil
	}
	if err := m.db.GetSetting("relay_mode", &settings); err != nil {
		return settings, err
	}
	settings.HubURL = strings.TrimRight(strings.TrimSpace(settings.HubURL), "/")
	return settings, nil
}

// Start loads the persisted relay mode and starts the sync/heartbeat loops when
// it is enabled and has a hub credential. It is safe to call again after Stop.
func (m *RelayModeManager) Start(ctx context.Context) error {
	if m == nil {
		return nil
	}
	settings, err := m.loadSettings()
	if err != nil {
		return err
	}
	m.mu.Lock()
	m.settings = settings
	m.lastError = settings.LastError
	m.mu.Unlock()

	if !settings.Enabled || settings.HubURL == "" || settings.AgentKey == "" {
		if m.proxy != nil {
			m.proxy.SetRelayPolicy(nil, m.localHostAllowed)
		}
		m.mu.Lock()
		m.policy = nil
		m.mu.Unlock()
		return nil
	}
	m.applyPolicy(settings)
	if ctx == nil {
		ctx = context.Background()
	}
	runCtx, cancel := context.WithCancel(ctx)
	m.mu.Lock()
	m.cancel = cancel
	m.wg.Add(2)
	m.mu.Unlock()

	hub := agent.NewHub(settings.HubURL)
	go func() {
		defer m.wg.Done()
		m.syncLoop(runCtx, hub, settings.AgentKey)
	}()
	go func() {
		defer m.wg.Done()
		m.heartbeatLoop(runCtx, hub, settings.AgentKey)
	}()
	log.Printf("[Relay Mode] serving as relay for %s as %s", settings.HubURL, settings.AgentID)
	return nil
}

// Stop cancels the loops and removes the relay policy, leaving local proxying
// exactly as it was.
func (m *RelayModeManager) Stop() {
	if m == nil {
		return
	}
	m.mu.Lock()
	cancel := m.cancel
	m.cancel = nil
	m.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	m.wg.Wait()
	m.mu.Lock()
	m.online = false
	m.policy = nil
	m.mu.Unlock()
	if m.proxy != nil {
		m.proxy.SetRelayPolicy(nil, nil)
	}
}

// Reload rereads the stored settings and restarts the loops.
func (m *RelayModeManager) Reload() error {
	m.Stop()
	return m.Start(context.Background())
}

func (m *RelayModeManager) applyPolicy(settings database.RelayModeSettings) {
	if m.proxy == nil {
		return
	}
	if !settings.Enabled || len(settings.Clients) == 0 {
		m.proxy.SetRelayPolicy(nil, m.localHostAllowed)
		m.mu.Lock()
		m.policy = nil
		m.mu.Unlock()
		return
	}
	policy := &relayModePolicy{
		hosts:   agent.NewAllowlist(settings.Hosts),
		clients: agent.NewClientList(settings.Clients),
	}
	m.proxy.SetRelayPolicy(policy, m.localHostAllowed)
	m.mu.Lock()
	m.policy = policy
	m.mu.Unlock()
}

func (m *RelayModeManager) syncLoop(ctx context.Context, hub *agent.Hub, key string) {
	m.mu.Lock()
	etag := m.settings.CatalogueETag
	if etag != "" && len(m.settings.Hosts) == 0 && len(m.settings.Clients) == 0 {
		etag = ""
	}
	m.mu.Unlock()

	wait := time.Duration(0)
	for {
		if ctx.Err() != nil {
			return
		}
		cat, newETag, notModified, err := hub.FetchConditional(ctx, key, etag, wait)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			m.setError(err)
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
		if newETag != "" {
			etag = newETag
		}
		m.applyCatalogue(cat, etag)
	}
}

func (m *RelayModeManager) applyCatalogue(cat agent.Catalogue, etag string) {
	// A fetch can finish after the operator disabled relay mode or changed the
	// hub/key. The database record is authoritative for those fields; merge only
	// the catalogue into the current record instead of writing a stale copy that
	// would resurrect a disabled relay.
	var current database.RelayModeSettings
	if err := m.db.GetSetting("relay_mode", &current); err != nil {
		m.setError(err)
		return
	}
	m.mu.Lock()
	expected := m.settings
	m.mu.Unlock()
	if !current.Enabled || current.HubURL != expected.HubURL || current.AgentKey != expected.AgentKey {
		return
	}
	current.Hosts = append([]string(nil), cat.Hosts...)
	current.Clients = append([]string(nil), cat.Clients...)
	current.Country = cat.Country
	current.CatalogueETag = etag
	current.LastSync = time.Now().UTC()
	current.LastError = ""
	if cat.IPv6Enabled != nil {
		current.IPv6Enabled = *cat.IPv6Enabled
	}
	m.mu.Lock()
	m.settings = current
	m.lastError = ""
	m.mu.Unlock()

	m.applyPolicy(current)
	if err := m.db.SetSetting("relay_mode", current); err != nil {
		m.setError(err)
		return
	}
	log.Printf("[Relay Mode] catalogue synced: %d hosts, %d clients", len(current.Hosts), len(current.Clients))
}

func (m *RelayModeManager) heartbeatLoop(ctx context.Context, hub *agent.Hub, key string) {
	// A first beat makes the hub show this relay online immediately.
	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		m.mu.Lock()
		settings := m.settings
		m.mu.Unlock()
		var in, out uint64
		if m.proxy != nil {
			in, out = m.proxy.RelayStats()
		}
		ipv6 := ""
		if m.publicIPv6 != nil {
			ipv6 = m.publicIPv6()
		}
		_, err := hub.Heartbeat(key, version.Display(), m.relayPort, in, out, ipv6, settings.CatalogueETag)
		if err != nil {
			m.mu.Lock()
			m.online = false
			m.lastError = err.Error()
			m.mu.Unlock()
		} else {
			m.mu.Lock()
			m.online = true
			m.lastHeartbeat = time.Now().UTC()
			m.lastError = ""
			m.mu.Unlock()
		}
		timer.Reset(30 * time.Second)
	}
}

func (m *RelayModeManager) setError(err error) {
	if err == nil {
		return
	}
	m.mu.Lock()
	m.lastError = err.Error()
	m.mu.Unlock()
	m.persistError(err)
}

func (m *RelayModeManager) persistError(err error) {
	var current database.RelayModeSettings
	if getErr := m.db.GetSetting("relay_mode", &current); getErr != nil {
		log.Printf("[Relay Mode] could not read settings for the last error: %v", getErr)
		return
	}
	if !current.Enabled {
		return
	}
	current.LastError = err.Error()
	if setErr := m.db.SetSetting("relay_mode", current); setErr != nil {
		log.Printf("[Relay Mode] could not persist the last error: %v", setErr)
	}
}

// Status returns the display view, including relay-only traffic counters.
func (m *RelayModeManager) Status() RelayModeView {
	if m == nil {
		return RelayModeView{RelayPort: 443}
	}
	m.mu.Lock()
	settings := m.settings
	online := m.online
	lastHeartbeat := m.lastHeartbeat
	lastError := m.lastError
	m.mu.Unlock()
	if settings.HubURL == "" {
		// A manager created before Start still reports the stored record.
		if loaded, err := m.loadSettings(); err == nil {
			settings = loaded
		}
	}
	var in, out uint64
	if m.proxy != nil {
		in, out = m.proxy.RelayStats()
	}
	return RelayModeView{
		Enabled: settings.Enabled, HubURL: settings.HubURL,
		AgentID: settings.AgentID, AgentKeySet: settings.AgentKey != "",
		Country: settings.Country, IPv6Enabled: settings.IPv6Enabled,
		Hosts: len(settings.Hosts), Clients: len(settings.Clients),
		Online: online, LastSync: settings.LastSync, LastHeartbeat: lastHeartbeat,
		LastError: lastError, RelayBytesIn: in, RelayBytesOut: out,
		RelayPort: m.relayPort,
	}
}

// Test performs a real catalogue request against the configured hub. It proves
// the hub URL, credential and network path without changing the active policy.
func (m *RelayModeManager) Test(ctx context.Context) (time.Duration, error) {
	if m == nil {
		return 0, fmt.Errorf("relay mode is unavailable")
	}
	settings, err := m.loadSettings()
	if err != nil {
		return 0, err
	}
	if !settings.Enabled || settings.HubURL == "" || settings.AgentKey == "" {
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
		ch <- result{enrolment: enrolment, err: err}
	}()
	select {
	case <-ctx.Done():
		return agent.Enrolment{}, ctx.Err()
	case res := <-ch:
		return res.enrolment, res.err
	}
}
