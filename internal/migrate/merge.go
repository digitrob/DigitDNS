package migrate

import (
	"fmt"
	"strings"

	"digitdns/internal/blocklist"
)

// Target is the live configuration shape the importer merges into. It is a
// deliberately small projection of the real settings, rules and filter-list
// records: the apply handler owns the database transaction and converts this
// projection into the concrete types, while Merge stays pure and testable.
//
// Records is the current custom-answer map: an A or AAAA address, a CNAME
// hostname, or one of the resolver's special rewrite values. Rewrites is kept in
// the projection for callers that still carry them separately; Merge folds any
// pre-existing entry into Records and stores incoming rewrites there too, so a
// CNAME import is served by the same lookup as a static address.
type Target struct {
	Records     map[string]string
	Clients     []Client
	Proxied     []string
	Blocked     []string
	Direct      []string
	Allowlist   []string
	FilterLists []FilterList
	Rewrites    []Rewrite
	Filtering   *Filtering
	Upstreams   []string
}

// NewTarget returns an empty target with the map initialized. A zero Target is
// also valid; Merge initializes what it needs.
func NewTarget() Target {
	return Target{Records: make(map[string]string)}
}

// MergeResult is the outcome of one additive merge. Added counts what the
// target gained; Skipped explains every item the merge refused, including a
// conflict with an existing value.
type MergeResult struct {
	Target  Target
	Added   Stats
	Skipped []Issue
}

// Merge applies bundle to target additively: existing records, clients, rules,
// lists, rewrites and upstreams are never removed or overwritten. A conflicting
// record or rewrite is reported in Skipped so the preview can show exactly what
// did not land. The input target is not mutated.
func Merge(target Target, bundle Bundle) MergeResult {
	bundle = bundle.Normalize()
	out := cloneTarget(target)
	result := MergeResult{Target: out}

	if out.Records == nil {
		out.Records = make(map[string]string)
	}
	for _, record := range bundle.Records {
		// The custom-answer map holds an A/AAAA address or a CNAME hostname; the
		// resolver picks the record type from the value. Types it cannot serve
		// (TXT, MX, NS, SRV, PTR) are reported rather than written.
		switch record.Type {
		case "A", "AAAA", "CNAME":
		default:
			result.Skipped = append(result.Skipped, Issue{
				Code:   "record-unsupported-apply",
				Item:   record.Name,
				Reason: fmt.Sprintf("%s records are not supported by the current custom-record store", record.Type),
			})
			continue
		}
		if existing, ok := out.Records[record.Name]; ok {
			if existing != record.Value {
				result.Skipped = append(result.Skipped, Issue{
					Code:   "record-conflict",
					Item:   record.Name,
					Reason: "a different address is already configured for this name",
				})
			}
			continue
		}
		out.Records[record.Name] = record.Value
		result.Added.Records++
	}

	clientNames := make(map[string]struct{}, len(out.Clients))
	clientIDs := make(map[string]struct{})
	for _, client := range out.Clients {
		clientNames[strings.ToLower(client.Name)] = struct{}{}
		for _, id := range client.IDs {
			clientIDs[id] = struct{}{}
		}
	}
	for _, client := range bundle.Clients {
		nameKey := strings.ToLower(client.Name)
		if _, exists := clientNames[nameKey]; exists {
			result.Skipped = append(result.Skipped, Issue{
				Code:   "client-duplicate",
				Item:   client.Name,
				Reason: "a client with this name is already configured",
			})
			continue
		}
		conflict := ""
		for _, id := range client.IDs {
			if _, exists := clientIDs[id]; exists {
				conflict = id
				break
			}
		}
		if conflict != "" {
			result.Skipped = append(result.Skipped, Issue{
				Code:   "client-id-conflict",
				Item:   conflict,
				Reason: "this identifier is already bound to another client",
			})
			continue
		}
		out.Clients = append(out.Clients, client)
		clientNames[nameKey] = struct{}{}
		for _, id := range client.IDs {
			clientIDs[id] = struct{}{}
		}
		result.Added.Clients++
	}

	var added int
	out.Proxied, added = appendUniqueDomains(out.Proxied, bundle.Proxied)
	result.Added.Proxied = added
	out.Blocked, added = appendUniqueDomains(out.Blocked, bundle.Blocked)
	result.Added.Blocked = added
	out.Direct, added = appendUniqueDomains(out.Direct, bundle.Direct)
	result.Added.Direct = added
	out.Allowlist, added = appendUniqueDomains(out.Allowlist, bundle.Allowlist)
	result.Added.Allowlist = added

	listKeys := make(map[string]struct{}, len(out.FilterLists))
	for _, list := range out.FilterLists {
		listKeys[filterListKey(list)] = struct{}{}
	}
	for _, list := range bundle.FilterLists {
		key := filterListKey(list)
		if _, exists := listKeys[key]; exists {
			result.Skipped = append(result.Skipped, Issue{
				Code:   "filter-list-duplicate",
				Item:   list.Name,
				Reason: "this list is already configured",
			})
			continue
		}
		out.FilterLists = append(out.FilterLists, list)
		listKeys[key] = struct{}{}
		result.Added.FilterLists++
		result.Added.FilterRules += filterListRules(list)
	}

	// Rewrites live in the same map as static records: the resolver answers a
	// non-IP value as a CNAME and an address as A/AAAA, so no separate store is
	// needed. Fold any pre-existing Rewrites entry in first so the duplicate and
	// conflict checks below see it.
	for _, rewrite := range out.Rewrites {
		if _, exists := out.Records[rewrite.Domain]; !exists {
			out.Records[rewrite.Domain] = rewrite.Target
		}
	}
	for _, rewrite := range bundle.Rewrites {
		if existing, exists := out.Records[rewrite.Domain]; exists {
			if existing != rewrite.Target {
				result.Skipped = append(result.Skipped, Issue{
					Code:   "rewrite-conflict",
					Item:   rewrite.Domain,
					Reason: "a different answer is already configured for this name",
				})
			}
			continue
		}
		out.Records[rewrite.Domain] = rewrite.Target
		result.Added.Rewrites++
	}

	if bundle.UpstreamsOptIn {
		out.Upstreams, added = appendUniqueStrings(out.Upstreams, bundle.Upstreams)
		result.Added.Upstreams = added
	} else if len(bundle.Upstreams) > 0 {
		result.Skipped = append(result.Skipped, Issue{
			Code:   "upstreams-not-opted-in",
			Reason: "upstream addresses were left out because the import did not opt in",
		})
	}

	mergedFiltering, filteringIssues := mergeFiltering(out.Filtering, bundle.Filtering)
	out.Filtering = mergedFiltering
	result.Skipped = append(result.Skipped, filteringIssues...)

	result.Added.Warnings = len(bundle.Warnings)
	result.Added.Skipped = len(result.Skipped)
	// result.Target was copied before the slices and the Filtering pointer were
	// replaced. Publish the final value so the caller sees every addition, not
	// the empty headers the copy captured at the start.
	result.Target = out
	return result
}

