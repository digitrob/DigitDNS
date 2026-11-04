package aggregate

import "sort"

// Counts is one bounded top-N table. Items holds the tracked keys, Other is the
// sum of the keys that were evicted by compaction. Keeping the overflow separate
// means a card can say "other" instead of silently presenting a top-N list as if
// it were the whole population.
//
// The bound is 2*max between compactions. A busy resolver can see a long tail of
// one-off names; growing a map per hour without a ceiling is exactly how a stats
// feature becomes a memory-exhaustion vector. Compaction keeps the heavy hitters
// and folds the rest into Other.
type Counts struct {
	Items map[string]uint64 `json:"items,omitempty"`
	Other uint64            `json:"other,omitempty"`
}

func (c *Counts) add(key string, n uint64, max int) {
	if n == 0 {
		return
	}
	if key == "" {
		key = "unknown"
	}
	if max <= 0 {
		c.Other += n
		return
	}
	if c.Items == nil {
		c.Items = make(map[string]uint64, max)
	}
	if cur, ok := c.Items[key]; ok {
		c.Items[key] = cur + n
		return
	}
	if len(c.Items) < max*2 {
		c.Items[key] = n
		return
	}
	c.compact(max)
	// compact leaves at most max entries, so this insert cannot overflow again.
	c.Items[key] = n
}

// compact keeps the max highest counts and folds the remainder into Other.
// Ties are ordered by key so a snapshot is deterministic in tests and stable
// across repeated flushes.
func (c *Counts) compact(max int) {
	if max <= 0 {
		for _, n := range c.Items {
			c.Other += n
		}
		c.Items = nil
		return
	}
	if len(c.Items) <= max {
		return
	}
	type entry struct {
		key string
		n   uint64
	}
	entries := make([]entry, 0, len(c.Items))
	for k, n := range c.Items {
		entries = append(entries, entry{k, n})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].n != entries[j].n {
			return entries[i].n > entries[j].n
		}
		return entries[i].key < entries[j].key
	})
	kept := make(map[string]uint64, max)
	for i, e := range entries {
		if i < max {
			kept[e.key] = e.n
		} else {
			c.Other += e.n
		}
	}
	c.Items = kept
}

func (c *Counts) merge(other Counts, max int) {
	if len(other.Items) == 0 {
		c.Other += other.Other
		return
	}
	for k, n := range other.Items {
		c.add(k, n, max)
	}
	c.Other += other.Other
	c.compact(max)
}

func (c *Counts) clone() Counts {
	out := Counts{Other: c.Other}
	if len(c.Items) == 0 {
		return out
	}
	out.Items = make(map[string]uint64, len(c.Items))
	for k, n := range c.Items {
		out.Items[k] = n
	}
	return out
}
