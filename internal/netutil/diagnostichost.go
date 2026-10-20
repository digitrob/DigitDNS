package netutil

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

// NormalizeDiagnosticHost validates and normalises an operator-entered diagnostic
// target host and returns the address the probe should dial.
//
// Two shapes are accepted:
//
//   - a bare host — an IP literal or a hostname — which is widened to :443, the
//     port the suite's latency figures are about;
//   - an explicit host:port, when the operator wants to measure a different port.
//
// The host part may be an IP literal or a DNS name. A DNS name is checked only
// for being a plausible hostname (no spaces, no scheme, no path), not resolved:
// resolution is what the probe measures, so resolving it here would both double
// the work and turn a DNS failure into a "bad input" error instead of the
// BLOCKED row the operator needs to see. An IPv6 literal is accepted bare
// ("2001:db8::1") and with a bracketed port ("[2001:db8::1]:8443"), per the
// usual host:port syntax.
func NormalizeDiagnosticHost(raw string) (string, error) {
	v := strings.TrimSpace(raw)
	if v == "" {
		return "", fmt.Errorf("diagnostic target host is empty")
	}
	// A scheme or a path is the common paste mistake: reject it with a message
	// that names the fix rather than passing "https://x/" to net.DialTimeout,
	// which reports it as a DNS failure and hides the real problem.
	if strings.Contains(v, "://") || strings.ContainsAny(v, "/ \t") {
		return "", fmt.Errorf("diagnostic target %q must be a host or host:port (no scheme or path)", raw)
	}

	// Bracketed IPv6 literal, optionally with a port.
	if strings.HasPrefix(v, "[") {
		host, port, err := net.SplitHostPort(v)
		if err != nil {
			// A bare "[2001:db8::1]" has no port; SplitHostPort rejects it, so
			// strip the brackets and treat what is left as the host.
			inner := strings.TrimSuffix(strings.TrimPrefix(v, "["), "]")
			if net.ParseIP(inner) == nil {
				return "", fmt.Errorf("diagnostic target %q is not a valid address", raw)
			}
			return net.JoinHostPort(inner, "443"), nil
		}
		if err := validatePort(port); err != nil {
			return "", fmt.Errorf("diagnostic target %q: %w", raw, err)
		}
		if net.ParseIP(host) == nil {
			return "", fmt.Errorf("diagnostic target %q is not a valid address", raw)
		}
		return net.JoinHostPort(host, port), nil
	}

	// host:port with exactly one colon is ambiguous for IPv6, so only treat the
	// last colon as a port separator when the value parses as host:port and the
	// host half is not itself an IPv6 literal.
	if strings.Count(v, ":") == 1 {
		host, port, err := net.SplitHostPort(v)
		if err == nil {
			if err := validatePort(port); err != nil {
				return "", fmt.Errorf("diagnostic target %q: %w", raw, err)
			}
			if err := validateHost(host); err != nil {
				return "", fmt.Errorf("diagnostic target %q: %w", raw, err)
			}
			return net.JoinHostPort(host, port), nil
		}
	}

	// A colon count above one with no bracket is a bare IPv6 literal.
	if ip := net.ParseIP(v); ip != nil {
		return net.JoinHostPort(v, "443"), nil
	}
	if strings.Contains(v, ":") {
		return "", fmt.Errorf("diagnostic target %q is not a valid host or host:port", raw)
	}
	if err := validateHost(v); err != nil {
		return "", fmt.Errorf("diagnostic target %q: %w", raw, err)
	}
	return net.JoinHostPort(v, "443"), nil
}

func validateHost(host string) error {
	if host == "" {
		return fmt.Errorf("host is empty")
	}
	if net.ParseIP(host) != nil {
		return nil
	}
	// A DNS name: letters, digits, hyphens and dots. Underscores appear in some
	// SRV-only names but not in anything a latency probe dials by A/AAAA, so
	// they are rejected rather than silently dialed and failed.
	for _, r := range host {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '-', r == '.':
		default:
			return fmt.Errorf("host %q contains an invalid character", host)
		}
	}
	if strings.HasPrefix(host, ".") || strings.HasSuffix(host, ".") || strings.Contains(host, "..") {
		return fmt.Errorf("host %q is malformed", host)
	}
	return nil
}

func validatePort(port string) error {
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return fmt.Errorf("port %q is out of range", port)
	}
	return nil
}
