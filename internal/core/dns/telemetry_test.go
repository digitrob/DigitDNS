package dns

import (
	"testing"

	"digitdns/internal/core/matcher"
	"digitdns/internal/database"

	"github.com/miekg/dns"
)

type recordingTelemetry struct {
	total int
	items []database.QueryLogItem
}

func (r *recordingTelemetry) RecordQuery() { r.total++ }
func (r *recordingTelemetry) PushQueryLog(item database.QueryLogItem) {
	r.items = append(r.items, item)
}

func TestProcessQueryLogsQTypeAndRcode(t *testing.T) {
	m := matcher.NewMatcher()
	m.SetCustomRules(nil, []string{"ads.example"}, nil, nil)
	sink := &recordingTelemetry{}
	h := NewHandler(&dummyAccess{}, nil, m, nil, sink, "198.51.100.1")

	req := new(dns.Msg)
	req.SetQuestion("ads.example.", dns.TypeAAAA)
	resp := h.ProcessQuery(req, "192.0.2.5")
	if resp == nil {
		t.Fatal("ProcessQuery returned nil for a blocked AAAA query")
	}
	if resp.Rcode != dns.RcodeSuccess {
		t.Fatalf("blocked response rcode = %d, want NOERROR", resp.Rcode)
	}
	if sink.total != 1 {
		t.Fatalf("RecordQuery calls = %d, want 1", sink.total)
	}
	if len(sink.items) != 1 {
		t.Fatalf("PushQueryLog calls = %d, want 1", len(sink.items))
	}
	item := sink.items[0]
	if item.QType != uint16(dns.TypeAAAA) {
		t.Errorf("QType = %d, want AAAA (%d)", item.QType, dns.TypeAAAA)
	}
	if item.Rcode != dns.RcodeSuccess {
		t.Errorf("Rcode = %d, want NOERROR", item.Rcode)
	}
	if item.Action != "BLOCK" {
		t.Errorf("Action = %q, want BLOCK", item.Action)
	}
}
