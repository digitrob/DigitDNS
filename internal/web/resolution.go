package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"digitdns/internal/core/resolution"
	"digitdns/internal/core/upstream"
	"digitdns/internal/httpx"
)

// resolutionView is what the dashboard's resolution settings card and the
// per-domain dialog read: the global transport choice, the saved profiles, and
// the upstream list so the dialog can offer them without a second request.
//
// EncryptedAvailable and EncryptedUpstreams are computed here rather than in
// each client: the transport picker disables its encrypted entry when the
// resolver list holds no DoT/DoH/DoQ/DNSCrypt upstream, and both clients dim the
// resolvers the active transport excludes. Duplicating the scheme list in JS and
// Kotlin would drift. EncryptedOnly is the derived legacy field older clients
// still read.
type resolutionView struct {
	Transport          string                        `json:"transport"`
	Transports         []string                      `json:"transports"`
	EncryptedOnly      bool                          `json:"encrypted_only"`
	EncryptedAvailable bool                          `json:"encrypted_available"`
	EncryptedUpstreams []string                      `json:"encrypted_upstreams"`
	DefaultUpstream    string                        `json:"default_upstream"`
	DefaultStrategy    string                        `json:"default_strategy"`
	AdaptiveInterval   int                           `json:"adaptive_interval_minutes"`
	AdaptiveLastRun    string                        `json:"adaptive_last_run"`
	AdaptiveCurrent    string                        `json:"adaptive_current"`
	AdaptiveLatencyMs  float64                       `json:"adaptive_last_latency_ms"`
	AdaptiveLastNote   string                        `json:"adaptive_last_note"`
	Profiles           map[string]resolution.Profile `json:"profiles"`
	Upstreams          []string                      `json:"upstreams"`
}

// handleResolutionSettings reads or writes the global encrypted-only filter and
// the per-domain resolution profiles. Saving applies them to the running pool and
// flushes the cache, so a changed rule cannot be hidden by an answer cached under
// the old one.
func (ws *WebServer) handleResolutionSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		ws.writeResolution(w)
	case http.MethodPost:
		ws.saveResolution(w, r)
	default:
		httpx.WriteMethodNotAllowed(w, "GET, POST")
	}
}

func (ws *WebServer) writeResolution(w http.ResponseWriter) {
	ws.dnsMu.Lock()
	cfg := ws.mutableDNSLocked()
	transports := resolution.NormalizeProtocols(cfg.Transports)
	transport := resolution.LegacyTransportFor(transports)
	defaultUpstream := canonicalUpstream(cfg.DefaultUpstream)
	defaultStrategy := resolution.NormalizeStrategy(cfg.DefaultStrategy)
	adaptiveInterval := resolution.NormalizeAdaptiveInterval(cfg.AdaptiveIntervalMinutes)
	profiles := make(map[string]resolution.Profile, len(cfg.Profiles))
	for k, v := range cfg.Profiles {
		v = v.Normalized()
		v.Strategy = resolution.NormalizeProfileStrategy(v.Strategy)
		v.Upstream = canonicalUpstream(v.Upstream)
		profiles[k] = v
	}
	upstreams := canonicalUpstreams(cfg.Upstreams)
	ws.dnsMu.Unlock()

	// The pool is the operational truth. Read the view from it so a resolver
	// added through any path (including a client built before the v3 handlers
	// persisted their changes) still enables the encrypted choice and appears in
	// the pickers. The settings record remains the restart-time source.
	if ws.upstreams != nil {
		if live := ws.upstreams.GetUpstreams(); len(live) > 0 {
			upstreams = canonicalUpstreams(live)
		}
	}

	encrypted := encryptedUpstreams(upstreams)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resolutionView{
		Transport:          transport,
		Transports:         transports,
		EncryptedOnly:      transport == resolution.TransportEncrypted,
		EncryptedAvailable: len(encrypted) > 0,
		EncryptedUpstreams: encrypted,
		DefaultUpstream:    defaultUpstream,
		DefaultStrategy:    defaultStrategy,
		AdaptiveInterval:   adaptiveInterval,
		AdaptiveLastRun:    cfg.AdaptiveLastRun,
		AdaptiveCurrent:    cfg.AdaptiveCurrent,
		AdaptiveLatencyMs:  cfg.AdaptiveLastLatencyMs,
		AdaptiveLastNote:   cfg.AdaptiveLastNote,
		Profiles:           profiles,
		Upstreams:          upstreams,
	})
}