func mergeFiltering(current, incoming *Filtering) (*Filtering, []Issue) {
	if incoming == nil {
		return cloneFiltering(current), nil
	}
	if current == nil {
		return cloneFiltering(incoming), nil
	}
	out := cloneFiltering(current)
	var issues []Issue
	// Rules, lists and clients are additive, but a filtering setting the source
	// actually specified is an explicit part of the preview. Applying it is the
	// point of the full-policy import; leaving an old block mode in place would
	// make the preview describe a state the operator never gets.
	if incoming.Enabled != nil {
		out.Enabled = incoming.Enabled
	}
	if incoming.BlockMode != "" {
		out.BlockMode = incoming.BlockMode
	}
	if incoming.BlockTTL != nil {
		out.BlockTTL = incoming.BlockTTL
	}
	if incoming.UpdateIntervalHours != nil {
		out.UpdateIntervalHours = incoming.UpdateIntervalHours
	}
	if strings.TrimSpace(incoming.CustomRules) != "" {
		combined := out.CustomRules
		if combined != "" {
			combined += "\n"
		}
		combined += incoming.CustomRules
		if len(combined) > MaxFilteringCustomRulesSize {
			issues = append(issues, Issue{
				Code:   "filtering-custom-rules-too-large",
				Reason: "the imported custom rules would exceed the resolver's size cap",
			})
		} else if len(CheckCustomRules(combined)) > 0 {
			issues = append(issues, Issue{
				Code:   "filtering-custom-rules-over-cap",
				Reason: "the imported custom rules would exceed the resolver's regex caps",
			})
		} else {
			out.CustomRules = combined
		}
	}
	return out, issues
}

func cloneTarget(in Target) Target {
	out := Target{
		Clients:     append([]Client(nil), in.Clients...),
		Proxied:     append([]string(nil), in.Proxied...),
		Blocked:     append([]string(nil), in.Blocked...),
		Direct:      append([]string(nil), in.Direct...),
		Allowlist:   append([]string(nil), in.Allowlist...),
		FilterLists: append([]FilterList(nil), in.FilterLists...),
		Rewrites:    append([]Rewrite(nil), in.Rewrites...),
		Filtering:   cloneFiltering(in.Filtering),
		Upstreams:   append([]string(nil), in.Upstreams...),
	}
	if in.Records != nil {
		out.Records = make(map[string]string, len(in.Records))
		for name, value := range in.Records {
			out.Records[name] = value
		}
	}
	return out
}

func cloneFiltering(in *Filtering) *Filtering {
	if in == nil {
		return nil
	}
	out := *in
	if in.Enabled != nil {
		v := *in.Enabled
		out.Enabled = &v
	}
	if in.BlockTTL != nil {
		v := *in.BlockTTL
		out.BlockTTL = &v
	}
	if in.UpdateIntervalHours != nil {
		v := *in.UpdateIntervalHours
		out.UpdateIntervalHours = &v
	}
	return &out
}

func appendUniqueDomains(dst, src []string) ([]string, int) {
	seen := make(map[string]struct{}, len(dst)+len(src))
	for _, d := range dst {
		seen[d] = struct{}{}
	}
	added := 0
	for _, d := range src {
		if _, exists := seen[d]; exists {
			continue
		}
		seen[d] = struct{}{}
		dst = append(dst, d)
		added++
	}
	return dst, added
}

func appendUniqueStrings(dst, src []string) ([]string, int) {
	seen := make(map[string]struct{}, len(dst)+len(src))
	for _, s := range dst {
		seen[s] = struct{}{}
	}
	added := 0
	for _, s := range src {
		if _, exists := seen[s]; exists {
			continue
		}
		seen[s] = struct{}{}
		dst = append(dst, s)
		added++
	}
	return dst, added
}

func filterListKey(list FilterList) string {
	if list.URL != "" {
		return "url:" + list.URL
	}
	return "content:" + list.Name + ":" + hashText(list.Content)
}

func filterListRules(list FilterList) int {
	if strings.TrimSpace(list.Content) == "" {
		return 0
	}
	return blocklist.Analyze(list.Content).Total()
}
