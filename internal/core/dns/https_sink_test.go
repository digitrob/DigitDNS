package dns

import (
	"net"
	"testing"

	"digitdns/internal/core/cache"
	"github.com/miekg/dns"
)

// httpsHintMsg is the upstream answer the guard must not serve: it advertises
// QUIC and carries both real address families.
func httpsHintMsg(name string) *dns.Msg {
	rr := &dns.HTTPS{SVCB: dns.SVCB{
		Hdr:      dns.RR_Header{Name: name, Rrtype: dns.TypeHTTPS, Class: dns.ClassINET, Ttl: 300},
		Priority: 1,
		Target:   ".",
		Value: []dns.SVCBKeyValue{
			&dns.SVCBAlpn{Alpn: []string{"h3", "h2"}},
			&dns.SVCBIPv4Hint{Hint: []net.IP{net.ParseIP("192.0.2.53")}},
			&dns.SVCBIPv6Hint{Hint: []net.IP{net.ParseIP("2001:db8::53")}},
		},
	}}
	m := new(dns.Msg)
	m.SetQuestion(name, dns.TypeHTTPS)
	m.Response = true
	m.Answer = []dns.RR{rr}
	return m
}

// A proxied name must not reveal the destination's QUIC capability or its real
// IPv4/IPv6 addresses through HTTPS/SVCB. Those records are requested by modern
// browsers before they connect, so serving the upstream answer would let the
// browser bypass the TCP relay even though A/AAAA point at the hub.
func TestProxiedHTTPSAndSVCBAreSunk(t *testing.T) {
	h, c, m := newGuardHandler(t)
	m.SetCustomRules([]string{"proxied.test"}, nil, nil, nil)

	for _, qtype := range []uint16{dns.TypeHTTPS, dns.TypeSVCB} {
		req := new(dns.Msg)
		req.SetQuestion("proxied.test.", qtype)
		req.SetEdns0(4096, true)

		resp := h.ProcessQuery(req, "127.0.0.1", "UDP")
		if resp == nil {
			t.Fatalf("%s: no reply; a client would retry or time out", dns.TypeToString[qtype])
		}
		if resp.Rcode != dns.RcodeSuccess {
			t.Errorf("%s: rcode = %s, want NOERROR", dns.TypeToString[qtype], dns.RcodeToString[resp.Rcode])
		}
		if len(resp.Answer) != 0 {
			t.Errorf("%s: answer = %v, want no address or ALPN hints", dns.TypeToString[qtype], resp.Answer)
		}
		if resp.AuthenticatedData {
			t.Errorf("%s: synthetic sink claimed AD", dns.TypeToString[qtype])
		}

		q := dns.Question{Name: "proxied.test.", Qtype: qtype, Qclass: dns.ClassINET}
		if got, ok := c.Get(q); ok && got != nil {
			t.Errorf("%s: synthetic sink entered the cache: %v", dns.TypeToString[qtype], got.Answer)
		}
	}
}

// An upstream HTTPS answer cached while the name was direct must never be
// served after the name becomes proxied. The guard runs before the cache; the
// stale entry stays in the cache but is unreachable for this name.
func TestProxiedHTTPSGuardWinsOverCachedUpstream(t *testing.T) {
	h, c, m := newGuardHandler(t)
	m.SetCustomRules([]string{"proxied.test"}, nil, nil, nil)

	q := dns.Question{Name: "proxied.test.", Qtype: dns.TypeHTTPS, Qclass: dns.ClassINET}
	opts := cache.QueryOptions{EDNS: true, DO: true}
	c.PutWithOptions(q, httpsHintMsg("proxied.test."), opts)
	if _, ok := c.GetWithOptions(q, opts); !ok {
		t.Fatal("test setup failed: the upstream HTTPS answer was not cached")
	}

	req := new(dns.Msg)
	req.SetQuestion("proxied.test.", dns.TypeHTTPS)
	req.SetEdns0(4096, true)
	resp := h.ProcessQuery(req, "127.0.0.1", "UDP")
	if resp == nil || resp.Rcode != dns.RcodeSuccess {
		t.Fatalf("proxied HTTPS = %v, want NOERROR", resp)
	}
	if len(resp.Answer) != 0 {
		t.Fatalf("proxied HTTPS served %v, want the local NODATA guard", resp.Answer)
	}
	if resp.AuthenticatedData {
		t.Fatal("proxied HTTPS sink claimed AD")
	}
}

