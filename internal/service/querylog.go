package service

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"digitdns/internal/database"
	bolt "go.etcd.io/bbolt"
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

	// cache is the query-path view of the saved settings. Push reads only this,
	// never the database: query logging is off by default, so a per-query DB read
	// would make every query pay for history that is not being kept. SaveSettings
	// and the constructor keep it current; the admin Settings() still reads the
	// store so it reports exactly what is persisted.
	cache atomic.Pointer[QueryLogSettings]
}

var queryLogBuckets = [][]byte{[]byte("queries"), []byte("meta")}

func NewQueryLogService(db *database.DB, path string) *QueryLogService {
	s := &QueryLogService{db: db, path: path, queue: make(chan database.QueryLogItem, 2048)}
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
		case <-s.done:
			flush()
			return
		}
	}
}

func (s *QueryLogService) writeBatch(items []database.QueryLogItem) {
	s.mu.Lock()
	db := s.store
	s.mu.Unlock()
	if db == nil {
		return
	}
	_ = db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("queries"))
		seq, _ := b.NextSequence()
		for i, item := range items {
			payload, err := json.Marshal(item)
			if err != nil {
				continue
			}
			key := make([]byte, 12)
			binary.BigEndian.PutUint64(key[:8], uint64(item.Timestamp.UnixNano()))
			binary.BigEndian.PutUint32(key[8:], uint32(seq)+uint32(i))
			if err := b.Put(key, payload); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *QueryLogService) cleaner() {
	timer := time.NewTicker(time.Hour)
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
	s.mu.Lock()
	db := s.store
	s.mu.Unlock()
	if db == nil {
		return
	}
	_ = db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("queries"))
		c := b.Cursor()
		for k, _ := c.First(); k != nil; k, _ = c.Next() {
			if len(k) < 8 {
				continue
			}
			if int64(binary.BigEndian.Uint64(k[:8])) >= cutoff {
				break
			}
			if err := c.Delete(); err != nil {
				return err
			}
		}
		return nil
	})
}

type QueryLogFilter struct {
	From, To       time.Time
	Client, Domain string
	Action, Proto  string
	Limit          int
}

func (s *QueryLogService) Search(filter QueryLogFilter) []database.QueryLogItem {
	s.mu.Lock()
	db := s.store
	s.mu.Unlock()
	if db == nil {
		return nil
	}
	if filter.Limit <= 0 || filter.Limit > 1000 {
		filter.Limit = 200
	}
	var out []database.QueryLogItem
	_ = db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("queries"))
		c := b.Cursor()
		for k, v := c.Last(); k != nil; k, v = c.Prev() {
			if len(k) < 8 {
				continue
			}
			ts := time.Unix(0, int64(binary.BigEndian.Uint64(k[:8])))
			if !filter.From.IsZero() && ts.Before(filter.From) {
				continue
			}
			if !filter.To.IsZero() && ts.After(filter.To) {
				break
			}
			var item database.QueryLogItem
			if json.Unmarshal(v, &item) != nil {
				continue
			}
			if filter.Client != "" && !strings.Contains(strings.ToLower(item.ClientIP), strings.ToLower(filter.Client)) && !strings.Contains(strings.ToLower(item.AccountName), strings.ToLower(filter.Client)) {
				continue
			}
			if filter.Domain != "" && !strings.Contains(strings.ToLower(item.Domain), strings.ToLower(filter.Domain)) {
				continue
			}
			if filter.Action != "" {
				allowed := false
				for _, a := range strings.Split(filter.Action, ",") {
					if strings.EqualFold(item.Action, strings.TrimSpace(a)) {
						allowed = true
						break
					}
				}
				if !allowed {
					continue
				}
			}
			if filter.Proto != "" {
				allowed := false
				for _, proto := range strings.Split(filter.Proto, ",") {
					if strings.EqualFold(item.Protocol, strings.TrimSpace(proto)) {
						allowed = true
						break
					}
				}
				if !allowed {
					continue
				}
			}
			out = append(out, item)
			if len(out) >= filter.Limit {
				break
			}
		}
		return nil
	})
	return out
}

// Clear removes every stored query while keeping the file and settings.
func (s *QueryLogService) Clear() error {
	s.mu.Lock()
	db := s.store
	s.mu.Unlock()
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

// Ensure deterministic ordering helper used by export.
func sortQueryLog(items []database.QueryLogItem) {
	sort.SliceStable(items, func(i, j int) bool { return items[i].Timestamp.After(items[j].Timestamp) })
}

var _ = fmt.Sprintf
