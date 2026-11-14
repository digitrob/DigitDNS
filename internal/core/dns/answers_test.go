package dns

import (
	"encoding/json"
	"net"
	"strings"
	"testing"

	"digitdns/internal/database"
	"github.com/miekg/dns"
)

// The live-log tooltip shows the resolver's answer section in wire order. That
// only works if the extraction walks resp.Answer and reads each record's own
// header, so this pins the CNAME→A chain the tooltip is meant to display.
func TestExtractAnswersPreservesOrderAndTTL(t *testing.T) {
	resp := new(dns.Msg)
	resp.Answer = []dns.RR{
		&dns.CNAME{
			Hdr:    dns.RR_Header{Name: "www.example.com.", Rrtype: dns.TypeCNAME, Ttl: 261},
			Target: "tls130rtt.spotifycdn.map.fastly.net.",
		},
		&dns.A{
			Hdr: dns.RR_Header{Name: "tls130rtt.spotifycdn.map.fastly.net.", Rrtype: dns.TypeA, Ttl: 21},
			A:   net.ParseIP("199.232.210.251"),
		},
		&dns.A{
			Hdr: dns.RR_Header{Name: "tls130rtt.spotifycdn.map.fastly.net.", Rrtype: dns.TypeA, Ttl: 21},
			A:   net.ParseIP("199.232.214.251"),
		},
	}

	got := extractAnswers(resp)
	want := []struct {
		typ, value string
		ttl        uint32
	}{
		{"CNAME", "tls130rtt.spotifycdn.map.fastly.net.", 261},
		{"A", "199.232.210.251", 21},
		{"A", "199.232.214.251", 21},
	}
	if len(got) != len(want) {
		t.Fatalf("extractAnswers returned %d records, want %d: %+v", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i].Type != w.typ || got[i].Value != w.value || got[i].TTL != w.ttl {
			t.Fatalf("record %d = %+v, want %s %s ttl=%d", i, got[i], w.typ, w.value, w.ttl)
		}
	}

	ips := answersToIPs(got)
	if len(ips) != 2 || ips[0] != "199.232.210.251" || ips[1] != "199.232.214.251" {
		t.Fatalf("answersToIPs = %v, want the two A records", ips)
	}
}

// The web and Android live logs read "answers" from the SSE frame, so the wire
// name is part of the contract: a rename would silently empty every tooltip.
func TestQueryLogItemAnswersWireShape(t *testing.T) {
	item := database.QueryLogItem{
		Domain:  "www.example.com",
		Answers: []database.DNSAnswer{{Type: "CNAME", Value: "cdn.example.net.", TTL: 261}, {Type: "A", Value: "199.232.210.251", TTL: 21}},
	}
	raw, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(raw)
	for _, want := range []string{`"answers":[`, `"type":"CNAME"`, `"value":"cdn.example.net."`, `"ttl":261`} {
		if !strings.Contains(s, want) {
			t.Fatalf("JSON %s does not contain %s", s, want)
		}
	}
}

func TestExtractAnswersEmpty(t *testing.T) {
	if got := extractAnswers(nil); got != nil {
		t.Fatalf("extractAnswers(nil) = %+v, want nil", got)
	}
	if got := extractAnswers(new(dns.Msg)); got != nil {
		t.Fatalf("extractAnswers(empty) = %+v, want nil", got)
	}
}
