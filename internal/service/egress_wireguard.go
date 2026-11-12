package service

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"

	"digitdns/internal/database"
	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun/netstack"
)

// Userspace WireGuard. The hub does not create a kernel interface: wireguard-go
// runs the tunnel in process and a gVisor netstack provides the TCP dialer. That
// keeps the daemon unprivileged and portable, and it means an egress can be
// added or removed without touching the host network configuration.

const (
	defaultWireGuardMTU = 1420
	wireGuardHandshake  = 15 * time.Second
)

// wireGuardTunnel is one live device created from a stored .conf.
type wireGuardTunnel struct {
	dev  *device.Device
	net  *netstack.Net
	mtu  int
	done sync.Once
}

// wireGuardConf is the subset of a wg-quick configuration the hub needs.
type wireGuardConf struct {
	privateKey string
	addresses  []netip.Addr
	dns        []netip.Addr
	mtu        int
	peers      []wireGuardPeer
}

type wireGuardPeer struct {
	publicKey    string
	presharedKey string
	endpoint     string
	allowedIPs   []string
	keepalive    int
}

// parseWireGuardConf reads the common wg-quick keys. It is intentionally
// tolerant of comments and unknown keys, but it rejects a config with no
// private key, no address or no peer: such a config cannot carry traffic.
func parseWireGuardConf(raw string) (wireGuardConf, error) {
	conf := wireGuardConf{mtu: defaultWireGuardMTU}
	section := ""
	var peer wireGuardPeer
	flushPeer := func() {
		if peer.publicKey != "" {
			conf.peers = append(conf.peers, peer)
		}
		peer = wireGuardPeer{}
	}
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			flushPeer()
			section = strings.ToLower(strings.Trim(line, "[]"))
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		switch section {
		case "interface":
			switch key {
			case "privatekey":
				conf.privateKey = value
			case "address":
				for _, part := range strings.Split(value, ",") {
					if addr, ok := parseConfAddr(part); ok {
						conf.addresses = append(conf.addresses, addr)
					}
				}
			case "dns":
				for _, part := range strings.Split(value, ",") {
					if addr, ok := parseConfAddr(part); ok {
						conf.dns = append(conf.dns, addr)
					}
				}
			case "mtu":
				if n, err := strconv.Atoi(value); err == nil && n >= 576 && n <= 9000 {
					conf.mtu = n
				}
			}
		case "peer":
			switch key {
			case "publickey":
				peer.publicKey = value
			case "presharedkey":
				peer.presharedKey = value
			case "endpoint":
				peer.endpoint = value
			case "allowedips":
				for _, part := range strings.Split(value, ",") {
					if part = strings.TrimSpace(part); part != "" {
						peer.allowedIPs = append(peer.allowedIPs, part)
					}
				}
			case "persistentkeepalive":
				if n, err := strconv.Atoi(value); err == nil {
					peer.keepalive = n
				}
			}
		}
	}
	flushPeer()
	if conf.privateKey == "" {
		return conf, fmt.Errorf("WireGuard config has no PrivateKey")
	}
	if len(conf.addresses) == 0 {
		return conf, fmt.Errorf("WireGuard config has no Address")
	}
	if len(conf.peers) == 0 {
		return conf, fmt.Errorf("WireGuard config has no Peer with a PublicKey")
	}
	return conf, nil
}

func parseConfAddr(raw string) (netip.Addr, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return netip.Addr{}, false
	}
	if strings.Contains(raw, "/") {
		prefix, err := netip.ParsePrefix(raw)
		if err != nil {
			return netip.Addr{}, false
		}
		return prefix.Addr(), true
	}
	addr, err := netip.ParseAddr(raw)
	return addr, err == nil
}

// wireGuardHexKey converts the base64 key form used by wg-quick to the hex form
// the device UAPI expects.
func wireGuardHexKey(value string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(value))
	if err != nil {
		return "", fmt.Errorf("invalid WireGuard key: %w", err)
	}
	if len(raw) != 32 {
		return "", fmt.Errorf("invalid WireGuard key length %d", len(raw))
	}
	return hex.EncodeToString(raw), nil
}

