package migrate

import (
	"strings"
	"testing"
)

func ptr[T any](value T) *T { return &value }

func TestNormalizeDomain(t *testing.T) {
	valid := []struct {
		in   string
		want string
	}{
		{"Example.COM.", "example.com"},
		{"*.Example.COM", "*.example.com"},
		{"api.example", "api.example"},
		{"_service.example", "_service.example"},
		{"bücher.example", "xn--bcher-kva.example"},
	}
	for _, tc := range valid {
		got, ok := NormalizeDomain(tc.in)
		if !ok || got != tc.want {
			t.Errorf("NormalizeDomain(%q) = (%q, %v), want (%q, true)", tc.in, got, ok, tc.want)
		}
	}

	invalid := []string{
		"", " ", "*", "192.0.2.1", "2001:db8::1", "bad..example",
		"-bad.example", "bad-.example", "bad example", strings.Repeat("a", 254),
	}
	for _, in := range invalid {
		if got, ok := NormalizeDomain(in); ok {
			t.Errorf("NormalizeDomain(%q) = (%q, true), want invalid", in, got)
		}
	}
}

func TestBundleNormalizeAndValidate(t *testing.T) {
	bundle := Bundle{
		SchemaVersion: SchemaVersion,
		Source:        Source{Kind: SourceAdGuardHome, FileName: "adguard-home.yaml"},
		Records: []Record{
			{Name: "Pin.Example.", Type: "a", Value: "192.0.2.10"},
			{Name: "pin.example", Type: "A", Value: "192.0.2.10"},
			{Name: "bad..example", Type: "A", Value: "192.0.2.11"},
			{Name: "v6.example", Type: "AAAA", Value: "2001:db8::1"},
		},
		Clients: []Client{
			{Name: "laptop", IDs: []string{"192.0.2.10", "192.0.2.10"}},
			{Name: "Laptop", IDs: []string{"192.0.2.12"}},
		},
		Proxied:   []string{"Example.COM", "example.com", "bad..example"},
		Blocked:   []string{"*.Ads.Example"},
		Direct:    []string{"direct.example"},
		Allowlist: []string{"allowed.example"},
		FilterLists: []FilterList{
			{Name: "Ads", URL: "https://lists.example.invalid/ads.txt", Enabled: true},
			{Name: "Ads again", URL: "https://lists.example.invalid/ads.txt", Enabled: true},
			{Name: "Inline", Content: "0.0.0.0 inline.example\n", Enabled: true},
		},
		Rewrites: []Rewrite{
			{Domain: "pin.example", Target: "192.0.2.10"},
			{Domain: "pin.example", Target: "192.0.2.10"},
		},
		Filtering: &Filtering{
			BlockMode:   "null_ip",
			BlockTTL:    ptr(10),
			CustomRules: "||ads.example^\n",
		},
		Upstreams: []string{"192.0.2.53", "https://user:pass@dns.example.invalid/dns-query"},
	}

	normalized := bundle.Normalize()
	if err := normalized.Validate(); err != nil {
		t.Fatalf("Validate() = %v", err)
	}

	if got := normalized.Stats.Records; got != 2 {
		t.Errorf("Stats.Records = %d, want 2", got)
	}
	if got := normalized.Stats.Clients; got != 1 {
		t.Errorf("Stats.Clients = %d, want 1", got)
	}
	if got := normalized.Stats.Proxied; got != 1 {
		t.Errorf("Stats.Proxied = %d, want 1", got)
	}
	if got := normalized.Stats.Blocked; got != 1 {
		t.Errorf("Stats.Blocked = %d, want 1", got)
	}
	if got := normalized.Stats.FilterLists; got != 2 {
		t.Errorf("Stats.FilterLists = %d, want 2", got)
	}
	if got := normalized.Stats.FilterRules; got != 1 {
		t.Errorf("Stats.FilterRules = %d, want 1", got)
	}
	if got := normalized.Stats.Rewrites; got != 1 {
		t.Errorf("Stats.Rewrites = %d, want 1", got)
	}
	if got := normalized.Stats.Upstreams; got != 1 {
		t.Errorf("Stats.Upstreams = %d, want 1", got)
	}
	if got := normalized.Stats.Skipped; got != 7 {
		t.Errorf("Stats.Skipped = %d, want 7 (%v)", got, normalized.Skipped)
	}
	if normalized.Records[0].Name != "pin.example" || normalized.Records[0].Type != "A" {
		t.Errorf("record was not normalized: %+v", normalized.Records[0])
	}
	if normalized.Blocked[0] != "*.ads.example" {
		t.Errorf("wildcard was not normalized: %q", normalized.Blocked[0])
	}
	if normalized.Filtering.BlockMode != "null_ip" {
		t.Errorf("BlockMode = %q, want null_ip", normalized.Filtering.BlockMode)
	}
}

