package service

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"digitdns/internal/database"
)

// AlertSettings is the persisted operator configuration. It is stored through
// SetSetting, which seals the record with the master key, so webhook URLs and
// bot tokens are encrypted at rest.
type AlertSettings struct {
	Enabled         bool            `json:"enabled"`
	CooldownMinutes int             `json:"cooldown_minutes"`
	Events          map[string]bool `json:"events"`
	Thresholds      AlertThresholds `json:"thresholds"`
	Channels        []AlertChannel  `json:"channels"`
}

// AlertThresholds lets an operator tune what "offline", "expiring" and
// "degraded" mean without editing code. Zero values fall back to the defaults.
type AlertThresholds struct {
	AgentOfflineMinutes  int `json:"agent_offline_minutes"`
	EgressOfflineMinutes int `json:"egress_offline_minutes"`
	CertExpiryDays       int `json:"cert_expiry_days"`
	UpstreamErrorPercent int `json:"upstream_error_percent"`
}

func (t AlertThresholds) normalized() AlertThresholds {
	if t.AgentOfflineMinutes <= 0 {
		t.AgentOfflineMinutes = 2
	}
	if t.EgressOfflineMinutes <= 0 {
		t.EgressOfflineMinutes = 2
	}
	if t.CertExpiryDays <= 0 {
		t.CertExpiryDays = 30
	}
	if t.UpstreamErrorPercent <= 0 {
		t.UpstreamErrorPercent = 50
	}
	return t
}

// AlertDelivery is one delivery attempt set, kept for the support view.
type AlertDelivery struct {
	Time     time.Time `json:"time"`
	Channel  string    `json:"channel"`
	Event    string    `json:"event"`
	Title    string    `json:"title"`
	OK       bool      `json:"ok"`
	Attempts int       `json:"attempts"`
	Error    string    `json:"error,omitempty"`
}

// AlertChannel is one delivery target. Kind selects which fields matter:
// ntfy/gotify/webhook/discord/slack use URL; telegram uses Token + ChatID.
type AlertChannel struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Name    string `json:"name"`
	URL     string `json:"url,omitempty"`
	Token   string `json:"token,omitempty"`
	ChatID  string `json:"chat_id,omitempty"`
	Enabled bool   `json:"enabled"`
}

// AlertEvent is one active condition. Key is stable per object (agent:<id>,
// egress:<id>, ...), so the manager can detect recovery when it disappears.
type AlertEvent struct {
	Key      string `json:"key"`
	Type     string `json:"type"`
	Title    string `json:"title"`
	Message  string `json:"message"`
	Severity string `json:"severity"`
}

// CertificateExpiryEvents scans the configured certificate pairs and reports
// any that are expired or inside the warning window. The default paths are
// included so an install that never set cert_path still gets warned about the
// pair the daemon actually serves.
func CertificateExpiryEvents(tlsCfg *database.TLSSettings, certs *database.CertificateSettings, days int) []AlertEvent {
	if days <= 0 {
		days = 30
	}
	type pair struct{ label, cert, key string }
	var pairs []pair
	add := func(label, cert, key string) {
		if strings.TrimSpace(cert) != "" && strings.TrimSpace(key) != "" {
			pairs = append(pairs, pair{label, cert, key})
		}
	}
	if certs != nil {
		for _, p := range []struct {
			label string
			cfg   database.CertificateConfig
		}{
			{"Panel", certs.Get(database.CertPurposePanel)},
			{"DoH/DoT", certs.Get(database.CertPurposeDoT)},
			{"Subscription", certs.Get(database.CertPurposeSubscription)},
		} {
			add(p.label, p.cfg.CertPath, p.cfg.KeyPath)
		}
	}
	if tlsCfg != nil {
		add("Panel", tlsCfg.CertPath, tlsCfg.KeyPath)
	}
	add("Panel", "certs/cert.pem", "certs/key.pem")
	add("Panel", "/opt/digitdns/certs/cert.pem", "/opt/digitdns/certs/key.pem")

	seen := map[string]bool{}
	var out []AlertEvent
	now := time.Now()
	for _, p := range pairs {
		if seen[p.cert] {
			continue
		}
		seen[p.cert] = true
		if _, err := os.Stat(p.cert); err != nil {
			continue
		}
		cert, err := tls.LoadX509KeyPair(p.cert, p.key)
		if err != nil || len(cert.Certificate) == 0 {
			continue
		}
		leaf, err := x509.ParseCertificate(cert.Certificate[0])
		if err != nil {
			continue
		}
		left := leaf.NotAfter.Sub(now)
		if left > time.Duration(days)*24*time.Hour {
			continue
		}
		sev := "warning"
		if left <= 0 {
			sev = "critical"
		}
		out = append(out, AlertEvent{
			Key: "cert:" + p.cert, Type: "certificates", Severity: sev,
			Title:   fmt.Sprintf("%s certificate expires in %d days", p.label, int(left.Hours()/24)),
			Message: fmt.Sprintf("%s (%s) is valid until %s.", p.label, p.cert, leaf.NotAfter.UTC().Format("2006-01-02")),
		})
	}
	return out
}

