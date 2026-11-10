package web

import (
	"crypto/x509"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"digitdns/internal/httpx"
	dnscryptlib "github.com/ameshkov/dnscrypt/v2"
)

// transportSettingsView is the encrypted-DNS card's state for the two
// client-facing QUIC transports. Both are opt-in and applied live; the view
// always carries the effective port and auth mode, including the defaults an
// older "dns" record does not have yet.
type transportSettingsView struct {
	DoQEnabled     bool     `json:"doq_enabled"`
	DoQPort        int      `json:"doq_port"`
	DoQAuth        string   `json:"doq_auth"`
	DoQTokenDomain string   `json:"doq_token_domain"`
	DoH3Enabled    bool     `json:"doh3_enabled"`
	DoQTokens      []string `json:"doq_tokens"`
	// DoHPort is the HTTPS/DoH/portal port. DoH3Port is the legacy alias kept
	// for older clients; both carry the same value.
	DoHPort  int `json:"doh_port"`
	DoH3Port int `json:"doh3_port"`
	// Plain DNS and DoH policy.
	PlainDNSEnabled   bool   `json:"plain_dns_enabled"`
	PlainDNSPort      int    `json:"plain_dns_port"`
	PlainDNSListening bool   `json:"plain_dns_listening"`
	DoHEnabled        bool   `json:"doh_enabled"`
	DoHListening      bool   `json:"doh_listening"`
	ServerHost        string `json:"server_host"`
	// DotHost is the name the DoT certificate is issued for. Android Private
	// DNS and TLS clients validate the certificate against the SNI, so DoT
	// advertises the domain rather than the public IP.
	DotHost string `json:"dot_host"`
	// DNSCrypt policy and read-only identity. The keys are never returned; the
	// stamp is the client-facing form of the provider public key.
	DNSCryptEnabled      bool   `json:"dnscrypt_enabled"`
	DNSCryptPort         int    `json:"dnscrypt_port"`
	DNSCryptProviderName string `json:"dnscrypt_provider_name"`
	DNSCryptStamp        string `json:"dnscrypt_stamp"`
	DNSCryptCertNotAfter int64  `json:"dnscrypt_cert_not_after"`
	DNSCryptListening    bool   `json:"dnscrypt_listening"`
	// DoT policy and the global mTLS gate.
	DoTEnabled     bool     `json:"dot_enabled"`
	DoTListening   bool     `json:"dot_listening"`
	DoTAuth        string   `json:"dot_auth"`
	DoTPort        int      `json:"dot_port"`
	DoTTokenDomain string   `json:"dot_token_domain"`
	DoTTokens      []string `json:"dot_tokens"`
	MTLSEnabled    bool     `json:"mtls_enabled"`
	MTLSCAPEM      string   `json:"mtls_ca_pem"`
	// Actual listener state. A saved toggle is the desired state; these say
	// whether the bind succeeded, so a failed start is visible instead of
	// looking like a running endpoint.
	DoH3Listening bool `json:"doh3_listening"`
	DoQListening  bool `json:"doq_listening"`
	// IPAllowlist explains where DoQ's "IP allowlist" mode gets its sources.
	// Both lists are shared with the resolver: per-client AllowedIPs and the
	// server-wide resolver allowlist. AllowAll is the live access mode.
	IPAllowlist ipAllowlistStatus `json:"ip_allowlist"`
	// LastError is the persistent reason from the last failed apply. Cleared on
	// the next successful save so the card can explain a bind/cert failure long
	// after the toast has gone.
	LastError string `json:"last_error"`
}

// ipAllowlistStatus is the read-only description of the IP gate behind DoQ. It
// carries counts rather than the addresses themselves: the settings card only
// needs to say where the entries come from and whether the gate is actually
// restricting access.
type ipAllowlistStatus struct {
	AllowAll        bool `json:"allow_all"`
	ClientAccounts  int  `json:"client_accounts"`
	ClientEntries   int  `json:"client_entries"`
	ResolverEntries int  `json:"resolver_entries"`
}

const defaultDoQPort = 853

