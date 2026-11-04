package service

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"digitdns/internal/blocklist"
	"digitdns/internal/core/matcher"
	"digitdns/internal/database"
	"digitdns/internal/filterpresets"
)

const (
	filterFetchTimeout   = 30 * time.Second
	filterMaxBytes       = 64 << 20
	filterUpdateCheck    = 15 * time.Minute
	filterListNameMax    = 80
	filterUploadMaxBytes = 16 << 20
)

// FilteringStatus is the dashboard/API view of the filter subsystem.
type FilteringStatus struct {
	Settings            database.FilteringSettings `json:"settings"`
	Lists               []database.FilterList      `json:"lists"`
	Presets             []filterpresets.Preset     `json:"presets"`
	PresetCounts        []database.PresetCount     `json:"preset_counts"`
	PresetCountsRunning bool                       `json:"preset_counts_running"`
	TotalRules          int                        `json:"total_rules"`
	TotalDomains        int                        `json:"total_domains"`
	Updating            []string                   `json:"updating"`
}

// PresetSkip explains why a selected preset was not installed.
type PresetSkip struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

// PresetAddResult is the batch add response for the preset picker.
type PresetAddResult struct {
	Added   []database.FilterList `json:"added"`
	Skipped []PresetSkip          `json:"skipped"`
	Failed  []PresetSkip          `json:"failed"`
}

var ErrDuplicateFilterList = fmt.Errorf("list URL is already configured")

// FilterService owns blocklist downloads, the compiled snapshot and the policy.
// Downloads and parsing happen in background goroutines; the DNS hot path only
// reads the immutable index installed in the matcher.
type FilterService struct {
	db      *database.DB
	matcher *matcher.Matcher
	client  *http.Client

	settings atomic.Pointer[database.FilteringSettings]
	rules    atomic.Int64
	// domains is the number of unique blocked names in the compiled index.
	// It is not the sum of per-list domain_count values: two lists can contain
	// the same name, and the DNS matcher only stores it once.
	domains atomic.Int64

	buildMu  sync.Mutex
	updateMu sync.Mutex
	stateMu  sync.RWMutex
	updating map[string]bool

	// saveMu guards the final write of a background refresh against a delete.
	// Without it, deleting a list while its first download is still running can
	// let the refresh save the record back after the delete removed it.
	saveMu  sync.Mutex
	deleted map[string]bool

	countMu      sync.RWMutex
	presetCounts map[string]database.PresetCount
	countRunning atomic.Bool

	stop      chan struct{}
	done      chan struct{}
	closeOnce sync.Once
	started   atomic.Bool
	jobs      sync.WaitGroup
}

func NewFilterService(db *database.DB, m *matcher.Matcher) *FilterService {
	return &FilterService{
		db:           db,
		matcher:      m,
		updating:     make(map[string]bool),
		deleted:      make(map[string]bool),
		presetCounts: make(map[string]database.PresetCount),
		stop:         make(chan struct{}),
		done:         make(chan struct{}),
		client: &http.Client{
			Timeout: filterFetchTimeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 5 {
					return fmt.Errorf("too many redirects")
				}
				// A public list URL must not redirect the updater onto an
				// internal address.
				return validateListURL(req.URL.String())
			},
		},
	}
}

// Start loads policy, publishes the first snapshot asynchronously and starts the
// periodic updater. DNS is already accepting while the snapshot builds.
func (s *FilterService) Start() {
	if !s.started.CompareAndSwap(false, true) {
		return
	}
	settings, err := s.db.LoadFilteringSettings()
	if err != nil {
		log.Printf("[Filters] could not read filtering settings; using defaults: %v", err)
		settings = database.DefaultFilteringSettings()
	}
	settings = settings.Normalize()
	s.settings.Store(&settings)
	if counts, err := s.db.ListPresetCounts(); err == nil {
		s.countMu.Lock()
		for _, count := range counts {
			s.presetCounts[count.PresetID] = count
		}
		s.countMu.Unlock()
	}
	s.async(s.rebuild)
	go s.loop()
	log.Printf("[Filters] filtering=%v mode=%s ttl=%ds interval=%dh", settings.Enabled, settings.BlockMode, settings.BlockTTL, settings.UpdateIntervalHours)
}

func (s *FilterService) Close() {
	if s == nil || !s.started.Load() {
		return
	}
	s.closeOnce.Do(func() {
		close(s.stop)
		<-s.done
		s.jobs.Wait()
	})
}

