// Package agent implements the geo agent: a relay that sits on a public IP in
// the country it serves, forwards only the hostnames and clients the control
// plane approves, and reports what it carried.
//
// It deliberately does not import internal/database or internal/web. Those are
// the hub's, and importing them would give the agent a way to become a second
// control plane. The only shared code is the handshake parser in
// internal/core/proxy, which is pure byte inspection.
package agent

import (
	"net"
	"strings"
)

// Allowlist is the set of hostnames this agent may relay. Two forms are
// accepted: an exact host, and a leading "*." which matches every subdomain at
// that suffix ("*.example.com" matches "a.example.com" and "a.b.example.com",
// but not "example.com" itself).
//
// Deliberately not a general glob or a regex. The list is built by the control
// plane from operator decisions, and a matcher with more ways to be wrong is a
// matcher that will be wrong in a way nobody tests.
type Allowlist struct {
	exact    map[string]struct{}
	suffixes []string
}

// NewAllowlist builds a matcher from configuration entries. Entries are
// lowercased and stripped of a trailing dot, because SNI is compared
// case-insensitively and a name written "Example.COM." in the panel has to
// match the same request as "example.com".
func NewAllowlist(entries []string) *Allowlist {
	a := &Allowlist{exact: make(map[string]struct{}, len(entries))}
	for _, raw := range entries {
		name := normaliseHost(raw)
		if name == "" {
			continue
		}
		if strings.HasPrefix(name, "*.") {
			suffix := strings.TrimPrefix(name, "*.")
			if suffix == "" {
				continue
			}
			a.suffixes = append(a.suffixes, "."+suffix)
			continue
		}
		a.exact[name] = struct{}{}
	}
	return a
}

// Allows reports whether host may be relayed.
func (a *Allowlist) Allows(host string) bool {
	if a == nil {
		return false
	}
	name := normaliseHost(host)
	if name == "" {
		return false
	}
	if _, ok := a.exact[name]; ok {
		return true
	}
	for _, suffix := range a.suffixes {
		if strings.HasSuffix(name, suffix) {
			return true
		}
	}
	return false
}

// Len is the number of entries after normalisation, for the status line. An
// entry that normalised away is not counted, so "0 hosts" honestly means the
// agent will refuse everything.
func (a *Allowlist) Len() int {
	if a == nil {
		return 0
	}
	return len(a.exact) + len(a.suffixes)
}

func normaliseHost(raw string) string {
	name := strings.ToLower(strings.TrimSpace(raw))
	name = strings.TrimSuffix(name, ".")
	// A bare "*" would match every host and turn the agent into an open relay,
	// so it is rejected rather than interpreted as "all".
	if name == "*" || name == "" {
		return ""
	}
	return name
}

// ClientList answers "is this source IP allowed to use me". Without it, anyone
// who learns the agent's address can point a browser at it and be relayed —
// DNS-level subscriber checks do not help, because they never touch DNS.
//
// Entries are a bare IP or a CIDR; a bare IP is widened to a single-host range
// so one code path covers both.
type ClientList struct {
	nets []*net.IPNet
	any  bool
}

// NewClientList builds a matcher. An empty list matches nothing, which is the
// safe default: an agent that has not synced yet refuses rather than serves.
func NewClientList(entries []string) *ClientList {
	c := &ClientList{}
	for _, raw := range entries {
		entry := strings.TrimSpace(raw)
		if entry == "" {
			continue
		}
		if entry == "any" {
			c.any = true
			continue
		}
		if _, network, err := net.ParseCIDR(entry); err == nil {
			c.nets = append(c.nets, network)
			continue
		}
		if ip := net.ParseIP(entry); ip != nil {
			bits := 32
			if v4 := ip.To4(); v4 != nil {
				// ParseIP returns a 16-byte form even for IPv4. Keeping it that
				// way against a 4-byte mask makes IPNet.Contains compare a
				// 16-byte address with a 32-bit network and never match, which
				// is how "127.0.0.1" silently refused localhost.
				ip = v4
			} else {
				bits = 128
			}
			c.nets = append(c.nets, &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)})
		}
	}
	return c
}

// Allows reports whether ip may be relayed for.
func (c *ClientList) Allows(ip net.IP) bool {
	if c == nil {
		return false
	}
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}
	if c.any {
		return true
	}
	for _, network := range c.nets {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

// Len is the number of configured entries, for the status line.
func (c *ClientList) Len() int {
	if c == nil {
		return 0
	}
	// "any" is one configured rule, not an empty list. Counting only explicit
	// networks made an allow-all agent print "0 client entries" and warn that
	// every connection would be refused while it was actually relaying for all.
	if c.any {
		return len(c.nets) + 1
	}
	return len(c.nets)
}
