package migrate

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"path"
	"strings"
)

// ErrPiHoleV6NeedsBrowser tells the dashboard that a Teleporter archive carries
// a v6 gravity.db and must be parsed by the in-panel SQLite-WASM path. The
// server has no SQLite driver by design; returning a clear sentinel is better
// than silently importing the TOML half and losing every gravity/adlist row.
var ErrPiHoleV6NeedsBrowser = errors.New("Pi-hole v6 Teleporter archives are parsed in the browser")

// maxArchiveBytes bounds the uncompressed content a Teleporter ZIP can expand
// to. The HTTP layer already caps the compressed upload; this is the zip-bomb
// guard for the decompression itself.
const maxArchiveBytes = 256 << 20

// ParseAdGuardYAML turns an AdGuard Home configuration into a Bundle. It reads
// only the allowlisted policy fields; users, TLS, HTTP and DHCP tables are never
// consulted, and the preview records that they were skipped.
func ParseAdGuardYAML(data []byte) (Bundle, error) {
	tree, err := DecodeYAML(data)
	if err != nil {
		return Bundle{}, err
	}
	bundle := Bundle{
		SchemaVersion: SchemaVersion,
		Source: Source{
			Kind:        SourceAdGuardHome,
			Version:     "adguard-home",
			Fingerprint: Fingerprint(data),
		},
	}
	parseAdGuardTree(tree, &bundle)
	return bundle, nil
}

func parseAdGuardTree(tree map[string]any, bundle *Bundle) {
	for _, entry := range sliceValue(tree["filters"]) {
		m := mapValue(entry)
		if m == nil {
			continue
		}
		url := stringValue(m["url"])
		if url == "" {
			bundle.Skipped = append(bundle.Skipped, Issue{
				Code:   "filter-list-invalid-url",
				Item:   stringValue(m["name"]),
				Reason: "filter has no URL",
			})
			continue
		}
		bundle.FilterLists = append(bundle.FilterLists, FilterList{
			Name:     stringValue(m["name"]),
			URL:      url,
			Homepage: stringValue(m["homepage"]),
			License:  stringValue(m["license"]),
			Enabled:  boolValueDefault(m["enabled"], true),
		})
	}
	for _, entry := range sliceValue(tree["whitelist_filters"]) {
		m := mapValue(entry)
		name := ""
		if m != nil {
			name = stringValue(m["name"])
		}
		bundle.Warnings = append(bundle.Warnings, Issue{
			Code:   "allowlist-list-not-imported",
			Item:   name,
			Reason: "allowlist subscriptions have no separate DigitDNS list type; import their rules as custom allow rules instead",
		})
	}

	if rules := stringSlice(tree["user_rules"]); len(rules) > 0 {
		ensureFiltering(bundle).CustomRules = strings.Join(rules, "\n")
	}
	if filtering := mapValue(tree["filtering"]); filtering != nil {
		if enabled, ok := boolValue(filtering["filtering_enabled"]); ok {
			ensureFiltering(bundle).Enabled = &enabled
		}
	}

	if dns := mapValue(tree["dns"]); dns != nil {
		bundle.Upstreams = append(bundle.Upstreams, stringSlice(dns["upstream_dns"])...)
		for _, entry := range sliceValue(dns["rewrites"]) {
			m := mapValue(entry)
			if m == nil {
				continue
			}
			domain := stringValue(m["domain"])
			answer := stringValue(m["answer"])
			if domain == "" || answer == "" {
				bundle.Skipped = append(bundle.Skipped, Issue{
					Code:   "rewrite-invalid",
					Item:   domain,
					Reason: "rewrite is missing a domain or an answer",
				})
				continue
			}
			if ip := net.ParseIP(strings.Trim(answer, "[]")); ip != nil {
				typ := "A"
				if ip.To4() == nil {
					typ = "AAAA"
				}
				bundle.Records = append(bundle.Records, Record{Name: domain, Type: typ, Value: ip.String()})
			} else {
				bundle.Rewrites = append(bundle.Rewrites, Rewrite{Domain: domain, Target: answer})
			}
		}
		if clients := stringSlice(dns["allowed_clients"]); len(clients) > 0 {
			bundle.Warnings = append(bundle.Warnings, Issue{
				Code:   "allowed-clients-not-imported",
				Item:   fmt.Sprintf("%d entries", len(clients)),
				Reason: "AdGuard allowed_clients is a global access list, not a subscriber account",
			})
		}
	}

	if clients := mapValue(tree["clients"]); clients != nil {
		parseAdGuardClients(sliceValue(clients["persistent"]), bundle)
	} else {
		parseAdGuardClients(sliceValue(tree["clients"]), bundle)
	}

	for _, skipped := range []string{"dhcp", "querylog", "users", "tls", "http"} {
		if _, ok := tree[skipped]; ok {
			bundle.Warnings = append(bundle.Warnings, Issue{
				Code:   "source-section-not-imported",
				Item:   skipped,
				Reason: "this section is never imported: it is secret material, server state or query history",
			})
		}
	}
}

