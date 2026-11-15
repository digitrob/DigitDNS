package service

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"digitdns/internal/database"
	bolt "go.etcd.io/bbolt"
)

const (
	// queryLogPageDefault is one screen of history. The panel and the Android app
	// both page rather than asking for the whole retention window in one response.
	queryLogPageDefault = 100
	// queryLogPageMax bounds a single page. It is deliberately lower than the old
	// 1000-row ceiling: a 1000-item page with answer sections is megabytes of JSON,
	// and the clients now ask for the next page instead.
	queryLogPageMax = 500
	// queryLogCleanupChunk caps how many keys one maintenance pass deletes. bbolt
	// has a single writer, so an unbounded retention purge can stall the query-log
	// writer (and the resolver behind it) for seconds. A chunk keeps each write
	// transaction short; the next tick continues where this one stopped.
	queryLogCleanupChunk = 20000
	// queryLogExportMax is the most records one CSV/JSON export will stream. It is
	// high enough to cover a full week of ordinary traffic and still bounds a
	// request from a very busy resolver.
	queryLogExportMax = 100000
)

// QueryLogSettings controls the optional persistent query history. The default
// is off; enabling it creates a separate bbolt file.
type QueryLogSettings struct {
	Enabled       bool     `json:"enabled"`
	RetentionDays int      `json:"retention_days"`
	Anonymize     bool     `json:"anonymize"`
	OptOut        []string `json:"opt_out,omitempty"`
	MaxMB         int      `json:"max_mb"`
}

func DefaultQueryLogSettings() QueryLogSettings {
	return QueryLogSettings{RetentionDays: 7, Anonymize: true, MaxMB: 256}
}

func (s QueryLogSettings) normalized() QueryLogSettings {
	if s.RetentionDays <= 0 {
		s.RetentionDays = 7
	}
	if s.MaxMB <= 0 {
		s.MaxMB = 256
	}
	if s.OptOut == nil {
		s.OptOut = []string{}
	}
	return s
}

// QueryLogService is the asynchronous writer and search surface. Push never
// blocks the resolver: a full queue drops the record.
type QueryLogService struct {
	db   *database.DB
	path string

	mu      sync.RWMutex
	store   *bolt.DB
	queue   chan database.QueryLogItem
	done    chan struct{}
	started bool

	// clearReqs is handled by the writer goroutine so a Clear cannot race a
	// pending batch back into the store. The writer flushes the batch it already
	// holds, deletes the bucket, and acknowledges on the channel.
	clearReqs chan chan error

	// cache is the query-path view of the saved settings. Push reads only this,
	// never the database: query logging is off by default, so a per-query DB read
	// would make every query pay for history that is not being kept. SaveSettings
	// and the constructor keep it current; the admin Settings() still reads the
	// store so it reports exactly what is persisted.
	cache atomic.Pointer[QueryLogSettings]
}

var queryLogBuckets = [][]byte{[]byte("queries"), []byte("meta")}

func NewQueryLogService(db *database.DB, path string) *QueryLogService {
	s := &QueryLogService{
		db:        db,
		path:      path,
		queue:     make(chan database.QueryLogItem, 2048),
		clearReqs: make(chan chan error, 1),
	}
	settings := s.readSettings()
	s.cache.Store(&settings)
	return s
}

// readSettings loads and normalizes the persisted config, defaulting when the
// record is absent or unreadable.
func (s *QueryLogService) readSettings() QueryLogSettings {
	var out QueryLogSettings
	if s == nil || s.db == nil {
		return DefaultQueryLogSettings()
	}
	if err := s.db.GetSetting("query_log", &out); err != nil {
		return DefaultQueryLogSettings()
	}
	if out.RetentionDays == 0 && !out.Enabled && out.MaxMB == 0 {
		return DefaultQueryLogSettings()
	}
	return out.normalized()
}

func (s *QueryLogService) Settings() QueryLogSettings {
	return s.readSettings()
}

