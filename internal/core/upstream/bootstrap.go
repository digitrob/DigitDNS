package upstream

import (
	"context"
	"crypto/tls"
	"net"
	"sync/atomic"
	"time"

	"github.com/quic-go/quic-go"
)

// newBootstrapResolver builds a Go resolver whose only DNS transport is the
// operator's bootstrap list. It is used to resolve encrypted upstream hostnames
// (DoH/DoT/DoQ/DoH3). Without it those lookups go through the system resolver,
// which on a box that points at itself is a loop.
//
// The resolver is tried server by server starting at a rotating index, so a bad
// first entry does not pin every lookup to it. PreferGo keeps the query on the
// configured servers instead of the C library's resolver.
func newBootstrapResolver(servers []string) *net.Resolver {
	var next atomic.Uint32
	return &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			if len(servers) == 0 {
				return nil, &net.DNSError{Err: "no bootstrap DNS servers configured", IsTimeout: false}
			}
			start := int(next.Add(1)) % len(servers)
			var lastErr error
			for i := range servers {
				server := servers[(start+i)%len(servers)]
				dialer := net.Dialer{Timeout: 3 * time.Second}
				conn, err := dialer.DialContext(ctx, network, server)
				if err == nil {
					return conn, nil
				}
				lastErr = err
			}
			return nil, lastErr
		},
	}
}

// resolveEndpoint turns host:port into ip:port using the bootstrap resolver.
// An IP literal, or an endpoint without a port, is returned unchanged.
func resolveEndpoint(ctx context.Context, resolver *net.Resolver, endpoint string) (string, error) {
	if resolver == nil {
		return endpoint, nil
	}
	host, port, err := net.SplitHostPort(endpoint)
	if err != nil || net.ParseIP(host) != nil {
		return endpoint, nil
	}
	ips, err := resolver.LookupHost(ctx, host)
	if err != nil {
		return "", err
	}
	if len(ips) == 0 {
		return "", &net.DNSError{Err: "bootstrap lookup returned no addresses", Name: host}
	}
	return net.JoinHostPort(ips[0], port), nil
}

// dialQUICWithResolver is the http3.Transport dial hook: resolve the URL host
// through the bootstrap servers, then dial the resulting IP. The TLS config
// already carries the hostname as ServerName, so certificate verification still
// checks the name in the URL.
func dialQUICWithResolver(ctx context.Context, resolver *net.Resolver, addr string, tlsCfg *tls.Config, cfg *quic.Config) (*quic.Conn, error) {
	dialAddr, err := resolveEndpoint(ctx, resolver, addr)
	if err != nil {
		return nil, err
	}
	return quic.DialAddr(ctx, dialAddr, tlsCfg, cfg)
}
