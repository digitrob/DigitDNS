// Package netresolve answers relay-target lookups without trusting a single
// nameserver.
//
// The relays used net.DefaultResolver for every proxied hostname. Go's resolver
// walks resolv.conf server by server and waits up to five seconds on the server
// it is currently talking to, while the proxy gives the whole lookup two
// seconds. One lost UDP packet — or one nameserver that accepts a query and
// never answers — therefore turned into a refused relay, even though the next
// server in the list would have replied in milliseconds. On a host whose
// resolv.conf still carries a dead stub resolver (a Tailscale node with DNS
// disabled, for example) that is a steady trickle of broken connections.
//
// This package sends the query to every configured server at once and returns
// the first usable answer. A silent or broken server costs nothing, and the
// lookup itself stays inside the caller's budget.
package netresolve

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"os"
	"strings"
	"time"
)

// resolvConfPath is a variable so the parser can be tested against a fixture.
var resolvConfPath = "/etc/resolv.conf"

const defaultPort = "53"

// Resolver races every configured nameserver for each lookup.
type Resolver struct {
	// servers is the normalised host:port list, kept for logging and diagnostics.
	servers []string
	// resolvers has one Go resolver per server. The index is preserved so a
	// future per-server setting can be added without changing the race.
	resolvers []*net.Resolver
}

// NewSystem builds a resolver from the host's resolv.conf. If the file cannot
// be read, or names no servers, the zero value falls back to net.DefaultResolver
// so a platform without a Unix resolv.conf keeps working.
func NewSystem() *Resolver {
	servers, err := SystemServers()
	if err != nil || len(servers) == 0 {
		return &Resolver{}
	}
	return New(servers)
}

// NewOrDefault is the helper for an optional operator setting: a non-empty list
// pins the race to those servers, while empty means "race the host's own
// nameservers". New(empty) deliberately stays a pure fallback to
// net.DefaultResolver, because an explicit empty list is not the same request.
func NewOrDefault(servers []string) *Resolver {
	if len(normalizeServers(servers)) == 0 {
		return NewSystem()
	}
	return New(servers)
}

// New builds a resolver that queries servers concurrently. Entries may be bare
// addresses ("9.9.9.9") or host:port pairs ("9.9.9.9:53"); bare entries get the
// standard DNS port. An empty list falls back to the system resolver.
func New(servers []string) *Resolver {
	normalized := normalizeServers(servers)
	r := &Resolver{servers: normalized}
	for _, server := range normalized {
		r.resolvers = append(r.resolvers, resolverFor(server))
	}
	return r
}

// Servers returns the normalised server list, for the start-up log line.
func (r *Resolver) Servers() []string {
	if r == nil {
		return nil
	}
	return append([]string(nil), r.servers...)
}

// LookupNetIP resolves host, returning the first answer any server produces.
// The returned slice is never empty on success.
func (r *Resolver) LookupNetIP(ctx context.Context, host string) ([]netip.Addr, error) {
	if r == nil || len(r.resolvers) == 0 {
		return net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	}
	// The relay's guard rejects literals before calling this, but resolving a
	// literal locally saves a pointless round trip for the agent and tools.
	if addr, err := netip.ParseAddr(host); err == nil {
		return []netip.Addr{addr}, nil
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	type result struct {
		addrs []netip.Addr
		err   error
	}
	// Buffered for every resolver, so a loser can always report and exit even
	// after the winner has cancelled the context.
	ch := make(chan result, len(r.resolvers))
	for _, res := range r.resolvers {
		go func(res *net.Resolver) {
			addrs, err := res.LookupNetIP(ctx, "ip", host)
			ch <- result{addrs: addrs, err: err}
		}(res)
	}

	var firstErr error
	for range r.resolvers {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case res := <-ch:
			if res.err == nil && len(res.addrs) > 0 {
				cancel()
				return res.addrs, nil
			}
			if firstErr == nil {
				firstErr = res.err
			}
		}
	}
	if firstErr == nil {
		firstErr = fmt.Errorf("netresolve: no addresses for %s", host)
	}
	return nil, firstErr
}

// LookupHost is LookupNetIP with the string form net.Resolver exposes. It keeps
// call sites that already speak LookupHost — the panel's diagnostic tool and the
// encrypted-upstream bootstrap — on the same racing path.
func (r *Resolver) LookupHost(ctx context.Context, host string) ([]string, error) {
	addrs, err := r.LookupNetIP(ctx, host)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(addrs))
	for _, addr := range addrs {
		out = append(out, addr.Unmap().String())
	}
	return out, nil
}

// SystemServers reads the nameserver lines from resolv.conf.
func SystemServers() ([]string, error) {
	data, err := os.ReadFile(resolvConfPath)
	if err != nil {
		return nil, err
	}
	return parseServers(string(data)), nil
}

// parseServers extracts nameserver entries, dropping duplicates. Comments and
// the other directives (search, options, domain) are ignored; the relay only
// needs the server addresses.
func parseServers(text string) []string {
	seen := make(map[string]struct{})
	var out []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[0] != "nameserver" {
			continue
		}
		server := fields[1]
		if server == "" {
			continue
		}
		if _, ok := seen[server]; ok {
			continue
		}
		seen[server] = struct{}{}
		out = append(out, server)
	}
	return out
}

// normalizeServers trims entries, adds the default port where it is missing and
// removes duplicates. Both "9.9.9.9" and "9.9.9.9:53" normalise to the latter.
func normalizeServers(servers []string) []string {
	seen := make(map[string]struct{}, len(servers))
	out := make([]string, 0, len(servers))
	for _, raw := range servers {
		server := strings.TrimSpace(raw)
		if server == "" {
			continue
		}
		if _, _, err := net.SplitHostPort(server); err != nil {
			server = net.JoinHostPort(server, defaultPort)
		}
		if _, ok := seen[server]; ok {
			continue
		}
		seen[server] = struct{}{}
		out = append(out, server)
	}
	return out
}

// resolverFor pins one Go resolver to one server. PreferGo keeps the query on
// the pinned server instead of handing it back to the C library, whose resolver
// would ignore the Dial hook and walk resolv.conf again.
func resolverFor(server string) *net.Resolver {
	return &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			dialer := net.Dialer{Timeout: 2 * time.Second}
			return dialer.DialContext(ctx, network, server)
		},
	}
}