func (s *QueryLogService) SaveSettings(in QueryLogSettings) error {
	if s == nil || s.db == nil {
		return nil
	}
	in = in.normalized()
	if err := s.db.SetSetting("query_log", in); err != nil {
		return err
	}
	s.cache.Store(&in)
	if in.Enabled {
		return s.ensureOpen()
	}
	return nil
}

// Enabled reports whether query logging is on, from the cached settings so the
// DNS hot path never touches the database to ask.
func (s *QueryLogService) Enabled() bool {
	if s == nil {
		return false
	}
	if v := s.cache.Load(); v != nil {
		return v.Enabled
	}
	return false
}

func (s *QueryLogService) ensureOpen() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.store != nil {
		return nil
	}
	db, err := bolt.Open(s.path, 0o600, &bolt.Options{Timeout: 3 * time.Second})
	if err != nil {
		return err
	}
	if err := db.Update(func(tx *bolt.Tx) error {
		for _, name := range queryLogBuckets {
			if _, err := tx.CreateBucketIfNotExists(name); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		_ = db.Close()
		return err
	}
	s.store = db
	if !s.started {
		s.started = true
		s.done = make(chan struct{})
		go s.writer()
		go s.cleaner()
	}
	return nil
}

// Push is the DNS hot-path entry. It never waits and never returns an error.
func (s *QueryLogService) Push(item database.QueryLogItem) {
	if s == nil {
		return
	}
	settings := s.cache.Load()
	if settings == nil || !settings.Enabled {
		return
	}
	item = s.privacy(item, *settings)
	select {
	case s.queue <- item:
	default:
	}
}

func (s *QueryLogService) privacy(item database.QueryLogItem, settings QueryLogSettings) database.QueryLogItem {
	for _, opt := range settings.OptOut {
		if strings.EqualFold(strings.TrimSpace(opt), strings.TrimSpace(item.ClientIP)) ||
			strings.EqualFold(strings.TrimSpace(opt), strings.TrimSpace(item.AccountName)) {
			item.ClientIP = ""
			item.AccountName = ""
			return item
		}
	}
	if settings.Anonymize {
		item.ClientIP = s.anon(item.ClientIP)
		item.AccountName = s.anon(item.AccountName)
	}
	return item
}

func (s *QueryLogService) anon(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	salt := s.salt()
	mac := hmac.New(sha256.New, salt)
	_, _ = mac.Write([]byte(value))
	return "anon-" + hex.EncodeToString(mac.Sum(nil))[:16]
}

func (s *QueryLogService) salt() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.store == nil {
		return []byte("digitdns-query-log")
	}
	var out []byte
	_ = s.store.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("meta"))
		if v := b.Get([]byte("salt")); len(v) == 32 {
			out = append([]byte(nil), v...)
			return nil
		}
		buf := make([]byte, 32)
		_, _ = rand.Read(buf)
		out = buf
		return b.Put([]byte("salt"), buf)
	})
	if len(out) == 0 {
		return []byte("digitdns-query-log")
	}
	return out
}

func (s *QueryLogService) writer() {
	batch := make([]database.QueryLogItem, 0, 64)
	timer := time.NewTicker(2 * time.Second)
	defer timer.Stop()
	flush := func() {
		if len(batch) == 0 {
			return
		}
		s.writeBatch(batch)
		batch = batch[:0]
	}
	for {
		select {
		case item := <-s.queue:
			batch = append(batch, item)
			if len(batch) >= 64 {
				flush()
			}
		case <-timer.C:
			flush()
		case ack := <-s.clearReqs:
			// Flush first: records queued before the Clear are history the operator
			// asked to delete, so they must be written and then removed in the same
			// goroutine. Records queued after this point belong to post-clear traffic.
			flush()
			ack <- s.clearStore()
		case <-s.done:
			flush()
			return
		}
	}
}

