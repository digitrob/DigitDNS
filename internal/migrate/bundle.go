// Package migrate defines the canonical, secret-free intermediate form shared
// by the Pi-hole and AdGuard Home importers.
//
// Nothing in this package writes to a database or starts a download. A parser
// turns one source archive into a Bundle, the dashboard previews that Bundle,
// and a later apply step merges it additively into the live configuration. The
// split exists so a source file can be inspected and rejected before any live
// state changes, and so the same normalized shape can be produced in a browser,
// on Android, or by a future command-line tool.
//
// A Bundle is deliberately not a dump of the source configuration. It carries
// only the fields DigitDNS can actually represent, normalized to the forms the
// resolver matches on. Credentials, TLS material, DHCP state, query history and
// session data are never part of the schema; Redact is the second line of
// defence for diagnostics that need to show a source tree without leaking it.
package migrate

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net"
	"net/url"
	"sort"
	"strings"
	"unicode/utf8"

	"golang.org/x/net/idna"

	"digitdns/internal/blocklist"
)

// SchemaVersion is the Bundle layout this build understands. A parser that
// learns a new field bumps it, and an apply path refuses a version it cannot
// interpret rather than guessing.
const SchemaVersion = 1

// Limits mirror the live configuration's own caps. They are duplicated here so
// an importer can report an oversized item before the apply path has to reject
// it; the resolver remains the authority for what it will actually compile.
const (
	MaxInlineListBytes          = 16 << 20
	MaxFilteringCustomRulesSize = 1 << 20
	MaxDomainLength             = 253
	MaxRecordValueBytes         = 4096
	MaxClientIDs                = 64
	MaxFilterLists              = 512
	MaxUpstreams                = 64
	MaxRewriteTargetBytes       = 512
)

// SourceKind names the product whose export produced a Bundle.
type SourceKind string

const (
	SourceAdGuardHome SourceKind = "adguard-home"
	SourcePiHoleV5    SourceKind = "pihole-v5"
	SourcePiHoleV6    SourceKind = "pihole-v6"
)

// Valid reports whether k is a source this schema names.
func (k SourceKind) Valid() bool {
	switch k {
	case SourceAdGuardHome, SourcePiHoleV5, SourcePiHoleV6:
		return true
	default:
		return false
	}
}

// Source describes where a Bundle came from. Fingerprint is the SHA-256 of the
// source archive or of the normalized payload, never of a secret-bearing file
// after redaction: it is an identity for the preview, not a place to smuggle
// bytes back in.
type Source struct {
	Kind        SourceKind `json:"kind"`
	Version     string     `json:"version,omitempty"`
	FileName    string     `json:"file_name,omitempty"`
	Fingerprint string     `json:"fingerprint,omitempty"`
}

// Bundle is the normalized, allowlisted intermediate form. Every field is
// something DigitDNS can represent; a parser that cannot map a source field
// records it in Skipped instead of inventing a destination.
type Bundle struct {
	SchemaVersion int          `json:"schema_version"`
	Source        Source       `json:"source"`
	Records       []Record     `json:"records,omitempty"`
	Clients       []Client     `json:"clients,omitempty"`
	Proxied       []string     `json:"proxied,omitempty"`
	Blocked       []string     `json:"blocked,omitempty"`
	Direct        []string     `json:"direct,omitempty"`
	Allowlist     []string     `json:"allowlist,omitempty"`
	FilterLists   []FilterList `json:"filter_lists,omitempty"`
	Rewrites      []Rewrite    `json:"rewrites,omitempty"`
	Filtering     *Filtering   `json:"filtering,omitempty"`
	Upstreams     []string     `json:"upstreams,omitempty"`
	// UpstreamsOptIn is the explicit consent the full-policy import requires
	// before it changes resolver addresses. A false value keeps Upstreams in the
	// preview but Merge ignores them.
	UpstreamsOptIn bool    `json:"upstreams_opt_in,omitempty"`
	Warnings       []Issue `json:"warnings,omitempty"`
	Skipped        []Issue `json:"skipped,omitempty"`
	Stats          Stats   `json:"stats"`
}

