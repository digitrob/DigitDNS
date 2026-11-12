package web

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"digitdns/internal/httpx"
	"digitdns/internal/service"
	"github.com/miekg/dns"
)

// SetQueryLogService installs the optional persistent query history.
func (ws *WebServer) SetQueryLogService(q *service.QueryLogService) { ws.queryLog = q }

// SetDNSQueryAddr installs the local resolver address used by the Tools DNS
// lookup. Empty means the lookup endpoint reports that it is unavailable.
func (ws *WebServer) SetDNSQueryAddr(addr string) { ws.dnsQueryAddr = addr }

// handleToolsDNS queries this server's own resolver with the requested record
// type and, optionally, the DNSSEC OK bit. The response includes the AD flag so
// the operator can tell a validated answer from a merely signed one.
func (ws *WebServer) handleToolsDNS(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		httpx.WriteMethodNotAllowed(w, "GET, HEAD")
		return
	}
	name := strings.TrimSpace(r.URL.Query().Get("name"))
	if name == "" {
		httpx.WriteJSONError(w, http.StatusBadRequest, "name is required")
		return
	}
	qtype, ok := dnsType(r.URL.Query().Get("type"))
	if !ok {
		httpx.WriteJSONError(w, http.StatusBadRequest, "unsupported record type")
		return
	}
	if ws.dnsQueryAddr == "" {
		httpx.WriteJSONError(w, http.StatusServiceUnavailable, "DNS query address is unavailable")
		return
	}
	do := r.URL.Query().Get("dnssec") == "1"
	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn(name), qtype)
	m.SetEdns0(4096, do)
	client := &dns.Client{Timeout: 6 * time.Second}
	resp, _, err := client.Exchange(m, ws.dnsQueryAddr)
	if err != nil {
		writeEgressJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	type answer struct {
		Name  string `json:"name"`
		Type  string `json:"type"`
		TTL   uint32 `json:"ttl"`
		Value string `json:"value"`
	}
	out := make([]answer, 0, len(resp.Answer))
	for _, rr := range resp.Answer {
		out = append(out, answer{
			Name: rr.Header().Name, Type: dns.TypeToString[rr.Header().Rrtype],
			TTL: rr.Header().Ttl, Value: strings.TrimPrefix(rr.String(), rr.Header().String()),
		})
	}
	writeEgressJSON(w, http.StatusOK, map[string]any{
		"ok": true, "name": name, "type": dns.TypeToString[qtype],
		"rcode": dns.RcodeToString[resp.Rcode], "ad": resp.AuthenticatedData,
		"dnssec": do, "answers": out,
	})
}

func dnsType(raw string) (uint16, bool) {
	raw = strings.ToUpper(strings.TrimSpace(raw))
	if raw == "" {
		raw = "A"
	}
	if t, ok := dns.StringToType[raw]; ok {
		return t, true
	}
	return 0, false
}

// handleToolsWhois uses RDAP over HTTPS, which is the modern replacement for
// port-43 WHOIS and returns structured JSON. A domain or an IP is accepted.
func (ws *WebServer) handleToolsWhois(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		httpx.WriteMethodNotAllowed(w, "GET, HEAD")
		return
	}
	query := strings.TrimSpace(r.URL.Query().Get("query"))
	if query == "" {
		httpx.WriteJSONError(w, http.StatusBadRequest, "query is required")
		return
	}
	kind := "domain"
	if ip := net.ParseIP(query); ip != nil {
		kind = "ip"
	}
	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()
	url := "https://rdap.org/" + kind + "/" + query
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		writeEgressJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	req.Header.Set("Accept", "application/rdap+json, application/json")
	resp, err := (&http.Client{Timeout: 12 * time.Second}).Do(req)
	if err != nil {
		writeEgressJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		writeEgressJSON(w, http.StatusOK, map[string]any{"ok": false, "error": fmt.Sprintf("RDAP answered %s", resp.Status)})
		return
	}
	var data map[string]any
	if err := json.Unmarshal(body, &data); err != nil {
		writeEgressJSON(w, http.StatusOK, map[string]any{"ok": false, "error": "RDAP returned invalid JSON"})
		return
	}
	writeEgressJSON(w, http.StatusOK, map[string]any{
		"ok": true, "kind": kind, "handle": data["handle"], "name": data["ldhName"],
		"status": data["status"], "events": data["events"], "nameservers": data["nameservers"],
	})
}

