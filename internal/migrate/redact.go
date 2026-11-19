package migrate

import (
	"fmt"
	"sort"
	"strings"
)

// The importer's first rule is that a source file is not a configuration dump.
// A parser reads only the fields it understands, and Redact is the second line
// of defence for any diagnostic that shows the source tree: credentials, TLS
// material, sessions, DHCP state and query history are removed before a map can
// reach a preview, a log line or an HTTP response.
//
// The list is intentionally conservative. Removing a harmless key is a cosmetic
// loss in a diagnostic; leaving a password or a private key in is a security
// incident. New source-specific secrets belong here, not in a parser-local
// exception.

// neverImportKeyNames are matched against the last segment of a path, at any
// depth. They are the names that mean "secret" in both Pi-hole and AdGuard Home
// exports and in the common surrounding tooling.
var neverImportKeyNames = map[string]struct{}{
	"pwhash":      {},
	"password":    {},
	"passwd":      {},
	"pass":        {},
	"api_key":     {},
	"apikey":      {},
	"api_token":   {},
	"token":       {},
	"secret":      {},
	"private_key": {},
	"privkey":     {},
	"key_pem":     {},
	"cert_pem":    {},
	"session":     {},
	"sessions":    {},
	"cookie":      {},
	"cookies":     {},
	"csrf":        {},
	"csrf_token":  {},
	"lease":       {},
	"leases":      {},
	"query_log":   {},
	"querylog":    {},
	"queries":     {},
	"history":     {},
}

// neverImportPathPrefixes cover whole subtrees whose every field is either a
// secret or state the importer must never replay. A path matches when it equals
// the prefix or sits below it.
var neverImportPathPrefixes = []string{
	"webserver.api.pwhash",
	"webserver.api.api_key",
	"webserver.api.sessions",
	"webserver.session",
	"webserver.tls",
	"tls",
	"dhcp",
	"queries",
	"query_log",
	"querylog",
	"database",
	"files",
	"sessions",
}

// IsNeverImport reports whether a dotted path names data the importer must not
// copy. Array indices are ignored: users[0].password and users[1].password both
// match the password rule.
func IsNeverImport(path string) bool {
	normalized := normalizeIssuePath(path)
	if normalized == "" {
		return false
	}
	for _, prefix := range neverImportPathPrefixes {
		if normalized == prefix || strings.HasPrefix(normalized, prefix+".") {
			return true
		}
	}
	for _, segment := range strings.Split(normalized, ".") {
		if _, ok := neverImportKeyNames[segment]; ok {
			return true
		}
	}
	return false
}

// NeverImportKeys returns the exact key names in lexical order. It exists for
// documentation and tests; the enforcement point is IsNeverImport.
func NeverImportKeys() []string {
	out := make([]string, 0, len(neverImportKeyNames))
	for key := range neverImportKeyNames {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

// Redact returns a deep copy of a decoded YAML/TOML tree with every never-import
// path removed. The original map is not mutated, and Removed is sorted so a
// preview and a test see the same order on every run.
func Redact(input map[string]any) (map[string]any, []string) {
	if input == nil {
		return map[string]any{}, nil
	}
	value, removed := redactValue("", input)
	out, ok := value.(map[string]any)
	if !ok {
		out = map[string]any{}
	}
	sort.Strings(removed)
	return out, removed
}

func redactValue(path string, value any) (any, []string) {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		var removed []string
		for _, key := range keys {
			childPath := joinIssuePath(path, key)
			if IsNeverImport(childPath) {
				removed = append(removed, childPath)
				continue
			}
			child, childRemoved := redactValue(childPath, typed[key])
			out[key] = child
			removed = append(removed, childRemoved...)
		}
		return out, removed
	case map[any]any:
		// YAML v2 and some hand-built maps use non-string keys. Convert them to
		// strings before applying the same rules; a key that cannot be rendered
		// is skipped rather than retained unredacted.
		out := make(map[string]any, len(typed))
		var removed []string
		for key, child := range typed {
			name, ok := key.(string)
			if !ok {
				name = fmt.Sprint(key)
			}
			childPath := joinIssuePath(path, name)
			if IsNeverImport(childPath) {
				removed = append(removed, childPath)
				continue
			}
			redacted, childRemoved := redactValue(childPath, child)
			out[name] = redacted
			removed = append(removed, childRemoved...)
		}
		return out, removed
	case []any:
		out := make([]any, len(typed))
		var removed []string
		for i, child := range typed {
			childPath := fmt.Sprintf("%s[%d]", path, i)
			redacted, childRemoved := redactValue(childPath, child)
			out[i] = redacted
			removed = append(removed, childRemoved...)
		}
		return out, removed
	default:
		return value, nil
	}
}

func joinIssuePath(parent, key string) string {
	if parent == "" {
		return key
	}
	return parent + "." + key
}

func normalizeIssuePath(path string) string {
	path = strings.TrimSpace(strings.ToLower(path))
	if path == "" {
		return ""
	}
	var segments []string
	for _, segment := range strings.Split(path, ".") {
		if i := strings.IndexByte(segment, '['); i >= 0 {
			segment = segment[:i]
		}
		if segment != "" {
			segments = append(segments, segment)
		}
	}
	return strings.Join(segments, ".")
}
