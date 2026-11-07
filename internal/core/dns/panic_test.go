package dns

import (
	"net"
	"testing"

	"digitdns/internal/safego"
	"github.com/miekg/dns"
)

// panicRemoteWriter simulates a ResponseWriter that panics before the handler
// has done any work. The handler must contain it, count it, and still answer
// SERVFAIL rather than taking the daemon down.
type panicRemoteWriter struct {
	captureWriter
}

func (w *panicRemoteWriter) RemoteAddr() net.Addr { panic("boom") }

func TestServeDNSSurvivesHandlerPanic(t *testing.T) {
	h, _, _ := newGuardHandler(t)
	before := safego.Count()
	w := &panicRemoteWriter{}

	h.serve(w, new(dns.Msg), "")

	if safego.Count() != before+1 {
		t.Fatalf("panic count = %d, want %d", safego.Count(), before+1)
	}
	if w.msg == nil {
		t.Fatal("handler panic produced no reply")
	}
	if w.msg.Rcode != dns.RcodeServerFailure {
		t.Fatalf("rcode = %s, want SERVFAIL", rcodeOf(w.msg))
	}
}
