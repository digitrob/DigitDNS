// Package safego contains the panic boundary used by long-lived request
// handlers. A panic in a goroutine that was not started by net/http would
// otherwise terminate the whole process: one malformed packet or nil pointer
// deep in a relay or an upstream exchange is a full outage, not a failed query.
package safego

import (
	"log"
	"runtime/debug"
	"sync/atomic"
)

var panics atomic.Uint64

// Count is the number of recovered panics since start. It is exported so the
// dashboard can show that something went wrong without the operator having to
// find the log line.
func Count() uint64 {
	return panics.Load()
}

// Handle records one recovered panic. rec is the value returned by recover().
func Handle(where string, rec any) {
	if rec == nil {
		return
	}
	panics.Add(1)
	log.Printf("[Panic] %s: %v\n%s", where, rec, debug.Stack())
}

// Recover is the deferred form: defer safego.Recover("dns.ServeDNS").
func Recover(where string) {
	if rec := recover(); rec != nil {
		Handle(where, rec)
	}
}
