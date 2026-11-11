package database

import (
	"slices"
	"testing"

	"digitdns/internal/core/resolution"
)

func TestNormalizeResolutionMigratesRetiredStrategies(t *testing.T) {
	cases := []struct {
		name       string
		settings   DNSSettings
		want       string
		wantChange bool
	}{
		{
			name:       "empty becomes the adaptive default",
			settings:   DNSSettings{FastestRacing: true},
			want:       resolution.StrategyAdaptive,
			wantChange: true,
		},
		{
			name:       "empty ignores the retired racing switch",
			settings:   DNSSettings{FastestRacing: false},
			want:       resolution.StrategyAdaptive,
			wantChange: true,
		},
		{
			name:       "fastest becomes hedged",
			settings:   DNSSettings{DefaultStrategy: resolution.StrategyFastest},
			want:       resolution.StrategyHedged,
			wantChange: true,
		},
		{
			name:       "explicit strategy is kept",
			settings:   DNSSettings{DefaultStrategy: resolution.StrategyBestKnown},
			want:       resolution.StrategyBestKnown,
			wantChange: true, // transports are also initialised
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			changed := c.settings.NormalizeResolution()
			if c.settings.DefaultStrategy != c.want {
				t.Fatalf("strategy = %q, want %q", c.settings.DefaultStrategy, c.want)
			}
			if changed != c.wantChange {
				t.Fatalf("changed = %v, want %v", changed, c.wantChange)
			}
		})
	}
}

func TestNormalizeResolutionMigratesTransportChips(t *testing.T) {
	cases := []struct {
		name       string
		settings   DNSSettings
		want       []string
		wantLegacy string
	}{
		{
			name:       "any becomes all protocols",
			settings:   DNSSettings{},
			want:       resolution.AllProtocols,
			wantLegacy: resolution.TransportAny,
		},
		{
			name:       "encrypted becomes encrypted protocols",
			settings:   DNSSettings{Transport: resolution.TransportEncrypted},
			want:       []string{resolution.ProtocolDoT, resolution.ProtocolDoH, resolution.ProtocolDoH3, resolution.ProtocolDoQ, resolution.ProtocolDNSCrypt},
			wantLegacy: resolution.TransportEncrypted,
		},
		{
			name:       "regular becomes plain",
			settings:   DNSSettings{Transport: resolution.TransportRegular},
			want:       []string{resolution.ProtocolPlain},
			wantLegacy: resolution.TransportRegular,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			c.settings.NormalizeResolution()
			if !slices.Equal(c.settings.Transports, c.want) {
				t.Fatalf("transports = %v, want %v", c.settings.Transports, c.want)
			}
			if c.settings.Transport != c.wantLegacy {
				t.Fatalf("legacy transport = %q, want %q", c.settings.Transport, c.wantLegacy)
			}
		})
	}
}

func TestNormalizeResolutionFillsAdaptiveDefaultsAndProfiles(t *testing.T) {
	settings := DNSSettings{
		DefaultStrategy: resolution.StrategyAdaptive,
		Profiles: map[string]resolution.Profile{
			"old.example": {Strategy: resolution.StrategyFastest},
			"new.example": {Strategy: resolution.StrategyAdaptive},
		},
	}
	if !settings.NormalizeResolution() {
		t.Fatal("NormalizeResolution reported no change")
	}
	if settings.AdaptiveIntervalMinutes != resolution.DefaultAdaptiveIntervalMinutes {
		t.Fatalf("adaptive interval = %d, want %d", settings.AdaptiveIntervalMinutes, resolution.DefaultAdaptiveIntervalMinutes)
	}
	if got := settings.Profiles["old.example"].Strategy; got != resolution.StrategyHedged {
		t.Fatalf("legacy profile strategy = %q, want %q", got, resolution.StrategyHedged)
	}
	if got := settings.Profiles["new.example"].Strategy; got != "" {
		t.Fatalf("adaptive profile strategy = %q, want inherit", got)
	}
}