func newWireGuardTunnel(cfg egressConfig) (*wireGuardTunnel, error) {
	conf, err := parseWireGuardConf(cfg.Config)
	if err != nil {
		return nil, err
	}
	dns := conf.dns
	if len(dns) == 0 {
		dns = []netip.Addr{netip.MustParseAddr("1.1.1.1"), netip.MustParseAddr("8.8.8.8")}
	}
	tun, tnet, err := netstack.CreateNetTUN(conf.addresses, dns, conf.mtu)
	if err != nil {
		return nil, fmt.Errorf("WireGuard netstack: %w", err)
	}
	dev := device.NewDevice(tun, conn.NewDefaultBind(), device.NewLogger(device.LogLevelError, "wg "+cfg.Name+": "))

	privateKey, err := wireGuardHexKey(conf.privateKey)
	if err != nil {
		dev.Close()
		return nil, err
	}
	var uapi strings.Builder
	fmt.Fprintf(&uapi, "private_key=%s\n", privateKey)
	uapi.WriteString("listen_port=0\n")
	for _, peer := range conf.peers {
		publicKey, err := wireGuardHexKey(peer.publicKey)
		if err != nil {
			dev.Close()
			return nil, err
		}
		fmt.Fprintf(&uapi, "public_key=%s\n", publicKey)
		if peer.presharedKey != "" {
			preshared, err := wireGuardHexKey(peer.presharedKey)
			if err != nil {
				dev.Close()
				return nil, err
			}
			fmt.Fprintf(&uapi, "preshared_key=%s\n", preshared)
		}
		if peer.endpoint != "" {
			fmt.Fprintf(&uapi, "endpoint=%s\n", peer.endpoint)
		}
		for _, allowed := range peer.allowedIPs {
			fmt.Fprintf(&uapi, "allowed_ip=%s\n", allowed)
		}
		if peer.keepalive > 0 {
			fmt.Fprintf(&uapi, "persistent_keepalive_interval=%d\n", peer.keepalive)
		}
	}
	if err := dev.IpcSet(uapi.String()); err != nil {
		dev.Close()
		return nil, fmt.Errorf("WireGuard config: %w", err)
	}
	if err := dev.Up(); err != nil {
		dev.Close()
		return nil, fmt.Errorf("WireGuard device up: %w", err)
	}
	return &wireGuardTunnel{dev: dev, net: tnet, mtu: conf.mtu}, nil
}

func (t *wireGuardTunnel) close() {
	if t == nil {
		return
	}
	t.done.Do(func() { t.dev.Close() })
}

// dial opens a TCP connection through the tunnel. addrs are the vetted
// addresses the proxy already resolved; when empty (the health probe path) the
// name is resolved inside the tunnel's own DNS.
func (t *wireGuardTunnel) dial(ctx context.Context, host string, addrs []netip.Addr, port int) (net.Conn, error) {
	if port < 1 || port > 65535 {
		return nil, fmt.Errorf("invalid WireGuard target port %d", port)
	}
	candidates := addrs
	if len(candidates) == 0 {
		ips, err := t.net.LookupHost(host)
		if err != nil {
			return nil, fmt.Errorf("resolve %s inside WireGuard: %w", host, err)
		}
		for _, raw := range ips {
			if addr, err := netip.ParseAddr(raw); err == nil {
				candidates = append(candidates, addr)
			}
		}
	}
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no address for %s", host)
	}
	ctx, cancel := context.WithTimeout(ctx, wireGuardHandshake)
	defer cancel()
	var lastErr error
	for _, addr := range candidates {
		conn, err := t.net.DialContextTCPAddrPort(ctx, netip.AddrPortFrom(addr.Unmap(), uint16(port)))
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no address for %s", host)
	}
	return nil, lastErr
}

// probeWireGuard verifies the tunnel by opening a TCP connection to the test
// host. The connection is closed immediately; a TLS handshake is unnecessary to
// prove the tunnel and peer are usable.
func (rt *egressRuntime) probeWireGuard(ctx context.Context) (time.Duration, error) {
	start := time.Now()
	tunnel, err := rt.wireGuardTunnel()
	if err != nil {
		return 0, err
	}
	host := strings.TrimSpace(rt.cfg.TestHost)
	if host == "" {
		host = "example.com:443"
	}
	port := 443
	if h, p, err := net.SplitHostPort(host); err == nil {
		host = h
		if parsed, err := strconv.Atoi(p); err == nil {
			port = parsed
		}
	}
	conn, err := tunnel.dial(ctx, host, nil, port)
	if err != nil {
		return 0, err
	}
	_ = conn.Close()
	return time.Since(start), nil
}

func (rt *egressRuntime) dialWireGuard(ctx context.Context, host string, addrs []netip.Addr, port int) (net.Conn, error) {
	tunnel, err := rt.wireGuardTunnel()
	if err != nil {
		return nil, err
	}
	return tunnel.dial(ctx, host, addrs, port)
}

// wireGuardTunnel lazily creates the device. A failure is not cached: the next
// probe or dial retries, so fixing a bad config and pressing Test works without
// a restart.
func (rt *egressRuntime) wireGuardTunnel() (*wireGuardTunnel, error) {
	rt.wgMu.Lock()
	defer rt.wgMu.Unlock()
	if rt.wg != nil {
		return rt.wg, nil
	}
	tunnel, err := newWireGuardTunnel(rt.cfg)
	if err != nil {
		return nil, err
	}
	rt.wg = tunnel
	return tunnel, nil
}

func (rt *egressRuntime) closeWireGuard() {
	rt.wgMu.Lock()
	tunnel := rt.wg
	rt.wg = nil
	rt.wgMu.Unlock()
	if tunnel != nil {
		tunnel.close()
	}
}

var _ = database.EgressKindWireGuard