// AlertSource returns every condition that is currently active.
type AlertSource func() []AlertEvent

// AlertManager evaluates sources on a timer and delivers transitions. Delivery
// is off the DNS/proxy hot path: checks run in their own goroutine and each HTTP
// send is bounded by a timeout.
type AlertManager struct {
	db      *database.DB
	client  *http.Client
	sources []AlertSource

	mu       sync.Mutex
	last     map[string]AlertEvent
	lastSent map[string]time.Time
	history  []AlertDelivery
}

func NewAlertManager(db *database.DB) *AlertManager {
	return &AlertManager{
		db:       db,
		client:   &http.Client{Timeout: 10 * time.Second},
		last:     map[string]AlertEvent{},
		lastSent: map[string]time.Time{},
	}
}

func (m *AlertManager) AddSource(src AlertSource) {
	if m == nil || src == nil {
		return
	}
	m.mu.Lock()
	m.sources = append(m.sources, src)
	m.mu.Unlock()
}

// Settings reads the persisted configuration. A missing record is a valid
// disabled configuration.
func (m *AlertManager) Settings() (AlertSettings, error) {
	var s AlertSettings
	if m == nil || m.db == nil {
		return s, nil
	}
	if err := m.db.GetSetting("alerts", &s); err != nil {
		return s, err
	}
	if s.CooldownMinutes <= 0 {
		s.CooldownMinutes = 15
	}
	if s.Events == nil {
		s.Events = map[string]bool{}
	}
	s.Thresholds = s.Thresholds.normalized()
	return s, nil
}

// Thresholds is the normalized current threshold set, safe to call from the
// source closures on every evaluation.
func (m *AlertManager) Thresholds() AlertThresholds {
	s, err := m.Settings()
	if err != nil {
		return AlertThresholds{}.normalized()
	}
	return s.Thresholds
}

// History returns the most recent deliveries, newest first.
func (m *AlertManager) History() []AlertDelivery {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]AlertDelivery, len(m.history))
	for i := range m.history {
		out[len(m.history)-1-i] = m.history[i]
	}
	return out
}

func (m *AlertManager) SaveSettings(s AlertSettings) error {
	if m == nil || m.db == nil {
		return nil
	}
	if s.CooldownMinutes <= 0 {
		s.CooldownMinutes = 15
	}
	if s.Events == nil {
		s.Events = map[string]bool{}
	}
	s.Thresholds = s.Thresholds.normalized()
	return m.db.SetSetting("alerts", s)
}

// Start evaluates immediately and then every minute.
func (m *AlertManager) Start(ctx context.Context) {
	if m == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	go func() {
		timer := time.NewTimer(5 * time.Second)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
				m.Evaluate(ctx)
				timer.Reset(time.Minute)
			}
		}
	}()
}

// Evaluate compares the current source set with the previous one and sends only
// transitions. A condition that disappears produces one recovery notice.
func (m *AlertManager) Evaluate(ctx context.Context) {
	settings, err := m.Settings()
	if err != nil || !settings.Enabled || len(settings.Channels) == 0 {
		return
	}
	m.mu.Lock()
	sources := append([]AlertSource(nil), m.sources...)
	m.mu.Unlock()

	active := map[string]AlertEvent{}
	for _, src := range sources {
		for _, ev := range src() {
			if ev.Key == "" || !eventEnabled(settings, ev.Type) {
				continue
			}
			active[ev.Key] = ev
		}
	}

	now := time.Now()
	m.mu.Lock()
	previous := m.last
	m.last = active
	toSend := make([]AlertEvent, 0)
	for key, ev := range active {
		if _, was := previous[key]; was {
			continue
		}
		if last, ok := m.lastSent[key]; ok && now.Sub(last) < time.Duration(settings.CooldownMinutes)*time.Minute {
			continue
		}
		m.lastSent[key] = now
		toSend = append(toSend, ev)
	}
	for key, ev := range previous {
		if _, still := active[key]; still {
			continue
		}
		recovery := AlertEvent{
			Key: key, Type: ev.Type, Severity: "info",
			Title: "Recovered: " + ev.Title, Message: ev.Message + " — the condition has cleared.",
		}
		toSend = append(toSend, recovery)
	}
	m.mu.Unlock()

	for _, ev := range toSend {
		m.deliver(ctx, settings, ev)
	}
}