func parseAdGuardClients(entries []any, bundle *Bundle) {
	for _, entry := range entries {
		m := mapValue(entry)
		if m == nil {
			continue
		}
		name := stringValue(m["name"])
		ids := stringSlice(m["ids"])
		if name == "" && len(ids) == 0 {
			continue
		}
		client := Client{
			Name:      name,
			IDs:       ids,
			Enabled:   boolValueDefault(m["enabled"], true),
			Upstreams: stringSlice(m["upstreams"]),
		}
		if enabled, ok := boolValue(m["filtering_enabled"]); ok {
			client.FilteringEnabled = &enabled
		}
		bundle.Clients = append(bundle.Clients, client)
	}
}

// ParsePiHoleTOML reads the policy half of a Pi-hole v6 pihole.toml. The
// browser supplies the gravity.db rows; this function supplies upstreams and
// filtering settings that live only in the TOML document.
func ParsePiHoleTOML(data []byte) (Bundle, error) {
	tree, err := DecodeTOML(data)
	if err != nil {
		return Bundle{}, err
	}
	bundle := Bundle{
		SchemaVersion: SchemaVersion,
		Source: Source{
			Kind:        SourcePiHoleV6,
			Version:     "v6",
			Fingerprint: Fingerprint(data),
		},
	}
	dns := mapValue(tree["dns"])
	if dns != nil {
		for _, raw := range stringSlice(dns["upstreams"]) {
			bundle.Upstreams = append(bundle.Upstreams, normalizePiHoleUpstream(raw))
		}
	}
	if filtering := mapValue(tree["filtering"]); filtering != nil {
		parsePiHoleFiltering(filtering, &bundle)
	}
	if dns != nil {
		parsePiHoleFiltering(dns, &bundle)
	}
	for _, skipped := range []string{"dhcp", "webserver", "database"} {
		if _, ok := tree[skipped]; ok {
			bundle.Warnings = append(bundle.Warnings, Issue{
				Code:   "source-section-not-imported",
				Item:   skipped,
				Reason: "this section is never imported: it is secret material, DHCP state or query history",
			})
		}
	}
	return bundle, nil
}

func parsePiHoleFiltering(m map[string]any, bundle *Bundle) {
	if enabled, ok := boolValue(m["enabled"]); ok {
		ensureFiltering(bundle).Enabled = &enabled
	}
	if enabled, ok := boolValue(m["blocking_enabled"]); ok {
		ensureFiltering(bundle).Enabled = &enabled
	}
	if enabled, ok := boolValue(m["blocking_active"]); ok {
		ensureFiltering(bundle).Enabled = &enabled
	}
	mode := stringValue(m["blocking_mode"])
	if mode == "" {
		mode = stringValue(m["blockingmode"])
	}
	if mapped := mapPiHoleBlockMode(mode); mapped != "" {
		ensureFiltering(bundle).BlockMode = mapped
	}
	if ttl, ok := intValue(m["blocking_ttl"]); ok {
		ensureFiltering(bundle).BlockTTL = &ttl
	}
	if ttl, ok := intValue(m["blockingttl"]); ok {
		ensureFiltering(bundle).BlockTTL = &ttl
	}
}

