package agent

import (
	"net"
	"testing"
)

func TestAllowlistAllowsExactAndWildcard(t *testing.T) {
	a := NewAllowlist([]string{
		"api.ard.de",
		"*.ard-mcdn.net",
		"  Example.COM.  ",
	})

	allowed := []string{
		"api.ard.de",
		"API.ARD.DE",       // case-insensitive
		"a.ard-mcdn.net",   // one label under the suffix
		"a.b.ard-mcdn.net", // deeper under the suffix
		"example.com",      // trailing dot and case normalised away
	}
	for _, host := range allowed {
		if !a.Allows(host) {
			t.Errorf("expected %q to be allowed", host)
		}
	}

	refused := []string{
		"",
		"ard.de",             // the exact parent is not covered by "*.ard-mcdn.net"
		"ard-mcdn.net",       // nor is the apex
		"evilard-mcdn.net",   // suffix must be a label boundary
		"api.ard.de.evil.io", // suffix match must not be substring
		"*",
	}
	for _, host := range refused {
		if a.Allows(host) {
			t.Errorf("expected %q to be refused", host)
		}
	}
}

func TestAllowlistRejectsBareWildcard(t *testing.T) {
	// A bare "*" would match everything and turn the agent into an open relay,
	// so it must not survive normalisation.
	a := NewAllowlist([]string{"*"})
	if got := a.Len(); got != 0 {
		t.Fatalf("bare wildcard should not produce entries, got %d", got)
	}
	if a.Allows("anything.example") {
		t.Fatal("bare wildcard must not allow any host")
	}
}

func TestNilAllowlistRefuses(t *testing.T) {
	var a *Allowlist
	if a.Allows("anything.example") {
		t.Fatal("nil allowlist must refuse")
	}
}

func TestClientListCIDRAndBareIP(t *testing.T) {
	c := NewClientList([]string{"203.0.113.5", "198.51.100.0/24", "2001:db8::/32"})

	for _, raw := range []string{"203.0.113.5", "198.51.100.9", "2001:db8::1"} {
		if !c.Allows(net.ParseIP(raw)) {
			t.Errorf("expected %s to be allowed", raw)
		}
	}
	for _, raw := range []string{"203.0.113.6", "198.51.101.1", "10.0.0.1"} {
		if c.Allows(net.ParseIP(raw)) {
			t.Errorf("expected %s to be refused", raw)
		}
	}
}

func TestEmptyClientListRefuses(t *testing.T) {
	// An agent that has not synced yet must refuse rather than serve.
	c := NewClientList(nil)
	if c.Allows(net.ParseIP("203.0.113.5")) {
		t.Fatal("empty client list must refuse everyone")
	}
}
