package netutil

import "testing"

func TestNormalizeDiagnosticHostShapes(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"bare ipv4 widens to 443", "203.0.113.5", "203.0.113.5:443"},
		{"bare hostname widens to 443", "game.example.com", "game.example.com:443"},
		{"explicit port kept", "game.example.com:27015", "game.example.com:27015"},
		{"ip with port kept", "203.0.113.5:8443", "203.0.113.5:8443"},
		{"padded host trimmed", "  game.example.com:443  ", "game.example.com:443"},
		{"bare ipv6 widens", "2001:db8::1", "[2001:db8::1]:443"},
		{"bracketed ipv6 with port", "[2001:db8::1]:8443", "[2001:db8::1]:8443"},
		{"bracketed ipv6 no port", "[2001:db8::1]", "[2001:db8::1]:443"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizeDiagnosticHost(tc.in)
			if err != nil {
				t.Fatalf("NormalizeDiagnosticHost(%q) = %v", tc.in, err)
			}
			if got != tc.want {
				t.Fatalf("NormalizeDiagnosticHost(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestNormalizeDiagnosticHostRejects(t *testing.T) {
	for _, bad := range []string{
		"",
		"   ",
		"https://game.example.com",
		"game.example.com/path",
		"game example.com",
		"game.example.com:0",
		"game.example.com:70000",
		"game.example.com:notaport",
		".",
		".game.example.com",
		"game..example.com",
		"game_example.com",
	} {
		if got, err := NormalizeDiagnosticHost(bad); err == nil {
			t.Errorf("NormalizeDiagnosticHost(%q) = %q, want rejection", bad, got)
		}
	}
}
