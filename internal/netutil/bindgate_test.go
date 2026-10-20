package netutil

import "testing"

func TestParseAPIBindSentinelShapes(t *testing.T) {
	tests := []struct {
		name    string
		bind    string
		peer    string
		allowed bool
	}{
		{"empty is loopback only", "", "203.0.113.5", false},
		{"empty admits loopback v4", "", "127.0.0.1", true},
		{"explicit loopback admits ::1", "127.0.0.1", "::1", true},
		{"explicit loopback refuses remote", "127.0.0.1", "203.0.113.5", false},
		{"allow-all token admits remote", "0.0.0.0", "203.0.113.5", true},
		{"allow-all v6 token admits remote", "::", "203.0.113.5", true},
		{"allow-all /0 admits remote", "0.0.0.0/0", "203.0.113.5", true},
		{"cidr admits in-range peer", "100.64.0.0/10", "100.64.1.2", true},
		{"cidr refuses out-of-range peer", "100.64.0.0/10", "203.0.113.5", false},
		{"bare ip widens to /32", "203.0.113.5", "203.0.113.5", true},
		{"bare ip refuses neighbour", "203.0.113.5", "203.0.113.6", false},
		{"loopback always wins over narrow rule", "100.64.0.0/10", "127.0.0.1", true},
		{"garbage peer refused", "0.0.0.0", "not-an-ip", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := APIBindAllowsPeer(tc.bind, tc.peer); got != tc.allowed {
				t.Fatalf("APIBindAllowsPeer(%q, %q) = %v, want %v", tc.bind, tc.peer, got, tc.allowed)
			}
		})
	}
}

func TestParseAPIBindRejectsGarbage(t *testing.T) {
	for _, bad := range []string{"example.com", "10.0.0.0/99", "10.0.0.256", "300.1.2.3/24"} {
		if _, err := ParseAPIBind(bad); err == nil {
			t.Errorf("ParseAPIBind(%q) = nil error, want rejection", bad)
		}
	}
}

func TestAPIBindAllowsPeerFailsClosedOnStoredTypo(t *testing.T) {
	if APIBindAllowsPeer("not-a-block", "203.0.113.5") {
		t.Fatal("an unparseable stored api_bind admitted a remote peer")
	}
	if !APIBindAllowsPeer("not-a-block", "127.0.0.1") {
		t.Fatal("an unparseable stored api_bind locked out loopback")
	}
}

func TestParseAPIBindListDropsBlanks(t *testing.T) {
	list, err := ParseAPIBindList([]string{"  100.64.0.0/10 ", "", "   ", "203.0.113.5"})
	if err != nil {
		t.Fatalf("ParseAPIBindList: %v", err)
	}
	if !list.Allows("100.64.1.2") || !list.Allows("203.0.113.5") {
		t.Fatal("a valid entry was dropped with the blanks")
	}
}

func TestParseAPIBindListEmptyIsLoopbackOnly(t *testing.T) {
	for _, entries := range [][]string{nil, {}, {"", "   "}} {
		list, err := ParseAPIBindList(entries)
		if err != nil {
			t.Fatalf("ParseAPIBindList(%v): %v", entries, err)
		}
		if list.Allows("203.0.113.5") {
			t.Fatalf("empty list %v admitted a remote peer", entries)
		}
		if !list.Allows("127.0.0.1") {
			t.Fatalf("empty list %v locked out loopback", entries)
		}
	}
}

func TestParseAPIBindListFailsOnOneBadEntry(t *testing.T) {
	if _, err := ParseAPIBindList([]string{"100.64.0.0/10", "example.com"}); err == nil {
		t.Fatal("ParseAPIBindList accepted a list with a bad entry")
	}
}

func TestAPIBindListUnionSemantics(t *testing.T) {
	// A home /32 and a tailnet /10: either must work on its own — the list
	// widens the single rule rather than intersecting two restrictions.
	list, err := ParseAPIBindList([]string{"203.0.113.5", "100.64.0.0/10"})
	if err != nil {
		t.Fatalf("ParseAPIBindList: %v", err)
	}
	if !list.Allows("203.0.113.5") {
		t.Fatal("home address not admitted by the tailnet list")
	}
	if !list.Allows("100.64.9.9") {
		t.Fatal("tailnet address not admitted")
	}
	if list.Allows("192.0.2.7") {
		t.Fatal("unlisted address admitted")
	}
}

func TestAPIBindListAllowsAnyRuleAndLoopback(t *testing.T) {
	entries := []string{"100.64.0.0/10"}
	if !APIBindListAllowsPeer(entries, "100.64.1.2") {
		t.Fatal("in-range peer refused")
	}
	if APIBindListAllowsPeer(entries, "203.0.113.5") {
		t.Fatal("out-of-range peer admitted")
	}
	if !APIBindListAllowsPeer(entries, "::1") {
		t.Fatal("loopback refused by a tailnet-only list")
	}
}

func TestAPIBindListAllowAllToken(t *testing.T) {
	list, err := ParseAPIBindList([]string{"0.0.0.0"})
	if err != nil {
		t.Fatalf("ParseAPIBindList: %v", err)
	}
	if !list.Allows("203.0.113.5") {
		t.Fatal("a 0.0.0.0 rule in the list did not admit every peer")
	}
}

func TestAPIBindListAllowsPeerFailsClosedOnStoredTypo(t *testing.T) {
	if APIBindListAllowsPeer([]string{"not-a-block"}, "203.0.113.5") {
		t.Fatal("an unparseable stored list admitted a remote peer")
	}
	if !APIBindListAllowsPeer([]string{"not-a-block"}, "127.0.0.1") {
		t.Fatal("an unparseable stored list locked out loopback")
	}
}

func TestNormalizeAPIBindListTrimsAndDropsBlanks(t *testing.T) {
	got := NormalizeAPIBindList([]string{"  100.64.0.0/10", "", "203.0.113.5  "})
	want := []string{"100.64.0.0/10", "203.0.113.5"}
	if len(got) != len(want) {
		t.Fatalf("NormalizeAPIBindList = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("NormalizeAPIBindList = %v, want %v", got, want)
		}
	}
}

func TestNormalizeAPIBindListEmptyReturnsNil(t *testing.T) {
	for _, entries := range [][]string{nil, {}, {"", "  "}} {
		if got := NormalizeAPIBindList(entries); got != nil {
			t.Fatalf("NormalizeAPIBindList(%v) = %v, want nil", entries, got)
		}
	}
}