func (s *FilterService) loop() {
	ticker := time.NewTicker(filterUpdateCheck)
	defer ticker.Stop()
	defer close(s.done)
	for {
		select {
		case <-s.stop:
			return
		case <-ticker.C:
			s.refreshDue()
		}
	}
}

// BlockPolicy is read by the DNS handler for every blocked answer. It reads an
// atomic settings pointer, not the database.
func (s *FilterService) BlockPolicy() (mode string, ttl uint32) {
	if s == nil {
		return database.FilterBlockNullIP, database.DefaultFilteringBlockTTL
	}
	settings := s.settings.Load()
	if settings == nil {
		return database.FilterBlockNullIP, database.DefaultFilteringBlockTTL
	}
	return settings.BlockMode, uint32(settings.BlockTTL)
}

func (s *FilterService) Settings() database.FilteringSettings {
	if settings := s.settings.Load(); settings != nil {
		return *settings
	}
	return database.DefaultFilteringSettings()
}

func (s *FilterService) SaveSettings(settings database.FilteringSettings) error {
	if err := settings.Validate(); err != nil {
		return err
	}
	settings = settings.Normalize()
	if err := s.db.SaveFilteringSettings(settings); err != nil {
		return err
	}
	s.settings.Store(&settings)
	s.async(s.rebuild)
	return nil
}

func (s *FilterService) Lists() ([]database.FilterList, error) {
	return s.db.ListFilterLists()
}

func (s *FilterService) Status() (FilteringStatus, error) {
	lists, err := s.db.ListFilterLists()
	if err != nil {
		return FilteringStatus{}, err
	}
	s.stateMu.RLock()
	updating := make([]string, 0, len(s.updating))
	for id, active := range s.updating {
		if active {
			updating = append(updating, id)
		}
	}
	s.stateMu.RUnlock()
	s.countMu.RLock()
	counts := make([]database.PresetCount, 0, len(s.presetCounts))
	for _, count := range s.presetCounts {
		counts = append(counts, count)
	}
	s.countMu.RUnlock()
	return FilteringStatus{
		Settings:            s.Settings(),
		Lists:               lists,
		Presets:             filterpresets.All(),
		PresetCounts:        counts,
		PresetCountsRunning: s.countRunning.Load(),
		TotalRules:          int(s.rules.Load()),
		TotalDomains:        int(s.domains.Load()),
		Updating:            updating,
	}, nil
}

// AddList creates a source and starts its first download/build in the background.
func (s *FilterService) AddList(name, listURL, content string) (database.FilterList, error) {
	return s.AddListWithMeta(name, listURL, content, "", "", "")
}

// AddListWithMeta is AddList plus the preset/homepage metadata shown in the
// installed-list row. Duplicate source URLs are rejected before a second copy is
// downloaded.
func (s *FilterService) AddListWithMeta(name, listURL, content, homepage, license, presetID string) (database.FilterList, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = strings.TrimSpace(listURL)
	}
	if name == "" {
		name = "Uploaded list"
	}
	if len(name) > filterListNameMax {
		name = name[:filterListNameMax]
	}
	listURL = strings.TrimSpace(listURL)
	if listURL != "" {
		if err := validateListURL(listURL); err != nil {
			return database.FilterList{}, err
		}
		lists, _ := s.db.ListFilterLists()
		for _, existing := range lists {
			if existing.URL == listURL {
				return existing, ErrDuplicateFilterList
			}
		}
	}
	homepage = strings.TrimSpace(homepage)
	if homepage != "" {
		u, err := url.Parse(homepage)
		if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
			return database.FilterList{}, fmt.Errorf("homepage must be an http or https URL")
		}
	}
	if listURL == "" && strings.TrimSpace(content) == "" {
		return database.FilterList{}, fmt.Errorf("a list URL or an uploaded file is required")
	}
	if len(content) > filterUploadMaxBytes {
		return database.FilterList{}, fmt.Errorf("uploaded list is larger than %d bytes", filterUploadMaxBytes)
	}
	id, err := newFilterID()
	if err != nil {
		return database.FilterList{}, err
	}
	list := database.FilterList{
		ID:             id,
		Name:           name,
		URL:            listURL,
		Homepage:       homepage,
		License:        strings.TrimSpace(license),
		PresetID:       strings.TrimSpace(presetID),
		Enabled:        true,
		Format:         "auto",
		SourceUploaded: content != "",
	}
	if err := s.db.SaveFilterList(list); err != nil {
		return database.FilterList{}, err
	}
	if content != "" {
		if err := s.db.SaveFilterSource(id, content); err != nil {
			return list, err
		}
	}
	s.async(func() { s.refreshOne(id, content) })
	return list, nil
}