// Record is one locally answered name. Type is uppercased and Value is kept in
// the source's canonical form: an address for A/AAAA, a normalized host for
// CNAME, and the trimmed text for every other supported type.
type Record struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	Value string `json:"value"`
	TTL   uint32 `json:"ttl,omitempty"`
}

// Client is one source-defined client. DigitDNS subscriber accounts have more
// fields than this; the importer only carries the identity and the filtering
// hint it can map, and leaves quotas, tokens and expiration at their defaults.
type Client struct {
	Name             string   `json:"name"`
	IDs              []string `json:"ids,omitempty"`
	Enabled          bool     `json:"enabled"`
	FilteringEnabled *bool    `json:"filtering_enabled,omitempty"`
	Upstreams        []string `json:"upstreams,omitempty"`
	Note             string   `json:"note,omitempty"`
}

// FilterList is one subscribed list or one uploaded body. Content is the text
// that becomes an uploaded FilterList when it is too large or too local to
// subscribe to; URL is the subscription form.
type FilterList struct {
	Name     string `json:"name"`
	URL      string `json:"url,omitempty"`
	Homepage string `json:"homepage,omitempty"`
	License  string `json:"license,omitempty"`
	Enabled  bool   `json:"enabled"`
	Content  string `json:"content,omitempty"`
}

// Rewrite is one local answer. Target is either an IP address or a hostname;
// the apply path decides which forms the current resolver can serve and reports
// the rest rather than silently dropping them.
type Rewrite struct {
	Domain string `json:"domain"`
	Target string `json:"target"`
}

// Filtering carries only the settings the source actually specified. Pointer
// fields distinguish "absent" from false or zero, so an import never turns a
// live feature off just because the source omitted it.
type Filtering struct {
	Enabled             *bool  `json:"enabled,omitempty"`
	BlockMode           string `json:"block_mode,omitempty"`
	BlockTTL            *int   `json:"block_ttl,omitempty"`
	UpdateIntervalHours *int   `json:"update_interval_hours,omitempty"`
	CustomRules         string `json:"custom_rules,omitempty"`
}

// Issue is one warning or one skipped item. Code is stable and machine-readable
// so the preview can group counts without parsing prose; Item is the source
// value when it is safe to echo, and Reason explains what happened.
type Issue struct {
	Code   string `json:"code"`
	Item   string `json:"item,omitempty"`
	Reason string `json:"reason,omitempty"`
}

// Stats is the preview's count summary. It is derived from the normalized
// bundle, not trusted from the source.
type Stats struct {
	Records     int `json:"records"`
	Clients     int `json:"clients"`
	Proxied     int `json:"proxied"`
	Blocked     int `json:"blocked"`
	Direct      int `json:"direct"`
	Allowlist   int `json:"allowlist"`
	FilterLists int `json:"filter_lists"`
	FilterRules int `json:"filter_rules"`
	Rewrites    int `json:"rewrites"`
	Upstreams   int `json:"upstreams"`
	Warnings    int `json:"warnings"`
	Skipped     int `json:"skipped"`
}

