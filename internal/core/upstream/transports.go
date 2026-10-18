package upstream

// DNS-over-QUIC and DNSCrypt live at the socket boundary like DoT and DoH, so
// they are implemented here against the real libraries rather than mocked. Both
// validate what they can: DoQ verifies the upstream's certificate exactly as DoT
// does, and DNSCrypt verifies the resolver certificate's signature against the
// provider public key carried in the stamp. Neither has an insecure escape hatch.

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/ameshkov/dnscrypt/v2"
	"github.com/ameshkov/dnsstamps"
	"github.com/miekg/dns"
	"github.com/quic-go/quic-go"
)

// doqALPN is the ALPN token RFC 9250 assigns to DNS-over-QUIC.
const doqALPN = "doq"

// exchangeDoQ sends one DNS query over DNS-over-QUIC (RFC 9250) and returns the
// decoded reply. Each query opens its own QUIC connection and bidi stream, which
// matches how the racer uses every other transport (one exchange, one deadline)
// and keeps a stalled upstream from pinning a connection for the whole pool.
func (p *UpstreamPool) exchangeDoQ(ctx context.Context, q *dns.Msg, endpoint string) (*dns.Msg, time.Duration, error) {
	start := time.Now()

	packed, err := q.Pack()
	if err != nil {
		return nil, 0, fmt.Errorf("pack DoQ query for %s: %w", endpoint, err)
	}
	if len(packed) > dns.MaxMsgSize {
		return nil, 0, fmt.Errorf("DoQ query for %s is too large to frame", endpoint)
	}

	// Copy the pool's TLS settings so the per-dial ServerName cannot leak back
	// into the shared config. Verification is never skipped: an IP literal is
	// checked against the certificate's IP SAN and a hostname against its DNS
	// names, exactly like DoT.
	tlsConf := p.quicTLS.Clone()
	if tlsConf.ServerName == "" {
		if host, _, err := net.SplitHostPort(endpoint); err == nil {
			tlsConf.ServerName = host
		}
	}

	conn, err := quic.DialAddr(ctx, endpoint, tlsConf, &quic.Config{
		HandshakeIdleTimeout: p.timeout,
		MaxIdleTimeout:       p.timeout,
	})
	if err != nil {
		return nil, time.Since(start), err
	}
	defer func() { _ = conn.CloseWithError(0, "") }()

	stream, err := conn.OpenStreamSync(ctx)
	if err != nil {
		return nil, time.Since(start), err
	}
	defer func() {
		stream.CancelRead(0)
		_ = stream.Close()
	}()

	if deadline, ok := ctx.Deadline(); ok {
		_ = stream.SetDeadline(deadline)
	}

	// RFC 9250 §4.2: the DNS message is prefixed with a 2-byte big-endian length,
	// the same TCP framing RFC 1035 §4.2.2 defines.
	frame := make([]byte, 2+len(packed))
	binary.BigEndian.PutUint16(frame, uint16(len(packed)))
	copy(frame[2:], packed)

	if _, err := stream.Write(frame); err != nil {
		return nil, time.Since(start), fmt.Errorf("write DoQ query to %s: %w", endpoint, err)
	}

	var lenBuf [2]byte
	if _, err := io.ReadFull(stream, lenBuf[:]); err != nil {
		return nil, time.Since(start), fmt.Errorf("read DoQ length from %s: %w", endpoint, err)
	}
	n := int(binary.BigEndian.Uint16(lenBuf[:]))
	// A DNS message is at least a header (12 bytes) and at most 64 KiB.
	if n < 12 || n > dns.MaxMsgSize {
		return nil, time.Since(start), fmt.Errorf("DoQ server %s announced an invalid message length %d", endpoint, n)
	}
	body := make([]byte, n)
	if _, err := io.ReadFull(stream, body); err != nil {
		return nil, time.Since(start), fmt.Errorf("read DoQ response from %s: %w", endpoint, err)
	}

	out := new(dns.Msg)
	if err := out.Unpack(body); err != nil {
		return nil, time.Since(start), fmt.Errorf("unpack DoQ response from %s: %w", endpoint, err)
	}
	return out, time.Since(start), nil
}

// dnscryptTransport speaks DNSCrypt. A DNSCrypt exchange is only possible once the
// resolver's certificate has been fetched and validated, so the transport keeps
// that ResolverInfo per stamp instead of re-fetching it for every query: the
// certificate is fetched once and refreshed whenever an exchange fails, which is
// what covers a server that rotates its short-term key.
type dnscryptTransport struct {
	mu      sync.Mutex
	cache   map[string]*dnscrypt.ResolverInfo
	timeout time.Duration
}

// newDNSCryptTransport builds the DNSCrypt transport with an empty certificate
// cache; certificates are fetched lazily on the first query to each stamp.
func newDNSCryptTransport(timeout time.Duration) *dnscryptTransport {
	return &dnscryptTransport{
		cache:   make(map[string]*dnscrypt.ResolverInfo),
		timeout: timeout,
	}
}

func (t *dnscryptTransport) client() *dnscrypt.Client {
	return &dnscrypt.Client{
		Net:     "udp",
		Timeout: t.timeout,
		UDPSize: 4096,
	}
}

// exchange runs one DNSCrypt query for the given sdns:// stamp. The stamp is the
// cache key as well as the identity, because it encodes the provider public key
// the certificate signature is checked against.
func (t *dnscryptTransport) exchange(stamp string, q *dns.Msg) (*dns.Msg, time.Duration, error) {
	start := time.Now()

	t.mu.Lock()
	ri := t.cache[stamp]
	t.mu.Unlock()

	if ri == nil {
		fetched, err := t.client().Dial(stamp)
		if err != nil {
			return nil, time.Since(start), err
		}
		ri = fetched
		t.mu.Lock()
		t.cache[stamp] = ri
		t.mu.Unlock()
	}

	resp, err := t.client().Exchange(q, ri)
	if err != nil {
		// A stale or rotated certificate is the usual cause, so drop the cached
		// info and let the next query Dial again rather than stay broken.
		t.mu.Lock()
		delete(t.cache, stamp)
		t.mu.Unlock()
	}
	return resp, time.Since(start), err
}

// normalizeStamp decodes a DNS stamp (https://dnscrypt.info/stamps) and maps it
// onto the pool's canonical upstream form. A DNSCrypt stamp is kept verbatim and
// spoken by the DNSCrypt client; a DoH, DoT or DoQ stamp is translated to the
// matching native scheme, so a pasted stamp and the same server typed by hand
// deduplicate. Plain, ODoH and relay stamps address transports this pool does not
// speak and are rejected rather than stored as a resolver that always fails.
func normalizeStamp(raw string) (string, bool) {
	stamp, err := dnsstamps.NewServerStampFromString(raw)
	if err != nil {
		return "", false
	}
	switch stamp.Proto {
	case dnsstamps.StampProtoTypeDNSCrypt:
		// Re-encode through the library so two spellings of the same stamp (for
		// example one with padding) collapse to a single entry.
		return stamp.String(), true
	case dnsstamps.StampProtoTypeDoH:
		return normalizeHTTPResolverURL(schemeHTTPS, stamp.ProviderName+stamp.Path)
	case dnsstamps.StampProtoTypeTLS:
		return normalizeHostPortScheme(schemeTLS, stamp.ProviderName, "853")
	case dnsstamps.StampProtoTypeDoQ:
		return normalizeHostPortScheme(schemeQUIC, stamp.ProviderName, "853")
	default:
		return "", false
	}
}