// EnsurePresetCounts starts a background scan for catalogue rows that are not
// installed and whose cached domain count is stale. The page keeps serving with
// "counting…" placeholders; DNS never waits for a catalogue download.
func (s *FilterService) EnsurePresetCounts() {
	if s == nil || s.countRunning.Load() {
		return
	}
	lists, err := s.db.ListFilterLists()
	if err != nil {
		return
	}
	installed := make(map[string]bool, len(lists))
	for _, list := range lists {
		if list.URL != "" {
			installed[list.URL] = true
		}
	}
	s.countMu.RLock()
	stale := false
	for _, preset := range filterpresets.All() {
		if installed[preset.URL] {
			continue
		}
		count, ok := s.presetCounts[preset.ID]
		if !ok || count.LastUpdated.IsZero() || time.Since(count.LastUpdated) >= 24*time.Hour {
			stale = true
			break
		}
		if count.LastError != "" && time.Since(count.LastUpdated) >= time.Hour {
			stale = true
			break
		}
	}
	s.countMu.RUnlock()
	if !stale || !s.countRunning.CompareAndSwap(false, true) {
		return
	}
	s.async(func() {
		defer s.countRunning.Store(false)
		s.scanPresetCounts(installed)
	})
}

func (s *FilterService) scanPresetCounts(installed map[string]bool) {
	queue := make([]filterpresets.Preset, 0, len(filterpresets.All()))
	for _, preset := range filterpresets.All() {
		if installed[preset.URL] {
			continue
		}
		count, ok := s.getPresetCount(preset.ID)
		if ok && !count.LastUpdated.IsZero() && time.Since(count.LastUpdated) < 24*time.Hour && count.LastError == "" {
			continue
		}
		if ok && count.LastError != "" && time.Since(count.LastUpdated) < time.Hour {
			continue
		}
		queue = append(queue, preset)
	}
	sem := make(chan struct{}, 2)
	var wg sync.WaitGroup
	for _, preset := range queue {
		wg.Add(1)
		go func(p filterpresets.Preset) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			s.countPreset(p)
		}(preset)
	}
	wg.Wait()
}

func (s *FilterService) countPreset(preset filterpresets.Preset) {
	existing, _ := s.getPresetCount(preset.ID)
	body, etag, lastModified, notModified, err := s.fetch(preset.URL, existing.ETag, existing.LastModified)
	count := database.PresetCount{PresetID: preset.ID, ETag: existing.ETag, LastModified: existing.LastModified}
	if err != nil {
		count.LastUpdated = time.Now()
		count.LastError = err.Error()
		s.savePresetCount(count)
		return
	}
	if notModified {
		count = existing
		count.LastUpdated = time.Now()
		count.LastError = ""
		s.savePresetCount(count)
		return
	}
	stats := blocklist.Analyze(body)
	count.DomainCount = stats.Domains
	count.RuleCount = stats.Blocked
	count.Bytes = len(body)
	count.LastUpdated = time.Now()
	count.LastError = ""
	count.ETag = etag
	count.LastModified = lastModified
	s.savePresetCount(count)
}

func (s *FilterService) getPresetCount(id string) (database.PresetCount, bool) {
	s.countMu.RLock()
	defer s.countMu.RUnlock()
	count, ok := s.presetCounts[id]
	return count, ok
}

func (s *FilterService) savePresetCount(count database.PresetCount) {
	s.countMu.Lock()
	s.presetCounts[count.PresetID] = count
	s.countMu.Unlock()
	_ = s.db.SavePresetCount(count)
}

