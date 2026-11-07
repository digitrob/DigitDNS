package web

import (
	"context"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// agentConfigNotifier is a level-triggered broadcast used by the agent
// catalogue long-poll. A config change closes the current channel and replaces
// it, waking every waiter; each waiter then re-checks the revision and either
// returns the new catalogue or goes back to sleep.
type agentConfigNotifier struct {
	mu sync.Mutex
	ch chan struct{}
}

func newAgentConfigNotifier() *agentConfigNotifier {
	return &agentConfigNotifier{ch: make(chan struct{})}
}

func (n *agentConfigNotifier) notify() {
	if n == nil {
		return
	}
	n.mu.Lock()
	close(n.ch)
	n.ch = make(chan struct{})
	n.mu.Unlock()
}

// wait blocks until a notification, the timeout, or the request context ends.
// It reports true only for a notification.
func (n *agentConfigNotifier) wait(ctx context.Context, d time.Duration) bool {
	if n == nil {
		select {
		case <-time.After(d):
			return false
		case <-ctx.Done():
			return false
		}
	}
	n.mu.Lock()
	ch := n.ch
	n.mu.Unlock()

	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ch:
		return true
	case <-timer.C:
		return false
	case <-ctx.Done():
		return false
	}
}

// agentRevision is the ETag for the current control-plane configuration. It is
// global: a change to any agent, rule or client list wakes every long-poll, and
// each agent recomputes only if its own revision no longer matches.
func (ws *WebServer) agentRevision() string {
	// Entity tags must be quoted (RFC 9110 §8.8.3). The value is opaque to the
	// agent; only equality matters.
	return `"v` + strconv.FormatUint(ws.agentRev.Load(), 10) + `"`
}

// agentConfigChanged bumps the revision and wakes every waiting agent. It is
// called from the mutation points that can change a catalogue: rules, client
// allowlists, agent details and the global IPv6 policy.
func (ws *WebServer) agentConfigChanged() {
	ws.agentRev.Add(1)
	ws.agentNotify.notify()
}

// agentMutation wraps a panel handler that can change the client allowlist. A
// successful (or even failed) non-GET is allowed to bump the control-plane
// revision; the cost of an unnecessary wake is one cheap 304 check per agent.
func (ws *WebServer) agentMutation(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		next(w, r)
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			return
		default:
			ws.agentConfigChanged()
		}
	}
}