// The dashboard switch is live. Disabling it restores the upstream HTTPS/SVCB
// path for proxied names; this is the explicit opt-out, not the default.
func TestProxyLeakProtectionCanBeDisabled(t *testing.T) {
	h, c, m := newGuardHandler(t)
	m.SetCustomRules([]string{"proxied.test"}, nil, nil, nil)
	// The IPv6-off state forces the HTTPS/SVCB sink independently of this
	// switch, so enable IPv6 to exercise the explicit proxy-leak opt-out.
	h.SetPublicIPv6("2001:db8::1")
	h.SetProxyIPv6Enabled(true)
	h.SetProxyLeakProtection(false)

	q := dns.Question{Name: "proxied.test.", Qtype: dns.TypeHTTPS, Qclass: dns.ClassINET}
	opts := cache.QueryOptions{EDNS: true, DO: true}
	c.PutWithOptions(q, httpsHintMsg("proxied.test."), opts)

	req := new(dns.Msg)
	req.SetQuestion("proxied.test.", dns.TypeHTTPS)
	req.SetEdns0(4096, true)
	resp := h.ProcessQuery(req, "127.0.0.1", "UDP")
	if resp == nil || resp.Rcode != dns.RcodeSuccess || len(resp.Answer) != 1 {
		t.Fatalf("disabled proxy leak protection = %v, want the cached HTTPS answer", resp)
	}
}

// Turning proxy leak protection off is not enough to reopen HTTPS/SVCB while
// proxied IPv6 is off: an upstream ipv6hint would still let the client bypass
// the relay. This is the hard leak guarantee the IPv6 switch exists for.
func TestHTTPSStaysSunkWhenIPv6OffEvenIfLeakGuardDisabled(t *testing.T) {
	h, c, m := newGuardHandler(t)
	m.SetCustomRules([]string{"proxied.test"}, nil, nil, nil)
	h.SetProxyLeakProtection(false)
	// proxyIPv6Enabled is false by default.

	q := dns.Question{Name: "proxied.test.", Qtype: dns.TypeHTTPS, Qclass: dns.ClassINET}
	opts := cache.QueryOptions{EDNS: true, DO: true}
	c.PutWithOptions(q, httpsHintMsg("proxied.test."), opts)

	req := new(dns.Msg)
	req.SetQuestion("proxied.test.", dns.TypeHTTPS)
	req.SetEdns0(4096, true)
	resp := h.ProcessQuery(req, "127.0.0.1", "UDP")
	if resp == nil || resp.Rcode != dns.RcodeSuccess {
		t.Fatalf("proxied HTTPS = %v, want NOERROR", resp)
	}
	if len(resp.Answer) != 0 {
		t.Fatalf("proxied HTTPS served %v with IPv6 off and the leak guard disabled, want a sink", resp.Answer)
	}
}

// A proxied AAAA follows the IPv6 policy: the stored public address is
// published only while the switch is on, and disabling it immediately returns
// the empty-NOERROR sink.
func TestProxiedAAAAFollowsTheIPv6Switch(t *testing.T) {
	h, _, m := newGuardHandler(t)
	m.SetCustomRules([]string{"proxied.test"}, nil, nil, nil)
	h.SetPublicIPv6("2001:db8::1")

	ask := func() *dns.Msg {
		req := new(dns.Msg)
		req.SetQuestion("proxied.test.", dns.TypeAAAA)
		return h.ProcessQuery(req, "127.0.0.1", "UDP")
	}

	if resp := ask(); resp == nil || resp.Rcode != dns.RcodeSuccess || len(resp.Answer) != 0 {
		t.Fatalf("AAAA with IPv6 off = %v, want the empty sink", resp)
	}

	h.SetProxyIPv6Enabled(true)
	resp := ask()
	if resp == nil || resp.Rcode != dns.RcodeSuccess || len(resp.Answer) != 1 {
		t.Fatalf("AAAA with IPv6 on = %v, want the public IPv6 answer", resp)
	}
	aaaa, ok := resp.Answer[0].(*dns.AAAA)
	if !ok || aaaa.AAAA.String() != "2001:db8::1" {
		t.Fatalf("AAAA answer = %v, want 2001:db8::1", resp.Answer[0])
	}

	h.SetProxyIPv6Enabled(false)
	if resp := ask(); resp == nil || resp.Rcode != dns.RcodeSuccess || len(resp.Answer) != 0 {
		t.Fatalf("AAAA after disabling IPv6 = %v, want the empty sink again", resp)
	}
}

