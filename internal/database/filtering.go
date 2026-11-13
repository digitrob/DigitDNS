package database

import (
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	bolt "go.etcd.io/bbolt"
)

// FilteringSettings controls the DNS ad-blocking engine. Lists themselves live
// in FilterList records; this is the policy that applies to every list.
type FilteringSettings struct {
	Enabled             bool     `json:"enabled"`
	BlockMode           string   `json:"block_mode"`
	BlockTTL            int      `json:"block_ttl"`
	UpdateIntervalHours int      `json:"update_interval_hours"`
	Allowlist           []string `json:"allowlist"`
	CustomRules         string   `json:"custom_rules,omitempty"`
}

const (
	FilterBlockNullIP   = "null_ip"
	FilterBlockNXDOMAIN = "nxdomain"
	FilterBlockRefused  = "refused"

	DefaultFilteringBlockTTL     = 10
	DefaultFilteringUpdateHours  = 24
	MinFilteringBlockTTL         = 1
	MaxFilteringBlockTTL         = 3600
	MinFilteringUpdateHours      = 1
	MaxFilteringUpdateHours      = 168
	MaxFilteringAllowlistEntries = 10000
	MaxFilteringCustomRulesBytes = 1 << 20
)

func DefaultFilteringSettings() FilteringSettings {
	return FilteringSettings{
		Enabled:             true,
		BlockMode:           FilterBlockNullIP,
		BlockTTL:            DefaultFilteringBlockTTL,
		UpdateIntervalHours: DefaultFilteringUpdateHours,
	}
}

func (s FilteringSettings) Normalize() FilteringSettings {
	switch s.BlockMode {
	case FilterBlockNullIP, FilterBlockNXDOMAIN, FilterBlockRefused:
	default:
		s.BlockMode = FilterBlockNullIP
	}
	if s.BlockTTL < MinFilteringBlockTTL || s.BlockTTL > MaxFilteringBlockTTL {
		s.BlockTTL = DefaultFilteringBlockTTL
	}
	if s.UpdateIntervalHours < MinFilteringUpdateHours || s.UpdateIntervalHours > MaxFilteringUpdateHours {
		s.UpdateIntervalHours = DefaultFilteringUpdateHours
	}
	s.Allowlist = normalizeDomains(s.Allowlist, MaxFilteringAllowlistEntries)
	s.CustomRules = strings.ToValidUTF8(strings.TrimSpace(s.CustomRules), "")
	if len(s.CustomRules) > MaxFilteringCustomRulesBytes {
		end := MaxFilteringCustomRulesBytes
		for end > 0 && !utf8.RuneStart(s.CustomRules[end]) {
			end--
		}
		s.CustomRules = s.CustomRules[:end]
	}
	return s
}

func (s FilteringSettings) Validate() error {
	switch s.BlockMode {
	case FilterBlockNullIP, FilterBlockNXDOMAIN, FilterBlockRefused:
	default:
		return fmt.Errorf("block_mode must be null_ip, nxdomain or refused")
	}
	if s.BlockTTL < MinFilteringBlockTTL || s.BlockTTL > MaxFilteringBlockTTL {
		return fmt.Errorf("block_ttl must be between %d and %d", MinFilteringBlockTTL, MaxFilteringBlockTTL)
	}
	if s.UpdateIntervalHours < MinFilteringUpdateHours || s.UpdateIntervalHours > MaxFilteringUpdateHours {
		return fmt.Errorf("update_interval_hours must be between %d and %d", MinFilteringUpdateHours, MaxFilteringUpdateHours)
	}
	if len(s.Allowlist) > MaxFilteringAllowlistEntries {
		return fmt.Errorf("allowlist cannot contain more than %d entries", MaxFilteringAllowlistEntries)
	}
	for _, entry := range s.Allowlist {
		if len(strings.TrimSpace(entry)) > 253 {
			return fmt.Errorf("allowlist entries must be 253 characters or fewer")
		}
	}
	return nil
}

