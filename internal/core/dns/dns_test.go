package dns

import (
	"testing"
	"time"

	"digitdns/internal/blocklist"
	"digitdns/internal/core/cache"
	"digitdns/internal/core/matcher"
	"digitdns/internal/core/upstream"
	"digitdns/internal/database"
	"github.com/miekg/dns"
)

type dummyAccess struct{}

func (d *dummyAccess) IsIPAllowed(ip string) (*database.Client, bool) {
	return &database.Client{Name: "VIP Gamer"}, true
}
func (d *dummyAccess) IsAllowAll() bool { return true }

func TestDNSHandler_ProcessQuery(t *testing.T) {
	c := cache.NewCache(1000, 60, 3600)
	defer c.Close()
	m := matcher.NewMatcher()
	m.SetCustomRules([]string{"playvalorant.com"}, nil, nil, nil)
	u := upstream.NewUpstreamPool([]string{"1.1.1.1:53", "8.8.8.8:53"}, 2*time.Second, true, "")

	handler := NewHandler(&dummyAccess{}, c, m, u, nil, "198.51.100.1")

	req := new(dns.Msg)
	req.SetQuestion("playvalorant.com.", dns.TypeA)

	resp := handler.ProcessQuery(req, "127.0.0.1")
	if resp == nil {
		t.Fatalf("expected non-nil response")
	}

	if len(resp.Answer) == 0 {
		t.Fatalf("expected answer for playvalorant.com")
	}

	aRecord, ok := resp.Answer[0].(*dns.A)
	if !ok || aRecord.A.String() != "198.51.100.1" {
		t.Errorf("expected A record to rewrite to 198.51.100.1, got %v", resp.Answer[0])
	}
}

func TestDNSHandler_ServesAdGuardDNSRewrite(t *testing.T) {
	c := cache.NewCache(1000, 60, 3600)
	defer c.Close()
	m := matcher.NewMatcher()
	index, _ := blocklist.Build([]blocklist.Source{{
		Name:    "custom rules",
		Enabled: true,
		Text: "||rewrite-v4.example^$dnsrewrite=192.0.2.44\n" +
			"||rewrite-v6.example^$dnsrewrite=2001:db8::44\n" +
			"||rewrite-cname.example^$dnsrewrite=alias.target.example\n",
	}}, nil)
	m.SetBlockIndex(index)
	u := upstream.NewUpstreamPool([]string{"1.1.1.1:53"}, time.Second, true, "")
	handler := NewHandler(&dummyAccess{}, c, m, u, nil, "198.51.100.1")

	for _, tc := range []struct {
		name   string
		qtype  uint16
		wantIP string
		wantRR bool
	}{
		{"rewrite-v4.example.", dns.TypeA, "192.0.2.44", true},
		{"child.rewrite-v4.example.", dns.TypeA, "192.0.2.44", true},
		{"rewrite-v6.example.", dns.TypeAAAA, "2001:db8::44", true},
		{"rewrite-cname.example.", dns.TypeA, "alias.target.example.", true},
		{"rewrite-cname.example.", dns.TypeCNAME, "alias.target.example.", true},
		// A rewrite for the other address family is a successful NODATA response,
		// not a fall-through to the upstream's real address.
		{"rewrite-v4.example.", dns.TypeAAAA, "", false},
		{"rewrite-v4.example.", dns.TypeCNAME, "", false},
	} {
		req := new(dns.Msg)
		req.SetQuestion(tc.name, tc.qtype)
		resp := handler.ProcessQuery(req, "127.0.0.1")
		if resp == nil || resp.Rcode != dns.RcodeSuccess {
			t.Fatalf("%s type=%d response = %#v, want NOERROR", tc.name, tc.qtype, resp)
		}
		if !tc.wantRR {
			if len(resp.Answer) != 0 {
				t.Errorf("%s type=%d returned unexpected records: %v", tc.name, tc.qtype, resp.Answer)
			}
			continue
		}
		if len(resp.Answer) != 1 {
			t.Fatalf("%s type=%d answer count = %d, want 1", tc.name, tc.qtype, len(resp.Answer))
		}
		got := resp.Answer[0].Header().Name
		switch rr := resp.Answer[0].(type) {
		case *dns.A:
			got = rr.A.String()
		case *dns.AAAA:
			got = rr.AAAA.String()
		case *dns.CNAME:
			got = rr.Target
		default:
			t.Fatalf("%s type=%d answer type = %T", tc.name, tc.qtype, resp.Answer[0])
		}
		if got != tc.wantIP {
			t.Errorf("%s type=%d answer = %q, want %q", tc.name, tc.qtype, got, tc.wantIP)
		}
	}
}
