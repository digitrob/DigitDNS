package web

import (
	"context"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"digitdns/internal/database"
	"digitdns/internal/version"
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

// loadAgentRevision reads the persisted control-plane counter. A missing record
// reads as zero, which is the correct starting point for a fresh install.
func loadAgentRevision(db *database.DB) uint64 {
	if db == nil {
		return 0
	}
	var rev uint64
	_ = db.GetSetting("agent_revision", &rev)
	return rev
}

// agentRevision is the ETag for the current control-plane configuration. It is
// global: a change to any agent, rule or client list wakes every long-poll, and
// each agent recomputes only if its own revision no longer matches.
//
// The build hash is part of the tag so an upgrade invalidates every cached
// catalogue even though the persisted counter carries over. A plain restart on
// the same build keeps the same tag, which is what prevents the restart herd.
func (ws *WebServer) agentRevision() string {
	// Entity tags must be quoted (RFC 9110 §8.8.3). The value is opaque to the
	// agent; only equality matters.
	tag := `"v` + strconv.FormatUint(ws.agentRev.Load(), 10)
	if build := version.Get().Hash; build != "" {
		tag += "-" + build
	}
	return tag + `"`
}

// agentConfigChanged bumps the revision and wakes every waiting agent. It is
// called from the mutation points that can change a catalogue: rules, client
// allowlists, agent details and the global IPv6 policy.
func (ws *WebServer) agentConfigChanged() {
	rev := ws.agentRev.Add(1)
	// Persist synchronously so a restart cannot reuse a counter an agent has
	// already matched. Every caller is a panel mutation that has finished its
	// own database write; this must not be called from inside a bbolt write
	// transaction, or the second write would deadlock.
	if ws.db != nil {
		if err := ws.db.SetSetting("agent_revision", rev); err != nil {
			log.Printf("[Web] Could not persist the agent revision: %v", err)
		}
	}
	ws.agentNotify.notify()
}

// agentCatalogueInputs is the per-revision snapshot shared by every agent's
// catalogue. The rules and the panel client list do not vary by agent; only the
// assignment walk does.
type agentCatalogueInputs struct {
	rev     uint64
	rules   map[string]any
	clients []string
}

// catalogueInputs returns the shared inputs for the current revision, reading
// them from the database only when the revision has moved.
func (ws *WebServer) catalogueInputs() (map[string]any, []string, error) {
	rev := ws.agentRev.Load()
	ws.agentInputsMu.Lock()
	defer ws.agentInputsMu.Unlock()
	if c := ws.agentInputs; c != nil && c.rev == rev {
		return c.rules, c.clients, nil
	}
	rules := ws.loadRulesConfig()
	clients, err := ws.panelClientIPs()
	if err != nil {
		return nil, nil, err
	}
	ws.agentInputs = &agentCatalogueInputs{rev: rev, rules: rules, clients: clients}
	return rules, clients, nil
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
