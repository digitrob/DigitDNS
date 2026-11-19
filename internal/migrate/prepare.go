package migrate

import (
	"fmt"
	"strings"

	"digitdns/internal/blocklist"
)

// MergeBundles combines two normalized fragments. The v6 path needs it because
// the browser reads gravity.db while the server reads pihole.toml; the two
// halves describe one source and must preview as one import.
func MergeBundles(a, b Bundle) Bundle {
	out := a
	out.Records = append(out.Records, b.Records...)
	out.Clients = append(out.Clients, b.Clients...)
	out.Proxied = append(out.Proxied, b.Proxied...)
	out.Blocked = append(out.Blocked, b.Blocked...)
	out.Direct = append(out.Direct, b.Direct...)
	out.Allowlist = append(out.Allowlist, b.Allowlist...)
	out.FilterLists = append(out.FilterLists, b.FilterLists...)
	out.Rewrites = append(out.Rewrites, b.Rewrites...)
	out.Upstreams = append(out.Upstreams, b.Upstreams...)
	out.Warnings = append(out.Warnings, b.Warnings...)
	out.Skipped = append(out.Skipped, b.Skipped...)
	out.UpstreamsOptIn = a.UpstreamsOptIn || b.UpstreamsOptIn
	if out.Source.Kind == "" {
		out.Source = b.Source
	}
	out.Filtering = mergeBundleFiltering(out.Filtering, b.Filtering)
	return out
}

func mergeBundleFiltering(a, b *Filtering) *Filtering {
	if b == nil {
		return a
	}
	if a == nil {
		return cloneFiltering(b)
	}
	out := cloneFiltering(a)
	if out.Enabled == nil && b.Enabled != nil {
		out.Enabled = b.Enabled
	}
	if out.BlockMode == "" && b.BlockMode != "" {
		out.BlockMode = b.BlockMode
	}
	if out.BlockTTL == nil && b.BlockTTL != nil {
		out.BlockTTL = b.BlockTTL
	}
	if out.UpdateIntervalHours == nil && b.UpdateIntervalHours != nil {
		out.UpdateIntervalHours = b.UpdateIntervalHours
	}
	if strings.TrimSpace(b.CustomRules) != "" {
		if strings.TrimSpace(out.CustomRules) != "" {
			out.CustomRules += "\n"
		}
		out.CustomRules += b.CustomRules
	}
	return out
}

// SplitOverCapCustomRules moves every regex the resolver would silently skip
// into Skipped. The remaining bundle is valid, and the report endpoint can hand
// the operator the exact patterns to fix or keep as a downloaded file.
func SplitOverCapCustomRules(b Bundle) (Bundle, []Issue) {
	if b.Filtering == nil || strings.TrimSpace(b.Filtering.CustomRules) == "" {
		return b, nil
	}
	var kept []string
	var issues []Issue
	regexCount := 0
	for _, raw := range strings.Split(b.Filtering.CustomRules, "\n") {
		line := strings.TrimSpace(strings.TrimSuffix(raw, "\r"))
		if line == "" {
			continue
		}
		prefix := ""
		check := line
		if strings.HasPrefix(check, "@@") {
			prefix = "@@"
			check = strings.TrimSpace(check[2:])
		}
		if strings.HasPrefix(check, "/") && strings.HasSuffix(check, "/") && len(check) >= 2 {
			regexCount++
			pattern := check[1 : len(check)-1]
			if len(pattern) > blocklist.MaxCustomRegexPatternSize {
				issues = append(issues, Issue{
					Code:   "regex-over-cap",
					Item:   pattern,
					Reason: fmt.Sprintf("regex is %d bytes; the resolver accepts at most %d", len(pattern), blocklist.MaxCustomRegexPatternSize),
				})
				continue
			}
			if regexCount > blocklist.MaxCustomRegexRules {
				issues = append(issues, Issue{
					Code:   "regex-over-cap",
					Item:   pattern,
					Reason: fmt.Sprintf("more than %d regex rules; the resolver accepts at most %d", blocklist.MaxCustomRegexRules, blocklist.MaxCustomRegexRules),
				})
				continue
			}
			kept = append(kept, prefix+check)
			continue
		}
		kept = append(kept, line)
	}
	b.Filtering.CustomRules = strings.Join(kept, "\n")
	b.Skipped = append(b.Skipped, issues...)
	// Stats were computed before the split, so the preview would otherwise say
	// nothing was skipped even though the report has entries.
	b.Stats = b.countStats()
	return b, issues
}
