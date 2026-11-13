// Package aggregate stores bounded hourly and daily DNS statistics in a
// separate bbolt database. It deliberately keeps no raw query history: the live
// log remains the in-memory ring in StatsService, and this package only retains
// counters and top-N tables for the dashboard's history cards.
package aggregate

import (
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"digitdns/internal/database"
)

const (
	defaultFlushInterval   = 30 * time.Second
	defaultHourlyRetention = 7 * 24 * time.Hour
	defaultRetentionDays   = 90
	defaultTopDomains      = 1000
	defaultTopClients      = 500
	defaultCacheTTL        = 30 * time.Second
)

// Config controls one aggregate store. Zero values are replaced by the defaults,
// so the daemon can open the store before the settings UI exists.
type Config struct {
	FlushInterval    time.Duration
	HourlyRetention  time.Duration
	RetentionDays    int
	TopDomains       int
	TopClients       int
	AnonymizeClients bool
	ExcludeClients   []string

	// excluded is built by normalized so the hot path does a map lookup rather
	// than scanning the operator's list on every query.
	excluded map[string]struct{}

	// CacheTTL bounds read-side result caching. A negative value disables it;
	// zero selects the default. Now is for tests; production leaves it nil.
	CacheTTL time.Duration
	Now      func() time.Time
}

func DefaultConfig() Config {
	return Config{
		FlushInterval:   defaultFlushInterval,
		HourlyRetention: defaultHourlyRetention,
		RetentionDays:   defaultRetentionDays,
		TopDomains:      defaultTopDomains,
		TopClients:      defaultTopClients,
		CacheTTL:        defaultCacheTTL,
	}
}

func (c Config) normalized() Config {
	if c.FlushInterval <= 0 {
		c.FlushInterval = defaultFlushInterval
	}
	if c.HourlyRetention <= 0 {
		c.HourlyRetention = defaultHourlyRetention
	}
	if c.RetentionDays <= 0 {
		c.RetentionDays = defaultRetentionDays
	}
	if c.TopDomains <= 0 {
		c.TopDomains = defaultTopDomains
	}
	if c.TopClients <= 0 {
		c.TopClients = defaultTopClients
	}
	if c.CacheTTL == 0 {
		c.CacheTTL = defaultCacheTTL
	}
	if c.Now == nil {
		c.Now = time.Now
	}
	if len(c.ExcludeClients) > 0 {
		c.excluded = make(map[string]struct{}, len(c.ExcludeClients))
		for _, client := range c.ExcludeClients {
			client = strings.TrimSpace(client)
			if client != "" {
				c.excluded[client] = struct{}{}
			}
		}
	}
	return c
}

func (c Config) excludesClient(client string) bool {
	if len(c.excluded) == 0 {
		return false
	}
	_, ok := c.excluded[client]
	return ok
}

// Aggregator owns the in-memory current hour and serialises writes to stats.db.
// Record hot-path methods only touch the current bucket under one short mutex;
// encoding and bbolt commits happen in the worker goroutine.
type Aggregator struct {
	cfg        Config
	store      *Store
	now        func() time.Time
	clientSalt []byte

	mu         sync.Mutex // guards rollover, pending and daily state only
	closed     atomic.Bool
	currentKey atomic.Value // string
	current    atomic.Pointer[Bucket]
	pending    []bucketPair
	lastDay    string

	flushMu sync.Mutex

	cacheMu sync.Mutex
	cache   map[string]cacheEntry

	flushCh   chan struct{}
	stop      chan struct{}
	done      chan struct{}
	closeOnce sync.Once
}

// Open opens (or creates) stats.db and loads the current UTC hour so a restart
// does not overwrite counters already flushed in this hour.
func Open(path string, cfg Config) (*Aggregator, error) {
	cfg = cfg.normalized()
	store, err := openStore(path)
	if err != nil {
		return nil, err
	}
	now := cfg.Now()
	a := &Aggregator{
		cfg:     cfg,
		store:   store,
		now:     cfg.Now,
		lastDay: dayKey(now),
		cache:   make(map[string]cacheEntry),
		flushCh: make(chan struct{}, 1),
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
	}
	a.currentKey.Store(hourKey(now))
	if b, readErr := store.readHour(hourKey(now)); readErr != nil {
		log.Printf("[Stats] could not read current stats hour: %v", readErr)
	} else if b != nil {
		a.current.Store(b)
	}
	if a.current.Load() == nil {
		a.current.Store(newBucket())
	}
	salt, saltErr := store.clientSalt()
	if saltErr != nil {
		log.Printf("[Stats] could not load client anonymization salt: %v", saltErr)
	} else {
		a.clientSalt = salt
	}

	// A daemon that was down across midnight still has the previous day's hourly
	// buckets; fold them before serving the first card.
	if err := a.recoverDaily(now); err != nil {
		log.Printf("[Stats] aggregate recovery failed: %v", err)
	}
	if err := a.prune(now); err != nil {
		log.Printf("[Stats] aggregate retention failed: %v", err)
	}

	go a.loop()
	return a, nil
}

