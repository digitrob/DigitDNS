// Package geoip resolves an IP address to an ISO 3166-1 alpha-2 country code,
// using a MaxMind-format database that lives on disk and is refreshed in the
// background.
//
// The database is not embedded: it is a 3–4 MB file that changes daily, and a
// resolver binary that carried a frozen copy would answer "where is this server"
// with last year's answer. It is downloaded once, kept next to the other settings
// in the data directory, and replaced atomically — a lookup that is in flight
// during an update finishes against the reader it started with.
//
// Nothing here talks to a per-query service. The only outbound request is the
// periodic download, which means the feature works offline for as long as the
// file is present and no client address ever leaves the box.
package geoip

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/oschwald/maxminddb-golang"
)

// FileName is the file kept in the service's directory. It is the name the
// public mirrors publish, so an operator can drop a hand-downloaded copy in
// place without renaming anything.
const FileName = "GeoLite2-Country.mmdb"

// DefaultSource is a public daily mirror of MaxMind's GeoLite2-Country database.
// No account or licence key is needed. The data itself is GeoLite2, licensed
// CC BY-SA 4.0 — see the attribution in NOTICE. Operators can point the service
// at another URL (including MaxMind's authenticated endpoint) through New.
const DefaultSource = "https://github.com/P3TERX/GeoLite.mmdb/raw/download/GeoLite2-Country.mmdb"

const (
	// maxDownload caps what an update is willing to read. The real file is ~4 MB;
	// the ceiling only exists so a wrong URL cannot fill the disk or the heap.
	maxDownload = 64 << 20

	// initialDelay keeps a 4 MB download off the critical path of startup.
	initialDelay = 20 * time.Second

	// refreshInterval is how often the background loop looks for a newer file.
	// The mirrors publish daily, so a day is the natural period.
	refreshInterval = 24 * time.Hour

	// staleAfter is when a file is considered old enough to re-download at
	// start. It is shorter than refreshInterval so a restart after a day picks
	// up the new file immediately instead of waiting another full period.
	staleAfter = 20 * time.Hour
)

// Status is the JSON-shaped view the settings card renders. Country is filled by
// the caller, which is the only layer that knows the server's public address.
type Status struct {
	Available    bool   `json:"available"`
	Country      string `json:"country,omitempty"`
	Source       string `json:"source"`
	Path         string `json:"path"`
	DatabaseDate string `json:"database_date,omitempty"`
	UpdatedAt    string `json:"updated_at,omitempty"`
	Error        string `json:"error,omitempty"`
}

// Service owns one database file and the reader currently answering from it.
type Service struct {
	dir    string
	source string
	client *http.Client

	// updateMu serialises downloads. A manual "Update now" and the daily loop
	// can otherwise race for the same temp file.
	updateMu sync.Mutex
	etag     string

	reader     atomic.Pointer[maxminddb.Reader]
	buildEpoch atomic.Int64
	updatedAt  atomic.Int64
	lastErr    atomic.Pointer[string]
}

// New creates a service rooted at dir. source may be empty to disable updates
// (a hand-placed file still loads and answers lookups).
func New(dir, source string) *Service {
	return &Service{
		dir:    dir,
		source: source,
		client: &http.Client{Timeout: 75 * time.Second},
	}
}

// Path is where the database lives on disk.
func (s *Service) Path() string {
	return filepath.Join(s.dir, FileName)
}

// Load reads the on-disk database, if there is one. A failure is reported but is
// not fatal: the rest of the daemon works, and the next update can create the
// file. It is called once at startup.
func (s *Service) Load() error {
	path := s.Path()
	body, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("geoip: read %s: %w", path, err)
	}
	reader, err := maxminddb.FromBytes(body)
	if err != nil {
		return fmt.Errorf("geoip: parse %s: %w", path, err)
	}
	s.reader.Store(reader)
	s.buildEpoch.Store(int64(reader.Metadata.BuildEpoch))
	if info, statErr := os.Stat(path); statErr == nil {
		s.updatedAt.Store(info.ModTime().Unix())
	}
	if etag, readErr := os.ReadFile(path + ".etag"); readErr == nil {
		s.etag = strings.TrimSpace(string(etag))
	}
	return nil
}