func TestBundleValidateRejectsUnsupportedSchema(t *testing.T) {
	bundle := Bundle{SchemaVersion: 99, Source: Source{Kind: SourceAdGuardHome}}
	if err := bundle.Validate(); err == nil {
		t.Fatal("Validate accepted an unsupported schema version")
	}
	bundle = Bundle{SchemaVersion: SchemaVersion, Source: Source{Kind: "unknown"}}
	if err := bundle.Validate(); err == nil {
		t.Fatal("Validate accepted an unknown source kind")
	}
}

func TestBundleValidateRejectsRegexOverCap(t *testing.T) {
	var rules strings.Builder
	for i := 0; i < 65; i++ {
		rules.WriteString("/^ad[0-9]+\\.example$/\n")
	}
	bundle := Bundle{
		SchemaVersion: SchemaVersion,
		Source:        Source{Kind: SourceAdGuardHome},
		Filtering:     &Filtering{CustomRules: rules.String()},
	}
	if err := bundle.Validate(); err == nil {
		t.Fatal("Validate accepted 65 regex rules; the resolver cap is 64")
	}
}

func TestCheckCustomRules(t *testing.T) {
	var allowed strings.Builder
	for i := 0; i < 64; i++ {
		allowed.WriteString("/^ad[0-9]+\\.example$/\n")
	}
	if issues := CheckCustomRules(allowed.String()); len(issues) != 0 {
		t.Fatalf("CheckCustomRules(64 rules) = %v, want no issues", issues)
	}

	tooMany := allowed.String() + "/^tracker[0-9]+\\.example$/\n"
	issues := CheckCustomRules(tooMany)
	if !hasIssueCode(issues, "regex-too-many") {
		t.Fatalf("CheckCustomRules(65 rules) = %v, want regex-too-many", issues)
	}

	long := "/" + strings.Repeat("a", 513) + "/\n"
	issues = CheckCustomRules(long)
	if !hasIssueCode(issues, "regex-too-long") {
		t.Fatalf("CheckCustomRules(long pattern) = %v, want regex-too-long", issues)
	}

	if issues := CheckCustomRules("@@/^safe[0-9]+\\.example$/\n"); len(issues) != 0 {
		t.Fatalf("CheckCustomRules(allow regex) = %v, want no issues", issues)
	}
}

