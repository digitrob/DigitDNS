package web

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"time"

	"digitdns/internal/geoip"
	"digitdns/internal/httpx"
)

// SetGeoIP attaches the country database. A nil service is valid — the lighter
// test harnesses never set one, and the handlers then answer 503 rather than
// pretending a database exists.
func (ws *WebServer) SetGeoIP(service *geoip.Service) {
	ws.geoIP = service
}

// countryOfServer resolves the public address the settings record advertises.
// It is cheap: the database is in memory and a lookup is a prefix search.
func (ws *WebServer) countryOfServer() string {
	if ws.geoIP == nil {
		return ""
	}
	ip := net.ParseIP(strings.TrimSpace(ws.settings.GetPublicIP()))
	if ip == nil {
		return ""
	}
	return ws.geoIP.Country(ip)
}

func (ws *WebServer) geoIPStatus() geoip.Status {
	if ws.geoIP == nil {
		return geoip.Status{Error: "the GeoIP database is not available on this server"}
	}
	status := ws.geoIP.Status()
	status.Country = ws.countryOfServer()
	return status
}

// handleGeoIPStatus reports the database's age, source and the country resolved
// for the server's own address. The Settings card renders it next to the flag.
func (ws *WebServer) handleGeoIPStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		httpx.WriteMethodNotAllowed(w, "GET, HEAD")
		return
	}
	if ws.geoIP == nil {
		httpx.WriteJSONError(w, http.StatusServiceUnavailable, "the GeoIP database is not available on this server")
		return
	}
	_ = json.NewEncoder(w).Encode(ws.geoIPStatus())
}

// handleGeoIPUpdate performs a manual refresh. It is the same call the daily
// loop makes; the download is bounded by a timeout so a slow mirror cannot hold
// the request open indefinitely.
func (ws *WebServer) handleGeoIPUpdate(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		httpx.WriteMethodNotAllowed(w, "POST")
		return
	}
	if ws.geoIP == nil {
		httpx.WriteJSONError(w, http.StatusServiceUnavailable, "the GeoIP database is not available on this server")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	if err := ws.geoIP.Update(ctx); err != nil {
		httpx.WriteJSONError(w, http.StatusBadGateway, err.Error())
		return
	}
	_ = json.NewEncoder(w).Encode(ws.geoIPStatus())
}
