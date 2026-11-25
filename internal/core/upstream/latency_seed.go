package upstream

import (
	"encoding/json"
	"log"
	"os"
	"time"
)

// Latency seeds let the first query after a restart start with the order the
// previous run measured, instead of the configured list order. The Adaptive
// choice is already persisted in settings; this covers Hedged/Best Known, whose
// per-query order otherwise has no history until the startup benchmark lands.
//
// The file is advisory: a missing, old or malformed file is ignored, and the
// startup benchmark overwrites it. It is deliberately separate from the
// database so a corrupt read can never stop the resolver from booting.
const (
	latencySeedMaxAge    = 7 * 24 * time.Hour
	latencySeedSaveEvery = time.Minute
)

type latencySeedFile struct {
	Updated   time.Time          `json:"updated"`
	Upstreams map[string]float64 `json:"upstreams_ms"`
}

// SetLatencySeedPath loads the previous run's measurements and remembers where
// to write the next ones. It is called once during startup, before listeners
// accept.
func (p *UpstreamPool) SetLatencySeedPath(path string) {
	if p == nil || path == "" {
		return
	}
	p.seedPath.Store(path)

	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var seed latencySeedFile
	if err := json.Unmarshal(data, &seed); err != nil {
		log.Printf("[Upstream] Ignoring unreadable latency seed %s: %v", path, err)
		return
	}
	if seed.Updated.IsZero() || time.Since(seed.Updated) > latencySeedMaxAge {
		return
	}

	restored := 0
	p.mu.RLock()
	for addr, ms := range seed.Upstreams {
		st := p.stats[addr]
		if st == nil || ms <= 0 {
			continue
		}
		st.mu.Lock()
		st.lastProbeLatency = time.Duration(ms * float64(time.Millisecond))
		st.lastProbeOK = true
		st.lastProbeAt = seed.Updated
		st.mu.Unlock()
		restored++
	}
	p.mu.RUnlock()
	if restored > 0 {
		p.orderView.Store(nil)
		log.Printf("[Upstream] Restored %d last-known resolver latencies from %s", restored, path)
	}
}

// persistLatencySeeds writes the current probe/live medians. Best effort: a
// read-only data directory must not affect query service, so every error is
// only logged.
func (p *UpstreamPool) persistLatencySeeds() {
	if p == nil {
		return
	}
	path, _ := p.seedPath.Load().(string)
	if path == "" {
		return
	}
	now := time.Now()
	last := p.seedSaved.Load()
	if now.Unix()-last < int64(latencySeedSaveEvery/time.Second) {
		return
	}
	if !p.seedSaved.CompareAndSwap(last, now.Unix()) {
		return
	}

	view := p.statsView.Load()
	if view == nil {
		return
	}
	seed := latencySeedFile{
		Updated:   now.UTC(),
		Upstreams: make(map[string]float64, len(*view)),
	}
	for addr, st := range *view {
		st.mu.Lock()
		var ms float64
		switch {
		case st.lastProbeOK && st.lastProbeLatency > 0:
			ms = float64(st.lastProbeLatency) / float64(time.Millisecond)
		case st.samples > 0:
			if p50, _, _ := st.latencyPercentiles(); p50 > 0 {
				ms = float64(p50) / float64(time.Millisecond)
			}
		}
		st.mu.Unlock()
		if ms > 0 {
			seed.Upstreams[addr] = ms
		}
	}
	if len(seed.Upstreams) == 0 {
		return
	}
	data, err := json.Marshal(seed)
	if err != nil {
		return
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
	}
}
