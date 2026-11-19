package migrate

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return data
}

func TestDecodeAdGuardFixture(t *testing.T) {
	tree, err := DecodeYAML(readFixture(t, "adguard-home.yaml"))
	if err != nil {
		t.Fatalf("DecodeYAML: %v", err)
	}
	redacted, removed := Redact(tree)

	for _, want := range []string{"users[0].password", "tls", "dhcp", "querylog"} {
		if !slices.Contains(removed, want) {
			t.Errorf("removed = %v, want %q", removed, want)
		}
	}
	if _, ok := tree["tls"]; !ok {
		t.Fatal("DecodeYAML did not parse the tls table")
	}

	filters := mustSlice(t, redacted["filters"])
	if len(filters) != 2 {
		t.Fatalf("filters has %d entries, want 2", len(filters))
	}
	first := mustMap(t, filters[0])
	if first["url"] != "https://lists.example.invalid/ads.txt" {
		t.Errorf("filters[0].url = %v", first["url"])
	}

	rules := mustSlice(t, redacted["user_rules"])
	if len(rules) != 5 {
		t.Fatalf("user_rules has %d entries, want 5", len(rules))
	}
	if rules[0] != "||ads.example^" {
		t.Errorf("user_rules[0] = %v", rules[0])
	}

	dns := mustMap(t, redacted["dns"])
	if got := len(mustSlice(t, dns["upstream_dns"])); got != 2 {
		t.Errorf("dns.upstreams has %d entries, want 2", got)
	}
	if got := len(mustSlice(t, dns["rewrites"])); got != 2 {
		t.Errorf("dns.rewrites has %d entries, want 2", got)
	}

	clients := mustMap(t, redacted["clients"])
	persistent := mustSlice(t, clients["persistent"])
	if len(persistent) != 2 {
		t.Fatalf("clients.persistent has %d entries, want 2", len(persistent))
	}
	if mustMap(t, persistent[0])["name"] != "laptop" {
		t.Errorf("clients.persistent[0].name = %v", mustMap(t, persistent[0])["name"])
	}
}

func TestDecodeAdGuardLegacyClients(t *testing.T) {
	tree, err := DecodeYAML(readFixture(t, "adguard-home-legacy.yaml"))
	if err != nil {
		t.Fatalf("DecodeYAML: %v", err)
	}
	clients, ok := tree["clients"].([]any)
	if !ok {
		t.Fatalf("clients is %T, want the legacy []any form", tree["clients"])
	}
	if len(clients) != 2 {
		t.Fatalf("clients has %d entries, want 2", len(clients))
	}
	if mustMap(t, clients[0])["name"] != "legacy-laptop" {
		t.Errorf("clients[0].name = %v", mustMap(t, clients[0])["name"])
	}
}

func TestDecodePiHoleTOML(t *testing.T) {
	tree, err := DecodeTOML(readFixture(t, "pihole.toml"))
	if err != nil {
		t.Fatalf("DecodeTOML: %v", err)
	}
	redacted, removed := Redact(tree)

	for _, want := range []string{"webserver.api.pwhash", "webserver.api.api_key", "webserver.tls", "dhcp"} {
		if !slices.Contains(removed, want) {
			t.Errorf("removed = %v, want %q", removed, want)
		}
	}

	dns := mustMap(t, redacted["dns"])
	upstreams := mustSlice(t, dns["upstreams"])
	if len(upstreams) != 2 {
		t.Fatalf("dns.upstreams has %d entries, want 2", len(upstreams))
	}
	if upstreams[0] != "192.0.2.53#53" {
		t.Errorf("dns.upstreams[0] = %v", upstreams[0])
	}
	filtering := mustMap(t, redacted["filtering"])
	if filtering["enabled"] != true {
		t.Errorf("filtering.enabled = %v, want true", filtering["enabled"])
	}
	if _, ok := redacted["dhcp"]; ok {
		t.Error("dhcp survived redaction")
	}
}

func TestDecodeRejectsMalformedDocuments(t *testing.T) {
	if _, err := DecodeYAML([]byte("filters: [unterminated")); err == nil {
		t.Error("DecodeYAML accepted malformed YAML")
	}
	if _, err := DecodeTOML([]byte("dns = [unterminated")); err == nil {
		t.Error("DecodeTOML accepted malformed TOML")
	}
}
