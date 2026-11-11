package api

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"
)

// TestOpenAPIV3CoversEveryRoute is the drift guard for the v3 contract. The
// spec used to be edited by hand and quietly fell behind the router: seven
// routes were reachable but undocumented. Parsing the router source keeps the
// two lists honest without a second hand-maintained inventory.
func TestOpenAPIV3CoversEveryRoute(t *testing.T) {
	src, err := os.ReadFile("router_v3.go")
	if err != nil {
		t.Fatalf("read router_v3.go: %v", err)
	}
	routeRe := regexp.MustCompile(`HandleFunc\("(/api/v3[^"]*)"`)
	matches := routeRe.FindAllStringSubmatch(string(src), -1)
	if len(matches) == 0 {
		t.Fatal("no v3 routes found in router_v3.go; the extraction pattern is stale")
	}

	var spec struct {
		Paths map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal(openAPIV3Spec, &spec); err != nil {
		t.Fatalf("openapi/v3.json is not valid JSON: %v", err)
	}
	paths := make([]string, 0, len(spec.Paths))
	for p := range spec.Paths {
		paths = append(paths, p)
	}

	paramRe := regexp.MustCompile(`\{[^}]+\}`)
	norm := func(p string) string {
		p = paramRe.ReplaceAllString(p, "{id}")
		if len(p) > 1 {
			p = strings.TrimSuffix(p, "/")
		}
		return p
	}
	// A route is documented when the spec has the exact path or a path below
	// it. A router prefix route ("/clients/") is what dispatches
	// "/clients/{id}", so a prefix with spec children counts. The same
	// predicate answers both directions: the router route is the prefix and
	// the spec path is the candidate.
	covered := func(prefix, candidate string) bool {
		return candidate == prefix ||
			strings.HasPrefix(candidate, prefix+"/") ||
			strings.HasPrefix(candidate, prefix+"{")
	}

	routes := make([]string, 0, len(matches))
	for _, m := range matches {
		routes = append(routes, m[1])
	}
	for _, route := range routes {
		ok := false
		for _, p := range paths {
			if covered(norm(route), norm(p)) {
				ok = true
				break
			}
		}
		if !ok {
			t.Errorf("v3 route %s is not documented in internal/api/openapi/v3.json", route)
		}
	}
	for _, p := range paths {
		ok := false
		for _, route := range routes {
			if covered(norm(route), norm(p)) {
				ok = true
				break
			}
		}
		if !ok {
			t.Errorf("openapi/v3.json documents %s but router_v3.go has no route for it", p)
		}
	}
}

// TestOpenAPIV3OperationsAreDescribed keeps the "detailed docs" promise from
// decaying into a list of bare summaries.
func TestOpenAPIV3OperationsAreDescribed(t *testing.T) {
	var spec struct {
		Paths map[string]map[string]struct {
			Summary     string `json:"summary"`
			Description string `json:"description"`
		} `json:"paths"`
	}
	if err := json.Unmarshal(openAPIV3Spec, &spec); err != nil {
		t.Fatalf("openapi/v3.json is not valid JSON: %v", err)
	}
	for path, ops := range spec.Paths {
		for method, op := range ops {
			switch method {
			case "get", "post", "put", "patch", "delete":
				if strings.TrimSpace(op.Description) == "" {
					t.Errorf("%s %s has no description", strings.ToUpper(method), path)
				}
			}
		}
	}
}