// Normalize returns a copy with every field in the form the rest of the importer
// and the resolver expect. Invalid items are moved to Skipped with a stable code
// rather than dropped silently; the returned bundle is safe to preview and to
// pass to Validate.
func (b Bundle) Normalize() Bundle {
	out := b
	if out.SchemaVersion == 0 {
		out.SchemaVersion = SchemaVersion
	}
	out.Source.Kind = SourceKind(strings.ToLower(strings.TrimSpace(string(out.Source.Kind))))
	out.Source.Version = strings.TrimSpace(out.Source.Version)
	out.Source.FileName = strings.TrimSpace(out.Source.FileName)
	out.Source.Fingerprint = strings.ToLower(strings.TrimSpace(out.Source.Fingerprint))
	out.Warnings = append([]Issue(nil), b.Warnings...)
	out.Skipped = append([]Issue(nil), b.Skipped...)

	out.Records = nil
	seenRecords := make(map[string]struct{}, len(b.Records))
	for _, record := range b.Records {
		normalized, issue, ok := normalizeRecord(record)
		if !ok {
			out.Skipped = append(out.Skipped, issue)
			continue
		}
		key := normalized.Name + "\x00" + normalized.Type + "\x00" + normalized.Value
		if _, exists := seenRecords[key]; exists {
			out.Skipped = append(out.Skipped, Issue{
				Code:   "record-duplicate",
				Item:   normalized.Name,
				Reason: "the same record is already present in this bundle",
			})
			continue
		}
		seenRecords[key] = struct{}{}
		out.Records = append(out.Records, normalized)
	}

	out.Clients = nil
	seenClients := make(map[string]struct{}, len(b.Clients))
	for _, client := range b.Clients {
		normalized, issue, ok := normalizeClient(client)
		if !ok {
			out.Skipped = append(out.Skipped, issue)
			continue
		}
		key := strings.ToLower(normalized.Name)
		if _, exists := seenClients[key]; exists {
			out.Skipped = append(out.Skipped, Issue{
				Code:   "client-duplicate",
				Item:   normalized.Name,
				Reason: "the same client is already present in this bundle",
			})
			continue
		}
		seenClients[key] = struct{}{}
		out.Clients = append(out.Clients, normalized)
	}

	out.Proxied = normalizeDomainList(b.Proxied, "proxied", &out)
	out.Blocked = normalizeDomainList(b.Blocked, "blocked", &out)
	out.Direct = normalizeDomainList(b.Direct, "direct", &out)
	out.Allowlist = normalizeDomainList(b.Allowlist, "allowlist", &out)

	out.FilterLists = nil
	seenLists := make(map[string]struct{}, len(b.FilterLists))
	for _, list := range b.FilterLists {
		normalized, issue, ok := normalizeFilterList(list)
		if !ok {
			out.Skipped = append(out.Skipped, issue)
			continue
		}
		key := normalized.URL
		if key == "" {
			key = "content:" + normalized.Name + ":" + hashText(normalized.Content)
		}
		if _, exists := seenLists[key]; exists {
			out.Skipped = append(out.Skipped, Issue{
				Code:   "filter-list-duplicate",
				Item:   normalized.Name,
				Reason: "the same list is already present in this bundle",
			})
			continue
		}
		seenLists[key] = struct{}{}
		out.FilterLists = append(out.FilterLists, normalized)
	}

	out.Rewrites = nil
	seenRewrites := make(map[string]struct{}, len(b.Rewrites))
	for _, rewrite := range b.Rewrites {
		normalized, issue, ok := normalizeRewrite(rewrite)
		if !ok {
			out.Skipped = append(out.Skipped, issue)
			continue
		}
		key := normalized.Domain + "\x00" + normalized.Target
		if _, exists := seenRewrites[key]; exists {
			out.Skipped = append(out.Skipped, Issue{
				Code:   "rewrite-duplicate",
				Item:   normalized.Domain,
				Reason: "the same rewrite is already present in this bundle",
			})
			continue
		}
		seenRewrites[key] = struct{}{}
		out.Rewrites = append(out.Rewrites, normalized)
	}

	if b.Filtering != nil {
		filtering, issues := normalizeFiltering(*b.Filtering)
		out.Filtering = filtering
		out.Skipped = append(out.Skipped, issues...)
	}

	out.Upstreams = normalizeUpstreams(b.Upstreams, &out)
	out.Stats = out.countStats()
	return out
}

