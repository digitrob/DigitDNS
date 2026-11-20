package dns

import (
	"testing"

	"digitdns/internal/core/matcher"
	"digitdns/internal/database"

	"github.com/miekg/dns"
)

type recordingTelemetry struct {
	total       int
	items       []database.QueryLogItem
	subscribers int64
}

func (r *recordingTelemetry) RecordQuery() { r.total++ }
func (r *recordingTelemetry) PushQueryLog(item database.QueryLogItem) {
	r.items = append(r.items, item)
}
func (r *recordingTelemetry) SSESubscribers() int64 { return r.subscribers }
func (r *recordingTelemetry) QueryLogEnabled() bool { return false }

func responseWithA(name string) *dns.Msg {
	m := new(dns.Msg)
	m.SetQuestion(name, dns.TypeA)
	rr, err := dns.NewRR(name + " 60 IN A 203.0.113.9")
	if err == nil {
		m.Answer = append(m.Answer, rr)
	}
	return m
}

func TestLoggingAnswersSkipsWorkNobodyReads(t *testing.T) {
	resp := responseWithA("cache.example.")

	// No sinks at all: rendering would be pure waste.
	h := NewHandler(&dummyAccess{}, nil, matcher.NewMatcher(), nil, nil, "198.51.100.1")
	if got := h.loggingAnswers(resp); got != nil {
		t.Fatalf("answers with no sinks = %+v, want nil", got)
	}

	// Telemetry sink present but no live-log subscriber and no persistent log:
	// the historical cache hit still needs its counters, not its answer text.
	sink := &recordingTelemetry{}
	h = NewHandler(&dummyAccess{}, nil, matcher.NewMatcher(), nil, sink, "198.51.100.1")
	h.SetQueryLogSink(func(database.QueryLogItem) {}, func() bool { return false })
	if got := h.loggingAnswers(resp); got != nil {
		t.Fatalf("answers with idle sinks = %+v, want nil", got)
	}

	// A live-log subscriber needs the rendered chain for the tooltip.
	sink.subscribers = 1
	if got := h.loggingAnswers(resp); len(got) != 1 || got[0].Type != "A" {
		t.Fatalf("answers with a subscriber = %+v, want one A record", got)
	}
	sink.subscribers = 0

	// Persisted history needs the same rendering even with no dashboard open.
	h.SetQueryLogSink(func(database.QueryLogItem) {}, func() bool { return true })
	if got := h.loggingAnswers(resp); len(got) != 1 || got[0].Type != "A" {
		t.Fatalf("answers with history enabled = %+v, want one A record", got)
	}
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