func (s *QueryLogService) writeBatch(items []database.QueryLogItem) {
	type encodedQuery struct {
		payload []byte
		ts      int64
	}
	// Marshal before opening the write transaction. bbolt has one writer, and JSON
	// encoding 64 answer sections while holding that lock delays every other write.
	encoded := make([]encodedQuery, 0, len(items))
	for _, item := range items {
		payload, err := json.Marshal(item)
		if err != nil {
			continue
		}
		encoded = append(encoded, encodedQuery{payload: payload, ts: item.Timestamp.UnixNano()})
	}
	if len(encoded) == 0 {
		return
	}
	s.mu.RLock()
	db := s.store
	s.mu.RUnlock()
	if db == nil {
		return
	}
	_ = db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("queries"))
		seq, _ := b.NextSequence()
		for i, e := range encoded {
			key := make([]byte, 12)
			binary.BigEndian.PutUint64(key[:8], uint64(e.ts))
			binary.BigEndian.PutUint32(key[8:], uint32(seq)+uint32(i))
			if err := b.Put(key, e.payload); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *QueryLogService) cleaner() {
	// Retention and the size cap are enforced on the same tick. Fifteen minutes is
	// frequent enough that a busy resolver's file does not overshoot its cap by an
	// hour's worth of traffic, and each tick deletes a bounded number of keys.
	timer := time.NewTicker(15 * time.Minute)
	defer timer.Stop()
	for {
		select {
		case <-timer.C:
			s.cleanup()
		case <-s.done:
			return
		}
	}
}

func (s *QueryLogService) cleanup() {
	settings := s.Settings()
	if !settings.Enabled {
		return
	}
	cutoff := time.Now().Add(-time.Duration(settings.RetentionDays) * 24 * time.Hour).UnixNano()
	s.mu.RLock()
	db := s.store
	s.mu.RUnlock()
	if db == nil {
		return
	}
	_ = db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("queries"))
		if b == nil {
			return nil
		}
		c := b.Cursor()
		removed := 0

		// Oldest-first, so the first record inside the window ends the pass.
		for k, _ := c.First(); k != nil && removed < queryLogCleanupChunk; k, _ = c.Next() {
			if len(k) < 8 {
				continue
			}
			if int64(binary.BigEndian.Uint64(k[:8])) >= cutoff {
				break
			}
			if err := c.Delete(); err != nil {
				return err
			}
			removed++
		}

		if settings.MaxMB <= 0 {
			return nil
		}
		// The file itself does not shrink when keys are deleted, so os.Stat is not
		// a usable gauge. Sum the live records instead; that is the number the
		// operator's "max_mb" actually promises.
		maxBytes := int64(settings.MaxMB) << 20
		var total int64
		for k, v := c.First(); k != nil; k, v = c.Next() {
			total += int64(len(k) + len(v))
		}
		if total <= maxBytes {
			return nil
		}
		// Trim to 90% so the next tick is not required immediately. The chunk cap
		// still applies: a purge larger than one tick continues on the next one.
		target := maxBytes * 9 / 10
		for k, v := c.First(); k != nil && total > target && removed < queryLogCleanupChunk*4; k, v = c.Next() {
			total -= int64(len(k) + len(v))
			if err := c.Delete(); err != nil {
				return err
			}
			removed++
		}
		return nil
	})
}

// QueryLogFilter is one history query. From/To are inclusive, Before is the opaque
// cursor returned by a previous page, and QTypes/Rcodes/Cached narrow the response
// rather than the request's transport.
type QueryLogFilter struct {
	From, To       time.Time
	Client, Domain string
	Action, Proto  string
	QTypes         []uint16
	Rcodes         []int
	Cached         *bool
	Before         string
	Limit          int
}

// QueryLogPage is one page of history plus the cursor for the next older page.
type QueryLogPage struct {
	Items      []database.QueryLogItem
	NextCursor string
	HasMore    bool
}

