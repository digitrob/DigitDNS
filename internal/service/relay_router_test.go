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
