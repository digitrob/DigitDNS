package web

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The seven resolution transports have one ramp across the dashboard and the
// companion app. This test reads the three source tables rather than trusting a
// comment: a future "small cleanup" that swaps UDP and TCP on one surface has
// to fail here before it reaches an operator.
func TestTransportColourParityAcrossPlatforms(t *testing.T) {
	css := readAsset(t, "css/style.css")
	js := readAsset(t, "js/app.js")
	kotlinPath := filepath.Join("..", "..", "android", "app", "src", "main", "java",
		"com", "digitrob", "digitdns", "ui", "theme", "ProtocolPalette.kt")
	kotlinBytes, err := os.ReadFile(kotlinPath)
	if err != nil {
		t.Fatalf("read %s: %v", kotlinPath, err)
	}
	kotlin := string(kotlinBytes)

	roles := []struct {
		wire   string
		css    string
		text   string
		kotlin string
	}{
		{"UDP", ".proto-udp", "text-cyan-400", `"UDP" -> colors.info`},
		{"TCP", ".proto-tcp", "text-blue-400", `"TCP" -> colors.blue`},
		{"DoT", ".proto-dot", "text-emerald-400", `"DOT" -> colors.success`},
		{"DoH", ".proto-doh", "text-purple-400", `"DOH" -> colors.purple`},
		{"DoQ", ".proto-doq", "text-amber-400", `"DOQ" -> colors.warning`},
		{"DoH3", ".proto-doh3", "text-orange-400", `"DOH3" -> colors.orange`},
		{"DNSCrypt", ".proto-dnscrypt", "text-fuchsia-400", `"DNSCRYPT" -> colors.fuchsia`},
	}
	for _, role := range roles {
		if !strings.Contains(css, role.css+" { color:") {
			t.Errorf("style.css lost %s", role.css)
		}
		wantJS := regexp.MustCompile(`(?s)` + regexp.QuoteMeta(strings.ToUpper(role.wire)) + `: \{ text: '` + regexp.QuoteMeta(role.text) + `', pill: '` + strings.TrimPrefix(role.css, ".") + `' \}`)
		if !wantJS.MatchString(js) {
			t.Errorf("app.js PROTOCOL_ROLES lost %s → %s / %s", role.wire, role.text, role.css)
		}
		if !strings.Contains(kotlin, role.kotlin) {
			t.Errorf("ProtocolPalette.kt lost %s", role.kotlin)
		}
	}

	// The literal fuchsia fill needs the light and grey steps written out; the
	// token-backed roles inherit them, so only DNSCrypt needs explicit overrides.
	for _, override := range []string{
		`[data-theme="light"] .proto-dnscrypt { color: rgb(134 25 143); }`,
		`[data-theme="grey"] .proto-dnscrypt { color: rgb(var(--c-slate-300)); }`,
	} {
		if !strings.Contains(css, override) {
			t.Errorf("style.css lost %q", override)
		}
	}
}
