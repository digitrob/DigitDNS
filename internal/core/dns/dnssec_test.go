package dns

import (
	"context"
	"testing"

	"digitdns/internal/core/cache"
	"digitdns/internal/core/dnssec"
	"github.com/miekg/dns"
)

type stubValidator struct {
	result dnssec.Result
}

func (s stubValidator) Validate(context.Context, dns.Question, *dns.Msg) dnssec.Result {
	return s.result
}

func TestValidateUpstreamSetsADOnlyOnSecure(t *testing.T) {
	q := dns.Question{Name: "example.test.", Qtype: dns.TypeA, Qclass: dns.ClassINET}

	for _, tc := range []struct {
		name   string
		result dnssec.Result
		wantAD bool
	}{
		{name: "secure", result: dnssec.Result{Status: dnssec.Secure}, wantAD: true},
		{name: "insecure", result: dnssec.Result{Status: dnssec.Insecure}, wantAD: false},
		{name: "bogus", result: dnssec.Result{Status: dnssec.Bogus}, wantAD: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := &Handler{validator: stubValidator{result: tc.result}}
			resp := new(dns.Msg)
			resp.SetQuestion(q.Name, q.Qtype)
			resp.AuthenticatedData = true // an upstream's claim must not survive
			q := q
			got := h.validateUpstream(q, resp)
			if got.Status != tc.result.Status {
				t.Fatalf("status = %s, want %s", got.Status, tc.result.Status)
			}
			if resp.AuthenticatedData != tc.wantAD {
				t.Errorf("AD = %v, want %v", resp.AuthenticatedData, tc.wantAD)
			}
		})
	}
}

func TestNormalizeForClientStripsForcedDNSSEC(t *testing.T) {
	makeResp := func() *dns.Msg {
		m := new(dns.Msg)
		m.SetQuestion("example.test.", dns.TypeA)
		m.Response = true
		m.Answer = []dns.RR{
			mustRR(t, "example.test. 300 IN A 192.0.2.1"),
			mustRR(t, "example.test. 300 IN RRSIG A 13 2 300 20300101000000 20200101000000 1 example.test. AAAA"),
		}
		m.Ns = []dns.RR{
			mustRR(t, "example.test. 300 IN NSEC next.example.test. A RRSIG NSEC"),
			mustRR(t, "example.test. 300 IN RRSIG NSEC 13 2 300 20300101000000 20200101000000 1 example.test. AAAA"),
		}
		m.SetEdns0(4096, true)
		return m
	}

	t.Run("DO=0 EDNS", func(t *testing.T) {
		resp := makeResp()
		normalizeForClient(resp, cache.QueryOptions{EDNS: true}, dns.TypeA)
		if len(resp.Answer) != 1 || resp.Answer[0].Header().Rrtype != dns.TypeA {
			t.Errorf("answer = %v, want only the A record", resp.Answer)
		}
		if len(resp.Ns) != 0 {
			t.Errorf("authority = %v, want the NSEC and its RRSIG removed", resp.Ns)
		}
		opt := resp.IsEdns0()
		if opt == nil {
			t.Fatal("EDNS client lost the OPT record")
		}
		if opt.Do() {
			t.Error("DO bit survived a DO=0 request")
		}
	})

	t.Run("no EDNS", func(t *testing.T) {
		resp := makeResp()
		normalizeForClient(resp, cache.QueryOptions{}, dns.TypeA)
		if resp.IsEdns0() != nil {
			t.Error("a client that sent no OPT was given one")
		}
	})

	t.Run("DO=1 keeps signatures", func(t *testing.T) {
		resp := makeResp()
		normalizeForClient(resp, cache.QueryOptions{EDNS: true, DO: true}, dns.TypeA)
		if len(resp.Answer) != 2 || len(resp.Ns) != 2 {
			t.Errorf("DO=1 response was stripped: answer=%d authority=%d", len(resp.Answer), len(resp.Ns))
		}
	})
}

func TestBogusEDEOnlyForOPTQueries(t *testing.T) {
	req := new(dns.Msg)
	req.SetQuestion("example.test.", dns.TypeA)
	req.SetEdns0(1232, true)

	m := new(dns.Msg)
	m.SetRcode(req, dns.RcodeServerFailure)
	attachBogusEDE(m, req)
	opt := m.IsEdns0()
	if opt == nil {
		t.Fatal("OPT query lost its OPT record on SERVFAIL")
	}
	found := false
	for _, o := range opt.Option {
		if ede, ok := o.(*dns.EDNS0_EDE); ok && ede.InfoCode == dns.ExtendedErrorCodeDNSBogus {
			found = true
		}
	}
	if !found {
		t.Fatal("SERVFAIL carried no DNSSEC Bogus EDE")
	}

	plain := new(dns.Msg)
	plain.SetQuestion("example.test.", dns.TypeA)
	pm := new(dns.Msg)
	pm.SetRcode(plain, dns.RcodeServerFailure)
	attachBogusEDE(pm, plain)
	if pm.IsEdns0() != nil {
		t.Fatal("a plain query was answered with an OPT record")
	}
}