// handleResolutionAdaptiveRun forces one Adaptive cycle now instead of waiting
// for the interval. The benchmark is synchronous so the response carries the
// resolver the server actually switched to.
func (ws *WebServer) handleResolutionAdaptiveRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.WriteMethodNotAllowed(w, "POST")
		return
	}
	if ws.upstreams == nil {
		httpx.WriteJSONError(w, http.StatusServiceUnavailable, "No upstream pool configured")
		return
	}
	if _, err := ws.runAdaptiveNow(true); err != nil {
		switch {
		case errors.Is(err, errAdaptiveNotSelected):
			httpx.WriteJSONError(w, http.StatusConflict, "Select Adaptive (fastest) before running a check")
		case errors.Is(err, errAdaptiveBusy):
			httpx.WriteJSONError(w, http.StatusConflict, "A benchmark is already in progress")
		default:
			httpx.WriteJSONError(w, http.StatusInternalServerError, "Could not complete the adaptive check")
		}
		return
	}
	ws.writeResolution(w)
}

func (ws *WebServer) saveResolution(w http.ResponseWriter, r *http.Request) {
	// A chip save and a transport switch save touch the same DNSSettings and
	// the same listeners. Serialise them so the last writer wins cleanly.
	ws.transportSaveMu.Lock()
	defer ws.transportSaveMu.Unlock()

	var body struct {
		Transport string `json:"transport"`
		// Transports is the authoritative multi-select protocol set. Nil means
		// an older client omitted it; an explicit empty list is rejected.
		Transports []string `json:"transports"`
		// EncryptedOnly is accepted from clients built before the transport
		// choice existed; it only applies when Transport is absent.
		EncryptedOnly   *bool  `json:"encrypted_only"`
		DefaultUpstream string `json:"default_upstream"`
		DefaultStrategy string `json:"default_strategy"`
		// AdaptiveIntervalMinutes is a pointer so an older client that does not
		// know the field preserves the stored interval instead of resetting it.
		AdaptiveIntervalMinutes *int                          `json:"adaptive_interval_minutes"`
		Profiles                map[string]resolution.Profile `json:"profiles"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		httpx.WriteJSONError(w, http.StatusBadRequest, "Could not read the resolution settings: "+err.Error())
		return
	}
	if len(body.Profiles) > 500 {
		httpx.WriteJSONError(w, http.StatusBadRequest, "Too many resolution profiles (limit 500)")
		return
	}
	if !resolution.ValidTransport(body.Transport) {
		httpx.WriteJSONError(w, http.StatusBadRequest, "Unknown transport")
		return
	}
	if body.Transports != nil && !resolution.ValidProtocols(body.Transports) {
		httpx.WriteJSONError(w, http.StatusBadRequest, "Select at least one valid transport protocol")
		return
	}
	if !resolution.ValidStrategy(body.DefaultStrategy) {
		httpx.WriteJSONError(w, http.StatusBadRequest, "Unknown default strategy")
		return
	}
	if body.AdaptiveIntervalMinutes != nil && !resolution.ValidAdaptiveInterval(*body.AdaptiveIntervalMinutes) {
		httpx.WriteJSONError(w, http.StatusBadRequest, "Unknown adaptive interval")
		return
	}
	// Empty and the retired "fastest" value both normalise onto the strategy
	// this release actually runs.
	normalizedStrategy := resolution.NormalizeStrategy(body.DefaultStrategy)
	defaultUpstream := strings.TrimSpace(body.DefaultUpstream)
	if defaultUpstream != "" {
		canonical, ok := upstream.NormalizeUpstream(defaultUpstream)
		if !ok {
			httpx.WriteJSONError(w, http.StatusBadRequest, "Invalid default upstream")
			return
		}
		defaultUpstream = canonical
	}
	allowed := ws.proxiedDomainPatterns()
	clean := make(map[string]resolution.Profile, len(body.Profiles))
	for rawKey, p := range body.Profiles {
		key, ok := normalizeProfileKey(rawKey)
		if !ok {
			httpx.WriteJSONError(w, http.StatusBadRequest, "Invalid domain pattern: "+rawKey)
			return
		}
		if !allowed[key] {
			httpx.WriteJSONError(w, http.StatusBadRequest, "Resolution rules are limited to domains in the Proxied Domains list: "+key)
			return
		}
		if !resolution.ValidTransport(p.Transport) {
			httpx.WriteJSONError(w, http.StatusBadRequest, "Unknown transport in the rule for "+key)
			return
		}
		p = p.Normalized()
		p.Upstream = strings.TrimSpace(p.Upstream)
		if p.Upstream != "" {
			canonical, ok := upstream.NormalizeUpstream(p.Upstream)
			if !ok {
				httpx.WriteJSONError(w, http.StatusBadRequest, "Invalid upstream in the rule for "+key)
				return
			}
			p.Upstream = canonical
		}
		if strings.EqualFold(strings.TrimSpace(p.Strategy), resolution.StrategyAdaptive) {
			httpx.WriteJSONError(w, http.StatusBadRequest, "Adaptive is a global strategy; a per-domain rule cannot carry its own interval")
			return
		}
		p.Strategy = resolution.NormalizeProfileStrategy(p.Strategy)
		if !resolution.ValidStrategy(p.Strategy) {
			httpx.WriteJSONError(w, http.StatusBadRequest, "Unknown strategy in the rule for "+key)
			return
		}
		switch p.Fallback {
		case "", resolution.FallbackPool, resolution.FallbackFail:
		default:
			httpx.WriteJSONError(w, http.StatusBadRequest, "Unknown fallback in the rule for "+key)
			return
		}
		clean[key] = p
	}

	ws.dnsMu.Lock()
	cfg := ws.mutableDNSLocked()
	before := *cfg
	transports := cfg.Transports
	switch {
	case body.Transports != nil:
		transports = body.Transports
	case body.Transport != "":
		transports = resolution.ProtocolsForTransport(body.Transport)
	case body.EncryptedOnly != nil && *body.EncryptedOnly:
		transports = resolution.ProtocolsForTransport(resolution.TransportEncrypted)
	}
	transports = applyProtocolsToClient(cfg, transports)
	cfg.Transports = transports
	cfg.Transport = resolution.LegacyTransportFor(transports)
	// Keep the legacy flag in step so a downgrade to 3.4.11 still means the
	// same thing for the one choice that release understands.
	cfg.EncryptedOnly = cfg.Transport == resolution.TransportEncrypted
	cfg.DefaultUpstream = defaultUpstream
	cfg.DefaultStrategy = normalizedStrategy
	if body.AdaptiveIntervalMinutes != nil {
		cfg.AdaptiveIntervalMinutes = *body.AdaptiveIntervalMinutes
	}
	if cfg.AdaptiveIntervalMinutes <= 0 {
		cfg.AdaptiveIntervalMinutes = resolution.DefaultAdaptiveIntervalMinutes
	}
	cfg.Profiles = clean
	adaptiveCleared := false
	if cfg.AdaptiveCurrent != "" {
		allowedCurrent := false
		for _, protocol := range transports {
			if resolution.Protocol(cfg.AdaptiveCurrent) == protocol {
				allowedCurrent = true
				break
			}
		}
		if !allowedCurrent {
			cfg.AdaptiveCurrent = ""
			cfg.AdaptiveLastNote = "current resolver disabled by the transport chips"
			adaptiveCleared = true
		}
	}
	// The resolver chips and the client transport switches are one policy:
	// apply the listener changes before persisting so a failed bind leaves the
	// stored record untouched. A nil applier is a test harness.
	tokens := ws.loadAccessConfig().DoQTokens
	if cfg.DNSCryptEnabled && ws.dnscryptApplier != nil {
		rc, cert := ws.dnscryptRuntime()
		if rc == nil {
			var keyErr error
			_, rc, cert, keyErr = ws.ensureDNSCrypt(cfg.DNSCryptProviderName)
			if keyErr != nil {
				*cfg = before
				ws.dnsMu.Unlock()
				httpx.WriteJSONError(w, http.StatusInternalServerError, "Could not prepare the DNSCrypt keys: "+keyErr.Error())
				return
			}
			if err := ws.dnscryptApplier(rc, cert); err != nil {
				*cfg = before
				ws.dnsMu.Unlock()
				httpx.WriteJSONError(w, http.StatusInternalServerError, "Could not install the DNSCrypt keys: "+err.Error())
				return
			}
		}
	}
	if ws.transportApplier != nil {
		if err := ws.transportApplier(*cfg, tokens); err != nil {
			_ = ws.transportApplier(before, tokens)
			*cfg = before
			ws.dnsMu.Unlock()
			httpx.WriteJSONError(w, http.StatusInternalServerError, "The transport settings were not applied: "+err.Error())
			return
		}
	}

	err := ws.db.SetSetting("dns", cfg)
	adaptiveCurrent := cfg.AdaptiveCurrent
	legacyTransport := cfg.Transport
	ws.dnsMu.Unlock()
	if err != nil {
		if ws.transportApplier != nil {
			_ = ws.transportApplier(before, tokens)
		}
		httpx.WriteJSONError(w, http.StatusInternalServerError, "Could not persist the resolution settings")
		return
	}

	ws.upstreams.SetPolicy(upstream.Policy{
		Transport:       legacyTransport,
		Transports:      transports,
		DefaultUpstream: defaultUpstream,
		DefaultStrategy: normalizedStrategy,
		AdaptiveCurrent: adaptiveCurrent,
		Profiles:        clean,
	})
	if adaptiveCleared {
		ws.upstreams.SetAdaptiveCurrent("")
	}
	if ws.cache != nil {
		ws.cache.Flush()
	}
	ws.nudgeAdaptive()
	ws.writeResolution(w)
}

// normalizeProfileKey accepts "example.com", "*.example.com" and trailing dots,
// and returns the canonical lowercase form. A wildcard must be the whole first
// label and must have a name after it.
func normalizeProfileKey(raw string) (string, bool) {
	k := strings.ToLower(strings.TrimSpace(raw))
	k = strings.TrimSuffix(k, ".")
	wild := strings.HasPrefix(k, "*.")
	if wild {
		k = strings.TrimPrefix(k, "*.")
	}
	if k == "" || strings.ContainsAny(k, "* \t\r\n/") || strings.HasPrefix(k, ".") || strings.HasSuffix(k, ".") {
		return "", false
	}
	if wild {
		return "*." + k, true
	}
	return k, true
}

// proxiedDomainPatterns returns the normalised set of domains the Proxied
// Domains page owns — both the active list and the paused one, because a paused
// domain is still listed and its rule takes effect while resolution is ordinary.
func (ws *WebServer) proxiedDomainPatterns() map[string]bool {
	allowed := map[string]bool{}
	for _, key := range []string{"custom_proxied", "custom_proxied_disabled"} {
		p, err := ws.db.GetPolicy(key)
		if err != nil || p == nil {
			continue
		}
		for _, d := range p.CustomDomains {
			if n, ok := normalizeProfileKey(d); ok {
				allowed[n] = true
			}
		}
	}
	return allowed
}

// applyResolutionPolicy installs the current DNS settings on the live pool.
// Called after any rules edit so a profile whose domain was removed stops
// applying immediately.
func (ws *WebServer) applyResolutionPolicy() {
	ws.dnsMu.Lock()
	cfg := ws.mutableDNSLocked()
	policy := upstream.Policy{
		Transport:       cfg.EffectiveTransport(),
		Transports:      cfg.Transports,
		DefaultUpstream: cfg.DefaultUpstream,
		DefaultStrategy: cfg.DefaultStrategy,
		AdaptiveCurrent: cfg.AdaptiveCurrent,
		Profiles:        cfg.Profiles,
	}
	ws.dnsMu.Unlock()
	ws.upstreams.SetPolicy(policy)
}

// pruneResolutionProfiles drops rules whose domain is no longer in either
// proxied list. It returns true when something was removed and persisted.
func (ws *WebServer) pruneResolutionProfiles(allowed map[string]bool) bool {
	ws.dnsMu.Lock()
	defer ws.dnsMu.Unlock()

	cfg := ws.mutableDNSLocked()
	if len(cfg.Profiles) == 0 {
		return false
	}
	changed := false
	for key := range cfg.Profiles {
		if !allowed[key] {
			delete(cfg.Profiles, key)
			changed = true
		}
	}
	if !changed {
		return false
	}
	_ = ws.db.SetSetting("dns", cfg)
	return true
}

// canonicalUpstream normalises one stored address the way the pool does, so the
// settings view (pickers, dimming, the encrypted gate) uses the exact strings the
// stats list reports. An unparseable legacy value is returned unchanged.
func canonicalUpstream(address string) string {
	if address == "" {
		return ""
	}
	if canonical, ok := upstream.NormalizeUpstream(address); ok {
		return canonical
	}
	return address
}

// canonicalUpstreams normalises the whole stored list. A resolver stored before
// the pool started appending the default port would otherwise not match its own
// stats row — the encrypted set and the Default upstream picker would disagree
// with the list they are meant to annotate.
func canonicalUpstreams(list []string) []string {
	out := make([]string, 0, len(list))
	for _, u := range list {
		out = append(out, canonicalUpstream(u))
	}
	return out
}

// encryptedUpstreams returns the subset of the resolver list carried over an
// encrypted transport. The clients use it both to gate the encrypted choice and
// to dim the resolvers the active transport excludes.
func encryptedUpstreams(upstreams []string) []string {
	out := make([]string, 0, len(upstreams))
	for _, u := range upstreams {
		if resolution.Encrypted(u) {
			out = append(out, u)
		}
	}
	return out
}
