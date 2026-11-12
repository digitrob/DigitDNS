package acme

import "testing"

func TestDNS01NameStripsWildcard(t *testing.T) {
	cases := []struct{ in, want string }{
		{"example.com", "_acme-challenge.example.com"},
		{"*.example.com", "_acme-challenge.example.com"},
		{"DNS.Example.com", "_acme-challenge.dns.example.com"},
	}
	for _, c := range cases {
		if got := dns01Name(c.in); got != c.want {
			t.Errorf("dns01Name(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
