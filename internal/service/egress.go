package service

import (
	"context"
	"fmt"
	"math"
	"net"
	"net/netip"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"digitdns/internal/database"
)

// egressConfig is the runtime copy of one stored egress. Secrets stay here and
// are never serialised to an API response.
type egressConfig struct {
	ID         string
	Kind       string
	Name       string
	Country    string
	Enabled    bool
	FailClosed bool
	Address    string
	Username   string
	Password   string
	Config     string
	Resolve    string
	TestHost   string
}

// EgressView is the safe, display-only shape the panel and API receive.
type EgressView struct {
	ID          string    `json:"id"`
	Kind        string    `json:"kind"`
	Name        string    `json:"name"`
	Country     string    `json:"country"`
	Enabled     bool      `json:"enabled"`
	FailClosed  bool      `json:"fail_closed"`
	Address     string    `json:"address,omitempty"`
	Username    string    `json:"username,omitempty"`
	PasswordSet bool      `json:"password_set"`
	ConfigSet   bool      `json:"config_set"`
	Resolve     string    `json:"resolve"`
	Health      string    `json:"health"`
	LatencyMs   float64   `json:"latency_ms"`
	LastError   string    `json:"last_error,omitempty"`
	LastSeen    time.Time `json:"last_seen,omitempty"`
	BytesIn     uint64    `json:"bytes_in"`
	BytesOut    uint64    `json:"bytes_out"`
}

type egressRuntime struct {
	cfg       egressConfig
	health    atomic.Value  // string: online, offline, unknown
	latencyMs atomic.Uint64 // float64 bits
	lastError atomic.Value  // string
	lastSeen  atomic.Int64  // UnixNano
	bytesIn   atomic.Uint64
	bytesOut  atomic.Uint64

	// wgMu guards the lazily-created userspace WireGuard device. SOCKS5 leaves
	// it nil.
	wgMu sync.Mutex
	wg   *wireGuardTunnel
}

// EgressRegistry owns the live egress configuration, health and traffic
// counters. The DNS path and the SNI relay read it without taking the DB lock.
type EgressRegistry struct {
	db *database.DB

	mu    sync.RWMutex
	items map[string]*egressRuntime

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func NewEgressRegistry(db *database.DB) *EgressRegistry {
	r := &EgressRegistry{db: db, items: map[string]*egressRuntime{}}
	return r
}

// Reload rebuilds the runtime map from the persisted collection. Existing
// runtimes keep their counters and health when their config is unchanged.
func (r *EgressRegistry) Reload() error {
	if r == nil || r.db == nil {
		return nil
	}
	items, err := r.db.ListEgresses()
	if err != nil {
		return err
	}
	next := make(map[string]*egressRuntime, len(items))
	r.mu.Lock()
	for _, item := range items {
		cfg := configFromEgress(item)
		if old, ok := r.items[item.ID]; ok && sameEgressConfig(old.cfg, cfg) {
			next[item.ID] = old
			continue
		}
		rt := &egressRuntime{cfg: cfg}
		rt.health.Store("unknown")
		rt.lastError.Store("")
		next[item.ID] = rt
	}
	removed := make([]*egressRuntime, 0)
	for id, old := range r.items {
		if _, keep := next[id]; !keep {
			removed = append(removed, old)
		}
	}
	r.items = next
	r.mu.Unlock()
	for _, old := range removed {
		old.closeWireGuard()
	}
	return nil
}

func configFromEgress(e database.Egress) egressConfig {
	resolve := strings.ToLower(strings.TrimSpace(e.Resolve))
	if resolve != "local" {
		resolve = "remote"
	}
	return egressConfig{
		ID: e.ID, Kind: e.Kind, Name: e.Name, Country: e.Country,
		Enabled: e.Enabled, FailClosed: e.FailClosed,
		Address: e.Address, Username: e.Username, Password: e.Password,
		Config: e.Config, Resolve: resolve, TestHost: e.TestHost,
	}
}

func sameEgressConfig(a, b egressConfig) bool {
	return a == b
}

// Start launches the health loop. It is safe to call once.
func (r *EgressRegistry) Start(ctx context.Context) {
	if r == nil {
		return
	}
	r.mu.Lock()
	if r.cancel != nil {
		r.mu.Unlock()
		return
	}
	runCtx, cancel := context.WithCancel(ctx)
	r.cancel = cancel
	r.mu.Unlock()
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		r.probeAll(runCtx)
		for {
			select {
			case <-runCtx.Done():
				return
			case <-ticker.C:
				r.probeAll(runCtx)
			}
		}
	}()
}

func (r *EgressRegistry) Stop() {
	if r == nil {
		return
	}
	r.mu.Lock()
	cancel := r.cancel
	r.cancel = nil
	items := make([]*egressRuntime, 0, len(r.items))
	for _, rt := range r.items {
		items = append(items, rt)
	}
	r.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	r.wg.Wait()
	for _, rt := range items {
		rt.closeWireGuard()
	}
}

func (r *EgressRegistry) probeAll(ctx context.Context) {
	r.mu.RLock()
	items := make([]*egressRuntime, 0, len(r.items))
	for _, rt := range r.items {
		items = append(items, rt)
	}
	r.mu.RUnlock()
	for _, rt := range items {
		if !rt.cfg.Enabled {
			rt.health.Store("unknown")
			continue
		}
		probeCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
		latency, err := r.probe(probeCtx, rt.cfg)
		cancel()
		if err != nil {
			rt.health.Store("offline")
			rt.lastError.Store(err.Error())
		} else {
			rt.health.Store("online")
			rt.latencyMs.Store(mathFloat64bits(latency.Seconds() * 1000))
			rt.lastError.Store("")
			rt.lastSeen.Store(time.Now().UnixNano())
		}
	}
}

