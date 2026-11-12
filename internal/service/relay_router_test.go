package service

import (
	"testing"
	"time"

	"digitdns/internal/database"
)

func relayAgent(id, name, country, ip string) database.Agent {
	return database.Agent{
		ID:        id,
		Name:      name,
		Country:   country,
		PublicIP:  ip,
		Enabled:   true,
		CreatedAt: time.Now(),
		LastSeen:  time.Now(),
	}
}

func relayAgentIPv6(id, name, country, ipv4, ipv6 string, ipv6Enabled bool) database.Agent {
	return database.Agent{
		ID:          id,
		Name:        name,
		Country:     country,
		PublicIP:    ipv4,
		PublicIPv6:  ipv6,
		IPv6Enabled: ipv6Enabled,
		Enabled:     true,
		CreatedAt:   time.Now(),
		LastSeen:    time.Now(),
	}
}

func TestRelayRouterIPv6OnlyAgentIsSelectable(t *testing.T) {
	r := NewRelayRouter()
	r.SetAgents([]database.Agent{
		relayAgentIPv6("v6", "IPv6 Only", "DE", "", "2001:db8::10", true),
	})
	r.SetAssignments(map[string]string{"example.com": "v6"}, nil, nil)

	target, ok := r.Lookup("example.com")
	if !ok {
		t.Fatal("an IPv6-only agent should be selectable")
	}
	if target.IPv4 != nil || target.IPv6.String() != "2001:db8::10" {
		t.Fatalf("target = %+v, want only the IPv6 address", target)
	}
}

func TestRelayRouterIgnoresIPv6WhenAgentOptedOut(t *testing.T) {
	r := NewRelayRouter()
	r.SetAgents([]database.Agent{
		relayAgentIPv6("v4", "IPv4 Only", "DE", "203.0.113.10", "2001:db8::10", false),
	})
	r.SetAssignments(map[string]string{"example.com": "v4"}, nil, nil)

	target, ok := r.Lookup("example.com")
	if !ok {
		t.Fatal("the IPv4 address should keep the agent selectable")
	}
	if target.IPv4.String() != "203.0.113.10" || target.IPv6 != nil {
		t.Fatalf("target = %+v, want IPv4 with IPv6 suppressed", target)
	}
}

func TestRelayRouterMatchesAssignedDomain(t *testing.T) {
	r := NewRelayRouter()
	r.SetAgents([]database.Agent{
		relayAgent("de", "Frankfurt", "DE", "203.0.113.10"),
		relayAgent("us", "New York", "US", "198.51.100.20"),
	})
	r.SetAssignments(map[string]string{
		"*.example.com":       "de",
		"*.media.example.com": "us",
	}, nil, nil)

	if target, ok := r.Lookup("live.example.com"); !ok || target.AgentID != "de" {
		t.Fatalf("live.example.com = %#v, %v", target, ok)
	}
	if target, ok := r.Lookup("cdn.media.example.com"); !ok || target.AgentID != "us" {
		t.Fatalf("longest wildcard = %#v, %v", target, ok)
	}
}

func TestRelayRouterServiceDefaultApplies(t *testing.T) {
	r := NewRelayRouter()
	r.SetAgents([]database.Agent{
		relayAgent("de", "Frankfurt", "DE", "203.0.113.10"),
		relayAgent("us", "New York", "US", "198.51.100.20"),
	})
	r.SetAssignments(
		nil,
		map[string]string{"tv": "de"},
		map[string]string{"service.example": "tv"},
	)
	if target, ok := r.Lookup("service.example"); !ok || target.AgentID != "de" {
		t.Fatalf("service default = %#v, %v", target, ok)
	}

	// A per-domain local override beats the service default.
	r.SetAssignments(
		map[string]string{"service.example": "__local__"},
		map[string]string{"tv": "de"},
		map[string]string{"service.example": "tv"},
	)
	if target, ok := r.Lookup("service.example"); ok {
		t.Fatalf("local override still routed through %#v", target)
	}

	// A per-domain agent overrides the service default too.
	r.SetAssignments(
		map[string]string{"service.example": "us"},
		map[string]string{"tv": "de"},
		map[string]string{"service.example": "tv"},
	)
	if target, ok := r.Lookup("service.example"); !ok || target.AgentID != "us" {
		t.Fatalf("direct override = %#v, %v", target, ok)
	}
}

