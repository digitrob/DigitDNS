package database

import (
	"crypto/rand"
	"encoding/hex"
	"sort"
	"strings"
	"sync"
	"time"
)

// Egress kinds. The values are persisted and also used by the UI and API.
const (
	EgressKindSOCKS5    = "socks5"
	EgressKindWireGuard = "wireguard"
)

// Egress is one outbound path the hub relay can dial through. It is deliberately
// separate from Agent: an agent is a public relay the client connects to, while
// an egress is only where the hub's relay dials out. The client still connects
// to the hub for SOCKS5 and WireGuard egress.
type Egress struct {
	ID         string `json:"id"`
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	Country    string `json:"country"`
	Enabled    bool   `json:"enabled"`
	FailClosed bool   `json:"fail_closed"`

	// SOCKS5.
	Address  string `json:"address,omitempty"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	// Resolve is "remote" (the SOCKS5 proxy resolves the name) or "local"
	// (send the hub-vetted address). Empty reads as remote.
	Resolve string `json:"resolve,omitempty"`
	// TestHost is the optional host:port the Test button dials through the
	// egress. Empty uses example.com:443.
	TestHost string `json:"test_host,omitempty"`

	// WireGuard. Config is the pasted .conf. It contains a private key and is
	// therefore encrypted with the rest of the settings record at rest.
	Config string `json:"config,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// EgressSettings is the persisted collection. One settings record keeps the
// secrets inside the same encrypted envelope as the rest of the database.
type EgressSettings struct {
	Items []Egress `json:"items"`
}

// egressWriteMu serialises read-modify-write operations on the single egress
// settings record. bbolt serialises writes, but two concurrent list/save
// operations would still lose one update without this.
var egressWriteMu sync.Mutex

// ListEgresses returns every configured egress, sorted by kind and name.
func (db *DB) ListEgresses() ([]Egress, error) {
	var settings EgressSettings
	if err := db.GetSetting("egresses", &settings); err != nil {
		return nil, err
	}
	out := append([]Egress(nil), settings.Items...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

// GetEgress returns one egress by ID.
func (db *DB) GetEgress(id string) (Egress, bool, error) {
	items, err := db.ListEgresses()
	if err != nil {
		return Egress{}, false, err
	}
	for _, item := range items {
		if item.ID == id {
			return item, true, nil
		}
	}
	return Egress{}, false, nil
}

// SaveEgress inserts or replaces one egress and persists the collection. It
// writes the generated ID and timestamps back through next, so a caller can
// return the saved record without a second lookup.
func (db *DB) SaveEgress(next *Egress) error {
	if next == nil {
		return nil
	}
	egressWriteMu.Lock()
	defer egressWriteMu.Unlock()

	var settings EgressSettings
	if err := db.GetSetting("egresses", &settings); err != nil {
		return err
	}
	now := time.Now().UTC()
	if next.ID == "" {
		next.ID = newEgressID()
		next.CreatedAt = now
	}
	next.UpdatedAt = now
	replaced := false
	for i := range settings.Items {
		if settings.Items[i].ID == next.ID {
			if next.CreatedAt.IsZero() {
				next.CreatedAt = settings.Items[i].CreatedAt
			}
			settings.Items[i] = *next
			replaced = true
			break
		}
	}
	if !replaced {
		settings.Items = append(settings.Items, *next)
	}
	return db.SetSetting("egresses", settings)
}

// DeleteEgress removes one egress. It is a no-op for an unknown ID.
func (db *DB) DeleteEgress(id string) error {
	egressWriteMu.Lock()
	defer egressWriteMu.Unlock()

	var settings EgressSettings
	if err := db.GetSetting("egresses", &settings); err != nil {
		return err
	}
	out := settings.Items[:0]
	for _, item := range settings.Items {
		if item.ID != id {
			out = append(out, item)
		}
	}
	settings.Items = out
	return db.SetSetting("egresses", settings)
}

func newEgressID() string {
	raw := make([]byte, 8)
	if _, err := rand.Read(raw); err != nil {
		return "egress-" + hex.EncodeToString([]byte(time.Now().UTC().Format("20060102150405")))
	}
	return "egress-" + hex.EncodeToString(raw)
}