func (r *EgressRegistry) probe(ctx context.Context, cfg egressConfig) (time.Duration, error) {
	switch cfg.Kind {
	case database.EgressKindSOCKS5:
		return probeSOCKS5(ctx, cfg)
	case database.EgressKindWireGuard:
		rt, ok := r.runtime(cfg.ID)
		if !ok {
			return 0, fmt.Errorf("egress not found")
		}
		return rt.probeWireGuard(ctx)
	default:
		return 0, fmt.Errorf("unknown egress kind %q", cfg.Kind)
	}
}

// Test runs one explicit health probe for the API's Test button.
func (r *EgressRegistry) Test(ctx context.Context, id string) (time.Duration, error) {
	r.mu.RLock()
	rt, ok := r.items[id]
	r.mu.RUnlock()
	if !ok {
		return 0, fmt.Errorf("egress not found")
	}
	latency, err := r.probe(ctx, rt.cfg)
	if err != nil {
		rt.health.Store("offline")
		rt.lastError.Store(err.Error())
		return 0, err
	}
	rt.health.Store("online")
	rt.latencyMs.Store(mathFloat64bits(latency.Seconds() * 1000))
	rt.lastError.Store("")
	rt.lastSeen.Store(time.Now().UnixNano())
	return latency, nil
}

// Dial opens the origin connection through the selected egress.
func (r *EgressRegistry) Dial(ctx context.Context, id, host string, addrs []netip.Addr, port int) (net.Conn, error) {
	r.mu.RLock()
	rt, ok := r.items[id]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("egress not found")
	}
	if !rt.cfg.Enabled {
		return nil, fmt.Errorf("egress is disabled")
	}
	var conn net.Conn
	var err error
	switch rt.cfg.Kind {
	case database.EgressKindSOCKS5:
		ips := make([]net.IP, 0, len(addrs))
		for _, addr := range addrs {
			ips = append(ips, net.IP(addr.AsSlice()))
		}
		conn, err = dialSOCKS5(ctx, rt.cfg, host, ips, port)
	case database.EgressKindWireGuard:
		conn, err = rt.dialWireGuard(ctx, host, addrs, port)
	default:
		err = fmt.Errorf("unknown egress kind %q", rt.cfg.Kind)
	}
	if err != nil {
		// A failed origin connection is not proof that the egress itself is
		// down: the target may simply refuse. Leave the health verdict to the
		// probe loop and the Test button, but keep the error visible.
		rt.lastError.Store(err.Error())
		return nil, err
	}
	rt.health.Store("online")
	rt.lastError.Store("")
	rt.lastSeen.Store(time.Now().UnixNano())
	return &countingConn{Conn: conn, in: &rt.bytesIn, out: &rt.bytesOut}, nil
}

func (r *EgressRegistry) runtime(id string) (*egressRuntime, bool) {
	r.mu.RLock()
	rt, ok := r.items[id]
	r.mu.RUnlock()
	return rt, ok
}

// Lookup returns a display view for one egress.
func (r *EgressRegistry) Lookup(id string) (EgressView, bool) {
	r.mu.RLock()
	rt, ok := r.items[id]
	r.mu.RUnlock()
	if !ok {
		return EgressView{}, false
	}
	return rt.view(), true
}

// Views returns every egress in display form.
func (r *EgressRegistry) Views() []EgressView {
	r.mu.RLock()
	items := make([]*egressRuntime, 0, len(r.items))
	for _, rt := range r.items {
		items = append(items, rt)
	}
	r.mu.RUnlock()
	out := make([]EgressView, 0, len(items))
	for _, rt := range items {
		out = append(out, rt.view())
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

func (rt *egressRuntime) view() EgressView {
	health, _ := rt.health.Load().(string)
	lastError, _ := rt.lastError.Load().(string)
	latency := mathFloat64frombits(rt.latencyMs.Load())
	var lastSeen time.Time
	if ns := rt.lastSeen.Load(); ns > 0 {
		lastSeen = time.Unix(0, ns).UTC()
	}
	return EgressView{
		ID: rt.cfg.ID, Kind: rt.cfg.Kind, Name: rt.cfg.Name, Country: rt.cfg.Country,
		Enabled: rt.cfg.Enabled, FailClosed: rt.cfg.FailClosed,
		Address: rt.cfg.Address, Username: rt.cfg.Username,
		PasswordSet: rt.cfg.Password != "", ConfigSet: rt.cfg.Config != "",
		Resolve: rt.cfg.Resolve, Health: health, LatencyMs: latency,
		LastError: lastError,
		LastSeen:  lastSeen,
		BytesIn:   rt.bytesIn.Load(), BytesOut: rt.bytesOut.Load(),
	}
}

type countingConn struct {
	net.Conn
	in  *atomic.Uint64
	out *atomic.Uint64
}

func (c *countingConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	if n > 0 && c.in != nil {
		c.in.Add(uint64(n))
	}
	return n, err
}

func (c *countingConn) Write(p []byte) (int, error) {
	n, err := c.Conn.Write(p)
	if n > 0 && c.out != nil {
		c.out.Add(uint64(n))
	}
	return n, err
}

// mathFloat64bits/frombits keep the hot struct free of a mutex. They are tiny
// wrappers so the imports stay local to this file.
func mathFloat64bits(f float64) uint64     { return math.Float64bits(f) }
func mathFloat64frombits(b uint64) float64 { return math.Float64frombits(b) }