// ParsePiHoleV5ZIP reads a Pi-hole v5 Teleporter archive. It recognises files by
// base name so a vendor that nests them under etc/pihole/ still works, and it
// never reads the password, DHCP leases or query database.
func ParsePiHoleV5ZIP(data []byte) (Bundle, error) {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return Bundle{}, fmt.Errorf("read Teleporter ZIP: %w", err)
	}
	bundle := Bundle{
		SchemaVersion: SchemaVersion,
		Source: Source{
			Kind:        SourcePiHoleV5,
			Version:     "v5",
			Fingerprint: Fingerprint(data),
		},
	}

	var gravity, whitelist, blacklist, custom, setupVars, adlists, ftlConf string
	var dnsmasq []string
	var sawHosts, sawLeases bool
	var expanded int64

	for _, file := range reader.File {
		name := strings.ToLower(file.Name)
		base := strings.ToLower(path.Base(name))
		if strings.HasSuffix(base, "gravity.db") || base == "pihole.toml" {
			return Bundle{}, ErrPiHoleV6NeedsBrowser
		}
		switch {
		case base == "gravity.list":
			gravity, err = readZipFile(file, &expanded)
		case base == "whitelist.txt":
			whitelist, err = readZipFile(file, &expanded)
		case base == "blacklist.txt":
			blacklist, err = readZipFile(file, &expanded)
		case base == "custom.list":
			custom, err = readZipFile(file, &expanded)
		case base == "setupvars.conf":
			setupVars, err = readZipFile(file, &expanded)
		case base == "adlists.list":
			adlists, err = readZipFile(file, &expanded)
		case base == "pihole-ftl.conf":
			ftlConf, err = readZipFile(file, &expanded)
		case strings.Contains(name, "dnsmasq.d/") && strings.HasSuffix(base, ".conf"):
			var text string
			text, err = readZipFile(file, &expanded)
			if text != "" {
				dnsmasq = append(dnsmasq, text)
			}
		case base == "hosts":
			sawHosts = true
		case base == "dhcp.leases":
			sawLeases = true
		}
		if err != nil {
			return Bundle{}, err
		}
	}

	parsePiHoleV5Lists(&bundle, gravity, whitelist, blacklist, custom, adlists)
	parsePiHoleV5SetupVars(&bundle, setupVars)
	parsePiHoleV5FTLConf(&bundle, ftlConf)
	for _, text := range dnsmasq {
		parseDnsmasqConf(&bundle, text)
	}
	if sawHosts {
		bundle.Warnings = append(bundle.Warnings, Issue{
			Code:   "hosts-file-not-imported",
			Item:   "/etc/hosts",
			Reason: "host-file entries are machine state, not operator policy",
		})
	}
	if sawLeases {
		bundle.Warnings = append(bundle.Warnings, Issue{
			Code:   "dhcp-leases-not-imported",
			Item:   "dhcp.leases",
			Reason: "DHCP leases are live state and are never imported",
		})
	}
	return bundle, nil
}

func readZipFile(file *zip.File, expanded *int64) (string, error) {
	if file.FileInfo().IsDir() {
		return "", nil
	}
	if file.UncompressedSize64 > maxArchiveBytes || *expanded+int64(file.UncompressedSize64) > maxArchiveBytes {
		return "", fmt.Errorf("Teleporter archive expands past %d bytes", maxArchiveBytes)
	}
	rc, err := file.Open()
	if err != nil {
		return "", fmt.Errorf("open %s: %w", file.Name, err)
	}
	defer rc.Close()
	body, err := io.ReadAll(io.LimitReader(rc, maxArchiveBytes+1))
	if err != nil {
		return "", fmt.Errorf("read %s: %w", file.Name, err)
	}
	*expanded += int64(len(body))
	return string(body), nil
}