// transportSettingsView reads the saved flags and the token list. The tokens
// live in the shared "access" record, so they survive a DoH-token save and
// every other read-modify-write of that record.
func (ws *WebServer) transportSettingsView() transportSettingsView {
	ws.dnsMu.Lock()
	cfg := ws.mutableDNSLocked()
	view := transportSettingsView{
		DoQEnabled:     cfg.DoQEnabled,
		DoQPort:        cfg.DoQPort,
		DoQAuth:        cfg.DoQAuth,
		DoQTokenDomain: cfg.DoQTokenDomain,
		// DoH3 shares the DoH listener's port and is always available over UDP;
		// the card shows it on and non-changeable.
		DoH3Enabled:          true,
		DoH3Port:             cfg.DoHPort,
		DNSCryptEnabled:      cfg.DNSCryptEnabled,
		DNSCryptPort:         cfg.DNSCryptPort,
		DNSCryptProviderName: cfg.DNSCryptProviderName,
		PlainDNSEnabled:      !cfg.PlainDNSDisabled,
		PlainDNSPort:         cfg.Port,
		DoHEnabled:           !cfg.DoHDisabled,
		DoHPort:              cfg.DoHPort,
		DoTEnabled:           !cfg.DoTDisabled,
		DoTAuth:              cfg.DoTAuth,
		DoTPort:              cfg.DoTPort,
		DoTTokenDomain:       cfg.DoTTokenDomain,
		DoTTokens:            append([]string(nil), cfg.DoTTokens...),
		MTLSEnabled:          cfg.MTLSEnabled,
		MTLSCAPEM:            cfg.MTLSCAPEM,
	}
	ws.dnsMu.Unlock()

	if view.DoQPort == 0 {
		view.DoQPort = defaultDoQPort
	}
	if view.PlainDNSPort == 0 {
		view.PlainDNSPort = 53
	}
	if view.DoHPort == 0 {
		view.DoHPort = 8443
	}
	view.DoH3Port = view.DoHPort
	if view.DNSCryptPort == 0 {
		view.DNSCryptPort = defaultDNSCryptPort
	}
	if view.DoTPort == 0 {
		view.DoTPort = 853
	}
	if view.DNSCryptProviderName == "" {
		view.DNSCryptProviderName = "2.dnscrypt-cert.digitdns.local"
	}
	// Prefer the public IP so every endpoint in the card is directly usable;
	// fall back to the DoT/panel domain only when no address is configured.
	host := ws.settings.GetPublicIP()
	if host == "" && ws.tlsSettings != nil {
		host = safeHostDisplay(ws.tlsSettings.GetDoTDomain())
		if host == "" {
			host = safeHostDisplay(ws.tlsSettings.GetDomain())
		}
	}
	view.ServerHost = host
	dotHost := ""
	if ws.tlsSettings != nil {
		dotHost = safeHostDisplay(ws.tlsSettings.GetDoTDomain())
		if dotHost == "" {
			dotHost = safeHostDisplay(ws.tlsSettings.GetDomain())
		}
	}
	if dotHost == "" {
		dotHost = host
	}
	view.DotHost = dotHost
	st := ws.loadDNSCryptSettings()
	view.DNSCryptStamp = ws.dnscryptStamp(st, view.DNSCryptPort)
	view.DNSCryptCertNotAfter = st.CertNotAfter
	view.DoQAuth = normalizeDoQAuth(view.DoQAuth)
	if view.DoTAuth == "" {
		view.DoTAuth = "ip"
	}
	if view.DoTTokens == nil {
		view.DoTTokens = []string{}
	}
	access := ws.loadAccessConfig()
	view.DoQTokens = access.DoQTokens
	if view.DoQTokens == nil {
		view.DoQTokens = []string{}
	}
	accounts, entries, resolverEntries, allowAll := ws.clients.AllowlistStats()
	view.IPAllowlist = ipAllowlistStatus{
		AllowAll:        allowAll,
		ClientAccounts:  accounts,
		ClientEntries:   entries,
		ResolverEntries: resolverEntries,
	}
	var listenerErr string
	if ws.transportStatus != nil {
		view.DoH3Listening, view.DoQListening, listenerErr = ws.transportStatus()
	}
	if ws.transportListeners != nil {
		listeners := ws.transportListeners()
		view.PlainDNSListening = listeners.PlainDNS
		view.DoTListening = listeners.DoT
		view.DoHListening = listeners.DoH
		view.DoH3Listening = listeners.DoH3
		view.DoQListening = listeners.DoQ
		view.DNSCryptListening = listeners.DNSCrypt
	}
	if ws.dnscryptStatus != nil {
		var dnscryptErr string
		view.DNSCryptListening, view.DNSCryptCertNotAfter, dnscryptErr = ws.dnscryptStatus()
		if dnscryptErr != "" {
			if listenerErr == "" {
				listenerErr = dnscryptErr
			} else {
				listenerErr += "; " + dnscryptErr
			}
		}
	}
	ws.transportErrMu.Lock()
	view.LastError = ws.transportLastError
	ws.transportErrMu.Unlock()
	// A save failure is the more specific explanation; otherwise surface the
	// asynchronous listener failure (a Serve goroutine that died after the save
	// returned, or a listener that failed at boot).
	if view.LastError == "" {
		view.LastError = listenerErr
	}
	return view
}

