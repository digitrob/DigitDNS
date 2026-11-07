package database

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"digitdns/internal/crypto"
)

func newAgentTestDB(t *testing.T) *DB {
	t.Helper()
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 1)
	}
	c, err := crypto.NewCipher(key)
	if err != nil {
		t.Fatalf("cipher: %v", err)
	}
	db, err := Open(filepath.Join(t.TempDir(), "agents.db"), c)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestAgentEnrolmentRoundTrip(t *testing.T) {
	db := newAgentTestDB(t)

	token, _, err := db.CreateEnrolmentToken("DE", "DE-1")
	if err != nil {
		t.Fatalf("create token: %v", err)
	}

	agent, secret, err := db.EnrolAgent(token, "91.0.0.1", "1.0.0")
	if err != nil {
		t.Fatalf("enrol: %v", err)
	}
	if agent.Country != "DE" || !agent.Enabled {
		t.Fatalf("unexpected agent: %+v", agent)
	}
	if secret == "" {
		t.Fatal("enrol must return a key")
	}

	// The key authenticates, and a wrong one does not.
	if _, err := db.AuthenticateAgent(agent.ID, secret); err != nil {
		t.Fatalf("authenticate with the issued key: %v", err)
	}
	if _, err := db.AuthenticateAgent(agent.ID, secret+"x"); !errors.Is(err, ErrAgentUnauthorized) {
		t.Fatalf("wrong key should be unauthorized, got %v", err)
	}
}

func TestEnrolmentTokenIsSingleUse(t *testing.T) {
	db := newAgentTestDB(t)

	token, _, err := db.CreateEnrolmentToken("DE", "")
	if err != nil {
		t.Fatalf("create token: %v", err)
	}
	if _, _, err := db.EnrolAgent(token, "91.0.0.1", "1.0.0"); err != nil {
		t.Fatalf("first enrol: %v", err)
	}
	if _, _, err := db.EnrolAgent(token, "91.0.0.2", "1.0.0"); !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("a spent token must be rejected, got %v", err)
	}
}

func TestEnrolmentTokenUnknownRejected(t *testing.T) {
	db := newAgentTestDB(t)
	if _, _, err := db.EnrolAgent("agt_nope", "91.0.0.1", "1.0.0"); !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("unknown token must be rejected, got %v", err)
	}
}

func TestCreateAgentAndRotateKey(t *testing.T) {
	db := newAgentTestDB(t)
	agent, key, err := db.CreateAgent("de", "Frankfurt")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if agent.ID == "" || agent.Country != "DE" || agent.Name != "Frankfurt" || key == "" {
		t.Fatalf("created agent = %+v key %q", agent, key)
	}
	_, secret, ok := strings.Cut(key, ":")
	if !ok {
		t.Fatalf("created key has no id:secret shape: %q", key)
	}
	if _, err := db.AuthenticateAgent(agent.ID, secret); err != nil {
		t.Fatalf("created key does not authenticate: %v", err)
	}
	storedKey, err := db.AgentKey(agent.ID)
	if err != nil || storedKey != key {
		t.Fatalf("AgentKey = %q, %v, want %q", storedKey, err, key)
	}
	rotated, err := db.RotateAgentKey(agent.ID)
	if err != nil {
		t.Fatalf("rotate: %v", err)
	}
	if rotated == key {
		t.Fatal("rotation returned the same key")
	}
	if _, err := db.AuthenticateAgent(agent.ID, secret); err == nil {
		t.Fatal("the old key still authenticates")
	}
	_, rotatedSecret, _ := strings.Cut(rotated, ":")
	if _, err := db.AuthenticateAgent(agent.ID, rotatedSecret); err != nil {
		t.Fatalf("the new key does not authenticate: %v", err)
	}
	storedKey, err = db.AgentKey(agent.ID)
	if err != nil || storedKey != rotated {
		t.Fatalf("AgentKey after rotate = %q, %v, want %q", storedKey, err, rotated)
	}
}

func TestAgentListTouchDisableDelete(t *testing.T) {
	db := newAgentTestDB(t)

	token, _, _ := db.CreateEnrolmentToken("DE", "DE-1")
	agent, secret, err := db.EnrolAgent(token, "91.0.0.1", "1.0.0")
	if err != nil {
		t.Fatalf("enrol: %v", err)
	}

	if err := db.TouchAgent(agent.ID, "1.0.1", "91.0.0.1", "2001:db8::5", 443, 100, 200, ""); err != nil {
		t.Fatalf("touch: %v", err)
	}
	got, err := db.GetAgent(agent.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Version != "1.0.1" || got.BytesIn != 100 || got.BytesOut != 200 || got.RelayPort != 443 {
		t.Fatalf("touch not applied: %+v", got)
	}
	if got.PublicIPv6 != "2001:db8::5" || got.IPv6Enabled {
		t.Fatalf("touch IPv6 fields = %q/%v, want the address and the conservative disabled opt-in",
			got.PublicIPv6, got.IPv6Enabled)
	}
	if err := db.RecordAgentPing(agent.ID, 12.5); err != nil {
		t.Fatalf("record ping: %v", err)
	}
	got, _ = db.GetAgent(agent.ID)
	if got.PingMs != 12.5 || got.PingAt.IsZero() {
		t.Fatalf("ping not recorded: %+v", got)
	}

	// EnabledAgentForCountry is what the DNS layer will call.
	if _, err := db.EnabledAgentForCountry("DE"); err != nil {
		t.Fatalf("enabled agent should be found: %v", err)
	}
	if err := db.SetAgentEnabled(agent.ID, false); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if _, err := db.EnabledAgentForCountry("DE"); !errors.Is(err, ErrAgentNotFound) {
		t.Fatalf("disabled agent must not be selected, got %v", err)
	}

	if err := db.DeleteAgent(agent.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := db.AuthenticateAgent(agent.ID, secret); !errors.Is(err, ErrAgentUnauthorized) {
		t.Fatalf("a deleted agent's key must stop working, got %v", err)
	}
}

func TestListAgentsHidesSecretHashes(t *testing.T) {
	db := newAgentTestDB(t)

	token, _, _ := db.CreateEnrolmentToken("DE", "DE-1")
	if _, _, err := db.EnrolAgent(token, "91.0.0.1", "1.0.0"); err != nil {
		t.Fatalf("enrol: %v", err)
	}

	agents, err := db.ListAgents()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(agents) != 1 {
		t.Fatalf("expected one agent, got %d - the key hash is leaking into the list", len(agents))
	}
}