// Validate checks the structural contract an apply path can rely on. It is
// intentionally strict about resolver caps: a bundle that would make the
// resolver silently skip a regex rule must be split by the parser and reported
// first, not accepted and then quietly lose rules.
func (b Bundle) Validate() error {
	if b.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported migration schema version %d (want %d)", b.SchemaVersion, SchemaVersion)
	}
	if !b.Source.Kind.Valid() {
		return fmt.Errorf("unsupported migration source %q", b.Source.Kind)
	}
	if len(b.FilterLists) > MaxFilterLists {
		return fmt.Errorf("bundle contains %d filter lists (maximum %d)", len(b.FilterLists), MaxFilterLists)
	}
	if len(b.Upstreams) > MaxUpstreams {
		return fmt.Errorf("bundle contains %d upstreams (maximum %d)", len(b.Upstreams), MaxUpstreams)
	}
	for _, list := range b.FilterLists {
		if len(list.Content) > MaxInlineListBytes {
			return fmt.Errorf("filter list %q is larger than %d bytes", list.Name, MaxInlineListBytes)
		}
		if list.URL == "" && strings.TrimSpace(list.Content) == "" {
			return fmt.Errorf("filter list %q has neither a URL nor content", list.Name)
		}
	}
	if b.Filtering != nil {
		if len(b.Filtering.CustomRules) > MaxFilteringCustomRulesSize {
			return fmt.Errorf("custom rules are larger than %d bytes", MaxFilteringCustomRulesSize)
		}
		if issues := CheckCustomRules(b.Filtering.CustomRules); len(issues) > 0 {
			return fmt.Errorf("custom rules exceed resolver caps: %s", issues[0].Reason)
		}
	}
	return nil
}

// CheckCustomRules reports every regex line that the resolver's custom-rule
// plane would skip because of its count or pattern-length cap. The importer uses
// this to move the offending lines into a downloadable report instead of
// pretending they were applied.
func CheckCustomRules(text string) []Issue {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	var issues []Issue
	count := 0
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(strings.TrimSuffix(raw, "\r"))
		if strings.HasPrefix(line, "@@") {
			line = strings.TrimSpace(line[2:])
		}
		if !strings.HasPrefix(line, "/") || !strings.HasSuffix(line, "/") || len(line) < 2 {
			continue
		}
		count++
		pattern := line[1 : len(line)-1]
		if len(pattern) > blocklist.MaxCustomRegexPatternSize {
			issues = append(issues, Issue{
				Code:   "regex-too-long",
				Item:   pattern,
				Reason: fmt.Sprintf("regex is %d bytes; the resolver accepts at most %d", len(pattern), blocklist.MaxCustomRegexPatternSize),
			})
		}
		if count > blocklist.MaxCustomRegexRules {
			issues = append(issues, Issue{
				Code:   "regex-too-many",
				Item:   pattern,
				Reason: fmt.Sprintf("more than %d regex rules; the resolver accepts at most %d", blocklist.MaxCustomRegexRules, blocklist.MaxCustomRegexRules),
			})
		}
	}
	return issues
}

// NormalizeDomain returns the canonical form of one rule domain. It accepts a
// plain host or a "*.parent" wildcard, lowercases and IDNA-encodes the name,
// and rejects an address, a bare "*" or a name the resolver could not match.
func NormalizeDomain(raw string) (string, bool) {
	s := strings.ToLower(strings.TrimSpace(raw))
	s = strings.TrimSuffix(s, ".")
	wildcard := strings.HasPrefix(s, "*.")
	if wildcard {
		s = strings.TrimSpace(s[2:])
	}
	if s == "" || len(s) > MaxDomainLength || strings.Contains(s, "*") {
		return "", false
	}
	if net.ParseIP(s) != nil {
		return "", false
	}
	ascii := s
	if !isASCII(s) {
		encoded, err := idna.Lookup.ToASCII(s)
		if err != nil {
			return "", false
		}
		ascii = encoded
	}
	for _, label := range strings.Split(ascii, ".") {
		if len(label) == 0 || len(label) > 63 {
			return "", false
		}
		if label[0] == '-' || label[len(label)-1] == '-' {
			return "", false
		}
		for _, r := range label {
			switch {
			case r >= 'a' && r <= 'z':
			case r >= '0' && r <= '9':
			case r == '-', r == '_':
			default:
				return "", false
			}
		}
	}
	if wildcard {
		return "*." + ascii, true
	}
	return ascii, true
}

