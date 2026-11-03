package database

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"sort"
	"strings"
	"time"

	bolt "go.etcd.io/bbolt"
)

// Agent is a geo relay: a box on a public IP in the country it exits as. It is
// policy data, not credential data — its key hash lives beside the record in the
// same bucket and an encrypted copy is kept under key/<id> for the authenticated
// copy action — so it lives as plain JSON for the same reason custom policy
// groups do.
type Agent struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Country   string    `json:"country"`
	PublicIP  string    `json:"public_ip"`
	RelayPort int       `json:"relay_port,omitempty"`
	Version   string    `json:"version"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	LastSeen  time.Time `json:"last_seen"`
	PingMs    float64   `json:"ping_ms,omitempty"`
	PingAt    time.Time `json:"ping_at,omitempty"`
	BytesIn   uint64    `json:"bytes_in"`
	BytesOut  uint64    `json:"bytes_out"`

	// secretHash is the SHA-256 of the key. Stored separately from the JSON so a
	// list response can never carry it. The encrypted clear copy under key/<id>
	// exists only for the authenticated copy action.
	secretHash []byte `json:"-"`
}

// HostBytes is one row of an agent's per-host traffic report.
type HostBytes struct {
	Host  string `json:"host"`
	Bytes uint64 `json:"bytes"`
}

// AgentEnrolmentToken is a one-time credential for adding an agent. It is bound
// to the country the operator declared, so a token issued for Germany cannot
// enrol a box that reports itself elsewhere once geo lookup is wired in.
type AgentEnrolmentToken struct {
	Hash      string    `json:"hash"`
	Country   string    `json:"country"`
	Name      string    `json:"name"`
	ExpiresAt time.Time `json:"expires_at"`
	UsedAt    time.Time `json:"used_at"`
}

// EnrolmentTokenTTL is deliberately short: the token is pasted into a shell on a
// fresh box within minutes, and a credential that mints agents should not be
// useful tomorrow.
const EnrolmentTokenTTL = 15 * time.Minute

var (
	// ErrAgentNotFound is returned when an ID names no stored agent.
	ErrAgentNotFound = errors.New("agent not found")
	// ErrTokenInvalid covers expired, unknown and already-used tokens with one
	// answer, so a caller cannot use the error to probe which tokens existed.
	ErrTokenInvalid = errors.New("enrolment token is invalid or has expired")
	// ErrAgentUnauthorized is returned when a key does not match an agent.
	ErrAgentUnauthorized = errors.New("agent key is not valid")
	// ErrAgentKeyUnavailable is returned for an agent enrolled before the panel
	// started keeping an encrypted copy of the clear key.
	ErrAgentKeyUnavailable = errors.New("agent key is not available; rotate it to store a new one")
)

// CreateEnrolmentToken mints a one-time token for a country and returns it in
// clear. The clear value is never stored and cannot be recovered.
func (db *DB) CreateEnrolmentToken(country, name string) (string, time.Time, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", time.Time{}, err
	}
	token := "agt_" + hex.EncodeToString(raw)
	expires := time.Now().Add(EnrolmentTokenTTL)

	record := AgentEnrolmentToken{
		Hash:      hex.EncodeToString(hashSecret(token)),
		Country:   country,
		Name:      name,
		ExpiresAt: expires,
	}
	encoded, err := json.Marshal(record)
	if err != nil {
		return "", time.Time{}, err
	}

	err = db.bolt.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists(bucketAgentTokens)
		if err != nil {
			return err
		}
		return b.Put([]byte(record.Hash), encoded)
	})
	if err != nil {
		return "", time.Time{}, err
	}
	return token, expires, nil
}

// EnrolAgent consumes a token and creates the agent record. It returns the
// agent's key in clear exactly once; only its hash is stored.
func (db *DB) EnrolAgent(token, publicIP, version string) (Agent, string, error) {
	var (
		agent  Agent
		secret string
	)

	hash := hex.EncodeToString(hashSecret(token))
	err := db.bolt.Update(func(tx *bolt.Tx) error {
		tokens := tx.Bucket(bucketAgentTokens)
		if tokens == nil {
			return ErrTokenInvalid
		}
		raw := tokens.Get([]byte(hash))
		if raw == nil {
			return ErrTokenInvalid
		}

		var record AgentEnrolmentToken
		if err := json.Unmarshal(raw, &record); err != nil {
			return ErrTokenInvalid
		}
		if !record.UsedAt.IsZero() || time.Now().After(record.ExpiresAt) {
			return ErrTokenInvalid
		}

		rawSecret := make([]byte, 24)
		if _, err := rand.Read(rawSecret); err != nil {
			return err
		}
		secret = "agk_" + hex.EncodeToString(rawSecret)

		id, err := randomID()
		if err != nil {
			return err
		}
		agent = Agent{
			ID:         id,
			Name:       record.Name,
			Country:    record.Country,
			Version:    version,
			Enabled:    true,
			CreatedAt:  time.Now().UTC(),
			RelayPort:  443,
			secretHash: hashSecret(secret),
		}
		if ip := net.ParseIP(strings.TrimSpace(publicIP)); ip != nil && ip.To4() != nil {
			agent.PublicIP = ip.String()
		}
		if agent.Name == "" {
			agent.Name = record.Country + "-" + id[:4]
		}

		// Mark the token spent before writing the agent, so a crash between the
		// two leaves a burned token rather than a reusable one.
		record.UsedAt = time.Now().UTC()
		encodedRecord, err := json.Marshal(record)
		if err != nil {
			return err
		}
		if err := tokens.Put([]byte(record.Hash), encodedRecord); err != nil {
			return err
		}

		agents, err := tx.CreateBucketIfNotExists(bucketAgents)
		if err != nil {
			return err
		}
		encodedAgent, err := json.Marshal(agent)
		if err != nil {
			return err
		}
		if err := agents.Put([]byte(agent.ID), encodedAgent); err != nil {
			return err
		}
		if err := agents.Put([]byte(secretKeyPrefix+agent.ID), agent.secretHash); err != nil {
			return err
		}
		return db.putAgentKey(tx, agent.ID, agent.ID+":"+secret)
	})
	if err != nil {
		return Agent{}, "", err
	}
	return agent, secret, nil
}

// CreateAgent makes the record when the operator adds it, so the panel can show
// the key once and the agent can start with it. The public address is learned
// from the first heartbeat; until then the agent is offline and the DNS router
// will not select it.
func (db *DB) CreateAgent(country, name string) (Agent, string, error) {
	country = strings.ToUpper(strings.TrimSpace(country))
	if country == "" {
		return Agent{}, "", ErrAgentNotFound
	}
	id, err := randomID()
	if err != nil {
		return Agent{}, "", err
	}
	secret, err := newAgentSecret()
	if err != nil {
		return Agent{}, "", err
	}
	agent := Agent{
		ID:         id,
		Name:       strings.TrimSpace(name),
		Country:    country,
		Enabled:    true,
		CreatedAt:  time.Now().UTC(),
		RelayPort:  443,
		secretHash: hashSecret(secret),
	}
	if agent.Name == "" {
		agent.Name = country + "-" + id[:4]
	}
	encoded, err := json.Marshal(agent)
	if err != nil {
		return Agent{}, "", err
	}
	err = db.bolt.Update(func(tx *bolt.Tx) error {
		agents, err := tx.CreateBucketIfNotExists(bucketAgents)
		if err != nil {
			return err
		}
		if err := agents.Put([]byte(agent.ID), encoded); err != nil {
			return err
		}
		if err := agents.Put([]byte(secretKeyPrefix+agent.ID), agent.secretHash); err != nil {
			return err
		}
		return db.putAgentKey(tx, agent.ID, agent.ID+":"+secret)
	})
	if err != nil {
		return Agent{}, "", err
	}
	return agent, id + ":" + secret, nil
}

// RotateAgentKey replaces the stored hash and returns the one new key. The old
// key stops working immediately, which is the point of rotating a credential.
func (db *DB) RotateAgentKey(id string) (string, error) {
	secret, err := newAgentSecret()
	if err != nil {
		return "", err
	}
	key := id + ":" + secret
	err = db.bolt.Update(func(tx *bolt.Tx) error {
		agents := tx.Bucket(bucketAgents)
		if agents == nil || agents.Get([]byte(id)) == nil {
			return ErrAgentNotFound
		}
		if err := agents.Put([]byte(secretKeyPrefix+id), hashSecret(secret)); err != nil {
			return err
		}
		return db.putAgentKey(tx, id, key)
	})
	if err != nil {
		return "", err
	}
	return key, nil
}

func newAgentSecret() (string, error) {
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return "agk_" + hex.EncodeToString(raw), nil
}

// AuthenticateAgent checks a key against a stored agent and returns the record.
func (db *DB) AuthenticateAgent(id, secret string) (Agent, error) {
	var agent Agent
	err := db.bolt.View(func(tx *bolt.Tx) error {
		agents := tx.Bucket(bucketAgents)
		if agents == nil {
			return ErrAgentUnauthorized
		}
		raw := agents.Get([]byte(id))
		if raw == nil {
			return ErrAgentUnauthorized
		}
		if err := json.Unmarshal(raw, &agent); err != nil {
			return ErrAgentUnauthorized
		}
		stored := agents.Get([]byte(secretKeyPrefix + id))
		if stored == nil || subtle.ConstantTimeCompare(stored, hashSecret(secret)) != 1 {
			return ErrAgentUnauthorized
		}
		return nil
	})
	if err != nil {
		return Agent{}, err
	}
	return agent, nil
}

// TouchAgent records a heartbeat: version, last-seen and traffic totals.
func (db *DB) TouchAgent(id, version, publicIP string, relayPort int, bytesIn, bytesOut uint64) error {
	return db.updateAgent(id, func(a *Agent) {
		a.Version = version
		if ip := net.ParseIP(strings.TrimSpace(publicIP)); ip != nil && ip.To4() != nil {
			a.PublicIP = ip.String()
		}
		if relayPort > 0 {
			a.RelayPort = relayPort
		}
		a.LastSeen = time.Now().UTC()
		a.BytesIn = bytesIn
		a.BytesOut = bytesOut
	})
}

// RecordAgentPing stores the result of the operator-triggered connection test.
func (db *DB) RecordAgentPing(id string, ms float64) error {
	return db.updateAgent(id, func(a *Agent) {
		a.PingMs = ms
		a.PingAt = time.Now().UTC()
	})
}

// SetAgentEnabled flips an agent in or out of DNS answers.
func (db *DB) SetAgentEnabled(id string, enabled bool) error {
	return db.updateAgent(id, func(a *Agent) { a.Enabled = enabled })
}

// UpdateAgentDetails changes the operator-editable fields. The key is untouched;
// rotating it is a separate action so an edit form cannot silently invalidate a
// running agent.
func (db *DB) UpdateAgentDetails(id, name, country, publicIP string) error {
	name = strings.TrimSpace(name)
	country = strings.ToUpper(strings.TrimSpace(country))
	publicIP = strings.TrimSpace(publicIP)
	if country == "" {
		return ErrAgentNotFound
	}
	return db.updateAgent(id, func(a *Agent) {
		if name != "" {
			a.Name = name
		}
		a.Country = country
		a.PublicIP = ""
		if ip := net.ParseIP(publicIP); ip != nil && ip.To4() != nil {
			a.PublicIP = ip.String()
		}
	})
}

// DeleteAgent removes an agent and its key.
func (db *DB) DeleteAgent(id string) error {
	return db.bolt.Update(func(tx *bolt.Tx) error {
		agents := tx.Bucket(bucketAgents)
		if agents == nil {
			return ErrAgentNotFound
		}
		if agents.Get([]byte(id)) == nil {
			return ErrAgentNotFound
		}
		if err := agents.Delete([]byte(secretKeyPrefix + id)); err != nil {
			return err
		}
		if err := agents.Delete([]byte(agentKeyPrefix + id)); err != nil {
			return err
		}
		return agents.Delete([]byte(id))
	})
}

// ListAgents returns every stored agent, ordered by country then name so the
// page is stable across reloads.
func (db *DB) ListAgents() ([]Agent, error) {
	var out []Agent
	err := db.bolt.View(func(tx *bolt.Tx) error {
		agents := tx.Bucket(bucketAgents)
		if agents == nil {
			return nil
		}
		return agents.ForEach(func(k, v []byte) error {
			if hasPrefix(k, secretKeyPrefix) || hasPrefix(k, agentKeyPrefix) {
				return nil
			}
			var a Agent
			if err := json.Unmarshal(v, &a); err != nil {
				return nil
			}
			out = append(out, a)
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Country != out[j].Country {
			return out[i].Country < out[j].Country
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// GetAgent returns one agent by ID.
func (db *DB) GetAgent(id string) (Agent, error) {
	var agent Agent
	err := db.bolt.View(func(tx *bolt.Tx) error {
		agents := tx.Bucket(bucketAgents)
		if agents == nil {
			return ErrAgentNotFound
		}
		raw := agents.Get([]byte(id))
		if raw == nil {
			return ErrAgentNotFound
		}
		return json.Unmarshal(raw, &agent)
	})
	return agent, err
}

// EnabledAgentForCountry returns the first enabled agent in a country, or
// ErrAgentNotFound. First-match is deliberate for v1: with more than one agent
// per country this becomes a weighted choice, and pretending otherwise now would
// hide the day that decision has to be made.
func (db *DB) EnabledAgentForCountry(country string) (Agent, error) {
	agents, err := db.ListAgents()
	if err != nil {
		return Agent{}, err
	}
	for _, a := range agents {
		if a.Enabled && a.Country == country {
			return a, nil
		}
	}
	return Agent{}, ErrAgentNotFound
}

// secretKeyPrefix separates key hashes from agent records inside one bucket, so
// listing agents does not have to know about a second bucket layout.
const secretKeyPrefix = "secret/"

// agentKeyPrefix holds an encrypted copy of the clear key. It exists only so an
// authenticated operator can copy the key again after the one-time reveal; it is
// never part of a list response and is encrypted with the database cipher.
const agentKeyPrefix = "key/"

func hasPrefix(k []byte, prefix string) bool {
	return len(k) > len(prefix) && string(k[:len(prefix)]) == prefix
}

func (db *DB) putAgentKey(tx *bolt.Tx, id, key string) error {
	if db.cipher == nil {
		return nil
	}
	sealed, err := db.cipher.EncryptString(key)
	if err != nil {
		return err
	}
	agents := tx.Bucket(bucketAgents)
	if agents == nil {
		return ErrAgentNotFound
	}
	return agents.Put([]byte(agentKeyPrefix+id), []byte(sealed))
}

// AgentKey returns the encrypted clear key for an agent, for the authenticated
// copy action. Legacy agents enrolled before this field existed have no copy and
// must be rotated once.
func (db *DB) AgentKey(id string) (string, error) {
	var sealed string
	err := db.bolt.View(func(tx *bolt.Tx) error {
		agents := tx.Bucket(bucketAgents)
		if agents == nil {
			return ErrAgentNotFound
		}
		raw := agents.Get([]byte(agentKeyPrefix + id))
		if raw == nil {
			return ErrAgentKeyUnavailable
		}
		sealed = string(raw)
		return nil
	})
	if err != nil {
		return "", err
	}
	if db.cipher == nil {
		return "", ErrAgentKeyUnavailable
	}
	return db.cipher.DecryptString(sealed)
}

func (db *DB) updateAgent(id string, mutate func(*Agent)) error {
	return db.bolt.Update(func(tx *bolt.Tx) error {
		agents := tx.Bucket(bucketAgents)
		if agents == nil {
			return ErrAgentNotFound
		}
		raw := agents.Get([]byte(id))
		if raw == nil {
			return ErrAgentNotFound
		}
		var agent Agent
		if err := json.Unmarshal(raw, &agent); err != nil {
			return err
		}
		mutate(&agent)
		encoded, err := json.Marshal(agent)
		if err != nil {
			return err
		}
		return agents.Put([]byte(id), encoded)
	})
}

func hashSecret(secret string) []byte {
	sum := sha256.Sum256([]byte(secret))
	return sum[:]
}

func randomID() (string, error) {
	raw := make([]byte, 8)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}