func (a *Aggregator) Path() string { return a.store.Path() }

// RecordTotal counts every query the resolver saw. It is separate from
// RecordDetail because the live log samples rate-limit refusals and skips
// malformed early returns; the total card must still be honest.
func (a *Aggregator) RecordTotal(at time.Time) {
	a.withBucket(at, func(b *Bucket, chunk *Chunk) {
		b.recordTotal()
		chunk.Queries++
	})
}

// RecordDetail folds the dimensions from one query-log item. It does not
// increment Queries; RecordTotal already did that at the top of ProcessQuery.
func (a *Aggregator) RecordDetail(item database.QueryLogItem) {
	at := item.Timestamp
	if at.IsZero() {
		at = a.now()
	}
	a.withBucket(at, func(b *Bucket, chunk *Chunk) {
		blocked, proxied := b.Blocked, b.Proxied
		cached, failed := b.Cached, b.Failed
		latency, latencyCount := b.LatencySumMs, b.LatencyCount
		b.record(item, a.cfg, a.clientSalt)
		chunk.Blocked += b.Blocked - blocked
		chunk.Proxied += b.Proxied - proxied
		chunk.Cached += b.Cached - cached
		chunk.Failed += b.Failed - failed
		chunk.LatencySumMs += b.LatencySumMs - latency
		chunk.LatencyCount += b.LatencyCount - latencyCount
	})
}

func (a *Aggregator) withBucket(at time.Time, fn func(*Bucket, *Chunk)) {
	key := hourKey(at)

	curKey, _ := a.currentKey.Load().(string)
	if curKey != "" && key >= curKey {
		b := a.current.Load()
		if b != nil && curKey == key {
			b.mu.Lock()
			fn(b, &b.Chunks[chunkSlot(at)])
			b.mu.Unlock()
			return
		}
	}

	a.mu.Lock()
	if a.closed.Load() {
		a.mu.Unlock()
		return
	}
	curKey, _ = a.currentKey.Load().(string)
	if curKey == "" {
		curKey = hourKey(at)
		a.currentKey.Store(curKey)
	}
	if key < curKey {
		key = curKey
	}
	if key != curKey {
		if b := a.current.Load(); b != nil {
			a.pending = append(a.pending, bucketPair{key: curKey, bucket: b.clone()})
		}
		a.currentKey.Store(key)
		a.current.Store(newBucket())
		select {
		case a.flushCh <- struct{}{}:
		default:
		}
	}
	b := a.current.Load()
	a.mu.Unlock()
	b.mu.Lock()
	fn(b, &b.Chunks[chunkSlot(at)])
	b.mu.Unlock()
}

// chunkSlot maps an instant to its five-minute slice of the UTC hour.
func chunkSlot(at time.Time) int {
	slot := at.UTC().Minute() / chunkMinutes
	if slot < 0 || slot >= chunkSlots {
		return 0
	}
	return slot
}

func (a *Aggregator) loop() {
	ticker := time.NewTicker(a.cfg.FlushInterval)
	defer ticker.Stop()
	defer close(a.done)

	for {
		select {
		case <-a.stop:
			if err := a.flush(); err != nil {
				log.Printf("[Stats] final aggregate flush failed: %v", err)
			}
			if err := a.store.close(); err != nil {
				log.Printf("[Stats] closing aggregate store: %v", err)
			}
			return
		case <-ticker.C:
			if err := a.flush(); err != nil {
				log.Printf("[Stats] aggregate flush failed: %v", err)
			}
			if err := a.maybeRollover(a.now()); err != nil {
				log.Printf("[Stats] aggregate rollover: %v", err)
			}
		case <-a.flushCh:
			if err := a.flush(); err != nil {
				log.Printf("[Stats] aggregate flush failed: %v", err)
			}
		}
	}
}