func normalizeRecord(record Record) (Record, Issue, bool) {
	name, ok := NormalizeDomain(record.Name)
	if !ok || strings.HasPrefix(name, "*.") {
		return Record{}, Issue{Code: "record-invalid-name", Item: record.Name, Reason: "record names must be plain hostnames"}, false
	}
	typ := strings.ToUpper(strings.TrimSpace(record.Type))
	switch typ {
	case "A", "AAAA", "CNAME", "TXT", "MX", "NS", "SRV", "PTR":
	default:
		return Record{}, Issue{Code: "record-unsupported-type", Item: record.Type, Reason: "record type is not supported by the importer"}, false
	}
	value := strings.TrimSpace(record.Value)
	if value == "" || len(value) > MaxRecordValueBytes {
		return Record{}, Issue{Code: "record-invalid-value", Item: record.Name, Reason: "record value is empty or too long"}, false
	}
	switch typ {
	case "A", "AAAA":
		ip := net.ParseIP(value)
		if ip == nil || (typ == "A" && ip.To4() == nil) || (typ == "AAAA" && ip.To4() != nil) {
			return Record{}, Issue{Code: "record-invalid-address", Item: value, Reason: "address does not match the record type"}, false
		}
		value = ip.String()
	case "CNAME", "NS", "PTR":
		target, ok := NormalizeDomain(value)
		if !ok || strings.HasPrefix(target, "*.") {
			return Record{}, Issue{Code: "record-invalid-target", Item: value, Reason: "target must be a plain hostname"}, false
		}
		value = target
	}
	return Record{Name: name, Type: typ, Value: value, TTL: record.TTL}, Issue{}, true
}

