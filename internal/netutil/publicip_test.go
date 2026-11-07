package netutil

import "testing"

func TestParsePublicIPv6(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"2001:db8::1", "2001:db8::1", true},
		{" 2001:4860:4860::8888 ", "2001:4860:4860::8888", true},
		{"2001:0db8:0:0:0:0:0:1", "2001:db8::1", true},
		{"", "", false},
		{"192.0.2.1", "", false},
		{"::1", "", false},
		{"fe80::1", "", false},
		{"fc00::1", "", false},
		{"ff02::1", "", false},
		{"::", "", false},
		{"2001:db8::/48", "", false},
		{"not-an-address", "", false},
	}
	for _, tc := range cases {
		got, ok := ParsePublicIPv6(tc.in)
		if ok != tc.ok {
			t.Errorf("ParsePublicIPv6(%q) ok = %v, want %v", tc.in, ok, tc.ok)
			continue
		}
		if !tc.ok {
			continue
		}
		if got.String() != tc.want {
			t.Errorf("ParsePublicIPv6(%q) = %q, want %q", tc.in, got.String(), tc.want)
		}
	}
}
