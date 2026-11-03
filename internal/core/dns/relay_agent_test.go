package dns

import (
	"testing"

	"digitdns/internal/core/matcher"
	"github.com/miekg/dns"
)

func relayHandler(v4 string) *Handler {
	m := matcher.NewMatcher()
	m.SetCustomRules([]string{"ard.example"}, nil, nil, nil)
	h := NewHandler(&dummyAccess{}, nil, m, nil, nil, "198.51.100.1")
	h.SetRelayLookup(func(domain string) (string, string, string, string, bool) {
		if domain != "ard.example" {
			return "", "", "", "", false
		}
		return v4, "agt_de", "Frankfurt", "DE", true
	})
	return h
}

func query(h *Handler, name string, qtype uint16) *dns.Msg {
	req := new(dns.Msg)
	req.SetQuestion(dns.Fqdn(name), qtype)
	return h.ProcessQuery(req, "127.0.0.1")
}

func TestProxiedAAnswerUsesRelayAgent(t *testing.T) {
	resp := query(relayHandler("203.0.113.10"), "ard.example", dns.TypeA)
	if len(resp.Answer) != 1 {
		t.Fatalf("answers = %d, want 1", len(resp.Answer))
	}
	a, ok := resp.Answer[0].(*dns.A)
	if !ok || a.A.String() != "203.0.113.10" {
		t.Fatalf("answer = %v, want the agent address", resp.Answer[0])
	}
}

func TestAgentWithoutIPv6SinksAAAA(t *testing.T) {
	resp := query(relayHandler("203.0.113.10"), "ard.example", dns.TypeAAAA)
	if len(resp.Answer) != 0 {
		t.Fatalf("answers = %v, want the AAAA leak guard", resp.Answer)
	}
}

func TestAgentWithoutIPv4FailsClosed(t *testing.T) {
	resp := query(relayHandler(""), "ard.example", dns.TypeA)
	if resp.Rcode != dns.RcodeServerFailure {
		t.Fatalf("rcode = %d, want SERVFAIL", resp.Rcode)
	}
	if len(resp.Answer) != 0 {
		t.Fatalf("answers = %v, want none", resp.Answer)
	}
}