// Country returns the ISO 3166-1 alpha-2 code for ip, or "" when the address is
// unknown, private, or no database is loaded.
func (s *Service) Country(ip net.IP) string {
	reader := s.reader.Load()
	if reader == nil || ip == nil {
		return ""
	}
	var record struct {
		Country struct {
			ISOCode string `maxminddb:"iso_code"`
		} `maxminddb:"country"`
	}
	if err := reader.Lookup(ip, &record); err != nil {
		return ""
	}
	code := strings.ToUpper(strings.TrimSpace(record.Country.ISOCode))
	if len(code) != 2 {
		return ""
	}
	return code
}

// Status reports the current database without touching it.
func (s *Service) Status() Status {
	status := Status{
		Available: s.reader.Load() != nil,
		Source:    s.source,
		Path:      s.Path(),
	}
	if err := s.lastErr.Load(); err != nil {
		status.Error = *err
	}
	if epoch := s.buildEpoch.Load(); epoch > 0 {
		status.DatabaseDate = time.Unix(epoch, 0).UTC().Format("02.01.2006")
	}
	if updated := s.updatedAt.Load(); updated > 0 {
		status.UpdatedAt = time.Unix(updated, 0).UTC().Format(time.RFC3339)
	}
	return status
}

// Update downloads the source and swaps it in. A 304 leaves everything as it is;
// a failed download keeps the previous database and records the error for the
// status card. Safe to call concurrently with lookups and other updates.
func (s *Service) Update(ctx context.Context) error {
	if s.source == "" {
		return errors.New("geoip: no update source configured")
	}

	s.updateMu.Lock()
	defer s.updateMu.Unlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.source, nil)
	if err != nil {
		return s.fail(fmt.Errorf("geoip: build request: %w", err))
	}
	req.Header.Set("Accept", "application/octet-stream")
	if s.etag != "" {
		req.Header.Set("If-None-Match", s.etag)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return s.fail(fmt.Errorf("geoip: download: %w", err))
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotModified {
		s.clearErr()
		return nil
	}
	if resp.StatusCode != http.StatusOK {
		return s.fail(fmt.Errorf("geoip: source answered %s", resp.Status))
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxDownload+1))
	if err != nil {
		return s.fail(fmt.Errorf("geoip: read body: %w", err))
	}
	if len(body) > maxDownload {
		return s.fail(fmt.Errorf("geoip: database is larger than %d bytes", maxDownload))
	}

	// Parse before persisting: a captive-portal HTML page must never replace a
	// working database, and FromBytes validates the whole structure.
	reader, err := maxminddb.FromBytes(body)
	if err != nil {
		return s.fail(fmt.Errorf("geoip: invalid database: %w", err))
	}

	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return s.fail(fmt.Errorf("geoip: create %s: %w", s.dir, err))
	}
	path := s.Path()
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, body, 0o644); err != nil {
		return s.fail(fmt.Errorf("geoip: write %s: %w", tmp, err))
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return s.fail(fmt.Errorf("geoip: replace %s: %w", path, err))
	}

	s.etag = resp.Header.Get("ETag")
	if s.etag != "" {
		_ = os.WriteFile(path+".etag", []byte(s.etag), 0o644)
	}
	s.reader.Store(reader)
	s.buildEpoch.Store(int64(reader.Metadata.BuildEpoch))
	s.updatedAt.Store(time.Now().Unix())
	s.clearErr()

	log.Printf("[GeoIP] database updated from %s (%s)", s.source, s.Status().DatabaseDate)
	return nil
}

// StartUpdater refreshes the database in the background. It returns immediately;
// the goroutine stops when ctx is cancelled. Nothing it does is fatal.
func (s *Service) StartUpdater(ctx context.Context) {
	if s.source == "" {
		return
	}
	go func() {
		select {
		case <-time.After(initialDelay):
		case <-ctx.Done():
			return
		}
		for {
			if s.stale() {
				if err := s.Update(ctx); err != nil && ctx.Err() == nil {
					log.Printf("[GeoIP] update failed: %v", err)
				}
			}
			select {
			case <-time.After(refreshInterval):
			case <-ctx.Done():
				return
			}
		}
	}()
}

// stale reports whether the file should be re-fetched on this pass.
func (s *Service) stale() bool {
	if s.reader.Load() == nil {
		return true
	}
	updated := s.updatedAt.Load()
	return updated == 0 || time.Since(time.Unix(updated, 0)) > staleAfter
}

func (s *Service) fail(err error) error {
	message := err.Error()
	s.lastErr.Store(&message)
	return err
}

func (s *Service) clearErr() {
	s.lastErr.Store(nil)
}
