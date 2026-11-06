package web

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"

	"digitdns/internal/core/dnssec"
	"digitdns/internal/httpx"
)

// dnssecView is what the Settings card reads. Running is the policy this
// process installed at boot; the saved record is what the next restart will
// use. When they differ the card says a restart is required instead of
// pretending the toggle already took effect.
type dnssecView struct {
	Enabled         bool     `json:"enabled"`
	TrustAnchors    []string `json:"trust_anchors"`
	DefaultAnchors  []string `json:"default_anchors"`
	Running         bool     `json:"running"`
	RestartRequired bool     `json:"restart_required"`
}

// handleDNSSECSettings reads or writes the startup-only DNSSEC policy. It is
// deliberately not a live toggle: the validator is constructed before the
// listeners start, so saving changes the persisted record and reports that a
// restart is pending.
func (ws *WebServer) handleDNSSECSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		ws.writeDNSSEC(w)
	case http.MethodPost:
		ws.saveDNSSEC(w, r)
	default:
		httpx.WriteMethodNotAllowed(w, "GET, POST")
	}
}

func (ws *WebServer) writeDNSSEC(w http.ResponseWriter) {
	ws.dnsMu.Lock()
	cfg := ws.mutableDNSLocked()
	enabled := cfg.DNSSECValidation
	anchors := make([]string, len(cfg.DNSSECTrustAnchors))
	copy(anchors, cfg.DNSSECTrustAnchors)
	ws.dnsMu.Unlock()

	running := ws.dnssecActive.Load()
	runningAnchors := []string(nil)
	if p := ws.dnssecAnchors.Load(); p != nil {
		runningAnchors = *p
	}
	restartRequired := enabled != running
	if enabled && running && !slices.Equal(anchors, runningAnchors) {
		restartRequired = true
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(dnssecView{
		Enabled:         enabled,
		TrustAnchors:    anchors,
		DefaultAnchors:  dnssec.DefaultTrustAnchorRecords(),
		Running:         running,
		RestartRequired: restartRequired,
	})
}

func (ws *WebServer) saveDNSSEC(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Enabled      bool     `json:"enabled"`
		TrustAnchors []string `json:"trust_anchors"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&body); err != nil {
		httpx.WriteJSONError(w, http.StatusBadRequest, "Could not read the DNSSEC settings: "+err.Error())
		return
	}

	anchors := normalizeDNSSECAnchors(body.TrustAnchors)
	if len(anchors) > 16 {
		httpx.WriteJSONError(w, http.StatusBadRequest, "Too many trust anchors (limit 16)")
		return
	}
	if err := dnssec.ValidateTrustAnchors(anchors); err != nil {
		httpx.WriteJSONError(w, http.StatusBadRequest, "Invalid DNSSEC trust anchor: "+err.Error())
		return
	}

	ws.dnsMu.Lock()
	cfg := ws.mutableDNSLocked()
	oldEnabled := cfg.DNSSECValidation
	oldAnchors := append([]string(nil), cfg.DNSSECTrustAnchors...)
	cfg.DNSSECValidation = body.Enabled
	cfg.DNSSECTrustAnchors = anchors
	err := ws.db.SetSetting("dns", cfg)
	if err != nil {
		cfg.DNSSECValidation = oldEnabled
		cfg.DNSSECTrustAnchors = oldAnchors
	}
	ws.dnsMu.Unlock()
	if err != nil {
		httpx.WriteJSONError(w, http.StatusInternalServerError, "Could not persist the DNSSEC settings")
		return
	}

	ws.writeDNSSEC(w)
}

// normalizeDNSSECAnchors trims blank lines and accepts a whole block pasted
// into one array entry, which is what a textarea naturally sends.
func normalizeDNSSECAnchors(in []string) []string {
	out := make([]string, 0, len(in))
	for _, raw := range in {
		for _, line := range strings.Split(raw, "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				out = append(out, line)
			}
		}
	}
	return out
}
