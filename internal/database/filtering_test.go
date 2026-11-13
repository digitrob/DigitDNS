package database

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestFilteringSettingsNormalizeCustomRules(t *testing.T) {
	got := (FilteringSettings{CustomRules: " \n||ads.example^\n "}).Normalize()
	if got.CustomRules != "||ads.example^" {
		t.Fatalf("CustomRules = %q, want trimmed rule", got.CustomRules)
	}

	// A byte limit may land in the middle of a multi-byte rune. Normalization
	// must keep the stored JSON string valid UTF-8 when it applies the cap.
	input := strings.Repeat("a", MaxFilteringCustomRulesBytes-1) + "é"
	got = (FilteringSettings{CustomRules: input}).Normalize()
	if len(got.CustomRules) > MaxFilteringCustomRulesBytes {
		t.Fatalf("CustomRules has %d bytes, limit is %d", len(got.CustomRules), MaxFilteringCustomRulesBytes)
	}
	if !utf8.ValidString(got.CustomRules) {
		t.Fatal("normalized CustomRules is not valid UTF-8")
	}
}
