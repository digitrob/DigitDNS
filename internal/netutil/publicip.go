package netutil

import (
	"net"
	"strings"
)

// ParsePublicIPv6 accepts a global-unicast IPv6 literal suitable for an AAAA
// answer. It deliberately rejects IPv4, loopback, link-local, ULA, multicast,
// unspecified and documentation-only special cases that cannot be reached from
// the public internet. The returned address is the 16-byte canonical form.
//
// The proxy leak guard depends on this being a real address: advertising a ULA
// or link-local value would hand IPv6 clients an address that answers nothing,
// and advertising a special-use range would make "IPv6 is on" look healthy while
// every client failed to connect.
func ParsePublicIPv6(raw string) (net.IP, bool) {
	ip := net.ParseIP(strings.TrimSpace(raw))
	if ip == nil || ip.To4() != nil {
		return nil, false
	}
	ip16 := ip.To16()
	if ip16 == nil {
		return nil, false
	}
	if ip.IsUnspecified() || ip.IsLoopback() || ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() || !ip.IsGlobalUnicast() {
		return nil, false
	}
	return ip16, true
}