// AddPresets installs the selected catalogue entries in one batch. A preset
// already present by source URL is skipped rather than downloaded twice.
func (s *FilterService) AddPresets(ids []string) PresetAddResult {
	var result PresetAddResult
	for _, id := range ids {
		preset, ok := filterpresets.ByID(id)
		if !ok {
			result.Failed = append(result.Failed, PresetSkip{ID: id, Reason: "unknown preset"})
			continue
		}
		list, err := s.AddListWithMeta(preset.Name, preset.URL, "", preset.Homepage, preset.License, preset.ID)
		switch {
		case err == nil:
			result.Added = append(result.Added, list)
		case err == ErrDuplicateFilterList:
			result.Skipped = append(result.Skipped, PresetSkip{ID: id, Name: preset.Name, Reason: "already installed"})
		default:
			result.Failed = append(result.Failed, PresetSkip{ID: id, Name: preset.Name, Reason: err.Error()})
		}
	}
	return result
}

func (s *FilterService) UpdateList(id string) {
	s.async(func() { s.refreshOne(id, "") })
}

func (s *FilterService) DeleteList(id string) error {
	s.saveMu.Lock()
	s.deleted[id] = true
	err := s.db.DeleteFilterList(id)
	s.saveMu.Unlock()
	if err != nil {
		s.saveMu.Lock()
		delete(s.deleted, id)
		s.saveMu.Unlock()
		return err
	}
	s.async(s.rebuild)
	return nil
}

// saveRefreshedList writes a background refresh only while the list still
// exists. DeleteList takes the same lock, so a delete either happens before the
// save (and wins) or after it (and removes what was just written).
func (s *FilterService) saveRefreshedList(list database.FilterList) error {
	s.saveMu.Lock()
	defer s.saveMu.Unlock()
	if s.deleted[list.ID] {
		return nil
	}
	return s.db.SaveFilterList(list)
}

func (s *FilterService) saveRefreshedListAndSource(list database.FilterList, text string) error {
	s.saveMu.Lock()
	defer s.saveMu.Unlock()
	if s.deleted[list.ID] {
		return nil
	}
	if text != "" {
		if err := s.db.SaveFilterSource(list.ID, text); err != nil {
			return err
		}
	}
	return s.db.SaveFilterList(list)
}

func (s *FilterService) SetListEnabled(id string, enabled bool) error {
	list, err := s.db.GetFilterList(id)
	if err != nil {
		return err
	}
	if list == nil {
		return fmt.Errorf("filter list not found")
	}
	list.Enabled = enabled
	if err := s.db.SaveFilterList(*list); err != nil {
		return err
	}
	s.async(s.rebuild)
	return nil
}

func (s *FilterService) RefreshAll() {
	s.async(func() {
		lists, err := s.db.ListFilterLists()
		if err != nil {
			log.Printf("[Filters] refresh all: %v", err)
			return
		}
		for _, list := range lists {
			if list.Enabled {
				s.refreshOne(list.ID, "")
			}
		}
	})
}

// async tracks background work so Close can drain it before the database closes.
func (s *FilterService) async(fn func()) {
	s.jobs.Add(1)
	go func() {
		defer s.jobs.Done()
		fn()
	}()
}

func (s *FilterService) refreshDue() {
	lists, err := s.db.ListFilterLists()
	if err != nil {
		return
	}
	interval := time.Duration(s.Settings().UpdateIntervalHours) * time.Hour
	now := time.Now()
	for _, list := range lists {
		if !list.Enabled || list.URL == "" {
			continue
		}
		if list.LastUpdated.IsZero() || now.Sub(list.LastUpdated) >= interval {
			s.refreshOne(list.ID, "")
		}
	}
}