func eventEnabled(s AlertSettings, kind string) bool {
	if len(s.Events) == 0 {
		return true
	}
	on, ok := s.Events[kind]
	return ok && on
}

// SendTest delivers a synthetic event to every enabled channel and reports the
// per-channel failures without failing the whole test.
func (m *AlertManager) SendTest(ctx context.Context) []string {
	settings, err := m.Settings()
	if err != nil {
		return []string{err.Error()}
	}
	ev := AlertEvent{
		Key: "test", Type: "test", Severity: "info",
		Title: "DigitDNS test alert", Message: "This is a test alert from your DigitDNS server.",
	}
	var failures []string
	enabled := 0
	for _, ch := range settings.Channels {
		if !ch.Enabled {
			continue
		}
		enabled++
		attempts, err := m.send(ctx, ch, ev)
		m.record(ch, ev, attempts, err)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", channelLabel(ch), err))
		}
	}
	if enabled == 0 {
		return []string{"no enabled channels"}
	}
	return failures
}

func (m *AlertManager) deliver(ctx context.Context, settings AlertSettings, ev AlertEvent) {
	for _, ch := range settings.Channels {
		if !ch.Enabled {
			continue
		}
		attempts, err := m.send(ctx, ch, ev)
		m.record(ch, ev, attempts, err)
	}
}

func (m *AlertManager) record(ch AlertChannel, ev AlertEvent, attempts int, err error) {
	entry := AlertDelivery{
		Time: time.Now().UTC(), Channel: channelLabel(ch), Event: ev.Type,
		Title: ev.Title, OK: err == nil, Attempts: attempts,
	}
	if err != nil {
		entry.Error = err.Error()
	}
	m.mu.Lock()
	m.history = append(m.history, entry)
	if len(m.history) > 100 {
		m.history = m.history[len(m.history)-100:]
	}
	m.mu.Unlock()
}

func channelLabel(ch AlertChannel) string {
	if strings.TrimSpace(ch.Name) != "" {
		return ch.Name
	}
	return ch.Kind
}

func (m *AlertManager) send(ctx context.Context, ch AlertChannel, ev AlertEvent) (int, error) {
	var err error
	attempts := 0
	for attempts = 1; attempts <= 2; attempts++ {
		err = m.sendOnce(ctx, ch, ev)
		if err == nil {
			return attempts, nil
		}
		if attempts < 2 {
			select {
			case <-ctx.Done():
				return attempts, err
			case <-time.After(500 * time.Millisecond):
			}
		}
	}
	return attempts, err
}

func (m *AlertManager) sendOnce(ctx context.Context, ch AlertChannel, ev AlertEvent) error {
	switch ch.Kind {
	case "ntfy":
		return m.postText(ctx, ch.URL, ev.Message, map[string]string{
			"Title": ev.Title, "Priority": "default", "Tags": "warning",
		})
	case "gotify":
		u := strings.TrimRight(ch.URL, "/") + "/message?token=" + url.QueryEscape(ch.Token)
		return m.postJSON(ctx, u, map[string]any{"title": ev.Title, "message": ev.Message, "priority": 5})
	case "discord":
		return m.postJSON(ctx, ch.URL, map[string]any{"content": ev.Title + "\n" + ev.Message})
	case "slack":
		return m.postJSON(ctx, ch.URL, map[string]any{"text": ev.Title + "\n" + ev.Message})
	case "telegram":
		if ch.Token == "" || ch.ChatID == "" {
			return fmt.Errorf("telegram needs a bot token and chat ID")
		}
		u := "https://api.telegram.org/bot" + ch.Token + "/sendMessage"
		return m.postJSON(ctx, u, map[string]any{"chat_id": ch.ChatID, "text": ev.Title + "\n" + ev.Message})
	case "webhook":
		return m.postJSON(ctx, ch.URL, map[string]any{
			"event": ev.Type, "key": ev.Key, "title": ev.Title,
			"message": ev.Message, "severity": ev.Severity, "time": time.Now().UTC().Format(time.RFC3339),
		})
	default:
		return fmt.Errorf("unknown channel kind %q", ch.Kind)
	}
}

func (m *AlertManager) postJSON(ctx context.Context, endpoint string, body any) error {
	if strings.TrimSpace(endpoint) == "" {
		return fmt.Errorf("missing webhook URL")
	}
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	return m.do(req)
}

func (m *AlertManager) postText(ctx context.Context, endpoint, text string, headers map[string]string) error {
	if strings.TrimSpace(endpoint) == "" {
		return fmt.Errorf("missing URL")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(text))
	if err != nil {
		return err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return m.do(req)
}

func (m *AlertManager) do(req *http.Request) error {
	resp, err := m.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("endpoint answered %s", resp.Status)
	}
	return nil
}