func normalizeDomains(in []string, max int) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, raw := range in {
		d := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(raw)), ".")
		if d == "" {
			continue
		}
		if _, ok := seen[d]; ok {
			continue
		}
		seen[d] = struct{}{}
		out = append(out, d)
		if len(out) == max {
			break
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (db *DB) LoadFilteringSettings() (FilteringSettings, error) {
	out := DefaultFilteringSettings()
	if err := db.GetSetting("filtering", &out); err != nil {
		return DefaultFilteringSettings(), err
	}
	return out.Normalize(), nil
}

func (db *DB) SaveFilteringSettings(s FilteringSettings) error {
	return db.SetSetting("filtering", s.Normalize())
}

// FilterList is one subscribed or uploaded rule list. The raw text is stored in
// a separate bucket so listing metadata stays cheap.
type FilterList struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	URL            string    `json:"url"`
	Homepage       string    `json:"homepage,omitempty"`
	License        string    `json:"license,omitempty"`
	PresetID       string    `json:"preset_id,omitempty"`
	Enabled        bool      `json:"enabled"`
	Format         string    `json:"format"`
	LastUpdated    time.Time `json:"last_updated"`
	LastError      string    `json:"last_error"`
	RuleCount      int       `json:"rule_count"`
	DomainCount    int       `json:"domain_count"`
	AllowedCount   int       `json:"allowed_count"`
	Skipped        int       `json:"skipped"`
	Bytes          int       `json:"bytes"`
	ETag           string    `json:"etag,omitempty"`
	LastModified   string    `json:"last_modified,omitempty"`
	SourceUploaded bool      `json:"source_uploaded"`
}

func (db *DB) ListFilterLists() ([]FilterList, error) {
	out := []FilterList{}
	err := db.bolt.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketFilterLists)
		if b == nil {
			return nil
		}
		return b.ForEach(func(_, v []byte) error {
			var item FilterList
			if err := db.decodeSetting(v, &item); err == nil {
				out = append(out, item)
			}
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func (db *DB) GetFilterList(id string) (*FilterList, error) {
	var out FilterList
	found := false
	err := db.bolt.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketFilterLists)
		if b == nil {
			return nil
		}
		raw := b.Get([]byte(id))
		if raw == nil {
			return nil
		}
		found = true
		return db.decodeSetting(raw, &out)
	})
	if err != nil || !found {
		return nil, err
	}
	return &out, nil
}

func (db *DB) SaveFilterList(item FilterList) error {
	data, err := db.encodeSetting(item)
	if err != nil {
		return err
	}
	return db.bolt.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketFilterLists)
		if b == nil {
			return fmt.Errorf("filter lists bucket is missing")
		}
		return b.Put([]byte(item.ID), data)
	})
}

func (db *DB) SaveFilterSource(id, text string) error {
	data, err := db.encodeSetting(text)
	if err != nil {
		return err
	}
	return db.bolt.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketFilterSources)
		if b == nil {
			return fmt.Errorf("filter sources bucket is missing")
		}
		return b.Put([]byte(id), data)
	})
}

func (db *DB) GetFilterSource(id string) (string, error) {
	var out string
	err := db.bolt.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketFilterSources)
		if b == nil {
			return nil
		}
		raw := b.Get([]byte(id))
		if raw == nil {
			return nil
		}
		return db.decodeSetting(raw, &out)
	})
	return out, err
}

func (db *DB) DeleteFilterList(id string) error {
	return db.bolt.Update(func(tx *bolt.Tx) error {
		lists := tx.Bucket(bucketFilterLists)
		sources := tx.Bucket(bucketFilterSources)
		if lists != nil {
			_ = lists.Delete([]byte(id))
		}
		if sources != nil {
			_ = sources.Delete([]byte(id))
		}
		return nil
	})
}

// PresetCount caches a catalogue list's domain count while it is not installed.
type PresetCount struct {
	PresetID     string    `json:"preset_id"`
	DomainCount  int       `json:"domain_count"`
	RuleCount    int       `json:"rule_count"`
	Bytes        int       `json:"bytes"`
	LastUpdated  time.Time `json:"last_updated"`
	LastError    string    `json:"last_error"`
	ETag         string    `json:"etag,omitempty"`
	LastModified string    `json:"last_modified,omitempty"`
}

func (db *DB) ListPresetCounts() ([]PresetCount, error) {
	out := []PresetCount{}
	err := db.bolt.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketPresetCounts)
		if b == nil {
			return nil
		}
		return b.ForEach(func(_, v []byte) error {
			var item PresetCount
			if err := db.decodeSetting(v, &item); err == nil {
				out = append(out, item)
			}
			return nil
		})
	})
	return out, err
}

func (db *DB) SavePresetCount(item PresetCount) error {
	data, err := db.encodeSetting(item)
	if err != nil {
		return err
	}
	return db.bolt.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketPresetCounts)
		if b == nil {
			return fmt.Errorf("preset counts bucket is missing")
		}
		return b.Put([]byte(item.PresetID), data)
	})
}