func (ws *WebServer) setTransportLastError(msg string) {
	ws.transportErrMu.Lock()
	ws.transportLastError = msg
	ws.transportErrMu.Unlock()
}

func normalizeDoQAuth(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "ip":
		return "ip"
	case "sni-token":
		return "sni-token"
	default:
		return ""
	}
}

// normalizeDoQTokens lower-cases, trims and de-duplicates token labels. A
// label is also a DNS label because it becomes the leftmost SNI label, so the
// ACME label rules apply unchanged.
func normalizeDoQTokens(raw []string) ([]string, string) {
	seen := map[string]bool{}
	out := make([]string, 0, len(raw))
	for _, entry := range raw {
		token := strings.ToLower(strings.TrimSpace(strings.TrimSuffix(entry, ".")))
		if token == "" {
			continue
		}
		if err := validACMELabel(token); err != nil {
			return nil, "invalid DoQ token " + strings.TrimSpace(entry) + ": " + err.Error()
		}
		if seen[token] {
			continue
		}
		seen[token] = true
		out = append(out, token)
	}
	if out == nil {
		out = []string{}
	}
	return out, ""
}

// handleTransportSettings reads or writes the DoQ/DoH3 policy. A save is
// persisted first, then handed to the running server; a bind failure is
// reported to the caller and leaves the listener off.
func (ws *WebServer) handleTransportSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet, http.MethodHead:
		ws.writeTransportSettings(w)
	case http.MethodPost:
		ws.saveTransportSettings(w, r)
	default:
		httpx.WriteMethodNotAllowed(w, "GET, HEAD, POST")
	}
}

func (ws *WebServer) writeTransportSettings(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(ws.transportSettingsView())
}