// handleToolsCheck is the operator's TCP reachability probe. mode selects the
// dial path: direct, the routing table for this hostname, or one named egress.
func (ws *WebServer) handleToolsCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		httpx.WriteMethodNotAllowed(w, "GET, HEAD")
		return
	}
	host := strings.TrimSpace(r.URL.Query().Get("host"))
	if host == "" || strings.Contains(host, "://") {
		httpx.WriteJSONError(w, http.StatusBadRequest, "host must be a hostname or IP address")
		return
	}
	port, err := atoiPort(r.URL.Query().Get("port"))
	if err != nil {
		httpx.WriteJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	mode := strings.TrimSpace(r.URL.Query().Get("mode"))
	if mode == "" {
		mode = "direct"
	}
	egressID := strings.TrimSpace(r.URL.Query().Get("egress"))
	clientIP := strings.TrimSpace(r.URL.Query().Get("client"))

	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	addrs, err := net.DefaultResolver.LookupHost(ctx, host)
	if err != nil {
		writeEgressJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error(), "mode": mode})
		return
	}

	label := "Direct"
	var conn net.Conn
	start := time.Now()
	switch {
	case mode == "egress":
		if egressID == "" {
			httpx.WriteJSONError(w, http.StatusBadRequest, "egress is required for egress mode")
			return
		}
		if ws.egressRegistry == nil {
			writeEgressJSON(w, http.StatusOK, map[string]any{"ok": false, "error": "egress registry is unavailable", "mode": mode})
			return
		}
		conn, err = ws.egressRegistry.Dial(ctx, egressID, host, toNetip(addrs), port)
		if v, ok := ws.egressRegistry.Lookup(egressID); ok {
			label = v.Name
		}
	case mode == "route" && ws.relayRouter != nil:
		target, _ := ws.relayRouter.LookupFor(host, clientIP)
		switch target.Kind {
		case "egress":
			if ws.egressRegistry == nil {
				writeEgressJSON(w, http.StatusOK, map[string]any{"ok": false, "error": "egress registry is unavailable", "mode": mode})
				return
			}
			conn, err = ws.egressRegistry.Dial(ctx, target.EgressID, host, toNetip(addrs), port)
			label = "Proxied via " + target.EgressName
		case "agent":
			writeEgressJSON(w, http.StatusOK, map[string]any{
				"ok": false, "mode": mode, "error": "relay agents carry proxied TLS names, not arbitrary TCP checks",
			})
			return
		default:
			conn, err = (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "tcp", net.JoinHostPort(host, fmt.Sprint(port)))
			label = "Proxied → local"
		}
	default:
		conn, err = (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "tcp", net.JoinHostPort(host, fmt.Sprint(port)))
	}
	latency := float64(time.Since(start).Microseconds()) / 1000.0
	if err != nil {
		writeEgressJSON(w, http.StatusOK, map[string]any{
			"ok": false, "error": err.Error(), "addresses": addrs, "latency_ms": latency, "mode": mode, "path": label,
		})
		return
	}
	_ = conn.Close()
	writeEgressJSON(w, http.StatusOK, map[string]any{
		"ok": true, "addresses": addrs, "latency_ms": latency, "mode": mode, "path": label,
	})
}

func atoiPort(raw string) (int, error) {
	var port int
	if _, err := fmt.Sscanf(strings.TrimSpace(raw), "%d", &port); err != nil || port < 1 || port > 65535 {
		return 0, fmt.Errorf("port must be between 1 and 65535")
	}
	return port, nil
}

func toNetip(addrs []string) []netip.Addr {
	out := make([]netip.Addr, 0, len(addrs))
	for _, raw := range addrs {
		if ip, err := netip.ParseAddr(raw); err == nil {
			out = append(out, ip)
		}
	}
	return out
}