// QueryLogUsage is what the history currently occupies, for the settings card.
type QueryLogUsage struct {
	Records int64 `json:"records"`
	Bytes   int64 `json:"bytes"`
}

func queryLogMatchCSV(value, csv string) bool {
	for _, want := range strings.Split(csv, ",") {
		if strings.EqualFold(value, strings.TrimSpace(want)) {
			return true
		}
	}
	return false
}

func (f QueryLogFilter) matches(item database.QueryLogItem) bool {
	if f.Client != "" &&
		!strings.Contains(strings.ToLower(item.ClientIP), strings.ToLower(f.Client)) &&
		!strings.Contains(strings.ToLower(item.AccountName), strings.ToLower(f.Client)) {
		return false
	}
	if f.Domain != "" && !strings.Contains(strings.ToLower(item.Domain), strings.ToLower(f.Domain)) {
		return false
	}
	if f.Action != "" && !queryLogMatchCSV(item.Action, f.Action) {
		return false
	}
	if f.Proto != "" && !queryLogMatchCSV(item.Protocol, f.Proto) {
		return false
	}
	if len(f.QTypes) > 0 && !slices.Contains(f.QTypes, item.QType) {
		return false
	}
	if len(f.Rcodes) > 0 && !slices.Contains(f.Rcodes, item.Rcode) {
		return false
	}
	if f.Cached != nil && item.Cached != *f.Cached {
		return false
	}
	return true
}

func queryLogCursorKey(before string) []byte {
	if before == "" {
		return nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(before)
	if err != nil || len(raw) != 12 {
		// A stale or malformed cursor starts from the newest record rather than
		// failing the whole search; the caller simply gets the first page again.
		return nil
	}
	return raw
}

func encodeQueryLogCursor(key []byte) string {
	if len(key) != 12 {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(key)
}

// walk visits matching records newest-first. The callback returns false to stop.
// It is the single place where the time window, the column filters and the cursor
// are applied, so SearchPage and Export cannot disagree about what a filter means.
func (s *QueryLogService) walk(filter QueryLogFilter, visit func(key []byte, item database.QueryLogItem) (bool, error)) error {
	s.mu.RLock()
	db := s.store
	s.mu.RUnlock()
	if db == nil {
		return nil
	}
	return db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("queries"))
		if b == nil {
			return nil
		}
		c := b.Cursor()
		var k, v []byte
		if cursor := queryLogCursorKey(filter.Before); cursor != nil {
			k, v = c.Seek(cursor)
			if k == nil {
				k, v = c.Last()
			} else {
				// The cursor names the last record already delivered, so the next
				// page starts strictly before it. Seek lands on it (or on the next
				// key if it has since been cleaned up); Prev skips it either way.
				k, v = c.Prev()
			}
		} else {
			k, v = c.Last()
		}
		for ; k != nil; k, v = c.Prev() {
			if len(k) < 8 {
				continue
			}
			ts := time.Unix(0, int64(binary.BigEndian.Uint64(k[:8])))
			// Keys ascend with time and this walk descends, so the two bounds are
			// not symmetric: once a key is older than From, every later key is too
			// (break), while a key newer than To is simply outside the window
			// (continue). These were reversed, which made a `to=` search return
			// nothing and a `from=` search sweep the whole file.
			if !filter.From.IsZero() && ts.Before(filter.From) {
				break
			}
			if !filter.To.IsZero() && ts.After(filter.To) {
				continue
			}
			var item database.QueryLogItem
			if json.Unmarshal(v, &item) != nil {
				continue
			}
			if !filter.matches(item) {
				continue
			}
			keepGoing, err := visit(k, item)
			if err != nil {
				return err
			}
			if !keepGoing {
				return nil
			}
		}
		return nil
	})
}