func (ws *WebServer) saveTransportSettings(w http.ResponseWriter, r *http.Request) {
	var req struct {
		DoQEnabled           *bool     `json:"doq_enabled"`
		DoQPort              *int      `json:"doq_port"`
		DoQAuth              *string   `json:"doq_auth"`
		DoQTokenDomain       *string   `json:"doq_token_domain"`
		DoH3Enabled          *bool     `json:"doh3_enabled"`
		DoQTokens            *[]string `json:"doq_tokens"`
		PlainDNSEnabled      *bool     `json:"plain_dns_enabled"`
		PlainDNSPort         *int      `json:"plain_dns_port"`
		DoHEnabled           *bool     `json:"doh_enabled"`
		DoHPort              *int      `json:"doh_port"`
		DoTPort              *int      `json:"dot_port"`
		DoTEnabled           *bool     `json:"dot_enabled"`
		DNSCryptEnabled      *bool     `json:"dnscrypt_enabled"`
		DNSCryptPort         *int      `json:"dnscrypt_port"`
		DNSCryptProviderName *string   `json:"dnscrypt_provider_name"`
		DoTAuth              *string   `json:"dot_auth"`
		DoTTokenDomain       *string   `json:"dot_token_domain"`
		DoTTokens            *[]string `json:"dot_tokens"`
		MTLSEnabled          *bool     `json:"mtls_enabled"`
		MTLSCAPEM            *string   `json:"mtls_ca_pem"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		httpx.WriteJSONError(w, http.StatusBadRequest, "Could not read the transport settings: "+err.Error())
		return
	}

	// A rapid switch toggle must not interleave two apply/persist transactions.
	ws.transportSaveMu.Lock()
	defer ws.transportSaveMu.Unlock()

	ws.dnsMu.Lock()
	cfg := ws.mutableDNSLocked()
	before := *cfg
	ws.dnsMu.Unlock()

	next := before

	plainEnabled := !before.PlainDNSDisabled
	if req.PlainDNSEnabled != nil {
		plainEnabled = *req.PlainDNSEnabled
	}
	plainPort := before.Port
	if plainPort == 0 {
		plainPort = 53
	}
	if req.PlainDNSPort != nil {
		plainPort = *req.PlainDNSPort
		if plainPort == 0 {
			plainPort = 53
		}
		if plainPort < 1 || plainPort > 65535 {
			httpx.WriteJSONError(w, http.StatusBadRequest, "plain_dns_port must be between 1 and 65535")
			return
		}
	}

	dohEnabled := !before.DoHDisabled
	if req.DoHEnabled != nil {
		dohEnabled = *req.DoHEnabled
	}
	dohPort := before.DoHPort
	if dohPort == 0 {
		dohPort = 8443
	}
	if req.DoHPort != nil {
		dohPort = *req.DoHPort
		if dohPort == 0 {
			dohPort = 8443
		}
		if dohPort < 1 || dohPort > 65535 {
			httpx.WriteJSONError(w, http.StatusBadRequest, "doh_port must be between 1 and 65535")
			return
		}
	}

	// DoH3 shares the DoH port and stays on; the switch is read-only in the
	// card. Ignore a client that still sends false.
	doh3Enabled := true

	dotPort := before.DoTPort
	if dotPort == 0 {
		dotPort = 853
	}
	if req.DoTPort != nil {
		dotPort = *req.DoTPort
		if dotPort == 0 {
			dotPort = 853
		}
		if dotPort < 1 || dotPort > 65535 {
			httpx.WriteJSONError(w, http.StatusBadRequest, "dot_port must be between 1 and 65535")
			return
		}
	}

	auth := normalizeDoQAuth(before.DoQAuth)
	if req.DoQAuth != nil {
		auth = normalizeDoQAuth(*req.DoQAuth)
		if auth == "" {
			httpx.WriteJSONError(w, http.StatusBadRequest, "doq_auth must be \"ip\" or \"sni-token\"")
			return
		}
	}
	port := before.DoQPort
	if port == 0 {
		port = defaultDoQPort
	}
	if req.DoQPort != nil {
		port = *req.DoQPort
		if port == 0 {
			port = defaultDoQPort
		}
		if port < 1 || port > 65535 {
			httpx.WriteJSONError(w, http.StatusBadRequest, "doq_port must be between 1 and 65535")
			return
		}
	}

	tokenDomain := strings.ToLower(strings.TrimSpace(before.DoQTokenDomain))
	if req.DoQTokenDomain != nil {
		tokenDomain = strings.ToLower(strings.TrimSpace(*req.DoQTokenDomain))
		if tokenDomain != "" {
			normalized, err := validACMEDomain(tokenDomain)
			if err != nil {
				httpx.WriteJSONError(w, http.StatusBadRequest, "doq_token_domain: "+err.Error())
				return
			}
			tokenDomain = normalized
		}
	}

	access := ws.loadAccessConfig()
	beforeTokens := access.DoQTokens
	tokens := beforeTokens
	if req.DoQTokens != nil {
		normalized, problem := normalizeDoQTokens(*req.DoQTokens)
		if problem != "" {
			httpx.WriteJSONError(w, http.StatusBadRequest, problem)
			return
		}
		tokens = normalized
	}
	doqEnabled := before.DoQEnabled
	if req.DoQEnabled != nil {
		doqEnabled = *req.DoQEnabled
	}
	if doqEnabled && auth == "sni-token" {
		if tokenDomain == "" {
			httpx.WriteJSONError(w, http.StatusBadRequest, "doq_token_domain is required when doq_auth is \"sni-token\"")
			return
		}
		if len(tokens) == 0 {
			httpx.WriteJSONError(w, http.StatusBadRequest, "at least one DoQ token is required when doq_auth is \"sni-token\"")
			return
		}
	}

	dnscryptEnabled := before.DNSCryptEnabled
	if req.DNSCryptEnabled != nil {
		dnscryptEnabled = *req.DNSCryptEnabled
	}
	dnscryptPort := before.DNSCryptPort
	if dnscryptPort == 0 {
		dnscryptPort = defaultDNSCryptPort
	}
	if req.DNSCryptPort != nil {
		dnscryptPort = *req.DNSCryptPort
		if dnscryptPort == 0 {
			dnscryptPort = defaultDNSCryptPort
		}
		if dnscryptPort < 1 || dnscryptPort > 65535 {
			httpx.WriteJSONError(w, http.StatusBadRequest, "dnscrypt_port must be between 1 and 65535")
			return
		}
	}
	dnscryptProvider := normalizeDNSCryptProviderName(before.DNSCryptProviderName)
	if req.DNSCryptProviderName != nil {
		suffix := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(*req.DNSCryptProviderName)), "2.dnscrypt-cert.")
		if suffix == "" {
			suffix = "digitdns.local"
		}
		if _, err := validACMEDomain(suffix); err != nil {
			httpx.WriteJSONError(w, http.StatusBadRequest, "dnscrypt_provider_name: "+err.Error())
			return
		}
		dnscryptProvider = "2.dnscrypt-cert." + suffix
	}

	dotEnabled := !before.DoTDisabled
	if req.DoTEnabled != nil {
		dotEnabled = *req.DoTEnabled
	}
	dotAuth := strings.ToLower(strings.TrimSpace(before.DoTAuth))
	if dotAuth == "" {
		dotAuth = "ip"
	}
	if req.DoTAuth != nil {
		dotAuth = strings.ToLower(strings.TrimSpace(*req.DoTAuth))
	}
	if dotAuth != "ip" && dotAuth != "sni-token" {
		httpx.WriteJSONError(w, http.StatusBadRequest, "dot_auth must be \"ip\" or \"sni-token\"")
		return
	}
	dotDomain := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(before.DoTTokenDomain), "."))
	if req.DoTTokenDomain != nil {
		dotDomain = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(*req.DoTTokenDomain), "."))
		if dotDomain != "" {
			normalized, err := validACMEDomain(dotDomain)
			if err != nil {
				httpx.WriteJSONError(w, http.StatusBadRequest, "dot_token_domain: "+err.Error())
				return
			}
			dotDomain = normalized
		}
	}
	dotTokens := append([]string(nil), before.DoTTokens...)
	if req.DoTTokens != nil {
		normalized, problem := normalizeDoTTokens(*req.DoTTokens)
		if problem != "" {
			httpx.WriteJSONError(w, http.StatusBadRequest, problem)
			return
		}
		dotTokens = normalized
	}
	if dotEnabled && dotAuth == "sni-token" {
		if dotDomain == "" {
			httpx.WriteJSONError(w, http.StatusBadRequest, "dot_token_domain is required when dot_auth is \"sni-token\"")
			return
		}
		if len(dotTokens) == 0 {
			httpx.WriteJSONError(w, http.StatusBadRequest, "at least one DoT token is required when dot_auth is \"sni-token\"")
			return
		}
	}
	mtlsEnabled := before.MTLSEnabled
	if req.MTLSEnabled != nil {
		mtlsEnabled = *req.MTLSEnabled
	}
	mtlsCA := strings.TrimSpace(before.MTLSCAPEM)
	if req.MTLSCAPEM != nil {
		mtlsCA = strings.TrimSpace(*req.MTLSCAPEM)
	}
	if mtlsEnabled {
		if mtlsCA == "" {
			httpx.WriteJSONError(w, http.StatusBadRequest, "mtls_ca_pem is required when mTLS is enabled")
			return
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM([]byte(mtlsCA)) {
			httpx.WriteJSONError(w, http.StatusBadRequest, "mtls_ca_pem contains no valid certificates")
			return
		}
	}

	if problem := transportPortConflict(
		plainEnabled, plainPort,
		dotEnabled, dotPort,
		dohPort,
		doh3Enabled,
		doqEnabled, port,
		dnscryptEnabled, dnscryptPort,
	); problem != "" {
		httpx.WriteJSONError(w, http.StatusBadRequest, problem)
		return
	}

	next.Port = plainPort
	next.PlainDNSDisabled = !plainEnabled
	next.DoHDisabled = !dohEnabled
	next.DoHPort = dohPort
	next.DoTPort = dotPort
	next.DoQEnabled = doqEnabled
	next.DoH3Enabled = doh3Enabled
	next.DoQPort = port
	next.DoQAuth = auth
	next.DoQTokenDomain = tokenDomain
	next.DNSCryptEnabled = dnscryptEnabled
	next.DNSCryptPort = dnscryptPort
	next.DNSCryptProviderName = dnscryptProvider
	next.DoTDisabled = !dotEnabled
	next.DoTAuth = dotAuth
	next.DoTTokenDomain = dotDomain
	next.DoTTokens = dotTokens
	next.MTLSEnabled = mtlsEnabled
	next.MTLSCAPEM = mtlsCA

	oldDNSCrypt := ws.loadDNSCryptSettings()
	oldRC, oldCert := ws.dnscryptRuntime()
	var newRC *dnscryptlib.ResolverConfig
	var newCert *dnscryptlib.Cert
	var err error
	keysChanged := false
	if dnscryptEnabled {
		if oldRC != nil && oldDNSCrypt.ProviderName == dnscryptProvider {
			// Same identity: reuse it. Re-signing a certificate and rewriting
			// the key record on every unrelated transport save would churn the
			// database and briefly stop a healthy listener.
			newRC, newCert = oldRC, oldCert
		} else {
			_, newRC, newCert, err = ws.ensureDNSCrypt(dnscryptProvider)
			if err != nil {
				httpx.WriteJSONError(w, http.StatusInternalServerError, "Could not prepare the DNSCrypt keys: "+err.Error())
				return
			}
			keysChanged = true
		}
	}
	dnscryptRunning := false
	if dnscryptEnabled && ws.dnscryptStatus != nil {
		dnscryptRunning, _, _ = ws.dnscryptStatus()
	}

	rollback := func(reason string) {
		ws.setTransportLastError(reason)
		log.Printf("[Transports] %s", reason)
		if ws.dnscryptApplier != nil {
			if oldRC != nil {
				_ = ws.dnscryptApplier(oldRC, oldCert)
			} else {
				_ = ws.dnscryptApplier(nil, nil)
			}
		}
		_ = ws.db.SetSetting("dnscrypt", oldDNSCrypt)
		if ws.transportApplier != nil {
			if err := ws.transportApplier(before, beforeTokens); err != nil {
				log.Printf("[Transports] rollback failed: %v", err)
			}
		}
	}

	if newRC != nil && ws.dnscryptApplier != nil && (!dnscryptRunning || keysChanged) {
		if err := ws.dnscryptApplier(newRC, newCert); err != nil {
			rollback("could not install the DNSCrypt keys: " + err.Error())
			httpx.WriteJSONError(w, http.StatusInternalServerError, "Could not install the DNSCrypt keys")
			return
		}
	}

	if ws.transportApplier != nil {
		if err := ws.transportApplier(next, tokens); err != nil {
			rollback("apply failed: " + err.Error())
			httpx.WriteJSONError(w, http.StatusInternalServerError, "The transport settings were not applied: "+err.Error())
			return
		}
	}

	if req.DoQTokens != nil {
		nextAccess := access
		nextAccess.DoQTokens = tokens
		if err := ws.db.SetSetting("access", nextAccess); err != nil {
			rollback("could not persist the DoQ tokens: " + err.Error())
			httpx.WriteJSONError(w, http.StatusInternalServerError, "Could not persist the DoQ tokens")
			return
		}
	}

	ws.dnsMu.Lock()
	current := ws.mutableDNSLocked()
	current.DoQEnabled = next.DoQEnabled
	current.DoH3Enabled = next.DoH3Enabled
	current.DoQPort = next.DoQPort
	current.DoQAuth = next.DoQAuth
	current.DoQTokenDomain = next.DoQTokenDomain
	current.DNSCryptEnabled = next.DNSCryptEnabled
	current.DNSCryptPort = next.DNSCryptPort
	current.DNSCryptProviderName = next.DNSCryptProviderName
	current.Port = next.Port
	current.PlainDNSDisabled = next.PlainDNSDisabled
	current.DoHDisabled = next.DoHDisabled
	current.DoHPort = next.DoHPort
	current.DoTPort = next.DoTPort
	current.DoTDisabled = next.DoTDisabled
	current.DoTAuth = next.DoTAuth
	current.DoTTokenDomain = next.DoTTokenDomain
	current.DoTTokens = next.DoTTokens
	current.MTLSEnabled = next.MTLSEnabled
	current.MTLSCAPEM = next.MTLSCAPEM
	err = ws.db.SetSetting("dns", current)
	ws.dnsMu.Unlock()
	if err != nil {
		ws.dnsMu.Lock()
		current.DoQEnabled = before.DoQEnabled
		current.DoH3Enabled = before.DoH3Enabled
		current.DoQPort = before.DoQPort
		current.DoQAuth = before.DoQAuth
		current.DoQTokenDomain = before.DoQTokenDomain
		current.DNSCryptEnabled = before.DNSCryptEnabled
		current.DNSCryptPort = before.DNSCryptPort
		current.DNSCryptProviderName = before.DNSCryptProviderName
		current.Port = before.Port
		current.PlainDNSDisabled = before.PlainDNSDisabled
		current.DoHDisabled = before.DoHDisabled
		current.DoHPort = before.DoHPort
		current.DoTPort = before.DoTPort
		current.DoTDisabled = before.DoTDisabled
		current.DoTAuth = before.DoTAuth
		current.DoTTokenDomain = before.DoTTokenDomain
		current.DoTTokens = before.DoTTokens
		current.MTLSEnabled = before.MTLSEnabled
		current.MTLSCAPEM = before.MTLSCAPEM
		ws.dnsMu.Unlock()
		if req.DoQTokens != nil {
			_ = ws.db.SetSetting("access", access)
		}
		rollback("could not persist the transport settings: " + err.Error())
		httpx.WriteJSONError(w, http.StatusInternalServerError, "Could not persist the transport settings")
		return
	}

	ws.setTransportLastError("")
	ws.writeTransportSettings(w)
}

func normalizeDoTTokens(raw []string) ([]string, string) {
	seen := map[string]bool{}
	out := make([]string, 0, len(raw))
	for _, entry := range raw {
		token := strings.ToLower(strings.TrimSpace(strings.TrimSuffix(entry, ".")))
		if token == "" {
			continue
		}
		if err := validACMELabel(token); err != nil {
			return nil, "invalid DoT token " + strings.TrimSpace(entry) + ": " + err.Error()
		}
		if seen[token] {
			continue
		}
		seen[token] = true
		out = append(out, token)
	}
	if out == nil {
		out = []string{}
	}
	return out, ""
}

// transportPortConflict catches two enabled listeners that would bind the same
// protocol and port before the save reaches the DNS server. The DoH port is
// always reserved for TCP because the HTTPS listener also serves the subscriber
// portal, even when the DoH endpoint itself is switched off.
func transportPortConflict(
	plainEnabled bool, plainPort int,
	dotEnabled bool, dotPort int,
	dohPort int,
	doh3Enabled bool,
	doqEnabled bool, doqPort int,
	dnscryptEnabled bool, dnscryptPort int,
) string {
	used := map[string]string{}
	claim := func(proto string, port int, name string) string {
		key := proto + ":" + strconv.Itoa(port)
		if other, ok := used[key]; ok {
			return fmt.Sprintf("%s and %s both need %s port %d", other, name, proto, port)
		}
		used[key] = name
		return ""
	}
	if plainEnabled {
		if p := claim("tcp", plainPort, "Plain DNS"); p != "" {
			return p
		}
		if p := claim("udp", plainPort, "Plain DNS"); p != "" {
			return p
		}
	}
	if dotEnabled {
		if p := claim("tcp", dotPort, "DoT"); p != "" {
			return p
		}
	}
	if p := claim("tcp", dohPort, "DoH/portal"); p != "" {
		return p
	}
	if doh3Enabled {
		if p := claim("udp", dohPort, "DoH3"); p != "" {
			return p
		}
	}
	if doqEnabled {
		if p := claim("udp", doqPort, "DoQ"); p != "" {
			return p
		}
	}
	if dnscryptEnabled {
		if p := claim("udp", dnscryptPort, "DNSCrypt"); p != "" {
			return p
		}
		if p := claim("tcp", dnscryptPort, "DNSCrypt"); p != "" {
			return p
		}
	}
	return ""
}
