package dns

import (
	"testing"
	"time"

	"digitdns/internal/core/cache"
	"digitdns/internal/core/matcher"
	"digitdns/internal/core/upstream"
	"digitdns/internal/database"
	"github.com/miekg/dns"
)

type countingAccess struct {
	client *database.Client
	ids    []string
}

func (a *countingAccess) IsIPAllowed(string) (*database.Client, bool) {
	return a.client, true
}

func (a *countingAccess) IsAllowAll() bool { return false }

func (a *countingAccess) RecordClientQuery(id string) { a.ids = append(a.ids, id) }

// TestProcessQueryCountsRecognisedClient: the resolver reports every lookup it
// answers for an account, before quota and rule handling, and reports nothing for
// sources that do not resolve to a client.
func TestProcessQueryCountsRecognisedClient(t *testing.T) {
	c := cache.NewCache(100, 60, 3600)
	defer c.Close()
	m := matcher.NewMatcher()
	m.SetCustomRules(nil, []string{"counted.test"}, nil, nil)
	u := upstream.NewUpstreamPool([]string{"1.1.1.1:53"}, time.Second, true, "")

	client := &database.Client{ID: "client-1", Name: "Query Counter", Enabled: true}
	access := &countingAccess{client: client}
	handler := NewHandler(access, c, m, u, nil, "198.51.100.1")

	req := new(dns.Msg)
	req.SetQuestion("counted.test.", dns.TypeA)
	handler.ProcessQuery(req, "203.0.113.9")

	if len(access.ids) != 1 || access.ids[0] != "client-1" {
		t.Fatalf("counted queries = %v, want [client-1]", access.ids)
	}

	// An allowlisted/public source has no account, so its lookup is not counted.
	access.client = nil
	handler.ProcessQuery(req, "203.0.113.10")
	if len(access.ids) != 1 {
		t.Fatalf("counted queries after an unidentified source = %v, want one entry", access.ids)
	}
}
