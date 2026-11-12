package dns

import (
	"net"
	"testing"
	"time"

	"digitdns/internal/core/matcher"
	"digitdns/internal/core/upstream"
	"github.com/miekg/dns"
)

func relayHandler(v4, v6 string) *Handler {
	m := matcher.NewMatcher()
	m.SetCustomRules([]string{"ard.example"}, nil, nil, nil)
	h := NewHandler(&dummyAccess{}, nil, m, nil, nil, "198.51.100.1")
	h.SetRelayLookup(func(domain string) RelayTarget {
		if domain != "ard.example" {
			return RelayTarget{Kind: "local"}
		}
		var ipv4, ipv6 net.IP
		if v4 != "" {
			ipv4 = net.ParseIP(v4).To4()
		}
		if v6 != "" {
			ipv6 = net.ParseIP(v6)
		}
		return RelayTarget{Kind: "agent", IPv4: ipv4, IPv6: ipv6, AgentID: "agt_de", AgentName: "Frankfurt", Country: "DE"}
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
	if a.Hdr.Ttl != agentProxyAnswerTTL {
		t.Fatalf("TTL = %d, want %d for an agent-backed answer", a.Hdr.Ttl, agentProxyAnswerTTL)
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
	if aaaa.Hdr.Ttl != agentProxyAnswerTTL {
		t.Fatalf("AAAA TTL = %d, want %d", aaaa.Hdr.Ttl, agentProxyAnswerTTL)
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

func egressHandler(failClosed, healthy bool) *Handler {
	m := matcher.NewMatcher()
	m.SetCustomRules([]string{"ard.example"}, nil, nil, nil)
	h := NewHandler(&dummyAccess{}, nil, m, nil, nil, "198.51.100.1")
	h.SetRelayLookup(func(domain string) RelayTarget {
		if domain != "ard.example" {
			return RelayTarget{Kind: "local"}
		}
		return RelayTarget{
			Kind: "egress", EgressID: "egress-1", EgressName: "NL proxy",
			EgressKind: "socks5", FailClosed: failClosed, Healthy: healthy,
		}
	})
	return h
}

func TestHealthyEgressAnswersWithHubAddress(t *testing.T) {
	resp := query(egressHandler(true, true), "ard.example", dns.TypeA)
	if resp.Rcode != dns.RcodeSuccess || len(resp.Answer) != 1 {
		t.Fatalf("response = %v, want the hub A answer", resp)
	}
	a, ok := resp.Answer[0].(*dns.A)
	if !ok || a.A.String() != "198.51.100.1" {
		t.Fatalf("answer = %v, want the hub address", resp.Answer[0])
	}
	if a.Hdr.Ttl != localProxyAnswerTTL {
		t.Fatalf("TTL = %d, want the local relay TTL for a hub-side egress", a.Hdr.Ttl)
	}
}

func TestFailClosedEgressReturnsSERVFAIL(t *testing.T) {
	resp := query(egressHandler(true, false), "ard.example", dns.TypeA)
	if resp.Rcode != dns.RcodeServerFailure {
		t.Fatalf("rcode = %d, want SERVFAIL for an offline fail-closed egress", resp.Rcode)
	}
	if len(resp.Answer) != 0 {
		t.Fatalf("answers = %v, want no fallback address", resp.Answer)
	}
}

func TestFailOpenEgressFallsBackToHub(t *testing.T) {
	// The router returns a local target for an unhealthy fail-open egress; this
	// test proves the DNS handler does not add its own refusal on top.
	resp := query(egressHandler(false, false), "ard.example", dns.TypeA)
	if resp.Rcode != dns.RcodeSuccess || len(resp.Answer) != 1 {
		t.Fatalf("response = %v, want the hub fallback", resp)
	}
}

func TestEngineOffBypassesProxyRules(t *testing.T) {
	m := matcher.NewMatcher()
	m.SetCustomRules([]string{"ard.example"}, nil, nil, nil)
	pool := upstream.NewUpstreamPool(nil, time.Second, false, "")
	h := NewHandler(&dummyAccess{}, nil, m, pool, nil, "198.51.100.1")
	h.SetEngineEnabled(false)
	resp := query(h, "ard.example", dns.TypeA)
	if resp == nil {
		return
	}
	for _, rr := range resp.Answer {
		if a, ok := rr.(*dns.A); ok && a.A.String() == "198.51.100.1" {
			t.Fatalf("engine off still answered with the hub proxy address: %v", rr)
		}
	}
}