// Non-proxied names keep their normal HTTPS answer. This is the control that
// proves the guard is scoped to ActionProxy rather than to the record type.
func TestDirectHTTPSStillServedFromCache(t *testing.T) {
	h, c, _ := newGuardHandler(t)

	q := dns.Question{Name: "direct.test.", Qtype: dns.TypeHTTPS, Qclass: dns.ClassINET}
	opts := cache.QueryOptions{EDNS: true, DO: true}
	c.PutWithOptions(q, httpsHintMsg("direct.test."), opts)

	req := new(dns.Msg)
	req.SetQuestion("direct.test.", dns.TypeHTTPS)
	req.SetEdns0(4096, true)
	resp := h.ProcessQuery(req, "127.0.0.1", "UDP")
	if resp == nil || resp.Rcode != dns.RcodeSuccess || len(resp.Answer) != 1 {
		t.Fatalf("direct HTTPS = %v, want the cached upstream answer", resp)
	}
	if _, ok := resp.Answer[0].(*dns.HTTPS); !ok {
		t.Fatalf("direct HTTPS answer = %T, want *dns.HTTPS", resp.Answer[0])
	}
}

// Blocked names have always returned the configured block response for every
// query type. Pin that here so a future proxy-only guard cannot reopen the
// HTTPS/SVCB path for blocked names.
func TestBlockedHTTPSAndSVCBStayEmpty(t *testing.T) {
	h, _, m := newGuardHandler(t)
	m.SetCustomRules(nil, []string{"blocked.test"}, nil, nil)

	for _, qtype := range []uint16{dns.TypeHTTPS, dns.TypeSVCB} {
		req := new(dns.Msg)
		req.SetQuestion("blocked.test.", qtype)
		resp := h.ProcessQuery(req, "127.0.0.1", "UDP")
		if resp == nil || resp.Rcode != dns.RcodeSuccess {
			t.Fatalf("blocked %s = %v, want NOERROR", dns.TypeToString[qtype], resp)
		}
		if len(resp.Answer) != 0 {
			t.Errorf("blocked %s served %v, want the null-IP shape", dns.TypeToString[qtype], resp.Answer)
		}
	}
}

// The HTTPS/SVCB guard sits beside the existing A/AAAA proxy answers and must
// not disturb them.
func TestProxiedAddressAnswersRemain(t *testing.T) {
	h, _, m := newGuardHandler(t)
	m.SetCustomRules([]string{"proxied.test"}, nil, nil, nil)

	reqA := new(dns.Msg)
	reqA.SetQuestion("proxied.test.", dns.TypeA)
	respA := h.ProcessQuery(reqA, "127.0.0.1", "UDP")
	if respA == nil || respA.Rcode != dns.RcodeSuccess || len(respA.Answer) != 1 {
		t.Fatalf("proxied A = %v, want one hub address", respA)
	}
	a, ok := respA.Answer[0].(*dns.A)
	if !ok || a.A.String() != "198.51.100.1" {
		t.Fatalf("proxied A = %v, want 198.51.100.1", respA.Answer[0])
	}

	reqAAAA := new(dns.Msg)
	reqAAAA.SetQuestion("proxied.test.", dns.TypeAAAA)
	respAAAA := h.ProcessQuery(reqAAAA, "127.0.0.1", "UDP")
	if respAAAA == nil || respAAAA.Rcode != dns.RcodeSuccess || len(respAAAA.Answer) != 0 {
		t.Fatalf("proxied AAAA = %v, want the existing IPv4-forcing sink", respAAAA)
	}
}
