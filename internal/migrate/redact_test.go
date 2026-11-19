package migrate

import (
	"slices"
	"testing"
)

func TestIsNeverImport(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"webserver.api.pwhash", true},
		{"webserver.api.api_key", true},
		{"webserver.tls.cert", true},
		{"tls.private_key", true},
		{"dhcp.active", true},
		{"dhcp.leases[0].mac", true},
		{"querylog.enabled", true},
		{"queries[0].domain", true},
		{"database.maxDBdays", true},
		{"users[0].password", true},
		{"users[0].name", false},
		{"dns.upstreams[0]", false},
		{"filtering.allowlist[0]", false},
		{"filters[0].url", false},
	}
	for _, tc := range cases {
		if got := IsNeverImport(tc.path); got != tc.want {
			t.Errorf("IsNeverImport(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

func TestNeverImportKeysAreSortedAndComplete(t *testing.T) {
	keys := NeverImportKeys()
	if !slices.IsSorted(keys) {
		t.Fatalf("NeverImportKeys() is not sorted: %v", keys)
	}
	for _, required := range []string{"pwhash", "password", "api_key", "private_key", "querylog", "leases"} {
		if !slices.Contains(keys, required) {
			t.Errorf("NeverImportKeys() is missing %q", required)
		}
	}
}

func TestRedactRemovesSecretsWithoutMutatingTheSource(t *testing.T) {
	input := map[string]any{
		"http": map[string]any{"address": "0.0.0.0:3000"},
		"users": []any{
			map[string]any{"name": "admin", "password": "placeholder"},
		},
		"webserver": map[string]any{
			"api": map[string]any{
				"pwhash":       "placeholder",
				"api_key":      "placeholder",
				"max_sessions": 16,
			},
			"tls": map[string]any{
				"cert": "/etc/pihole/tls.pem",
				"key":  "/etc/pihole/tls.key",
			},
		},
		"dhcp": map[string]any{
			"active": true,
			"leases": []any{map[string]any{"mac": "00:11:22:33:44:55"}},
		},
		"dns": map[string]any{
			"upstreams": []any{"192.0.2.53", "https://dns.example.invalid/dns-query"},
		},
		"filtering": map[string]any{
			"enabled":   true,
			"allowlist": []any{"allowed.example"},
		},
	}

	redacted, removed := Redact(input)

	for _, want := range []string{
		"users[0].password",
		"webserver.api.pwhash",
		"webserver.api.api_key",
		"webserver.tls",
		"dhcp",
	} {
		if !slices.Contains(removed, want) {
			t.Errorf("Redact() removed %v, want %q", removed, want)
		}
	}

	if _, ok := input["users"].([]any)[0].(map[string]any)["password"]; !ok {
		t.Fatal("Redact mutated the source map; it must return a copy")
	}

	http := mustMap(t, redacted["http"])
	if http["address"] != "0.0.0.0:3000" {
		t.Errorf("http.address = %v, want the allowlisted value", http["address"])
	}
	users := mustSlice(t, redacted["users"])
	user := mustMap(t, users[0])
	if user["name"] != "admin" {
		t.Errorf("users[0].name = %v, want admin", user["name"])
	}
	if _, ok := user["password"]; ok {
		t.Error("users[0].password survived redaction")
	}
	api := mustMap(t, mustMap(t, redacted["webserver"])["api"])
	if api["max_sessions"] != 16 {
		t.Errorf("webserver.api.max_sessions = %v, want 16", api["max_sessions"])
	}
	if _, ok := api["pwhash"]; ok {
		t.Error("webserver.api.pwhash survived redaction")
	}
	dns := mustMap(t, redacted["dns"])
	if got := len(mustSlice(t, dns["upstreams"])); got != 2 {
		t.Errorf("dns.upstreams has %d entries, want 2", got)
	}
	filtering := mustMap(t, redacted["filtering"])
	if filtering["enabled"] != true {
		t.Errorf("filtering.enabled = %v, want true", filtering["enabled"])
	}
}

func TestRedactHandlesNonStringMapKeys(t *testing.T) {
	input := map[string]any{
		"legacy": map[any]any{
			"password": "placeholder",
			"name":     "kept",
		},
	}
	redacted, removed := Redact(input)
	if !slices.Contains(removed, "legacy.password") {
		t.Fatalf("removed = %v, want legacy.password", removed)
	}
	legacy := mustMap(t, redacted["legacy"])
	if _, ok := legacy["password"]; ok {
		t.Error("password survived redaction in a map[any]any")
	}
	if legacy["name"] != "kept" {
		t.Errorf("legacy.name = %v, want kept", legacy["name"])
	}
}

func mustMap(t *testing.T, value any) map[string]any {
	t.Helper()
	out, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("value is %T, want map[string]any", value)
	}
	return out
}

func mustSlice(t *testing.T, value any) []any {
	t.Helper()
	out, ok := value.([]any)
	if !ok {
		t.Fatalf("value is %T, want []any", value)
	}
	return out
}