func normalizeClient(client Client) (Client, Issue, bool) {
	name := strings.TrimSpace(client.Name)
	ids := make([]string, 0, len(client.IDs))
	seen := make(map[string]struct{}, len(client.IDs))
	for _, raw := range client.IDs {
		id := normalizeClientID(raw)
		if id == "" {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
		if len(ids) == MaxClientIDs {
			break
		}
	}
	if name == "" && len(ids) > 0 {
		name = ids[0]
	}
	if name == "" {
		return Client{}, Issue{Code: "client-invalid", Item: client.Name, Reason: "client has neither a name nor an identifier"}, false
	}
	if len(name) > 120 {
		name = name[:120]
	}
	upstreams := make([]string, 0, len(client.Upstreams))
	seenUpstreams := make(map[string]struct{}, len(client.Upstreams))
	for _, raw := range client.Upstreams {
		upstream := strings.TrimSpace(raw)
		if upstream == "" || strings.Contains(upstream, "@") {
			continue
		}
		if _, exists := seenUpstreams[upstream]; exists {
			continue
		}
		seenUpstreams[upstream] = struct{}{}
		upstreams = append(upstreams, upstream)
	}
	return Client{
		Name:             name,
		IDs:              ids,
		Enabled:          client.Enabled,
		FilteringEnabled: client.FilteringEnabled,
		Upstreams:        upstreams,
		Note:             strings.TrimSpace(client.Note),
	}, Issue{}, true
}

func normalizeClientID(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	if strings.Contains(s, "/") {
		if _, network, err := net.ParseCIDR(s); err == nil {
			return network.String()
		}
	}
	if ip := net.ParseIP(strings.Trim(s, "[]")); ip != nil {
		return ip.String()
	}
	// A MAC address is case-insensitive; every other clientid form is kept as
	// written so an operator-recognized label does not change shape.
	if strings.Count(s, ":") == 5 {
		return strings.ToLower(s)
	}
	return s
}

func normalizeFilterList(list FilterList) (FilterList, Issue, bool) {
	name := strings.TrimSpace(list.Name)
	listURL := strings.TrimSpace(list.URL)
	if listURL != "" {
		normalized, ok := normalizeHTTPURL(listURL)
		if !ok {
			return FilterList{}, Issue{Code: "filter-list-invalid-url", Item: listURL, Reason: "list URL must be an http or https URL without embedded credentials"}, false
		}
		listURL = normalized
	}
	content := list.Content
	if len(content) > MaxInlineListBytes {
		return FilterList{}, Issue{Code: "filter-list-too-large", Item: name, Reason: fmt.Sprintf("list is larger than %d bytes", MaxInlineListBytes)}, false
	}
	if name == "" {
		name = listURL
	}
	if name == "" {
		name = "Imported list"
	}
	if len(name) > 80 {
		name = truncateUTF8(name, 80)
	}
	homepage := strings.TrimSpace(list.Homepage)
	if homepage != "" {
		if normalized, ok := normalizeHTTPURL(homepage); ok {
			homepage = normalized
		} else {
			homepage = ""
		}
	}
	if listURL == "" && strings.TrimSpace(content) == "" {
		return FilterList{}, Issue{Code: "filter-list-empty", Item: name, Reason: "list has neither a URL nor content"}, false
	}
	return FilterList{
		Name:     name,
		URL:      listURL,
		Homepage: homepage,
		License:  strings.TrimSpace(list.License),
		Enabled:  list.Enabled,
		Content:  content,
	}, Issue{}, true
}

func normalizeRewrite(rewrite Rewrite) (Rewrite, Issue, bool) {
	domain, ok := NormalizeDomain(rewrite.Domain)
	if !ok || strings.HasPrefix(domain, "*.") {
		return Rewrite{}, Issue{Code: "rewrite-invalid-domain", Item: rewrite.Domain, Reason: "rewrite domain must be a plain hostname"}, false
	}
	target := strings.TrimSpace(rewrite.Target)
	if target == "" || len(target) > MaxRewriteTargetBytes {
		return Rewrite{}, Issue{Code: "rewrite-invalid-target", Item: rewrite.Target, Reason: "rewrite target is empty or too long"}, false
	}
	if ip := net.ParseIP(strings.Trim(target, "[]")); ip != nil {
		target = ip.String()
	} else {
		host, ok := NormalizeDomain(target)
		if !ok || strings.HasPrefix(host, "*.") {
			return Rewrite{}, Issue{Code: "rewrite-invalid-target", Item: rewrite.Target, Reason: "rewrite target must be an address or a plain hostname"}, false
		}
		target = host
	}
	return Rewrite{Domain: domain, Target: target}, Issue{}, true
}

func normalizeFiltering(in Filtering) (*Filtering, []Issue) {
	out := &Filtering{
		Enabled:             in.Enabled,
		BlockMode:           strings.TrimSpace(in.BlockMode),
		BlockTTL:            in.BlockTTL,
		UpdateIntervalHours: in.UpdateIntervalHours,
		CustomRules:         strings.TrimSpace(in.CustomRules),
	}
	var issues []Issue
	switch out.BlockMode {
	case "", "null_ip", "nxdomain", "refused":
	default:
		issues = append(issues, Issue{Code: "filtering-invalid-block-mode", Item: out.BlockMode, Reason: "block mode is not one this resolver implements"})
		out.BlockMode = ""
	}
	if out.BlockTTL != nil && (*out.BlockTTL < 1 || *out.BlockTTL > 3600) {
		issues = append(issues, Issue{Code: "filtering-invalid-block-ttl", Item: fmt.Sprint(*out.BlockTTL), Reason: "block TTL must be between 1 and 3600 seconds"})
		out.BlockTTL = nil
	}
	if out.UpdateIntervalHours != nil && (*out.UpdateIntervalHours < 1 || *out.UpdateIntervalHours > 168) {
		issues = append(issues, Issue{Code: "filtering-invalid-update-interval", Item: fmt.Sprint(*out.UpdateIntervalHours), Reason: "update interval must be between 1 and 168 hours"})
		out.UpdateIntervalHours = nil
	}
	if len(out.CustomRules) > MaxFilteringCustomRulesSize {
		issues = append(issues, Issue{Code: "filtering-custom-rules-too-large", Reason: fmt.Sprintf("custom rules are larger than %d bytes", MaxFilteringCustomRulesSize)})
		out.CustomRules = ""
	}
	return out, issues
}

func normalizeUpstreams(in []string, bundle *Bundle) []string {
	out := make([]string, 0, len(in))
	seen := make(map[string]struct{}, len(in))
	for _, raw := range in {
		upstream := strings.TrimSpace(raw)
		if upstream == "" || len(upstream) > 512 {
			bundle.Skipped = append(bundle.Skipped, Issue{Code: "upstream-invalid", Item: upstream, Reason: "upstream is empty or too long"})
			continue
		}
		// A URL with userinfo carries a credential. Never import one; the
		// operator can add a credential-bearing upstream deliberately later.
		if strings.Contains(upstream, "@") {
			bundle.Skipped = append(bundle.Skipped, Issue{Code: "upstream-credential", Item: upstream, Reason: "upstreams with embedded credentials are not imported"})
			continue
		}
		if _, exists := seen[upstream]; exists {
			continue
		}
		seen[upstream] = struct{}{}
		out = append(out, upstream)
		if len(out) == MaxUpstreams {
			break
		}
	}
	return out
}

func normalizeDomainList(in []string, label string, bundle *Bundle) []string {
	out := make([]string, 0, len(in))
	seen := make(map[string]struct{}, len(in))
	for _, raw := range in {
		domain, ok := NormalizeDomain(raw)
		if !ok {
			bundle.Skipped = append(bundle.Skipped, Issue{Code: label + "-invalid-domain", Item: raw, Reason: "not a domain this resolver can match"})
			continue
		}
		if _, exists := seen[domain]; exists {
			continue
		}
		seen[domain] = struct{}{}
		out = append(out, domain)
	}
	return out
}

func normalizeDomains(in []string) []string {
	out := make([]string, 0, len(in))
	seen := make(map[string]struct{}, len(in))
	for _, raw := range in {
		domain, ok := NormalizeDomain(raw)
		if !ok {
			continue
		}
		if _, exists := seen[domain]; exists {
			continue
		}
		seen[domain] = struct{}{}
		out = append(out, domain)
	}
	return out
}

func normalizeHTTPURL(raw string) (string, bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return "", false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", false
	}
	if u.User != nil {
		return "", false
	}
	return u.String(), true
}

