package migrate

import (
	"archive/zip"
	"bytes"
	"fmt"
	"strings"
	"testing"
)

func TestParseAdGuardFixture(t *testing.T) {
	bundle, err := ParseAdGuardYAML(readFixture(t, "adguard-home.yaml"))
	if err != nil {
		t.Fatalf("ParseAdGuardYAML: %v", err)
	}
	bundle = bundle.Normalize()
	if err := bundle.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	if got := len(bundle.FilterLists); got != 2 {
		t.Errorf("FilterLists = %d, want 2", got)
	}
	if got := len(bundle.Clients); got != 2 {
		t.Errorf("Clients = %d, want 2", got)
	}
	if got := len(bundle.Records); got != 1 {
		t.Errorf("Records = %d, want 1", got)
	}
	if got := len(bundle.Rewrites); got != 1 {
		t.Errorf("Rewrites = %d, want 1", got)
	}
	if got := len(bundle.Upstreams); got != 2 {
		t.Errorf("Upstreams = %d, want 2", got)
	}
	if bundle.Filtering == nil || !strings.Contains(bundle.Filtering.CustomRules, "||ads.example^") {
		t.Errorf("CustomRules = %+v, want the imported user rules", bundle.Filtering)
	}
	if !hasIssueCode(bundle.Warnings, "allowlist-list-not-imported") {
		t.Errorf("Warnings = %v, want allowlist-list-not-imported", bundle.Warnings)
	}
	if !hasIssueCode(bundle.Warnings, "source-section-not-imported") {
		t.Errorf("Warnings = %v, want source-section-not-imported", bundle.Warnings)
	}
	if bundle.Clients[0].Name != "laptop" || len(bundle.Clients[0].IDs) != 1 {
		t.Errorf("client = %+v, want laptop with one ID", bundle.Clients[0])
	}
}

func TestParseAdGuardLegacyClients(t *testing.T) {
	bundle, err := ParseAdGuardYAML(readFixture(t, "adguard-home-legacy.yaml"))
	if err != nil {
		t.Fatalf("ParseAdGuardYAML: %v", err)
	}
	bundle = bundle.Normalize()
	if got := len(bundle.Clients); got != 2 {
		t.Fatalf("Clients = %d, want the legacy clients list", got)
	}
	if bundle.Clients[0].Name != "legacy-laptop" {
		t.Errorf("Clients[0].Name = %q", bundle.Clients[0].Name)
	}
}

func TestParsePiHoleTOML(t *testing.T) {
	bundle, err := ParsePiHoleTOML(readFixture(t, "pihole.toml"))
	if err != nil {
		t.Fatalf("ParsePiHoleTOML: %v", err)
	}
	bundle = bundle.Normalize()
	if err := bundle.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if got := len(bundle.Upstreams); got != 2 {
		t.Fatalf("Upstreams = %d, want 2", got)
	}
	if bundle.Upstreams[0] != "192.0.2.53:53" {
		t.Errorf("Upstreams[0] = %q, want the # port converted to :", bundle.Upstreams[0])
	}
	if bundle.Filtering == nil || bundle.Filtering.BlockMode != "null_ip" || bundle.Filtering.BlockTTL == nil || *bundle.Filtering.BlockTTL != 10 {
		t.Errorf("Filtering = %+v, want null_ip/10", bundle.Filtering)
	}
	if bundle.Filtering.Enabled == nil || !*bundle.Filtering.Enabled {
		t.Errorf("Filtering.Enabled = %v, want true", bundle.Filtering.Enabled)
	}
	for _, code := range []string{"source-section-not-imported"} {
		if !hasIssueCode(bundle.Warnings, code) {
			t.Errorf("Warnings = %v, want %s", bundle.Warnings, code)
		}
	}
}

func TestParsePiHoleV5ZIP(t *testing.T) {
	zipBytes := buildPiHoleV5Zip(t)
	bundle, err := ParsePiHoleV5ZIP(zipBytes)
	if err != nil {
		t.Fatalf("ParsePiHoleV5ZIP: %v", err)
	}
	bundle = bundle.Normalize()
	if err := bundle.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	if got := len(bundle.FilterLists); got != 2 {
		t.Errorf("FilterLists = %d, want 2 adlists", got)
	}
	if got := len(bundle.Allowlist); got != 2 {
		t.Errorf("Allowlist = %d, want 2", got)
	}
	if got := len(bundle.Blocked); got != 2 {
		t.Errorf("Blocked = %d, want 2", got)
	}
	if got := len(bundle.Records); got != 3 {
		t.Errorf("Records = %d, want 3 (custom.list plus dnsmasq)", got)
	}
	if got := len(bundle.Rewrites); got != 1 {
		t.Errorf("Rewrites = %d, want 1 cname", got)
	}
	if got := len(bundle.Upstreams); got != 3 {
		t.Errorf("Upstreams = %d, want 3", got)
	}
	if bundle.Filtering == nil || bundle.Filtering.BlockMode != "nxdomain" {
		t.Errorf("Filtering = %+v, want nxdomain from pihole-FTL.conf", bundle.Filtering)
	}
	if bundle.Filtering.BlockTTL == nil || *bundle.Filtering.BlockTTL != 30 {
		t.Errorf("BlockTTL = %v, want 30", bundle.Filtering.BlockTTL)
	}
	for _, code := range []string{"gravity-not-imported", "web-password-not-imported", "hosts-file-not-imported", "dhcp-leases-not-imported"} {
		if !hasIssueCode(bundle.Warnings, code) {
			t.Errorf("Warnings = %v, want %s", bundle.Warnings, code)
		}
	}
}

