package matcher

import (
	"slices"
	"testing"
)

// A single-line Add box used to store a pasted list as one space-separated
// string. The matcher could never match that pattern, so every domain in it
// stayed unblocked. SplitDomainEntries is the repair path: it flattens any entry
// that carries several names.
func TestSplitDomainEntriesFlattensPastedLists(t *testing.T) {
	got := SplitDomainEntries([]string{
		"ads.one.example ads.two.example,ads.three.example;ads.four.example",
		"*.wild.example",
		"  ",
		"ads.five.example\nads.six.example",
	})
	want := []string{
		"ads.one.example",
		"ads.two.example",
		"ads.three.example",
		"ads.four.example",
		"*.wild.example",
		"ads.five.example",
		"ads.six.example",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("SplitDomainEntries = %v, want %v", got, want)
	}
}

// A blob stored before the fix must still block the names it carries. This is
// the behaviour the operator actually lost: the list looked populated while the
// engine matched nothing.
func TestSetCustomRulesSplitsAStoredBlob(t *testing.T) {
	m := NewMatcher()
	m.SetCustomRules(nil, []string{"ads.one.example ads.two.example"}, nil, nil)

	for _, domain := range []string{"ads.one.example", "ads.two.example"} {
		if action, _ := m.Match(domain); action != ActionBlock {
			t.Errorf("Match(%q) = %v, want ActionBlock", domain, action)
		}
	}
}