func TestMergeIsAdditiveOnly(t *testing.T) {
	target := Target{
		Records:   map[string]string{"existing.example": "192.0.2.1"},
		Clients:   []Client{{Name: "existing", IDs: []string{"192.0.2.5"}}},
		Proxied:   []string{"existing.example"},
		Blocked:   []string{"blocked.example"},
		Direct:    []string{"direct.example"},
		Allowlist: []string{"allowed.example"},
		FilterLists: []FilterList{
			{Name: "Existing", URL: "https://lists.example.invalid/existing.txt"},
		},
		Rewrites:  []Rewrite{{Domain: "pin.example", Target: "192.0.2.10"}},
		Filtering: &Filtering{BlockMode: "null_ip", BlockTTL: ptr(10)},
		Upstreams: []string{"192.0.2.53"},
	}
	bundle := Bundle{
		SchemaVersion: SchemaVersion,
		Source:        Source{Kind: SourcePiHoleV5},
		Records: []Record{
			{Name: "existing.example", Type: "A", Value: "192.0.2.2"},
			{Name: "new.example", Type: "A", Value: "192.0.2.3"},
			{Name: "v6.example", Type: "AAAA", Value: "2001:db8::1"},
		},
		Clients: []Client{
			{Name: "existing", IDs: []string{"192.0.2.6"}},
			{Name: "new-client", IDs: []string{"192.0.2.7"}},
		},
		Proxied:   []string{"existing.example", "new.example"},
		Blocked:   []string{"blocked.example", "new-blocked.example"},
		Direct:    []string{"new-direct.example"},
		Allowlist: []string{"allowed.example", "new-allowed.example"},
		FilterLists: []FilterList{
			{Name: "Existing", URL: "https://lists.example.invalid/existing.txt"},
			{Name: "New", URL: "https://lists.example.invalid/new.txt"},
		},
		Rewrites: []Rewrite{
			{Domain: "pin.example", Target: "192.0.2.10"},
			{Domain: "new-pin.example", Target: "192.0.2.11"},
			{Domain: "cname.example", Target: "target.example"},
		},
		Filtering: &Filtering{
			BlockMode:   "nxdomain",
			BlockTTL:    ptr(20),
			CustomRules: "||new.example^\n",
		},
		Upstreams: []string{"192.0.2.54"},
	}

	result := Merge(target, bundle)
	if target.Records["existing.example"] != "192.0.2.1" {
		t.Fatal("Merge mutated the input target")
	}
	if result.Target.Records["existing.example"] != "192.0.2.1" {
		t.Errorf("existing record was overwritten: %q", result.Target.Records["existing.example"])
	}
	if result.Target.Records["new.example"] != "192.0.2.3" {
		t.Errorf("new record was not added: %v", result.Target.Records)
	}
	if !hasIssueCode(result.Skipped, "record-conflict") {
		t.Errorf("Skipped = %v, want record-conflict", result.Skipped)
	}
	if result.Target.Records["v6.example"] != "2001:db8::1" {
		t.Errorf("AAAA record was not imported: %v", result.Target.Records)
	}
	if result.Target.Records["cname.example"] != "target.example" {
		t.Errorf("CNAME rewrite was not imported: %v", result.Target.Records)
	}
	if hasIssueCode(result.Skipped, "record-unsupported-apply") {
		t.Errorf("Skipped = %v, did not expect record-unsupported-apply for A/AAAA/CNAME", result.Skipped)
	}
	if !hasIssueCode(result.Skipped, "client-duplicate") {
		t.Errorf("Skipped = %v, want client-duplicate", result.Skipped)
	}
	if !hasIssueCode(result.Skipped, "upstreams-not-opted-in") {
		t.Errorf("Skipped = %v, want upstreams-not-opted-in", result.Skipped)
	}
	if len(result.Target.Upstreams) != 1 {
		t.Errorf("Upstreams = %v, want the existing one only", result.Target.Upstreams)
	}
	if result.Target.Filtering.BlockMode != "nxdomain" {
		t.Errorf("BlockMode = %q, want the imported nxdomain", result.Target.Filtering.BlockMode)
	}
	if *result.Target.Filtering.BlockTTL != 20 {
		t.Errorf("BlockTTL = %d, want the imported 20", *result.Target.Filtering.BlockTTL)
	}
	if result.Target.Filtering.CustomRules != "||new.example^" {
		t.Errorf("CustomRules = %q, want the imported rule", result.Target.Filtering.CustomRules)
	}
	if result.Added.Proxied != 1 || result.Added.Blocked != 1 || result.Added.Direct != 1 {
		t.Errorf("Added = %+v, want one new domain in each routing list", result.Added)
	}
	if result.Added.Allowlist != 1 {
		t.Errorf("Added.Allowlist = %d, want 1", result.Added.Allowlist)
	}
	if result.Added.FilterLists != 1 || result.Added.Rewrites != 2 || result.Added.Clients != 1 {
		t.Errorf("Added = %+v, want one new list, two rewrites and one client", result.Added)
	}
}

func TestMergeImportsUpstreamsOnlyWithOptIn(t *testing.T) {
	target := NewTarget()
	bundle := Bundle{
		SchemaVersion:  SchemaVersion,
		Source:         Source{Kind: SourcePiHoleV6},
		Upstreams:      []string{"192.0.2.53", "https://dns.example.invalid/dns-query"},
		UpstreamsOptIn: true,
	}
	result := Merge(target, bundle)
	if len(result.Target.Upstreams) != 2 {
		t.Fatalf("Upstreams = %v, want both after opt-in", result.Target.Upstreams)
	}
	if result.Added.Upstreams != 2 {
		t.Errorf("Added.Upstreams = %d, want 2", result.Added.Upstreams)
	}
}

func hasIssueCode(issues []Issue, code string) bool {
	for _, issue := range issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}
