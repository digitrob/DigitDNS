package agent

import (
	"sort"
	"sync"
	"sync/atomic"
)

// Counters records relayed bytes per hostname. The per-host split is the whole
// point of the agent: a channel whose video host carries megabytes is one that
// cannot be served api-only, and this is the only place that can be measured
// from the exit rather than guessed at.
type Counters struct {
	mu    sync.RWMutex
	hosts map[string]*hostCounter
	in    atomic.Uint64
	out   atomic.Uint64
}

type hostCounter struct {
	in  atomic.Uint64
	out atomic.Uint64
}

// HostStat is one row of a counter snapshot.
type HostStat struct {
	Host  string `json:"host"`
	Bytes uint64 `json:"bytes"`
}

// NewCounters returns an empty counter set.
func NewCounters() *Counters {
	return &Counters{hosts: make(map[string]*hostCounter)}
}

func (c *Counters) counter(host string) *hostCounter {
	c.mu.RLock()
	hc := c.hosts[host]
	c.mu.RUnlock()
	if hc != nil {
		return hc
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if hc = c.hosts[host]; hc == nil {
		hc = &hostCounter{}
		c.hosts[host] = hc
	}
	return hc
}

// addIn records bytes travelling client → target.
func (c *Counters) addIn(host string, n uint64) {
	if c == nil || n == 0 {
		return
	}
	c.counter(host).in.Add(n)
	c.in.Add(n)
}

// addOut records bytes travelling target → client.
func (c *Counters) addOut(host string, n uint64) {
	if c == nil || n == 0 {
		return
	}
	c.counter(host).out.Add(n)
	c.out.Add(n)
}

// Totals returns the relayed byte counts in each direction, summed over hosts.
func (c *Counters) Totals() (in, out uint64) {
	if c == nil {
		return 0, 0
	}
	return c.in.Load(), c.out.Load()
}

// Snapshot returns per-host totals, largest first, so a status view leads with
// whatever is actually carrying the traffic.
func (c *Counters) Snapshot() []HostStat {
	if c == nil {
		return nil
	}
	c.mu.RLock()
	defer c.mu.RUnlock()

	out := make([]HostStat, 0, len(c.hosts))
	for host, hc := range c.hosts {
		out = append(out, HostStat{Host: host, Bytes: hc.in.Load() + hc.out.Load()})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Bytes != out[j].Bytes {
			return out[i].Bytes > out[j].Bytes
		}
		return out[i].Host < out[j].Host
	})
	return out
}