func (s *FilterService) refreshOne(id, inlineContent string) {
	s.updateMu.Lock()
	defer s.updateMu.Unlock()

	list, err := s.db.GetFilterList(id)
	if err != nil || list == nil {
		return
	}
	s.stateMu.Lock()
	s.updating[id] = true
	s.stateMu.Unlock()
	defer func() {
		s.stateMu.Lock()
		delete(s.updating, id)
		s.stateMu.Unlock()
	}()

	text := inlineContent
	if text == "" {
		text, _ = s.db.GetFilterSource(id)
	}
	etag, lastModified := list.ETag, list.LastModified

	if list.URL != "" {
		body, newETag, newLastModified, notModified, err := s.fetch(list.URL, etag, lastModified)
		if err != nil {
			list.LastError = err.Error()
			_ = s.saveRefreshedList(*list)
			log.Printf("[Filters] %s: %v", list.Name, err)
			return
		}
		if notModified {
			// A conditional refresh still needs to backfill counts introduced by
			// newer code; the stored source is available and parsing it does not
			// touch the live matcher.
			if strings.TrimSpace(text) != "" {
				stats := blocklist.Analyze(text)
				list.RuleCount = stats.Blocked
				list.DomainCount = stats.Domains
				list.AllowedCount = stats.Allowed
				list.Skipped = stats.Skipped
				list.Bytes = len(text)
			}
			list.LastUpdated = time.Now()
			list.LastError = ""
			_ = s.saveRefreshedList(*list)
			return
		}
		text = body
		list.ETag = newETag
		list.LastModified = newLastModified
	}
	if strings.TrimSpace(text) == "" {
		list.LastError = "list is empty"
		_ = s.saveRefreshedList(*list)
		return
	}

	stats := blocklist.Analyze(text)
	list.RuleCount = stats.Blocked
	list.DomainCount = stats.Domains
	list.AllowedCount = stats.Allowed
	list.Skipped = stats.Skipped
	list.Bytes = len(text)
	list.LastUpdated = time.Now()
	list.LastError = ""
	if err := s.saveRefreshedListAndSource(*list, text); err != nil {
		list.LastError = err.Error()
		_ = s.saveRefreshedList(*list)
		log.Printf("[Filters] save %s: %v", list.Name, err)
		return
	}
	s.rebuild()
}

func (s *FilterService) rebuild() {
	s.buildMu.Lock()
	defer s.buildMu.Unlock()

	settings, err := s.db.LoadFilteringSettings()
	if err != nil {
		log.Printf("[Filters] rebuild settings: %v", err)
		return
	}
	settings = settings.Normalize()
	s.settings.Store(&settings)
	if !settings.Enabled {
		s.matcher.SetBlockIndex(nil)
		s.rules.Store(0)
		s.domains.Store(0)
		return
	}
	lists, err := s.db.ListFilterLists()
	if err != nil {
		log.Printf("[Filters] rebuild lists: %v", err)
		return
	}
	sources := make([]blocklist.Source, 0, len(lists))
	for _, list := range lists {
		if !list.Enabled {
			continue
		}
		text, err := s.db.GetFilterSource(list.ID)
		if err != nil || strings.TrimSpace(text) == "" {
			continue
		}
		sources = append(sources, blocklist.Source{Name: list.Name, Text: text, Enabled: true})
	}
	index, _ := blocklist.Build(sources, settings.Allowlist)
	count := index.RuleCount()
	domains := index.DomainCount()
	if count == 0 {
		// No rules means no extra lookup on the query path. Allowlist-only
		// configurations also land here.
		s.matcher.SetBlockIndex(nil)
		s.rules.Store(0)
		s.domains.Store(0)
		return
	}
	s.matcher.SetBlockIndex(index)
	s.rules.Store(int64(count))
	s.domains.Store(int64(domains))
	log.Printf("[Filters] compiled %d rules / %d unique domains from %d list(s)", count, domains, len(sources))
}

func (s *FilterService) fetch(rawURL, etag, lastModified string) (body, newETag, newLastModified string, notModified bool, err error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return "", "", "", false, err
	}
	req.Header.Set("User-Agent", "DigitDNS-FilterUpdater/1.0")
	req.Header.Set("Accept", "text/plain, */*;q=0.5")
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	if lastModified != "" {
		req.Header.Set("If-Modified-Since", lastModified)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return "", "", "", false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotModified {
		return "", etag, lastModified, true, nil
	}
	if resp.StatusCode != http.StatusOK {
		return "", "", "", false, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, filterMaxBytes+1))
	if err != nil {
		return "", "", "", false, err
	}
	if len(data) > filterMaxBytes {
		return "", "", "", false, fmt.Errorf("list is larger than %d bytes", filterMaxBytes)
	}
	return string(data), resp.Header.Get("ETag"), resp.Header.Get("Last-Modified"), false, nil
}

func validateListURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return fmt.Errorf("invalid list URL")
	}
	if u.Scheme != "https" {
		return fmt.Errorf("list URLs must use HTTPS")
	}
	host := u.Hostname()
	ips, err := net.LookupIP(host)
	if err != nil || len(ips) == 0 {
		return fmt.Errorf("could not resolve list host")
	}
	for _, ip := range ips {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() {
			return fmt.Errorf("list URL must resolve to a public address")
		}
	}
	return nil
}

func newFilterID() (string, error) {
	raw := make([]byte, 8)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}
