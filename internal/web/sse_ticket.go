package web

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"digitdns/internal/httpx"
)

// One-time SSE tickets (v2.1.0 A-06 remediation).
//
// EventSource cannot set headers, so the live-log stream used to accept the
// long-lived session token in the query string — a credential that leaks into
// access logs, browser history and Referer headers. The fix keeps the query
// string but never puts a long-lived credential in it: the dashboard exchanges
// its Bearer token once, via POST, for a ticket that is single-use and expires
// in 60 seconds. An intercepted URL therefore buys an attacker nothing after
// the first connection, and nothing at all if it is captured before use.

const (
	sseTicketTTL  = 60 * time.Second
	sseTicketSize = 16 // 128 bits of entropy

	// maxOutstandingSSETickets bounds how many unconsumed tickets one session
	// may hold at once (audit needs-validation fix, v2.8). The endpoint is
	// admin-gated, so this is not an unauthenticated lever — but the panel and
	// the resolver share one process, so a session hammering the endpoint for
	// the whole TTL could otherwise grow the store without limit, and every
	// mint sweeps the store under one mutex. The cap bounds both; a request over
	// the cap is refused rather than served.
	maxOutstandingSSETickets = 64
)

// sessionKeyContextKey is the context key the auth gate uses to carry the
// caller's session token. A comparable, non-zero pointer type: context lookup
// keys must not collide across packages.
var sessionKeyContextKey = &struct{ session string }{}

// sseTicketStore holds outstanding tickets. The mutex is fine at this scale:
// one ticket per operator tab connecting, not per query.
type sseTicketStore struct {
	mu            sync.Mutex
	tickets       map[string]time.Time // value = expiry
	ownerByTicket map[string]string    // ticket -> session key (for the cap accounting)
	byOwner       map[string]int       // session key -> outstanding count
}

func newSSETicketStore() *sseTicketStore {
	return &sseTicketStore{
		tickets:       make(map[string]time.Time),
		ownerByTicket: make(map[string]string),
		byOwner:       make(map[string]int),
	}
}

// mint creates a fresh ticket bound to nothing but itself; the stream handler
// validates the caller's real credential separately after the redirect-free
// connect. Tickets are not tokens: they authorize exactly one /events/stream
// or /api/stream/queries handshake within the TTL and are consumed on use.
//
// owner keys the per-session cap. An owner at maxOutstandingSSETickets gets no
// new ticket: mint returns "" and the handler answers 429, so one session can
// never grow the store past the cap the way it could before the cap existed.
func (s *sseTicketStore) mint(owner string) string {
	b := make([]byte, sseTicketSize)
	_, _ = rand.Read(b)
	ticket := hex.EncodeToString(b)
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	// Opportunistic sweep: the map holds at most a bounded number of entries,
	// but a swept map is a bounded map regardless of how many tabs were opened.
	for k, exp := range s.tickets {
		if exp.Before(now) {
			delete(s.tickets, k)
		}
	}
	if n := s.byOwner[owner]; n >= maxOutstandingSSETickets {
		return ""
	}
	s.tickets[ticket] = now.Add(sseTicketTTL)
	s.byOwner[owner]++
	s.ownerByTicket[ticket] = owner
	return ticket
}

// consume validates and burns a ticket in one step.
func (s *sseTicketStore) consume(ticket string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	exp, ok := s.tickets[ticket]
	if !ok || exp.Before(time.Now()) {
		if ok {
			s.drop(ticket)
		}
		return false
	}
	s.drop(ticket)
	return true
}

// drop removes a ticket and takes one off its owner's outstanding count. The
// caller holds the mutex.
func (s *sseTicketStore) drop(ticket string) {
	delete(s.tickets, ticket)
	owner, ok := s.ownerByTicket[ticket]
	if !ok {
		return
	}
	delete(s.ownerByTicket, ticket)
	if n := s.byOwner[owner]; n > 1 {
		s.byOwner[owner] = n - 1
	} else {
		delete(s.byOwner, owner)
	}
}

// handleSSETicket exchanges a Bearer-authenticated request for a one-time
// ticket the EventSource can carry in its URL. POST only, auth required —
// the same gate as every other dashboard call, which is what makes the
// ticket's brevity safe.
func (ws *WebServer) handleSSETicket(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		httpx.WriteMethodNotAllowed(w, "POST")
		return
	}
	// The ticket is minted only for a caller already holding a valid session;
	// authGate with allowQueryToken=false keeps the query-string path out of
	// the exchange too.
	_ = r.Body.Close()
	// The cap is per session key: the Bearer token the handler was reached with
	// already proved it, and a dashboard never needs more than a couple of tabs.
	sessionKey := ""
	if v := r.Context().Value(sessionKeyContextKey); v != nil {
		if s, ok := v.(string); ok {
			sessionKey = s
		}
	}
	if sessionKey == "" {
		sessionKey = r.RemoteAddr
	}
	ticket := ws.sseTickets.mint(sessionKey)
	if ticket == "" {
		httpx.WriteJSONError(w, http.StatusTooManyRequests, "too many outstanding stream tickets; open fewer dashboard tabs or wait for them to expire")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ticket":     ticket,
		"expires_in": int(sseTicketTTL.Seconds()),
	})
}

// sseTicketAuthorized reports whether the ?ticket= on the request is a live,
// unconsumed one-time ticket.
func (ws *WebServer) sseTicketAuthorized(r *http.Request) bool {
	t := r.URL.Query().Get("ticket")
	return t != "" && ws.sseTickets.consume(t)
}
