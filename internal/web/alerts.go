package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"digitdns/internal/httpx"
	"digitdns/internal/service"
)

// handleAlerts reads and saves the alert configuration. The record is sealed by
// SetSetting, so webhook URLs and bot tokens are encrypted at rest.
func (ws *WebServer) handleAlerts(w http.ResponseWriter, r *http.Request) {
	if ws.alerts == nil {
		httpx.WriteJSONError(w, http.StatusServiceUnavailable, "alerts are unavailable")
		return
	}
	switch r.Method {
	case http.MethodGet, http.MethodHead:
		settings, err := ws.alerts.Settings()
		if err != nil {
			httpx.WriteJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"alerts": redactAlerts(settings), "history": ws.alerts.History()})
	case http.MethodPost:
		var req service.AlertSettings
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			httpx.WriteJSONError(w, http.StatusBadRequest, "Invalid JSON payload")
			return
		}
		// The API returns tokens redacted. A save that round-trips the redacted
		// value must keep the stored token rather than replacing it with dots.
		if current, err := ws.alerts.Settings(); err == nil {
			byID := map[string]string{}
			for _, ch := range current.Channels {
				byID[ch.ID] = ch.Token
			}
			for i := range req.Channels {
				tok := strings.TrimSpace(req.Channels[i].Token)
				if (tok == "" || strings.HasPrefix(tok, "••")) && byID[req.Channels[i].ID] != "" {
					req.Channels[i].Token = byID[req.Channels[i].ID]
				}
			}
		}
		if err := validateAlerts(req); err != nil {
			httpx.WriteJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := ws.alerts.SaveSettings(req); err != nil {
			httpx.WriteJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"alerts": redactAlerts(req), "history": ws.alerts.History()})
	default:
		httpx.WriteMethodNotAllowed(w, "GET, HEAD, POST")
	}
}

// handleAlertsTest sends a synthetic event to every enabled channel.
func (ws *WebServer) handleAlertsTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		httpx.WriteMethodNotAllowed(w, "POST")
		return
	}
	if ws.alerts == nil {
		httpx.WriteJSONError(w, http.StatusServiceUnavailable, "alerts are unavailable")
		return
	}
	failures := ws.alerts.SendTest(r.Context())
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": len(failures) == 0, "errors": failures})
}

// redactAlerts never returns a bot token. URLs are returned because the operator
// needs to see which webhook is configured; tokens are write-only.
func redactAlerts(s service.AlertSettings) service.AlertSettings {
	out := s
	out.Channels = make([]service.AlertChannel, len(s.Channels))
	for i, ch := range s.Channels {
		if ch.Token != "" {
			ch.Token = "••••••"
		}
		out.Channels[i] = ch
	}
	return out
}

func validateAlerts(s service.AlertSettings) error {
	if len(s.Channels) > 10 {
		return fmt.Errorf("at most 10 alert channels are supported")
	}
	seen := map[string]bool{}
	for i := range s.Channels {
		ch := &s.Channels[i]
		ch.Kind = strings.ToLower(strings.TrimSpace(ch.Kind))
		ch.Name = strings.TrimSpace(ch.Name)
		ch.URL = strings.TrimSpace(ch.URL)
		ch.Token = strings.TrimSpace(ch.Token)
		ch.ChatID = strings.TrimSpace(ch.ChatID)
		switch ch.Kind {
		case "ntfy", "gotify", "discord", "slack", "webhook":
			if ch.URL == "" {
				return fmt.Errorf("%s channel %q needs a URL", ch.Kind, ch.Name)
			}
			u, err := url.Parse(ch.URL)
			if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
				return fmt.Errorf("%s channel %q needs an http(s) URL", ch.Kind, ch.Name)
			}
		case "telegram":
			if ch.Token == "" || ch.ChatID == "" {
				return fmt.Errorf("telegram channel %q needs a bot token and chat ID", ch.Name)
			}
		default:
			return fmt.Errorf("unknown alert channel kind %q", ch.Kind)
		}
		if ch.ID == "" {
			ch.ID = fmt.Sprintf("ch-%d-%d", time.Now().UnixNano(), i)
		}
		if seen[ch.ID] {
			return fmt.Errorf("duplicate alert channel id %q", ch.ID)
		}
		seen[ch.ID] = true
	}
	if s.CooldownMinutes <= 0 {
		s.CooldownMinutes = 15
	}
	return nil
}
