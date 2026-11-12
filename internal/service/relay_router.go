package service

import (
	"net"
	"strings"
	"sync"
	"time"

	"digitdns/internal/database"
	"digitdns/internal/netutil"
)

// RelayTarget is the egress an assigned domain should use. IPv4 and IPv6 are
// resolved separately because an agent may publish only one family; a missing
// family is deliberately left empty so the DNS handler can sink that query
// instead of falling back to the hub's address.
type RelayTarget struct {
	// Kind is "local", "agent" or "egress". A local target has no addresses
	// here; the DNS handler fills the hub's public IP.
	Kind string
	// Agent fields are populated for Kind == "agent".
	AgentID string
	Name    string
	Country string
	IPv4    net.IP
	IPv6    net.IP
	// Egress fields are populated for Kind == "egress".
	EgressID   string
	EgressName string
	EgressKind string
	FailClosed bool
	Healthy    bool
	// FallbackReason explains a local fallback or a fail-closed refusal.
	FallbackReason string
}

// RelayRouter holds the live geo-agent assignment map. It is read on the DNS
// hot path and replaced wholesale whenever the rules or the agent list change,
// so a query never takes a database lock.
type RelayRouter struct {
	mu sync.RWMutex

	agents map[string]database.Agent
	// agentIPv4 and agentIPv6 hold the parsed, canonical addresses for each
	// agent. They are built with the agent snapshot so the query path never
	// re-parses the same literal. An agent may publish one family, both, or
	// neither; a missing family is sunk by the DNS handler.
	agentIPv4      map[string]net.IP
	agentIPv6      map[string]net.IP
	domainAgents   assignmentIndex
	serviceAgents  map[string]string
	domainServices assignmentIndex
	egresses       *EgressRegistry

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

const (
	// AgentOnlineFor is how recently an agent must have heartbeated to be used
	// in DNS. Three 30-second beats plus jitter: long enough to ride out one
	// delayed beat, short enough that a dead agent stops receiving new clients
	// before a two-minute outage.
	AgentOnlineFor = 90 * time.Second
)

func (r *RelayRouter) SetEgressRegistry(e *EgressRegistry) {
	r.mu.Lock()
	r.egresses = e
	r.mu.Unlock()
}

func NewRelayRouter() *RelayRouter {
	return &RelayRouter{
		agents:         map[string]database.Agent{},
		agentIPv4:      map[string]net.IP{},
		agentIPv6:      map[string]net.IP{},
		domainAgents:   newAssignmentIndex(),
		serviceAgents:  map[string]string{},
		domainServices: newAssignmentIndex(),
		onlineFor:      AgentOnlineFor,
		now:            time.Now,
	}
}

// SetAgents replaces the agent snapshot. Callers refresh it after enrolment,
// heartbeat, enable/disable and delete.
func (r *RelayRouter) SetAgents(agents []database.Agent) {
	next := make(map[string]database.Agent, len(agents))
	ipv4 := make(map[string]net.IP, len(agents))
	ipv6 := make(map[string]net.IP, len(agents))
	for _, a := range agents {
		next[a.ID] = a
		if ip := net.ParseIP(strings.TrimSpace(a.PublicIP)).To4(); ip != nil {
			ipv4[a.ID] = ip
		}
		if a.IPv6Enabled {
			if ip, ok := netutil.ParsePublicIPv6(a.PublicIPv6); ok {
				ipv6[a.ID] = ip
			}
		}
	}
	r.mu.Lock()
	r.agents = next
	r.agentIPv4 = ipv4
	r.agentIPv6 = ipv6
	r.mu.Unlock()
}

// UpdateAgent replaces one agent's live snapshot without rebuilding the whole
// map. The heartbeat path calls it once per beat; a full SetAgents reload is
// reserved for enrolment, deletion and enable/disable.
func (r *RelayRouter) UpdateAgent(a database.Agent) {
	var ipv4, ipv6 net.IP
	if ip := net.ParseIP(strings.TrimSpace(a.PublicIP)).To4(); ip != nil {
		ipv4 = ip
	}
	if a.IPv6Enabled {
		if ip, ok := netutil.ParsePublicIPv6(a.PublicIPv6); ok {
			ipv6 = ip
		}
	}
	r.mu.Lock()
	r.agents[a.ID] = a
	if ipv4 != nil {
		r.agentIPv4[a.ID] = ipv4
	} else {
		delete(r.agentIPv4, a.ID)
	}
	if ipv6 != nil {
		r.agentIPv6[a.ID] = ipv6
	} else {
		delete(r.agentIPv6, a.ID)
	}
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

// LookupFor resolves a domain to the egress that should carry it. It is the
// DNS/proxy entry point: unlike Lookup it always returns a target, using
// Kind "local" when no assignment applies. A deleted or disabled egress falls
// back to local unless it is fail-closed, in which case the caller must fail
// closed rather than leak the origin.
func (r *RelayRouter) LookupFor(domain, clientIP string) (RelayTarget, bool) {
	d := normalizeRelayDomain(domain)
	if d == "" {
		return RelayTarget{Kind: "local", Healthy: true}, true
	}
	r.mu.RLock()
	targetID := r.domainAgents.lookup(d)
	if targetID == "__inherit__" {
		targetID = ""
	}
	if targetID == "" {
		if serviceID := r.domainServices.lookup(d); serviceID != "" {
			targetID = r.serviceAgents[serviceID]
		}
	}
	agent, hasAgent := r.agents[targetID]
	ipv4 := r.agentIPv4[targetID]
	ipv6 := r.agentIPv6[targetID]
	egresses := r.egresses
	r.mu.RUnlock()

	if strings.HasPrefix(targetID, "egress:") {
		id := strings.TrimPrefix(targetID, "egress:")
		if egresses == nil {
			return RelayTarget{Kind: "local", Healthy: true}, true
		}
		view, ok := egresses.Lookup(id)
		if !ok {
			return RelayTarget{Kind: "local", Healthy: true}, true
		}
		if !view.Enabled {
			// A disabled egress is still an explicit routing policy. Fail closed
			// rather than silently leaking the origin through the hub when the
			// operator asked for no fallback.
			if view.FailClosed {
				return RelayTarget{
					Kind: "egress", EgressID: id, EgressName: view.Name,
					EgressKind: view.Kind, Country: view.Country, FailClosed: true, Healthy: false,
					FallbackReason: "egress disabled (fail-closed)",
				}, true
			}
			return RelayTarget{Kind: "local", Healthy: true, FallbackReason: "egress disabled"}, true
		}
		if view.Health != "online" {
			if view.FailClosed {
				return RelayTarget{
					Kind: "egress", EgressID: id, EgressName: view.Name,
					EgressKind: view.Kind, Country: view.Country, FailClosed: true, Healthy: false,
					FallbackReason: "egress offline (fail-closed)",
				}, true
			}
			return RelayTarget{Kind: "local", Healthy: true, FallbackReason: "egress offline"}, true
		}
		return RelayTarget{
			Kind: "egress", EgressID: id, EgressName: view.Name,
			EgressKind: view.Kind, Country: view.Country, Healthy: true,
		}, true
	}

	if !hasAgent || !r.healthy(agent) {
		return RelayTarget{Kind: "local", Healthy: true}, true
	}
	return RelayTarget{
		Kind: "agent", AgentID: agent.ID, Name: agent.Name, Country: agent.Country,
		IPv4: ipv4, IPv6: ipv6, Healthy: true,
	}, true
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
	ipv6 := r.agentIPv6[agentID]
	r.mu.RUnlock()

	if !ok || !r.healthy(agent) {
		return RelayTarget{}, false
	}
	target := RelayTarget{
		AgentID: agent.ID,
		Name:    agent.Name,
		Country: agent.Country,
		IPv4:    ipv4,
		IPv6:    ipv6,
	}
	// An agent with no usable family at all is not a target. A healthy
	// IPv6-only agent is: its A queries fail closed while its AAAA queries work.
	if len(target.IPv4) == 0 && len(target.IPv6) == 0 {
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
