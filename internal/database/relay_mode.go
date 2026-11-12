package database

import "time"

// RelayModeSettings lets this full DigitDNS instance act as a relay agent for
// another DigitDNS hub. It is separate from Agent: the agent record lives on the
// hub, while this record holds the hub URL, the credential and the last
// catalogue this instance synced.
//
// AgentKey is a write-only secret. It is stored inside the encrypted settings
// envelope and never returned by the API.
type RelayModeSettings struct {
	Enabled  bool   `json:"enabled"`
	HubURL   string `json:"hub_url"`
	AgentKey string `json:"agent_key,omitempty"`
	AgentID  string `json:"agent_id,omitempty"`
	Country  string `json:"country,omitempty"`

	// Cached catalogue. A hub outage must not turn the relay off: the last
	// approved hosts/clients keep working until a sync succeeds.
	Hosts         []string  `json:"hosts,omitempty"`
	Clients       []string  `json:"clients,omitempty"`
	IPv6Enabled   bool      `json:"ipv6_enabled"`
	CatalogueETag string    `json:"catalogue_etag,omitempty"`
	LastSync      time.Time `json:"last_sync,omitempty"`
	LastError     string    `json:"last_error,omitempty"`
}
