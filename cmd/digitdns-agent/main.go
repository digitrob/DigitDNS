// Command digitdns-agent is the geo agent: a relay that runs on a public IP in
// the country it serves.
//
// It is deliberately not a second DigitDNS. It has no panel, no database and no
// DNS server; it receives a host and client allowlist from the hub, applies it,
// and reports what it carried. Keeping it this small is what makes it safe to
// leave running unattended in a foreign datacentre.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	"digitdns/internal/agent"
	"digitdns/internal/version"
)

func main() {
	var (
		configPath   = flag.String("config", "", "path to a JSON config file (cached hub config)")
		listen       = flag.String("listen", "", "comma-separated listen addresses, e.g. :80,:443 (default from config, else :80,:443)")
		country      = flag.String("country", "", "country this agent exits as, e.g. DE")
		hosts        = flag.String("hosts", "", "comma-separated host allowlist (overrides config)")
		clients      = flag.String("clients", "", "comma-separated client IP/CIDR allowlist (overrides config)")
		httpsPort    = flag.Int("port", 0, "TLS port dialled on the far side (default 443)")
		httpPort     = flag.Int("http-port", 0, "plain HTTP port dialled on the far side (default 80)")
		allowPrivate = flag.Bool("allow-private", false, "relay to private addresses too - TESTING ONLY")
		writeDefault = flag.String("write-config", "", "write the effective config to this path and exit")
		hubURL       = flag.String("hub", "", "control plane base URL, e.g. https://hub.example:8080")
		token        = flag.String("token", "", "one-time enrolment token (first run only)")
		key          = flag.String("key", "", "agent key from the panel (id:secret); skips enrolment")
		statePath    = flag.String("state", "agent-state.json", "where the issued key and last catalogue are cached")
		showVersion  = flag.Bool("version", false, "print the version and exit")
	)
	flag.Parse()

	if *showVersion {
		fmt.Printf("digitdns-agent %s\n", version.Display())
		return
	}

	cfg, err := loadConfig(*configPath)
	if err != nil {
		log.Fatalf("[agent] %v", err)
	}

	if *listen != "" {
		cfg.Listen = *listen
	}
	if cfg.Listen == "" {
		cfg.Listen = ":80,:443"
	}
	if *country != "" {
		cfg.Country = *country
	}
	if *hosts != "" {
		cfg.Hosts = splitList(*hosts)
	}
	if *clients != "" {
		cfg.Clients = splitList(*clients)
	}
	if *httpsPort != 0 {
		cfg.HTTPSPort = *httpsPort
	}
	if *httpPort != 0 {
		cfg.HTTPPort = *httpPort
	}
	if *allowPrivate {
		cfg.AllowPrivate = true
	}
	if cfg.HTTPSPort == 0 {
		cfg.HTTPSPort = 443
	}
	if cfg.HTTPPort == 0 {
		cfg.HTTPPort = 80
	}

	if *writeDefault != "" {
		if err := writeConfig(*writeDefault, cfg); err != nil {
			log.Fatalf("[agent] %v", err)
		}
		return
	}

	// Identity first: a cached key lets a restart serve without the enrolment
	// token, which is spent after the first run.
	state, err := agent.LoadState(*statePath)
	if err != nil {
		log.Fatalf("[agent] %v", err)
	}
	if *key != "" {
		id, secret, ok := strings.Cut(strings.TrimSpace(*key), ":")
		if !ok || id == "" || secret == "" {
			log.Fatalf("[agent] -key must be the id:secret value shown when the agent was added")
		}
		state.AgentID = id
		state.Key = strings.TrimSpace(*key)
	}

	var hub *agent.Hub
	if *hubURL != "" {
		hub = agent.NewHub(*hubURL)
	}

	switch {
	case hub != nil && state.Key == "" && *token != "":
		enr, err := hub.Enrol(*token, version.Display())
		if err != nil {
			log.Fatalf("[agent] enrolment failed: %v", err)
		}
		state.AgentID = enr.AgentID
		state.Key = enr.Key
		state.Country = enr.Country
		if cfg.Country == "" {
			cfg.Country = enr.Country
		}
		log.Printf("[agent] enrolled as %s (%s)", enr.AgentID, enr.Country)
	case hub != nil && state.Key == "":
		log.Fatalf("[agent] no key cached and no --token given: enrol once with --hub and --token")
	}

	if hub != nil && state.Key != "" {
		if cat, err := hub.Fetch(state.Key); err != nil {
			// A hub outage must not stop playback: keep the cached list and say so.
			log.Printf("[agent] catalogue fetch failed (%v) - serving the cached list", err)
		} else {
			state.Hosts = cat.Hosts
			state.Clients = cat.Clients
			state.SyncedAt = time.Now().UTC()
			if cat.Country != "" {
				state.Country = cat.Country
			}
			log.Printf("[agent] catalogue synced: %d hosts, %d clients", len(cat.Hosts), len(cat.Clients))
		}
	}

	// Flags win over the synced list, so a local test can pin a single host
	// without editing the state file.
	if len(cfg.Hosts) == 0 {
		cfg.Hosts = state.Hosts
	}
	if len(cfg.Clients) == 0 {
		cfg.Clients = state.Clients
	}
	if cfg.Country == "" {
		cfg.Country = state.Country
	}
	if err := agent.SaveState(*statePath, state); err != nil {
		log.Printf("[agent] could not write state: %v", err)
	}

	srv := agent.NewServer(cfg)

	// The startup line is the whole configuration in one place. An agent that
	// refuses everything because it synced an empty list looks identical to one
	// that is working until you read these counts, so they are printed, not
	// inferred.
	log.Printf("[agent] %s · country %s · listening %s · dialling http :%d / https :%d",
		version.Display(), orDash(cfg.Country), cfg.Listen, cfg.HTTPPort, cfg.HTTPSPort)
	log.Printf("[agent] %d hosts allowed · %d client entries · private guard %s",
		srv.Allowlist().Len(), srv.ClientList().Len(), guardState(cfg.AllowPrivate))
	if srv.Allowlist().Len() == 0 {
		log.Printf("[agent] WARNING: empty host allowlist - every connection will be refused")
	}
	if srv.ClientList().Len() == 0 {
		log.Printf("[agent] WARNING: empty client allowlist - every connection will be refused")
	}

	if err := srv.Start(); err != nil {
		log.Fatalf("[agent] %v", err)
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	// A periodic snapshot makes the per-host split visible without waiting for a
	// heartbeat, which matters when the hub is not running yet.
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	syncTicker := time.NewTicker(60 * time.Second)
	defer syncTicker.Stop()
	beatTicker := time.NewTicker(30 * time.Second)
	defer beatTicker.Stop()

	for {
		select {
		case <-stop:
			_ = srv.Close()
			logSummary(srv)
			return

		case <-ticker.C:
			logSummary(srv)

		case <-beatTicker.C:
			if hub == nil || state.Key == "" {
				continue
			}
			in, out := srv.Counters().Totals()
			if err := hub.Heartbeat(state.Key, version.Display(), advertisedPort(cfg.Listen), in, out); err != nil {
				// Logged, never fatal: a hub that is down cannot be allowed to
				// stop a relay that is working.
				log.Printf("[agent] heartbeat failed: %v", err)
			} else {
				_, _, _, _ = srv.Stats()
			}

		case <-syncTicker.C:
			if hub == nil || state.Key == "" {
				continue
			}
			cat, err := hub.Fetch(state.Key)
			if err != nil {
				log.Printf("[agent] catalogue resync failed (%v) - keeping the current list", err)
				continue
			}
			// Applied in place rather than rebuilding the server: a resync must
			// not drop connections that are mid-stream.
			srv.UpdateAllowlists(cat.Hosts, cat.Clients)
			state.Hosts, state.Clients = cat.Hosts, cat.Clients
			state.SyncedAt = time.Now().UTC()
			if err := agent.SaveState(*statePath, state); err != nil {
				log.Printf("[agent] could not write state: %v", err)
			}
			log.Printf("[agent] catalogue resynced: %d hosts, %d clients", len(cat.Hosts), len(cat.Clients))
		}
	}
}

// advertisedPort is the relay port the hub should dial for its connection test.
// Prefer 443 when the agent binds it; otherwise use the first configured port.
// A local test agent listening on 18443 therefore reports the port it actually
// uses, not a production assumption.
func advertisedPort(listen string) int {
	ports := []int{}
	for _, addr := range strings.Split(listen, ",") {
		addr = strings.TrimSpace(addr)
		i := strings.LastIndex(addr, ":")
		if i < 0 || i == len(addr)-1 {
			continue
		}
		port := 0
		for _, c := range addr[i+1:] {
			if c < '0' || c > '9' {
				port = 0
				break
			}
			port = port*10 + int(c-'0')
		}
		if port > 0 {
			ports = append(ports, port)
		}
	}
	for _, p := range ports {
		if p == 443 {
			return 443
		}
	}
	if len(ports) > 0 {
		return ports[0]
	}
	return 443
}

func logSummary(srv *agent.Server) {
	active, total, refused, nohost := srv.Stats()
	in, out := srv.Counters().Totals()
	log.Printf("[agent] connections: %d active · %d total · %d refused · %d unreadable · traffic up %s down %s",
		active, total, refused, nohost, humanBytes(in), humanBytes(out))
	for _, h := range srv.Counters().Snapshot() {
		log.Printf("[agent]   %-40s %s", h.Host, humanBytes(h.Bytes))
	}
}

func loadConfig(path string) (agent.Config, error) {
	var cfg agent.Config
	if path == "" {
		return cfg, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("read config: %w", err)
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return cfg, fmt.Errorf("parse config %s: %w", path, err)
	}
	return cfg, nil
}

func writeConfig(path string, cfg agent.Config) error {
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(raw, '\n'), 0o600)
}

func splitList(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}

func guardState(allowPrivate bool) string {
	if allowPrivate {
		return "OFF (testing)"
	}
	return "on"
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func humanBytes(n uint64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := uint64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}