func (b Bundle) countStats() Stats {
	stats := Stats{
		Records:     len(b.Records),
		Clients:     len(b.Clients),
		Proxied:     len(b.Proxied),
		Blocked:     len(b.Blocked),
		Direct:      len(b.Direct),
		Allowlist:   len(b.Allowlist),
		FilterLists: len(b.FilterLists),
		Rewrites:    len(b.Rewrites),
		Upstreams:   len(b.Upstreams),
		Warnings:    len(b.Warnings),
		Skipped:     len(b.Skipped),
	}
	for _, list := range b.FilterLists {
		if strings.TrimSpace(list.Content) == "" {
			continue
		}
		stats.FilterRules += blocklist.Analyze(list.Content).Total()
	}
	return stats
}

// Fingerprint returns the SHA-256 of a byte slice in the lowercase hex form the
// Bundle's Source field uses.
func Fingerprint(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func hashText(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}

func truncateUTF8(s string, max int) string {
	if len(s) <= max {
		return s
	}
	end := max
	for end > 0 && !utf8.RuneStart(s[end]) {
		end--
	}
	return s[:end]
}

// SortedIssueCodes is a small helper for deterministic previews and tests. It
// returns the distinct codes in lexical order without changing the bundle.
func SortedIssueCodes(issues []Issue) []string {
	seen := make(map[string]struct{}, len(issues))
	for _, issue := range issues {
		seen[issue.Code] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for code := range seen {
		out = append(out, code)
	}
	sort.Strings(out)
	return out
}