// SearchPage returns one page. HasMore costs one extra matching scan position,
// which is what lets the clients hide "Load older" on the final page instead of
// guessing from a full page size.
func (s *QueryLogService) SearchPage(filter QueryLogFilter) QueryLogPage {
	if filter.Limit <= 0 {
		filter.Limit = queryLogPageDefault
	}
	if filter.Limit > queryLogPageMax {
		filter.Limit = queryLogPageMax
	}
	page := QueryLogPage{Items: make([]database.QueryLogItem, 0, filter.Limit)}
	var lastKey []byte
	_ = s.walk(filter, func(key []byte, item database.QueryLogItem) (bool, error) {
		if len(page.Items) >= filter.Limit {
			page.HasMore = true
			return false, nil
		}
		page.Items = append(page.Items, item)
		lastKey = append(lastKey[:0], key...)
		return true, nil
	})
	if page.HasMore {
		page.NextCursor = encodeQueryLogCursor(lastKey)
	}
	return page
}

// Search is the legacy slice view used by callers that do not page.
func (s *QueryLogService) Search(filter QueryLogFilter) []database.QueryLogItem {
	return s.SearchPage(filter).Items
}

// Export streams every matching record (oldest retention permitting) through visit.
// It returns the number written and whether the hard cap stopped the export early.
func (s *QueryLogService) Export(filter QueryLogFilter, visit func(database.QueryLogItem) error) (int, bool, error) {
	count := 0
	truncated := false
	err := s.walk(filter, func(_ []byte, item database.QueryLogItem) (bool, error) {
		if count >= queryLogExportMax {
			truncated = true
			return false, nil
		}
		if err := visit(item); err != nil {
			return false, err
		}
		count++
		return true, nil
	})
	return count, truncated, err
}

// Usage reports live records and their logical bytes. The bbolt file does not
// shrink on delete, so the settings card must not present the file size as usage.
func (s *QueryLogService) Usage() QueryLogUsage {
	s.mu.RLock()
	db := s.store
	s.mu.RUnlock()
	if db == nil {
		return QueryLogUsage{}
	}
	var out QueryLogUsage
	_ = db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("queries"))
		if b == nil {
			return nil
		}
		return b.ForEach(func(k, v []byte) error {
			out.Records++
			out.Bytes += int64(len(k) + len(v))
			return nil
		})
	})
	return out
}

// Clear removes every stored query while keeping the file and settings. When the
// writer is running the request goes through it, so a batch that was already
// flushed cannot land after the delete and resurrect "cleared" rows.
func (s *QueryLogService) Clear() error {
	s.mu.RLock()
	db := s.store
	started := s.started
	reqs := s.clearReqs
	done := s.done
	s.mu.RUnlock()
	if db == nil {
		return nil
	}
	if !started || reqs == nil || done == nil {
		return s.clearStore()
	}
	ack := make(chan error, 1)
	select {
	case reqs <- ack:
	case <-done:
		return s.clearStore()
	}
	select {
	case err := <-ack:
		return err
	case <-done:
		return s.clearStore()
	}
}

func (s *QueryLogService) clearStore() error {
	s.mu.RLock()
	db := s.store
	s.mu.RUnlock()
	if db == nil {
		return nil
	}
	return db.Update(func(tx *bolt.Tx) error {
		if err := tx.DeleteBucket([]byte("queries")); err != nil && err != bolt.ErrBucketNotFound {
			return err
		}
		_, err := tx.CreateBucketIfNotExists([]byte("queries"))
		return err
	})
}

// Start opens the store when enabled and launches retention cleanup.
func (s *QueryLogService) Start() {
	if s.Settings().Enabled {
		if err := s.ensureOpen(); err != nil {
			log.Printf("[Query Log] disabled: %v", err)
		}
	}
}

// Stop flushes the queue and closes the store.
func (s *QueryLogService) Stop() {
	s.mu.Lock()
	if s.started {
		close(s.done)
		s.started = false
	}
	db := s.store
	s.store = nil
	s.mu.Unlock()
	if db != nil {
		_ = db.Close()
	}
}

var _ = fmt.Sprintf
