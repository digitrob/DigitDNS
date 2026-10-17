package netutil

import (
	"fmt"
	"net"
	"strings"
)

// APIBind is a parsed api_bind rule. The stored value is a single address or
// block the REST API listens for; it is matched against the immediate peer of
// a request, never a forwarding header (see SecurityMiddleware).
//
// Three shapes are meaningful:
//
//   - loopback only — the default, and the fail-closed answer for an empty or
//     missing record. The value is stored as "127.0.0.1" but the match is
//     IsLoopback(), so a dual-stack listener's ::1 peer is accepted too and a
//     hand-written "127.0.0.1" is not silently narrower than the default.
//   - allow all — "0.0.0.0" (or "::" / the /0 blocks). Every peer is accepted.
//   - a single IP or a CIDR such as "100.64.0.0/10" — the peer must fall inside
//     it. A bare IP is widened to a /32 (IPv4) or /128 (IPv6).
//
// Loopback is ALWAYS accepted, whatever the rule. Binding the REST API to a
// tailnet block should not also lock out the machine's own scripts and the
// root-local control plane, and a loopback peer cannot be spoofed from off-box.
type APIBind struct {
	allowAll bool
	loopback bool
	ipnet    *net.IPNet
}

// ParseAPIBind validates and normalises an api_bind value. It is the single
// definition of what the settings field may hold: the dashboard validator, the
// REST middleware and the v2 settings route all read the rule through it, so a
// value one of them accepts cannot be one another rejects.
func ParseAPIBind(bind string) (APIBind, error) {
	v := strings.TrimSpace(bind)
	switch v {
	case "", "127.0.0.1":
		return APIBind{loopback: true}, nil
	case "0.0.0.0", "::", "0.0.0.0/0", "::/0":
		return APIBind{allowAll: true}, nil
	}
	if _, ipnet, err := net.ParseCIDR(v); err == nil {
		return APIBind{ipnet: ipnet}, nil
	}
	if ip := net.ParseIP(v); ip != nil {
		bits := 128
		if ip.To4() != nil {
			bits = 32
		}
		_, ipnet, err := net.ParseCIDR(fmt.Sprintf("%s/%d", ip.String(), bits))
		if err == nil {
			return APIBind{ipnet: ipnet}, nil
		}
	}
	return APIBind{}, fmt.Errorf("api_bind %q is not an IP address or CIDR block", bind)
}

// Allows reports whether peer may reach the REST API under this rule. A peer
// that is not a parseable IP is refused; loopback is always allowed.
func (b APIBind) Allows(peer string) bool {
	ip := net.ParseIP(strings.TrimSpace(peer))
	if ip == nil {
		return false
	}
	if ip.IsLoopback() {
		return true
	}
	if b.allowAll {
		return true
	}
	return b.ipnet != nil && b.ipnet.Contains(ip)
}

// APIBindAllowsPeer is the request-path convenience: parse the stored rule and
// match the peer. An unparseable stored value fails closed to loopback-only,
// which is the same posture an absent record takes, rather than opening the API
// because a hand-edited config carried a typo.
func APIBindAllowsPeer(bind, peer string) bool {
	rule, err := ParseAPIBind(bind)
	if err != nil {
		ip := net.ParseIP(strings.TrimSpace(peer))
		return ip != nil && ip.IsLoopback()
	}
	return rule.Allows(peer)
}