func (a *Aggregator) flush() error {
	a.flushMu.Lock()
	defer a.flushMu.Unlock()

	a.mu.Lock()
	pendingCount := len(a.pending)
	pending := append([]bucketPair(nil), a.pending...)
	currentKey, _ := a.currentKey.Load().(string)
	var current *Bucket
	if b := a.current.Load(); b != nil {
		current = b.clone()
	}
	a.mu.Unlock()

	for _, pair := range pending {
		if pair.bucket != nil {
			pair.bucket.compact(a.cfg)
		}
	}
	if current != nil {
		current.compact(a.cfg)
	}
	pairs := append([]bucketPair{}, pending...)
	if current != nil {
		pairs = append(pairs, bucketPair{key: currentKey, bucket: current})
	}
	if err := a.store.writeBuckets(pairs); err != nil {
		// The transaction is all-or-nothing, so every pending bucket is still
		// unwritten. Keep the in-memory copies (they are never removed before a
		// successful commit) and retry on the next tick.
		return err
	}
	a.mu.Lock()
	// Records for the current hour may have been appended while the commit ran;
	// only the prefix captured above is now durable.
	if pendingCount <= len(a.pending) {
		a.pending = a.pending[pendingCount:]
	}
	a.mu.Unlock()
	return nil
}

// maybeRollover rolls the previous UTC day into a daily bucket the first time
// the worker sees a new day. flush() runs first so the final hourly bucket is on
// disk before it is read back.
func (a *Aggregator) maybeRollover(now time.Time) error {
	today := dayKey(now)
	if today == a.lastDay {
		return nil
	}
	if err := a.flush(); err != nil {
		return err
	}
	// recoverDaily scans for every missing past day, so a process that was
	// suspended or paused across more than one midnight catches up here too.
	if err := a.recoverDaily(now); err != nil {
		return err
	}
	if err := a.prune(now); err != nil {
		return err
	}
	// Advance only after both steps succeed, so a transient failure is retried
	// on the next tick instead of silently losing the day.
	a.lastDay = today
	return nil
}

// recoverDaily fills any daily bucket missing from a previous process lifetime,
// without touching a daily bucket that already exists.
func (a *Aggregator) recoverDaily(now time.Time) error {
	keys, err := a.store.allHourKeys()
	if err != nil {
		return err
	}
	today := dayKey(now)
	seen := make(map[string]bool)
	for _, key := range keys {
		if len(key) < 8 {
			continue
		}
		day := key[:8]
		if day >= today || seen[day] {
			continue
		}
		seen[day] = true
		exists, err := a.store.dailyExists(day)
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		if err := a.rollupDay(day); err != nil {
			return err
		}
	}
	return nil
}

func (a *Aggregator) rollupDay(day string) error {
	exists, err := a.store.dailyExists(day)
	if err != nil || exists {
		return err
	}
	keys, err := a.store.hourKeysForDay(day)
	if err != nil {
		return err
	}
	if len(keys) == 0 {
		return nil
	}
	daily := newBucket()
	for _, key := range keys {
		b, err := a.store.readHour(key)
		if err != nil {
			return err
		}
		if b != nil {
			daily.merge(b, a.cfg)
		}
	}
	daily.compact(a.cfg)
	return a.store.writeDaily(day, daily)
}

func (a *Aggregator) prune(now time.Time) error {
	hourCutoff := now.UTC().Add(-a.cfg.HourlyRetention)
	dayCutoff := truncateDay(now).AddDate(0, 0, -a.cfg.RetentionDays)
	if err := a.store.pruneHourly(hourCutoff); err != nil {
		return err
	}
	return a.store.pruneDaily(dayCutoff)
}

// Close stops the worker and flushes the current bucket before closing stats.db.
// Safe to call more than once.
func (a *Aggregator) Close() {
	if a == nil {
		return
	}
	a.closeOnce.Do(func() {
		a.closed.Store(true)
		close(a.stop)
		<-a.done
	})
}

func hourKey(t time.Time) string {
	return t.UTC().Format("2006010215")
}

func dayKey(t time.Time) string {
	return t.UTC().Format("20060102")
}

func truncateDay(t time.Time) time.Time {
	t = t.UTC()
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}