func parsePiHoleV5Lists(bundle *Bundle, gravity, whitelist, blacklist, custom, adlists string) {
	urls := listLines(adlists)
	for _, raw := range urls {
		if !strings.HasPrefix(raw, "http://") && !strings.HasPrefix(raw, "https://") {
			bundle.Skipped = append(bundle.Skipped, Issue{Code: "filter-list-invalid-url", Item: raw, Reason: "adlist is not an http or https URL"})
			continue
		}
		bundle.FilterLists = append(bundle.FilterLists, FilterList{Name: raw, URL: raw, Enabled: true})
	}
	if gravity != "" {
		if len(urls) > 0 {
			bundle.Warnings = append(bundle.Warnings, Issue{
				Code:   "gravity-not-imported",
				Item:   "gravity.list",
				Reason: "adlist subscriptions were imported instead; the compiled gravity list would duplicate them",
			})
		} else {
			bundle.FilterLists = append(bundle.FilterLists, FilterList{
				Name:    "Pi-hole gravity snapshot",
				Enabled: true,
				Content: gravity,
			})
		}
	}
	for _, line := range listLines(whitelist) {
		if domain := domainFromRuleLine(line); domain != "" {
			bundle.Allowlist = append(bundle.Allowlist, domain)
		}
	}
	for _, line := range listLines(blacklist) {
		if domain := domainFromRuleLine(line); domain != "" {
			bundle.Blocked = append(bundle.Blocked, domain)
		}
	}
	for _, line := range listLines(custom) {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		ip := net.ParseIP(strings.Trim(fields[0], "[]"))
		domain := fields[1]
		if ip == nil {
			ip = net.ParseIP(strings.Trim(fields[1], "[]"))
			domain = fields[0]
		}
		if ip == nil {
			bundle.Skipped = append(bundle.Skipped, Issue{Code: "record-invalid-address", Item: line, Reason: "custom.list line has no address"})
			continue
		}
		typ := "A"
		if ip.To4() == nil {
			typ = "AAAA"
		}
		bundle.Records = append(bundle.Records, Record{Name: domain, Type: typ, Value: ip.String()})
	}
}

func parsePiHoleV5SetupVars(bundle *Bundle, text string) {
	for _, line := range listLines(text) {
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(strings.Trim(value, `"'`))
		switch {
		case strings.HasPrefix(key, "PIHOLE_DNS_"):
			if value != "" {
				bundle.Upstreams = append(bundle.Upstreams, normalizePiHoleUpstream(value))
			}
		case key == "BLOCKING_ENABLED":
			if enabled, ok := parseBool(value); ok {
				ensureFiltering(bundle).Enabled = &enabled
			}
		case key == "WEBPASSWORD":
			bundle.Warnings = append(bundle.Warnings, Issue{
				Code:   "web-password-not-imported",
				Item:   key,
				Reason: "the Pi-hole web password is a credential and is never imported",
			})
		}
	}
}

func parsePiHoleV5FTLConf(bundle *Bundle, text string) {
	for _, line := range listLines(text) {
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch strings.ToUpper(strings.TrimSpace(key)) {
		case "BLOCKINGMODE":
			if mapped := mapPiHoleBlockMode(value); mapped != "" {
				ensureFiltering(bundle).BlockMode = mapped
			}
		case "BLOCKINGTTL":
			if ttl, err := parseInt(value); err == nil {
				ensureFiltering(bundle).BlockTTL = &ttl
			}
		}
	}
}

