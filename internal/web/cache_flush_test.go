package web

import (
	"net"
	"net/http"
	"testing"

	"github.com/miekg/dns"
)

// A rule save used to flush the entire DNS cache even when the edit only moved a
// name between Direct, Proxy and Block. The matcher runs before the cache, so
// that entry can never bypass the new rule; the flush only cold-started every
// other name. Pin the narrower behaviour: a plain rule save keeps the cache.
func TestRuleSaveWithoutProfileChangeKeepsCache(t *testing.T) {
	ws, h, tok, cleanup := authedServer(t)
	defer cleanup()

	const name = "kept.example."
	msg := new(dns.Msg)
	msg.SetQuestion(name, dns.TypeA)
	msg.Response = true
	msg.Answer = []dns.RR{&dns.A{
		Hdr: dns.RR_Header{Name: name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 300},
		A:   net.IPv4(192, 0, 2, 1),
	}}
	ws.cache.Put(msg.Question[0], msg)
	if got := ws.cache.Count(); got != 1 {
		t.Fatalf("cache count before save = %d, want 1", got)
	}

	if w := postJSON(t, h, "/api/config/rules", `{"custom_proxied":["kept.example"]}`, tok); w.Code != http.StatusOK {
		t.Fatalf("rule save = %d — %s", w.Code, w.Body.String())
	}

	if got := ws.cache.Count(); got != 1 {
		t.Fatalf("cache count after rule save = %d, want 1 (unconditional flush returned)", got)
	}
}
