package service

import (
	"net"
	"strings"
	"sync"
	"time"

	"digitdns/internal/database"
)

// RelayTarget is the egress an assigned domain should use. IPv4 and IPv6 are
// resolved separately because an agent may publish only one family; a missing
// family is deliberately left empty so the DNS handler can sink that query
// instead of falling back to the hub's address.
type RelayTarget struct {
	AgentID string
	Name    string
	Country string
	IPv4    string
}

// RelayRouter holds the live geo-agent assignment map. It is read on the DNS
// hot path and replaced wholesale whenever the rules or the agent list change,
// so a query never takes a database lock.
type RelayRouter struct {
	mu sync.RWMutex

	agents         map[string]database.Agent
	domainAgents   map[string]string
	serviceAgents  map[string]string
	domainServices map[string]string

	onlineFor time.Duration
	now       func() time.Time
}

func NewRelayRouter() *RelayRouter {
	return &RelayRouter{
		agents:         map[string]database.Agent{},
		domainAgents:   map[string]string{},
		serviceAgents:  map[string]string{},
		domainServices: map[string]string{},
		onlineFor:      2 * time.Minute,
		now:            time.Now,
	}
}

// SetAgents replaces the agent snapshot. Callers refresh it after enrolment,
// heartbeat, enable/disable and delete.
func (r *RelayRouter) SetAgents(agents []database.Agent) {
	next := make(map[string]database.Agent, len(agents))
	for _, a := range agents {
		next[a.ID] = a
	}
	r.mu.Lock()
	r.agents = next
	r.mu.Unlock()
}

// SetAssignments replaces the domain and service maps that decide where a query
// exits. A nil or empty map is a valid "everything local" state.
func (r *RelayRouter) SetAssignments(domainAgents, serviceAgents, domainServices map[string]string) {
	copyMap := func(in map[string]string) map[string]string {
		out := make(map[string]string, len(in))
		for k, v := range in {
			k = normalizeRelayDomain(k)
			v = strings.TrimSpace(v)
			if k != "" && v != "" {
				out[k] = v
			}
		}
		return out
	}
	r.mu.Lock()
	r.domainAgents = copyMap(domainAgents)
	r.serviceAgents = copyMap(serviceAgents)
	r.domainServices = copyMap(domainServices)
	r.mu.Unlock()
}

// Reload rebuilds the snapshot from the database. It is used once at startup;
// afterwards the web layer pushes updates as it saves them.
func (r *RelayRouter) Reload(db *database.DB) error {
	agents, err := db.ListAgents()
	if err != nil {
		return err
	}
	policies, err := db.ListPolicies()
	if err != nil {
		return err
	}
	domainAgents := map[string]string{}
	serviceAgents := map[string]string{}
	domainServices := map[string]string{}
	for _, p := range policies {
		var target map[string]string
		switch p.Key {
		case "domain_agents":
			target = domainAgents
		case "service_agents":
			target = serviceAgents
		case "domain_services":
			target = domainServices
		default:
			continue
		}
		for _, entry := range p.CustomDomains {
			k, v, ok := strings.Cut(entry, "=")
			if !ok {
				continue
			}
			k, v = strings.TrimSpace(k), strings.TrimSpace(v)
			if k != "" && v != "" {
				target[k] = v
			}
		}
	}
	r.SetAgents(agents)
	r.SetAssignments(domainAgents, serviceAgents, domainServices)
	return nil
}

// Lookup returns the agent that should answer a proxied domain. A domain-level
// assignment wins over a service default; a plain pattern also covers its
// subdomains, matching the rule matcher's plain-rule behaviour.
func (r *RelayRouter) Lookup(domain string) (RelayTarget, bool) {
	d := normalizeRelayDomain(domain)
	if d == "" {
		return RelayTarget{}, false
	}

	r.mu.RLock()
	agentID := matchAssignment(r.domainAgents, d)
	// "__inherit__" is a UI convenience written only while clearing a
	// per-domain override; it means "use the service default".
	if agentID == "__inherit__" {
		agentID = ""
	}
	if agentID == "" {
		if serviceID := matchAssignment(r.domainServices, d); serviceID != "" {
			agentID = r.serviceAgents[serviceID]
		}
	}
	agent, ok := r.agents[agentID]
	r.mu.RUnlock()

	if !ok || !r.healthy(agent) {
		return RelayTarget{}, false
	}
	target := RelayTarget{AgentID: agent.ID, Name: agent.Name, Country: agent.Country}
	if ip := net.ParseIP(strings.TrimSpace(agent.PublicIP)); ip != nil && ip.To4() != nil {
		target.IPv4 = ip.String()
	}
	if target.IPv4 == "" {
		return RelayTarget{}, false
	}
	return target, true
}

func (r *RelayRouter) healthy(a database.Agent) bool {
	if !a.Enabled || a.LastSeen.IsZero() {
		return false
	}
	return r.now().Sub(a.LastSeen) <= r.onlineFor
}

// matchAssignment returns the most specific matching pattern's value. Patterns
// are stored without the trailing dot; "*.example.com" matches subdomains only,
// while "example.com" matches the host and its subdomains, exactly like the
// matcher's plain-rule index.
func matchAssignment(assignments map[string]string, domain string) string {
	best, bestScore := "", -1
	for pattern, value := range assignments {
		if value == "" {
			continue
		}
		matches, score := assignmentMatch(pattern, domain)
		if !matches || score <= bestScore {
			continue
		}
		best, bestScore = value, score
	}
	return best
}

func assignmentMatch(pattern, domain string) (bool, int) {
	p := normalizeRelayDomain(pattern)
	if p == "" {
		return false, 0
	}
	if strings.HasPrefix(p, "*.") {
		base := strings.TrimPrefix(p, "*.")
		if base != "" && strings.HasSuffix(domain, "."+base) {
			return true, len(base) + 1
		}
		return false, 0
	}
	if domain == p {
		return true, len(p) + 1000
	}
	if strings.HasSuffix(domain, "."+p) {
		return true, len(p)
	}
	return false, 0
}

func normalizeRelayDomain(d string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(d)), ".")
}
