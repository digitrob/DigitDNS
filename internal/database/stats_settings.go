package database

import (
	"fmt"
	"strings"
	"unicode"
)

// StatsSettings controls the optional 90-day aggregate store. It is persisted
// under its own "stats" settings key so adding it does not widen ServerSettings
// or its mutable-field lock.
type StatsSettings struct {
	Enabled          bool     `json:"enabled"`
	RetentionDays    int      `json:"retention_days"`
	TopDomains       int      `json:"top_domains"`
	TopClients       int      `json:"top_clients"`
	AnonymizeClients bool     `json:"anonymize_clients"`
	ExcludeClients   []string `json:"exclude_clients"`
}

const (
	DefaultStatsRetentionDays = 90
	MinStatsRetentionDays     = 7
	MaxStatsRetentionDays     = 365
	DefaultStatsTopDomains    = 1000
	MinStatsTopDomains        = 100
	MaxStatsTopDomains        = 10000
	DefaultStatsTopClients    = 500
	MinStatsTopClients        = 50
	MaxStatsTopClients        = 5000
	// MaxStatsExcludedClients bounds the opt-out list so a hand-edited record
	// cannot turn the per-query lookup into an unbounded scan.
	MaxStatsExcludedClients = 500
	// MaxStatsClientKeyLen matches the longest DNS name/account key the
	// dashboard uses; anything longer is not a client identifier.
	MaxStatsClientKeyLen = 253
)

func DefaultStatsSettings() StatsSettings {
	return StatsSettings{
		Enabled:       true,
		RetentionDays: DefaultStatsRetentionDays,
		TopDomains:    DefaultStatsTopDomains,
		TopClients:    DefaultStatsTopClients,
	}
}

// Validate checks the values a client submitted before Normalize clamps them.
// Both the dashboard handler and the v3 handler call it so a mobile or API
// client cannot store a value the panel would reject.
func (s StatsSettings) Validate() error {
	if s.RetentionDays < MinStatsRetentionDays || s.RetentionDays > MaxStatsRetentionDays {
		return fmt.Errorf("retention_days must be between %d and %d", MinStatsRetentionDays, MaxStatsRetentionDays)
	}
	if s.TopDomains < MinStatsTopDomains || s.TopDomains > MaxStatsTopDomains {
		return fmt.Errorf("top_domains must be between %d and %d", MinStatsTopDomains, MaxStatsTopDomains)
	}
	if s.TopClients < MinStatsTopClients || s.TopClients > MaxStatsTopClients {
		return fmt.Errorf("top_clients must be between %d and %d", MinStatsTopClients, MaxStatsTopClients)
	}
	if len(s.ExcludeClients) > MaxStatsExcludedClients {
		return fmt.Errorf("exclude_clients cannot contain more than %d entries", MaxStatsExcludedClients)
	}
	for _, raw := range s.ExcludeClients {
		client := strings.TrimSpace(raw)
		if len(client) > MaxStatsClientKeyLen {
			return fmt.Errorf("exclude_clients entries must be %d characters or fewer", MaxStatsClientKeyLen)
		}
		for _, r := range client {
			if unicode.IsControl(r) {
				return fmt.Errorf("exclude_clients entries must not contain control characters")
			}
		}
	}
	return nil
}

// Normalize fills absent fields with defaults and clamps hand-edited values to
// the ranges the settings endpoint accepts. The daemon must still boot with a
// usable store if a config or database record was written by hand.
func (s StatsSettings) Normalize() StatsSettings {
	if s.RetentionDays == 0 {
		s.RetentionDays = DefaultStatsRetentionDays
	}
	if s.TopDomains == 0 {
		s.TopDomains = DefaultStatsTopDomains
	}
	if s.TopClients == 0 {
		s.TopClients = DefaultStatsTopClients
	}
	s.RetentionDays = clampInt(s.RetentionDays, MinStatsRetentionDays, MaxStatsRetentionDays)
	s.TopDomains = clampInt(s.TopDomains, MinStatsTopDomains, MaxStatsTopDomains)
	s.TopClients = clampInt(s.TopClients, MinStatsTopClients, MaxStatsTopClients)
	s.ExcludeClients = NormalizeExcludedClients(s.ExcludeClients)
	return s
}

// NormalizeExcludedClients trims, drops blanks and duplicates, and caps the
// list. It preserves case because account names are matched verbatim.
func NormalizeExcludedClients(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	seen := make(map[string]struct{}, len(in))
	for _, raw := range in {
		client := strings.TrimSpace(raw)
		if client == "" {
			continue
		}
		if len(client) > MaxStatsClientKeyLen {
			client = client[:MaxStatsClientKeyLen]
		}
		if _, ok := seen[client]; ok {
			continue
		}
		seen[client] = struct{}{}
		out = append(out, client)
		if len(out) == MaxStatsExcludedClients {
			break
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func clampInt(value, min, max int) int {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func (db *DB) LoadStatsSettings() (StatsSettings, error) {
	out := DefaultStatsSettings()
	if err := db.GetSetting("stats", &out); err != nil {
		return DefaultStatsSettings(), err
	}
	return out.Normalize(), nil
}

func (db *DB) SaveStatsSettings(s StatsSettings) error {
	return db.SetSetting("stats", s.Normalize())
}