func parseDnsmasqConf(bundle *Bundle, text string) {
	for _, line := range listLines(text) {
		lower := strings.ToLower(line)
		switch {
		case strings.HasPrefix(lower, "address=/"):
			parts := strings.Split(line, "/")
			if len(parts) < 3 {
				continue
			}
			domain := strings.TrimSpace(parts[1])
			target := strings.TrimSpace(parts[2])
			if domain == "" || target == "" {
				continue
			}
			if ip := net.ParseIP(strings.Trim(target, "[]")); ip != nil {
				typ := "A"
				if ip.To4() == nil {
					typ = "AAAA"
				}
				bundle.Records = append(bundle.Records, Record{Name: domain, Type: typ, Value: ip.String()})
			} else {
				bundle.Rewrites = append(bundle.Rewrites, Rewrite{Domain: domain, Target: target})
			}
		case strings.HasPrefix(lower, "server="):
			value := strings.TrimSpace(line[len("server="):])
			if value == "" || strings.Contains(value, "/") {
				continue
			}
			bundle.Upstreams = append(bundle.Upstreams, normalizePiHoleUpstream(value))
		case strings.HasPrefix(lower, "cname="):
			value := strings.TrimSpace(line[len("cname="):])
			domain, target, ok := strings.Cut(value, ",")
			if ok {
				bundle.Rewrites = append(bundle.Rewrites, Rewrite{Domain: strings.TrimSpace(domain), Target: strings.TrimSpace(target)})
			}
		case strings.HasPrefix(lower, "host-record="):
			value := strings.TrimSpace(line[len("host-record="):])
			fields := strings.Split(value, ",")
			if len(fields) >= 2 {
				if ip := net.ParseIP(strings.Trim(fields[1], "[]")); ip != nil {
					typ := "A"
					if ip.To4() == nil {
						typ = "AAAA"
					}
					bundle.Records = append(bundle.Records, Record{Name: fields[0], Type: typ, Value: ip.String()})
				}
			}
		}
	}
}

func listLines(text string) []string {
	var out []string
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(strings.TrimSuffix(raw, "\r"))
		line = strings.TrimPrefix(line, "\ufeff")
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "!") || strings.HasPrefix(line, "//") {
			continue
		}
		out = append(out, line)
	}
	return out
}

func domainFromRuleLine(line string) string {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return ""
	}
	if net.ParseIP(strings.Trim(fields[0], "[]")) != nil {
		if len(fields) < 2 {
			return ""
		}
		return fields[1]
	}
	return fields[0]
}

func normalizePiHoleUpstream(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" || strings.Contains(s, "://") {
		return s
	}
	if i := strings.LastIndex(s, "#"); i > 0 {
		host := s[:i]
		port := s[i+1:]
		if host != "" && port != "" {
			if ip := net.ParseIP(host); ip != nil {
				return net.JoinHostPort(host, port)
			}
			return host + ":" + port
		}
	}
	return s
}

func mapPiHoleBlockMode(raw string) string {
	switch strings.ToUpper(strings.TrimSpace(raw)) {
	case "NXDOMAIN":
		return "nxdomain"
	case "NULL", "IP", "IP-NODATA-AAAA":
		return "null_ip"
	case "NODATA":
		return "refused"
	default:
		return ""
	}
}

func ensureFiltering(bundle *Bundle) *Filtering {
	if bundle.Filtering == nil {
		bundle.Filtering = &Filtering{}
	}
	return bundle.Filtering
}

func mapValue(value any) map[string]any {
	switch typed := value.(type) {
	case map[string]any:
		return typed
	case map[any]any:
		out := make(map[string]any, len(typed))
		for key, child := range typed {
			out[fmt.Sprint(key)] = child
		}
		return out
	default:
		return nil
	}
}

func sliceValue(value any) []any {
	out, _ := value.([]any)
	return out
}

func stringValue(value any) string {
	if value == nil {
		return ""
	}
	if s, ok := value.(string); ok {
		return strings.TrimSpace(s)
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func stringSlice(value any) []string {
	switch typed := value.(type) {
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if s := stringValue(item); s != "" {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return append([]string(nil), typed...)
	default:
		return nil
	}
}

func boolValue(value any) (bool, bool) {
	switch typed := value.(type) {
	case bool:
		return typed, true
	case string:
		return parseBool(typed)
	default:
		return false, false
	}
}

func boolValueDefault(value any, fallback bool) bool {
	if parsed, ok := boolValue(value); ok {
		return parsed
	}
	return fallback
}

func parseBool(raw string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "true", "1", "yes", "on":
		return true, true
	case "false", "0", "no", "off":
		return false, true
	default:
		return false, false
	}
}

func intValue(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int64:
		return int(typed), true
	case float64:
		return int(typed), true
	case string:
		parsed, err := parseInt(typed)
		if err != nil {
			return 0, false
		}
		return parsed, true
	default:
		return 0, false
	}
}

func parseInt(raw string) (int, error) {
	var out int
	_, err := fmt.Sscanf(strings.TrimSpace(raw), "%d", &out)
	return out, err
}
