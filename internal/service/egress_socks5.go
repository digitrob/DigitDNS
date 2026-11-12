package service

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
)

// dialSOCKS5 opens a SOCKS5 CONNECT tunnel. The destination is sent as a
// hostname by default so the proxy resolves it from its own country; local
// resolution mode sends the vetted IP instead.
func dialSOCKS5(ctx context.Context, cfg egressConfig, host string, addrs []net.IP, port int) (net.Conn, error) {
	address := strings.TrimSpace(cfg.Address)
	if address == "" {
		return nil, fmt.Errorf("SOCKS5 address is empty")
	}
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, fmt.Errorf("connect to SOCKS5 %s: %w", address, err)
	}
	ok := false
	defer func() {
		if !ok {
			_ = conn.Close()
		}
	}()

	if deadline, has := ctx.Deadline(); has {
		_ = conn.SetDeadline(deadline)
	} else {
		_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	}

	if err := socks5Handshake(conn, cfg.Username, cfg.Password); err != nil {
		return nil, err
	}
	targetHost := host
	if cfg.Resolve == "local" {
		if len(addrs) == 0 {
			return nil, fmt.Errorf("no vetted address for %s", host)
		}
		targetHost = addrs[0].String()
	}
	if err := socks5Connect(conn, targetHost, port); err != nil {
		return nil, err
	}
	_ = conn.SetDeadline(time.Time{})
	ok = true
	return conn, nil
}

func socks5Handshake(conn net.Conn, username, password string) error {
	methods := []byte{0x00}
	if username != "" || password != "" {
		methods = append(methods, 0x02)
	}
	if _, err := conn.Write(append([]byte{0x05, byte(len(methods))}, methods...)); err != nil {
		return fmt.Errorf("SOCKS5 greeting: %w", err)
	}
	reply := make([]byte, 2)
	if _, err := io.ReadFull(conn, reply); err != nil {
		return fmt.Errorf("SOCKS5 greeting reply: %w", err)
	}
	if reply[0] != 0x05 {
		return fmt.Errorf("SOCKS5 proxy returned version %d", reply[0])
	}
	switch reply[1] {
	case 0x00:
		return nil
	case 0x02:
		if username == "" && password == "" {
			return fmt.Errorf("SOCKS5 proxy requires username/password")
		}
		user := []byte(username)
		pass := []byte(password)
		if len(user) > 255 || len(pass) > 255 {
			return fmt.Errorf("SOCKS5 username or password is too long")
		}
		req := append([]byte{0x01, byte(len(user))}, user...)
		req = append(req, byte(len(pass)))
		req = append(req, pass...)
		if _, err := conn.Write(req); err != nil {
			return fmt.Errorf("SOCKS5 auth: %w", err)
		}
		authReply := make([]byte, 2)
		if _, err := io.ReadFull(conn, authReply); err != nil {
			return fmt.Errorf("SOCKS5 auth reply: %w", err)
		}
		if authReply[1] != 0x00 {
			return fmt.Errorf("SOCKS5 authentication failed")
		}
		return nil
	default:
		return fmt.Errorf("SOCKS5 proxy offered no supported auth method")
	}
}

func socks5Connect(conn net.Conn, host string, port int) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("invalid SOCKS5 target port %d", port)
	}
	req := []byte{0x05, 0x01, 0x00}
	if ip := net.ParseIP(host); ip != nil {
		if v4 := ip.To4(); v4 != nil {
			req = append(req, 0x01)
			req = append(req, v4...)
		} else {
			req = append(req, 0x04)
			req = append(req, ip.To16()...)
		}
	} else {
		name := []byte(strings.TrimSuffix(host, "."))
		if len(name) == 0 || len(name) > 255 {
			return fmt.Errorf("invalid SOCKS5 target hostname")
		}
		req = append(req, 0x03, byte(len(name)))
		req = append(req, name...)
	}
	var portBytes [2]byte
	binary.BigEndian.PutUint16(portBytes[:], uint16(port))
	req = append(req, portBytes[:]...)
	if _, err := conn.Write(req); err != nil {
		return fmt.Errorf("SOCKS5 connect request: %w", err)
	}

	head := make([]byte, 4)
	if _, err := io.ReadFull(conn, head); err != nil {
		return fmt.Errorf("SOCKS5 connect reply: %w", err)
	}
	if head[0] != 0x05 {
		return fmt.Errorf("SOCKS5 proxy returned version %d", head[0])
	}
	if head[1] != 0x00 {
		return fmt.Errorf("SOCKS5 connect refused (code %d)", head[1])
	}
	var skip int
	switch head[3] {
	case 0x01:
		skip = 4
	case 0x04:
		skip = 16
	case 0x03:
		length := make([]byte, 1)
		if _, err := io.ReadFull(conn, length); err != nil {
			return fmt.Errorf("SOCKS5 bound address: %w", err)
		}
		skip = int(length[0])
	default:
		return fmt.Errorf("SOCKS5 proxy returned unknown address type %d", head[3])
	}
	if skip > 0 {
		if _, err := io.CopyN(io.Discard, conn, int64(skip)); err != nil {
			return fmt.Errorf("SOCKS5 bound address: %w", err)
		}
	}
	if _, err := io.CopyN(io.Discard, conn, 2); err != nil {
		return fmt.Errorf("SOCKS5 bound port: %w", err)
	}
	return nil
}

// probeSOCKS5 checks that the proxy accepts a CONNECT for a public host. It is
// used by the Test action and the health loop.
func probeSOCKS5(ctx context.Context, cfg egressConfig) (time.Duration, error) {
	start := time.Now()
	host := strings.TrimSpace(cfg.TestHost)
	if host == "" {
		host = "example.com"
	}
	port := 443
	if h, p, err := net.SplitHostPort(host); err == nil {
		host = h
		if parsed, err := strconv.Atoi(p); err == nil {
			port = parsed
		}
	}
	// The probe asks the proxy to resolve a public name even when the egress is
	// configured for local resolution: there is no vetted target in this path,
	// and connectivity is what the probe is measuring.
	cfg.Resolve = "remote"
	conn, err := dialSOCKS5(ctx, cfg, host, nil, port)
	if err != nil {
		return 0, err
	}
	_ = conn.Close()
	return time.Since(start), nil
}
