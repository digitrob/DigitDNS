package httpx

import (
	"net"
	"net/http"
	"time"
)

// OutboundTransport is the shared connection pool for the panel's server-side
// fetches: blocklist sources, AdGuard imports, service-icon search and RDAP
// lookups. Each of those used to build its own client (or use
// http.DefaultClient), so refreshing a second list paid a fresh TLS handshake
// and a default two-connection-per-host pool. Per-request deadlines stay on the
// request context; this only bounds dial/TLS and keeps idle connections warm.
var OutboundTransport = &http.Transport{
	Proxy:                 http.ProxyFromEnvironment,
	DialContext:           (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
	ForceAttemptHTTP2:     true,
	MaxIdleConns:          64,
	MaxIdleConnsPerHost:   8,
	IdleConnTimeout:       90 * time.Second,
	TLSHandshakeTimeout:   5 * time.Second,
	ExpectContinueTimeout: 1 * time.Second,
}

// Outbound is the shared client. The 30 s ceiling only stops a request that
// forgot its own deadline; every caller sets a shorter context timeout.
var Outbound = &http.Client{Transport: OutboundTransport, Timeout: 30 * time.Second}