func TestParsePiHoleV6ZipNeedsBrowser(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	file, err := zw.Create("etc/pihole/gravity.db")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = file.Write([]byte("not a real database"))
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := ParsePiHoleV5ZIP(buf.Bytes()); err != ErrPiHoleV6NeedsBrowser {
		t.Fatalf("ParsePiHoleV5ZIP = %v, want ErrPiHoleV6NeedsBrowser", err)
	}
}

func TestSplitOverCapCustomRules(t *testing.T) {
	var rules strings.Builder
	for i := 0; i < 65; i++ {
		fmt.Fprintf(&rules, "/^ad[0-9]+\\.example%d$/\n", i)
	}
	rules.WriteString("||keep.example^\n")
	bundle := Bundle{
		SchemaVersion: SchemaVersion,
		Source:        Source{Kind: SourceAdGuardHome},
		Filtering:     &Filtering{CustomRules: rules.String()},
	}
	bundle, issues := SplitOverCapCustomRules(bundle)
	if len(issues) != 1 || issues[0].Code != "regex-over-cap" {
		t.Fatalf("issues = %v, want one regex-over-cap", issues)
	}
	if err := bundle.Validate(); err != nil {
		t.Fatalf("Validate after split: %v", err)
	}
	if got := strings.Count(bundle.Filtering.CustomRules, "/^ad[0-9]+"); got != 64 {
		t.Errorf("kept %d regex rules, want 64", got)
	}
	if !strings.Contains(bundle.Filtering.CustomRules, "||keep.example^") {
		t.Error("the non-regex rule was dropped")
	}
}

func TestMergeBundles(t *testing.T) {
	a := Bundle{
		SchemaVersion: SchemaVersion,
		Source:        Source{Kind: SourcePiHoleV6},
		Allowlist:     []string{"a.example"},
		Filtering:     &Filtering{BlockMode: "nxdomain"},
	}
	b := Bundle{
		SchemaVersion: SchemaVersion,
		Source:        Source{Kind: SourcePiHoleV6},
		Upstreams:     []string{"192.0.2.53"},
		Allowlist:     []string{"b.example"},
		Filtering:     &Filtering{CustomRules: "||b.example^"},
	}
	merged := MergeBundles(a, b)
	if merged.Filtering.BlockMode != "nxdomain" {
		t.Errorf("BlockMode = %q, want the first bundle's value", merged.Filtering.BlockMode)
	}
	if len(merged.Allowlist) != 2 {
		t.Errorf("Allowlist = %v, want both", merged.Allowlist)
	}
	if merged.Filtering.CustomRules != "||b.example^" {
		t.Errorf("CustomRules = %q", merged.Filtering.CustomRules)
	}
	if len(merged.Upstreams) != 1 {
		t.Errorf("Upstreams = %v", merged.Upstreams)
	}
}

func buildPiHoleV5Zip(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	files := map[string]string{
		"etc/pihole/gravity.list":      string(readFixture(t, "pihole-v5-gravity.txt")),
		"etc/pihole/whitelist.txt":     string(readFixture(t, "pihole-v5-whitelist.txt")),
		"etc/pihole/blacklist.txt":     string(readFixture(t, "pihole-v5-blacklist.txt")),
		"etc/pihole/custom.list":       string(readFixture(t, "pihole-v5-custom.list")),
		"etc/pihole/setupVars.conf":    string(readFixture(t, "pihole-v5-setupVars.conf")),
		"etc/pihole/adlists.list":      string(readFixture(t, "pihole-v5-adlists.list")),
		"etc/pihole/pihole-FTL.conf":   string(readFixture(t, "pihole-v5-pihole-FTL.conf")),
		"etc/dnsmasq.d/90-custom.conf": string(readFixture(t, "pihole-v5-dnsmasq.conf")),
		"etc/hosts":                    "192.0.2.2 synthetic-host\n",
		"etc/pihole/dhcp.leases":       "1700000000 00:11:22:33:44:55 192.0.2.20 synthetic-client *\n",
	}
	for name, body := range files {
		file, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
