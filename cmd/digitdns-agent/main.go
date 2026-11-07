// Command digitdns-agent is the geo agent: a relay that runs on a public IP in
// the country it serves.
//
// It is deliberately not a second DigitDNS. It has no panel, no database and no
// DNS server; it receives a host and client allowlist from the hub, applies it,
// and reports what it carried. Keeping it this small is what makes it safe to
// leave running unattended in a foreign datacentre.
package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math/rand/v2"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	"digitdns/internal/agent"
	"digitdns/internal/netutil"
	"digitdns/internal/version"
)

func main() {
	var (
		configPath   = flag.String("config", "", "path to a JSON config file (cached hub config)")
		listen       = flag.String("listen", "", "comma-separated listen addresses, e.g. :80,:443 (default from config, else :80,:443)")
		country      = flag.String("country", "", "country this agent exits as, e.g. DE")
		publicIPv6   = flag.String("public-ipv6", "", "globally routable IPv6 this agent publishes for proxied AAAA (auto-detected when empty)")
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
	if *publicIPv6 != "" {
		cfg.PublicIPv6 = strings.TrimSpace(*publicIPv6)
	}
	if cfg.PublicIPv6 != "" {
		ip, ok := netutil.ParsePublicIPv6(cfg.PublicIPv6)
		if !ok {
			log.Fatalf("[agent] -public-ipv6 %q is not a globally routable IPv6 address", cfg.PublicIPv6)
		}
		cfg.PublicIPv6 = ip.String()
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
	// The cached hub policy is the fallback while the hub is unreachable. A
	// zero value from an old state file means IPv4-only, which is the safe
	// default for an upgraded agent.
	if state.AgentID != "" {
		cfg.IPv6Enabled = state.IPv6Enabled
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
		// An ETag with no cached catalogue (an old or hand-edited state file)
		// would make the hub answer 304 and leave the agent with an empty
		// allowlist. Force one full fetch in that case.
		if state.CatalogueETag != "" && len(state.Hosts) == 0 && len(state.Clients) == 0 {
			state.CatalogueETag = ""
		}
		cat, etag, notModified, err := hub.FetchConditional(context.Background(), state.Key, state.CatalogueETag, 0)
		switch {
		case err != nil:
			// A hub outage must not stop playback: keep the cached list and say so.
			log.Printf("[agent] catalogue fetch failed (%v) - serving the cached list", err)
		case notModified:
			log.Printf("[agent] catalogue unchanged at %s", state.CatalogueETag)
		default:
			state.Hosts = cat.Hosts
			state.Clients = cat.Clients
			state.SyncedAt = time.Now().UTC()
			if etag != "" {
				state.CatalogueETag = etag
			}
			if cat.Country != "" {
				state.Country = cat.Country
			}
			if cat.IPv6Enabled != nil {
				cfg.IPv6Enabled = *cat.IPv6Enabled
				state.IPv6Enabled = *cat.IPv6Enabled
			}
			log.Printf("[agent] catalogue synced: %d hosts, %d clients · IPv6 %s",
				len(cat.Hosts), len(cat.Clients), onOff(cfg.IPv6Enabled))
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

	// Auto-detect only after the hub policy is known. A disabled agent must not
	// probe for IPv6 at all; an enabled one probes once if it has no manual
	// address.
	if cfg.IPv6Enabled && cfg.PublicIPv6 == "" {
		if detected := detectPublicIPv6(); detected != "" {
			cfg.PublicIPv6 = detected
			log.Printf("[agent] auto-detected public IPv6: %s", detected)
		} else {
			log.Printf("[agent] no public IPv6 detected; proxied AAAA for this agent stays sunk")
		}
	}
	if !cfg.IPv6Enabled {
		log.Printf("[agent] IPv6 disabled by the hub; listening IPv4-only and reporting no IPv6")
	}
	if err := agent.SaveState(*statePath, state); err != nil {
		log.Printf("[agent] could not write state: %v", err)
	}

	srv := agent.NewServer(cfg)

	// applyIPv6Policy is the live hub control. It rebinds the listeners before
	// updating the reported policy, so "off" really stops accepting IPv6
	// connections rather than only hiding the address from DNS.
	applyIPv6Policy := func(enabled bool) {
		if enabled == cfg.IPv6Enabled {
			return
		}
		if err := srv.SetIPv6Enabled(enabled); err != nil {
			log.Printf("[agent] could not apply hub IPv6 policy %s: %v", onOff(enabled), err)
			return
		}
		cfg.IPv6Enabled = enabled
		if enabled && cfg.PublicIPv6 == "" {
			if detected := detectPublicIPv6(); detected != "" {
				cfg.PublicIPv6 = detected
				log.Printf("[agent] auto-detected public IPv6: %s", detected)
			} else {
				log.Printf("[agent] no public IPv6 detected; proxied AAAA for this agent stays sunk")
			}
		}
		state.IPv6Enabled = enabled
		if err := agent.SaveState(*statePath, state); err != nil {
			log.Printf("[agent] could not write state: %v", err)
		}
		log.Printf("[agent] hub set IPv6 %s", onOff(enabled))
	}

	// The startup line is the whole configuration in one place. An agent that
	// refuses everything because it synced an empty list looks identical to one
	// that is working until you read these counts, so they are printed, not
	// inferred.
	displayIPv6 := "-"
	if cfg.IPv6Enabled {
		displayIPv6 = orDash(cfg.PublicIPv6)
	}
	log.Printf("[agent] %s · country %s · IPv6 %s · public IPv6 %s · listening %s · dialling http :%d / https :%d",
		version.Display(), orDash(cfg.Country), onOff(cfg.IPv6Enabled),
		displayIPv6, cfg.Listen, cfg.HTTPPort, cfg.HTTPSPort)
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

	// Catalogue control plane. One long-poll is in flight at a time; the main
	// loop owns every write to state/cfg, so the fetch goroutine only carries a
	// request in and a result out. A 304 costs no catalogue computation on the
	// hub and no apply work here.
	type catalogueRequest struct {
		etag string
		wait time.Duration
	}
	type catalogueResult struct {
		cat         agent.Catalogue
		etag        string
		notModified bool
		err         error
	}
	var (
		syncReq    chan catalogueRequest
		syncRes    chan catalogueResult
		syncCtx    context.Context
		cancelSync context.CancelFunc
	)
	if hub != nil && state.Key != "" {
		syncCtx, cancelSync = context.WithCancel(context.Background())
		syncReq = make(chan catalogueRequest, 1)
		syncRes = make(chan catalogueResult, 1)
		go func() {
			for req := range syncReq {
				cat, etag, notModified, err := hub.FetchConditional(syncCtx, state.Key, req.etag, req.wait)
				select {
				case syncRes <- catalogueResult{cat: cat, etag: etag, notModified: notModified, err: err}:
				case <-syncCtx.Done():
					return
				}
			}
		}()
		defer func() {
			cancelSync()
			close(syncReq)
		}()
	}

	applyCatalogue := func(cat agent.Catalogue, etag string) {
		// Applied in place rather than rebuilding the server: a resync must not
		// drop connections that are mid-stream.
		srv.UpdateAllowlists(cat.Hosts, cat.Clients)
		state.Hosts, state.Clients = cat.Hosts, cat.Clients
		state.SyncedAt = time.Now().UTC()
		if etag != "" {
			state.CatalogueETag = etag
		}
		if cat.Country != "" {
			state.Country = cat.Country
		}
		if cat.IPv6Enabled != nil {
			applyIPv6Policy(*cat.IPv6Enabled)
		}
		if err := agent.SaveState(*statePath, state); err != nil {
			log.Printf("[agent] could not write state: %v", err)
		}
		log.Printf("[agent] catalogue resynced: %d hosts, %d clients · IPv6 %s · %s",
			len(cat.Hosts), len(cat.Clients), onOff(cfg.IPv6Enabled), orDash(state.CatalogueETag))
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	// A periodic snapshot makes the per-host split visible without waiting for a
	// heartbeat, which matters when the hub is not running yet.
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	var syncTimer *time.Timer
	var syncTick <-chan time.Time
	if syncReq != nil {
		syncTimer = time.NewTimer(0)
		syncTick = syncTimer.C
		defer syncTimer.Stop()
	}
	beatTimer := time.NewTimer(jitteredInterval(agentHeartbeatBase, 0))
	defer beatTimer.Stop()
	syncFailures, heartbeatFailures := 0, 0
	syncInFlight := false
	lastFullSync := time.Now()

	for {
		select {
		case <-stop:
			_ = srv.Close()
			logSummary(srv)
			return

		case <-ticker.C:
			logSummary(srv)

		case <-beatTimer.C:
			if hub != nil && state.Key != "" {
				in, out := srv.Counters().Totals()
				publicIPv6 := ""
				if cfg.IPv6Enabled {
					publicIPv6 = cfg.PublicIPv6
				}
				hb, err := hub.Heartbeat(state.Key, version.Display(), advertisedPort(cfg.Listen), in, out, publicIPv6, state.CatalogueETag)
				if err != nil {
					// Logged, never fatal: a hub that is down cannot be allowed to
					// stop a relay that is working.
					heartbeatFailures++
					log.Printf("[agent] heartbeat failed: %v", err)
				} else {
					heartbeatFailures = 0
					if hb.IPv6Enabled != nil {
						applyIPv6Policy(*hb.IPv6Enabled)
					}
					// If the long-poll dropped during a change, the heartbeat is
					// the fallback that notices the agent is behind.
					if hb.ConfigRevision != "" && hb.ConfigRevision != state.CatalogueETag && !syncInFlight && syncTimer != nil {
						syncTimer.Reset(0)
					}
					_, _, _, _ = srv.Stats()
				}
			}
			beatTimer.Reset(jitteredInterval(agentHeartbeatBase, heartbeatFailures))

		case <-syncTick:
			if syncInFlight {
				continue
			}
			req := catalogueRequest{etag: state.CatalogueETag, wait: 25 * time.Second}
			// A full fetch every five minutes catches a hub-side change whose
			// notifier was missed; the normal path is ETag/304.
			if time.Since(lastFullSync) >= 5*time.Minute {
				req.etag = ""
				lastFullSync = time.Now()
			}
			syncInFlight = true
			select {
			case syncReq <- req:
			default:
				// The worker is still busy despite syncInFlight; retry shortly.
				syncInFlight = false
				if syncTimer != nil {
					syncTimer.Reset(time.Second)
				}
			}

		case res := <-syncRes:
			syncInFlight = false
			if res.err != nil {
				syncFailures++
				log.Printf("[agent] catalogue resync failed (%v) - keeping the current list", res.err)
				if syncTimer != nil {
					syncTimer.Reset(jitteredInterval(agentSyncBase, syncFailures))
				}
				continue
			}
			syncFailures = 0
			if !res.notModified {
				applyCatalogue(res.cat, res.etag)
			}
			// An old hub has no ETag: fall back to the periodic pull rather than
			// re-fetching the same document in a hot loop.
			if res.etag == "" {
				if syncTimer != nil {
					syncTimer.Reset(jitteredInterval(agentSyncBase, 0))
				}
			} else if syncTimer != nil {
				syncTimer.Reset(0)
			}
		}
	}
}

const (
	agentHeartbeatBase  = 30 * time.Second
	agentSyncBase       = 60 * time.Second
	agentSyncMaxBackoff = 5 * time.Minute
	agentSyncMaxShift   = 6
)

// jitteredInterval spreads heartbeats and catalogue pulls by ±20%, and backs
// off exponentially after a failure. Without the jitter, every agent that was
// running when the hub restarted reconnects on the same tick and the control
// plane sees a self-inflicted stampede.
func jitteredInterval(base time.Duration, failures int) time.Duration {
	d := base
	if failures > 0 {
		shift := failures
		if shift > agentSyncMaxShift {
			shift = agentSyncMaxShift
		}
		d = base << shift
		if d > agentSyncMaxBackoff {
			d = agentSyncMaxBackoff
		}
	}
	spread := d / 5
	if spread <= 0 {
		return d
	}
	return d - spread + rand.N(2*spread+1)
}

// detectPublicIPv6 asks an IPv6-only echo endpoint for this host's address. The
// dialer is forced to tcp6, so an agent with no IPv6 route fails quickly rather
// than silently reporting an IPv4 answer as if it were IPv6. The total budget is
// short because this runs at startup and must not hold the relay down.
func detectPublicIPv6() string {
	endpoints := []string{
		"https://api6.ipify.org",
		"https://ipv6.icanhazip.com",
		"https://ipv6.ifconfig.co",
	}
	dialer := &net.Dialer{Timeout: 3 * time.Second}
	client := &http.Client{
		Timeout: 4 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return dialer.DialContext(ctx, "tcp6", addr)
			},
			TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12},
		},
	}
	deadline := time.Now().Add(6 * time.Second)
	for _, ep := range endpoints {
		if time.Now().After(deadline) {
			break
		}
		resp, err := client.Get(ep)
		if err != nil {
			continue
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 128))
		_ = resp.Body.Close()
		if readErr != nil || resp.StatusCode != http.StatusOK {
			continue
		}
		if ip, ok := netutil.ParsePublicIPv6(string(body)); ok {
			return ip.String()
		}
	}
	return ""
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
	refusedIP := srv.RefusedIP()
	in, out := srv.Counters().Totals()
	log.Printf("[agent] connections: %d active · %d total · %d refused · %d bare-IP · %d unreadable · traffic up %s down %s",
		active, total, refused, refusedIP, nohost, humanBytes(in), humanBytes(out))
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

func onOff(v bool) string {
	if v {
		return "on"
	}
	return "off"
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
