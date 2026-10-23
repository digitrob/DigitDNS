package web

import (
	"reflect"
	"testing"
)

// TestDedupeDomainsKeepsFirstOccurrence is the regression for a crash, not a
// cosmetic: a live install carried "xxx.heimaoip.com" twice in custom_proxied, the
// Android Domains list keys rows by status+domain, and duplicate keys abort that
// screen on open.
func TestDedupeDomainsKeepsFirstOccurrence(t *testing.T) {
	in := []string{"a.com", "b.com", "a.com", "", "b.com", "c.com"}
	got := dedupeDomains(in)
	want := []string{"a.com", "b.com", "c.com"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("dedupeDomains(%v) = %v, want %v", in, got, want)
	}
}

func TestDedupeDomainsShortInputsPassThrough(t *testing.T) {
	for _, in := range [][]string{nil, {}, {"only.com"}} {
		got := dedupeDomains(in)
		if len(in) == 1 && (len(got) != 1 || got[0] != in[0]) {
			t.Fatalf("dedupeDomains(%v) = %v, want unchanged", in, got)
		}
		if len(in) == 0 && len(got) != 0 {
			t.Fatalf("dedupeDomains(%v) = %v, want empty", in, got)
		}
	}
}
