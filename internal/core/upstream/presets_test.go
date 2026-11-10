package upstream

import (
	"net"
	"strings"
	"testing"
)

func TestPresetCatalogIsCompleteAndCanonical(t *testing.T) {
	providers := PresetProviders()
	if len(providers) != 10 {
		t.Fatalf("preset providers = %d, want 10", len(providers))
	}

	wantNames := map[string]bool{
		"cloudflare":    true,
		"google":        true,
		"quad9":         true,
		"opendns":       true,
		"adguard":       true,
		"controld":      true,
		"dnssb":         true,
		"yandex":        true,
		"nextdns":       true,
		"cleanbrowsing": true,
	}
	seen := make(map[string]bool)
	addresses := make(map[string]string)
	for _, provider := range providers {
		if provider.ID == "" || provider.Name == "" {
			t.Errorf("provider has an empty id or name: %+v", provider)
		}
		if !wantNames[provider.ID] {
			t.Errorf("unexpected provider id %q", provider.ID)
		}
		delete(wantNames, provider.ID)
		if len(provider.Servers) == 0 {
			t.Errorf("provider %q has no servers", provider.ID)
		}
		if provider.ID == "nextdns" && provider.Note == "" {
			t.Errorf("NextDNS must explain that encrypted endpoints need a configuration ID")
		}
		for _, server := range provider.Servers {
			if server.Address == "" || server.Label == "" || server.Protocol == "" {
				t.Errorf("%s has an incomplete server: %+v", provider.ID, server)
			}
			if !seen[server.Address] {
				seen[server.Address] = true
			} else {
				t.Errorf("duplicate preset address %q", server.Address)
			}
			if previous, ok := addresses[server.Address]; ok {
				t.Errorf("%q appears under both %q and %q", server.Address, previous, provider.ID)
			}
			addresses[server.Address] = provider.ID
			canonical, ok := NormalizeUpstream(server.Address)
			if !ok {
				t.Errorf("%s/%s: %q does not normalize", provider.ID, server.Label, server.Address)
				continue
			}
			if canonical != server.Address {
				t.Errorf("%s/%s: catalogue stores %q, canonical form is %q", provider.ID, server.Label, server.Address, canonical)
			}
			if !protocolMatchesAddress(server.Protocol, server.Address) {
				t.Errorf("%s/%s: protocol %q does not match %q", provider.ID, server.Label, server.Protocol, server.Address)
			}
		}
	}
	if len(wantNames) != 0 {
		t.Errorf("missing providers: %v", wantNames)
	}
}

func protocolMatchesAddress(protocol, address string) bool {
	switch protocol {
	case "UDP":
		host, port, err := net.SplitHostPort(address)
		return err == nil && host != "" && port == "53" && !strings.Contains(address, "://")
	case "DNSCrypt":
		return strings.HasPrefix(address, "sdns://")
	default:
		return ProtocolOf(address) == protocol
	}
}
