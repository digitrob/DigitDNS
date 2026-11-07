package dns

import (
	"net"
	"testing"

	"digitdns/internal/core/matcher"
	"github.com/miekg/dns"
)

func relayHandler(v4, v6 string) *Handler {
	m := matcher.NewMatcher()
	m.SetCustomRules([]string{"ard.example"}, nil, nil, nil)
	h := NewHandler(&dummyAccess{}, nil, m, nil, nil, "198.51.100.1")
	h.SetRelayLookup(func(domain string) (net.IP, net.IP, string, string, string, bool) {
		if domain != "ard.example" {
			return nil, nil, "", "", "", false
		}
		var ipv4, ipv6 net.IP
		if v4 != "" {
			ipv4 = net.ParseIP(v4).To4()
		}
		if v6 != "" {
			ipv6 = net.ParseIP(v6)
		}
		return ipv4, ipv6, "agt_de", "Frankfurt", "DE", true
	})
	return h
}

func query(h *Handler, name string, qtype uint16) *dns.Msg {
	req := new(dns.Msg)
	req.SetQuestion(dns.Fqdn(name), qtype)
	return h.ProcessQuery(req, "127.0.0.1")
}

func TestProxiedAAnswerUsesRelayAgent(t *testing.T) {
	resp := query(relayHandler("203.0.113.10", ""), "ard.example", dns.TypeA)
	if len(resp.Answer) != 1 {
		t.Fatalf("answers = %d, want 1", len(resp.Answer))
	}
	a, ok := resp.Answer[0].(*dns.A)
	if !ok || a.A.String() != "203.0.113.10" {
		t.Fatalf("answer = %v, want the agent address", resp.Answer[0])
	}
}

func TestAgentWithoutIPv6SinksAAAA(t *testing.T) {
	resp := query(relayHandler("203.0.113.10", ""), "ard.example", dns.TypeAAAA)
	if len(resp.Answer) != 0 {
		t.Fatalf("answers = %v, want the AAAA leak guard", resp.Answer)
	}
}

func TestProxiedAAAAAnswerUsesRelayAgentIPv6(t *testing.T) {
	h := relayHandler("203.0.113.10", "2001:db8::10")
	h.SetProxyIPv6Enabled(true)
	resp := query(h, "ard.example", dns.TypeAAAA)
	if len(resp.Answer) != 1 {
		t.Fatalf("answers = %v, want the agent IPv6", resp.Answer)
	}
	aaaa, ok := resp.Answer[0].(*dns.AAAA)
	if !ok || aaaa.AAAA.String() != "2001:db8::10" {
		t.Fatalf("answer = %v, want 2001:db8::10", resp.Answer[0])
	}
}

func TestAgentIPv6IsSunkWhenGlobalIPv6Off(t *testing.T) {
	// Default global switch is off; the agent's stored/public address must not
	// leak just because the relay target carries one.
	resp := query(relayHandler("203.0.113.10", "2001:db8::10"), "ard.example", dns.TypeAAAA)
	if len(resp.Answer) != 0 {
		t.Fatalf("answers = %v, want the global IPv6 sink", resp.Answer)
	}
}

func TestAgentWithoutIPv4FailsClosed(t *testing.T) {
	resp := query(relayHandler("", ""), "ard.example", dns.TypeA)
	if resp.Rcode != dns.RcodeServerFailure {
		t.Fatalf("rcode = %d, want SERVFAIL", resp.Rcode)
	}
	if len(resp.Answer) != 0 {
		t.Fatalf("answers = %v, want none", resp.Answer)
	}
}
