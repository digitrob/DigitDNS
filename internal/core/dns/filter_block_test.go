package dns

import (
	"testing"

	"digitdns/internal/blocklist"
	"digitdns/internal/core/cache"
	"digitdns/internal/core/matcher"
	"github.com/miekg/dns"
)

func TestBlockAnswerModesAndTTL(t *testing.T) {
	c := cache.NewCache(100, 60, 3600)
	defer c.Close()
	m := matcher.NewMatcher()
	index, _ := blocklist.Build([]blocklist.Source{{
		Name: "Test", Enabled: true, Text: "||ads.example^\n",
	}}, nil)
	m.SetBlockIndex(index)
	h := NewHandler(&dummyAccess{}, c, m, nil, nil, "198.51.100.1")

	mode := "nxdomain"
	h.SetBlockPolicy(func() (string, uint32) { return mode, 30 })
	req := new(dns.Msg)
	req.SetQuestion("ads.example.", dns.TypeA)
	if resp := h.ProcessQuery(req, "127.0.0.1"); resp.Rcode != dns.RcodeNameError {
		t.Fatalf("nxdomain rcode = %d", resp.Rcode)
	}

	mode = "refused"
	if resp := h.ProcessQuery(req, "127.0.0.1"); resp.Rcode != dns.RcodeRefused {
		t.Fatalf("refused rcode = %d", resp.Rcode)
	}

	mode = "null_ip"
	resp := h.ProcessQuery(req, "127.0.0.1")
	if len(resp.Answer) != 1 {
		t.Fatalf("null-ip answers = %v", resp.Answer)
	}
	rr, ok := resp.Answer[0].(*dns.A)
	if !ok || rr.A.String() != "0.0.0.0" || rr.Hdr.Ttl != 30 {
		t.Fatalf("null-ip answer = %#v", resp.Answer[0])
	}
}
