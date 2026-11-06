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

	agents map[string]database.Agent
	// agentIPv4 holds the parsed, canonical IPv4 for each agent. It is built with
	// the agent snapshot so the query path never re-parses the same literal.
	agentIPv4      map[string]string
	domainAgents   assignmentIndex
	serviceAgents  map[string]string
	domainServices assignmentIndex

	onlineFor time.Duration
	now       func() time.Time
}

// assignmentIndex is a compiled relay assignment map. A plain pattern covers the
// host and its subdomains; a "*.base" pattern covers strict subdomains only. The
// old lookup scanned every entry and normalised each pattern on every query; this
// turns it into one map hit plus a label walk.
type assignmentIndex struct {
	plain    map[string]string
	wildcard map[string]string
}

func newAssignmentIndex() assignmentIndex {
	return assignmentIndex{
		plain:    make(map[string]string),
		wildcard: make(map[string]string),
	}
}

func compileAssignments(in map[string]string) assignmentIndex {
	idx := newAssignmentIndex()
	for pattern, value := range in {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		p := normalizeRelayDomain(pattern)
		if p == "" {
			continue
		}
		if strings.HasPrefix(p, "*.") {
			base := strings.TrimPrefix(p, "*.")
			if base != "" {
				idx.wildcard[base] = value
			}
			continue
		}
		idx.plain[p] = value
	}
	return idx
}

// lookup returns the most specific assignment for domain. A plain host match
// wins; otherwise the label walk finds the longest wildcard, and at one parent a
// wildcard is more specific than a plain suffix.
func (idx *assignmentIndex) lookup(domain string) string {
	if domain == "" {
		return ""
	}
	if value, ok := idx.plain[domain]; ok {
		return value
	}
	if len(idx.plain) == 0 && len(idx.wildcard) == 0 {
		return ""
	}
	for i := 0; i < len(domain); {
		dot := strings.IndexByte(domain[i:], '.')
		if dot < 0 {
			break
		}
		i += dot + 1
		if i >= len(domain) {
			break
		}
		parent := domain[i:]
		if value, ok := idx.wildcard[parent]; ok {
			return value
		}
		if value, ok := idx.plain[parent]; ok {
			return value
		}
	}
	return ""
}

func NewRelayRouter() *RelayRouter {
	return &RelayRouter{
		agents:         map[string]database.Agent{},
		agentIPv4:      map[string]string{},
		domainAgents:   newAssignmentIndex(),
		serviceAgents:  map[string]string{},
		domainServices: newAssignmentIndex(),
		onlineFor:      2 * time.Minute,
		now:            time.Now,
	}
}

// SetAgents replaces the agent snapshot. Callers refresh it after enrolment,
// heartbeat, enable/disable and delete.
func (r *RelayRouter) SetAgents(agents []database.Agent) {
	next := make(map[string]database.Agent, len(agents))
	ipv4 := make(map[string]string, len(agents))
	for _, a := range agents {
		next[a.ID] = a
		if ip := net.ParseIP(strings.TrimSpace(a.PublicIP)); ip != nil && ip.To4() != nil {
			ipv4[a.ID] = ip.String()
		}
	}
	r.mu.Lock()
	r.agents = next
	r.agentIPv4 = ipv4
	r.mu.Unlock()
}

// SetAssignments replaces the domain and service maps that decide where a query
// exits. A nil or empty map is a valid "everything local" state.
func (r *RelayRouter) SetAssignments(domainAgents, serviceAgents, domainServices map[string]string) {
	serviceCopy := make(map[string]string, len(serviceAgents))
	for k, v := range serviceAgents {
		k = normalizeRelayDomain(k)
		v = strings.TrimSpace(v)
		if k != "" && v != "" {
			serviceCopy[k] = v
		}
	}
	domainIndex := compileAssignments(domainAgents)
	serviceIndex := compileAssignments(domainServices)
	r.mu.Lock()
	r.domainAgents = domainIndex
	r.serviceAgents = serviceCopy
	r.domainServices = serviceIndex
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
	agentID := r.domainAgents.lookup(d)
	// "__inherit__" is a UI convenience written only while clearing a
	// per-domain override; it means "use the service default".
	if agentID == "__inherit__" {
		agentID = ""
	}
	if agentID == "" {
		if serviceID := r.domainServices.lookup(d); serviceID != "" {
			agentID = r.serviceAgents[serviceID]
		}
	}
	agent, ok := r.agents[agentID]
	ipv4 := r.agentIPv4[agentID]
	r.mu.RUnlock()

	if !ok || !r.healthy(agent) {
		return RelayTarget{}, false
	}
	target := RelayTarget{AgentID: agent.ID, Name: agent.Name, Country: agent.Country, IPv4: ipv4}
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

func normalizeRelayDomain(d string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(d)), ".")
}