func TestRelayRouterPlainPatternCoversSubdomains(t *testing.T) {
	r := NewRelayRouter()
	r.SetAgents([]database.Agent{relayAgent("de", "Frankfurt", "DE", "203.0.113.10")})
	r.SetAssignments(map[string]string{"example.com": "de"}, nil, nil)

	for _, domain := range []string{"example.com", "www.example.com", "a.b.example.com"} {
		if target, ok := r.Lookup(domain); !ok || target.AgentID != "de" {
			t.Errorf("%s = %#v, %v", domain, target, ok)
		}
	}
	if _, ok := r.Lookup("notexample.com"); ok {
		t.Error("a suffix match crossed the label boundary")
	}
	if _, ok := r.Lookup("unassigned.example.net"); ok {
		t.Error("an unassigned domain should use the local relay")
	}
}

func TestRelayRouterWildcardBeatsPlainParentAtTheSameLevel(t *testing.T) {
	r := NewRelayRouter()
	r.SetAgents([]database.Agent{
		relayAgent("de", "Frankfurt", "DE", "203.0.113.10"),
		relayAgent("us", "New York", "US", "198.51.100.20"),
	})
	r.SetAssignments(map[string]string{
		"example.com":         "de",
		"*.media.example.com": "us",
	}, nil, nil)

	if target, ok := r.Lookup("cdn.media.example.com"); !ok || target.AgentID != "us" {
		t.Fatalf("wildcard child = %#v, %v", target, ok)
	}
	if target, ok := r.Lookup("www.example.com"); !ok || target.AgentID != "de" {
		t.Fatalf("plain parent suffix = %#v, %v", target, ok)
	}
}

func TestRelayRouterSkipsOfflineAndDisabledAgents(t *testing.T) {
	now := time.Now()
	r := NewRelayRouter()
	r.now = func() time.Time { return now }
	old := relayAgent("old", "Old", "DE", "203.0.113.10")
	old.LastSeen = now.Add(-5 * time.Minute)
	disabled := relayAgent("off", "Off", "DE", "203.0.113.11")
	disabled.Enabled = false
	r.SetAgents([]database.Agent{old, disabled})
	r.SetAssignments(map[string]string{"example.com": "old", "other.example": "off"}, nil, nil)

	if _, ok := r.Lookup("example.com"); ok {
		t.Error("an agent last seen five minutes ago was considered healthy")
	}
	if _, ok := r.Lookup("other.example"); ok {
		t.Error("a disabled agent was considered healthy")
	}
}

func TestRelayRouterClientDefaultEgressApplies(t *testing.T) {
	r := NewRelayRouter()
	r.SetAgents([]database.Agent{
		relayAgent("de", "Frankfurt", "DE", "203.0.113.10"),
		relayAgent("us", "New York", "US", "198.51.100.20"),
	})
	r.SetClientEgressLookup(func(ip string) string {
		if ip == "192.0.2.10" {
			return "de"
		}
		return ""
	})

	target, ok := r.LookupFor("unassigned.example", "192.0.2.10")
	if !ok || target.Kind != "agent" || target.AgentID != "de" {
		t.Fatalf("client default = %#v, %v; want the DE agent", target, ok)
	}

	if target, _ := r.LookupFor("unassigned.example", "192.0.2.11"); target.Kind != "local" {
		t.Fatalf("unknown client = %#v; want local", target)
	}
}

func TestRelayRouterDomainAndServiceBeatClientDefault(t *testing.T) {
	r := NewRelayRouter()
	r.SetAgents([]database.Agent{
		relayAgent("de", "Frankfurt", "DE", "203.0.113.10"),
		relayAgent("us", "New York", "US", "198.51.100.20"),
	})
	r.SetClientEgressLookup(func(string) string { return "de" })
	r.SetAssignments(
		map[string]string{"override.example": "us"},
		map[string]string{"tv": "us"},
		map[string]string{"service.example": "tv"},
	)

	if target, _ := r.LookupFor("override.example", "192.0.2.10"); target.AgentID != "us" {
		t.Fatalf("domain override = %#v; want US", target)
	}
	if target, _ := r.LookupFor("service.example", "192.0.2.10"); target.AgentID != "us" {
		t.Fatalf("service default = %#v; want US", target)
	}
	if target, _ := r.LookupFor("plain.example", "192.0.2.10"); target.AgentID != "de" {
		t.Fatalf("client fallback = %#v; want DE", target)
	}
}
