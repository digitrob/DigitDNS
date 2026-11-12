package web

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"
	"time"

	"digitdns/internal/database"
	"digitdns/internal/httpx"
	"digitdns/internal/service"
)

// Hub-side egress endpoints.
//
// These are panel-only and admin-gated, unlike the agent-facing /agent/ routes.
// A SOCKS5 password and a WireGuard private key are write-only: the API accepts
// them on save and never returns them. The UI only learns whether one is set.

// SetEgressRegistry installs the live egress registry shared with the DNS and
// SNI paths.
func (ws *WebServer) SetEgressRegistry(r *service.EgressRegistry) {
	ws.egressRegistry = r
}

// refreshEgresses rebuilds the registry from the database after a mutation.
func (ws *WebServer) refreshEgresses() error {
	if ws.egressRegistry == nil {
		return nil
	}
	return ws.egressRegistry.Reload()
}

func (ws *WebServer) egressViews() ([]service.EgressView, error) {
	if ws.egressRegistry != nil {
		return ws.egressRegistry.Views(), nil
	}
	// Lighter test harnesses may not wire a registry; still list the stored
	// records so the page works, with an unknown health state.
	items, err := ws.db.ListEgresses()
	if err != nil {
		return nil, err
	}
	out := make([]service.EgressView, 0, len(items))
	for _, item := range items {
		out = append(out, service.EgressView{
			ID: item.ID, Kind: item.Kind, Name: item.Name, Country: item.Country,
			Enabled: item.Enabled, FailClosed: item.FailClosed, Address: item.Address,
			Username: item.Username, PasswordSet: item.Password != "", ConfigSet: item.Config != "",
			Resolve: item.Resolve, Health: "unknown",
		})
	}
	return out, nil
}

// handleEgresses is the collection endpoint: GET lists, POST creates.
func (ws *WebServer) handleEgresses(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		views, err := ws.egressViews()
		if err != nil {
			httpx.WriteJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeEgressJSON(w, http.StatusOK, map[string]any{"egresses": views})

	case http.MethodPost:
		req, ok := decodeEgressRequest(w, r)
		if !ok {
			return
		}
		item, err := ws.egressFromRequest(database.Egress{}, req, true)
		if err != nil {
			httpx.WriteJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := ws.db.SaveEgress(&item); err != nil {
			httpx.WriteJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if err := ws.refreshEgresses(); err != nil {
			httpx.WriteJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		view, _ := ws.egressView(item.ID)
		writeEgressJSON(w, http.StatusCreated, map[string]any{"egress": view})

	default:
		httpx.WriteMethodNotAllowed(w, "GET, POST")
	}
}

// handleEgressByID is the item endpoint. The optional suffix is /test,
// /enable or /disable.
func (ws *WebServer) handleEgressByID(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimPrefix(r.URL.Path, "/api/egresses/")
	parts := strings.SplitN(strings.Trim(rest, "/"), "/", 2)
	id := parts[0]
	action := ""
	if len(parts) == 2 {
		action = parts[1]
	}
	if id == "" {
		httpx.WriteJSONError(w, http.StatusBadRequest, "an egress id is required")
		return
	}

	switch r.Method {
	case http.MethodGet:
		if action != "" {
			httpx.WriteJSONError(w, http.StatusBadRequest, "unexpected action "+action)
			return
		}
		view, ok := ws.egressView(id)
		if !ok {
			httpx.WriteJSONError(w, http.StatusNotFound, "egress not found")
			return
		}
		writeEgressJSON(w, http.StatusOK, map[string]any{"egress": view})

	case http.MethodPut:
		if action != "" {
			httpx.WriteJSONError(w, http.StatusBadRequest, "unexpected action "+action)
			return
		}
		current, ok, err := ws.db.GetEgress(id)
		if err != nil {
			httpx.WriteJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if !ok {
			httpx.WriteJSONError(w, http.StatusNotFound, "egress not found")
			return
		}
		req, ok := decodeEgressRequest(w, r)
		if !ok {
			return
		}
		item, err := ws.egressFromRequest(current, req, false)
		if err != nil {
			httpx.WriteJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := ws.db.SaveEgress(&item); err != nil {
			httpx.WriteJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if err := ws.refreshEgresses(); err != nil {
			httpx.WriteJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		view, _ := ws.egressView(id)
		writeEgressJSON(w, http.StatusOK, map[string]any{"egress": view})

	case http.MethodDelete:
		if action != "" {
			httpx.WriteJSONError(w, http.StatusBadRequest, "unexpected action "+action)
			return
		}
		if err := ws.db.DeleteEgress(id); err != nil {
			httpx.WriteJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if err := ws.refreshEgresses(); err != nil {
			httpx.WriteJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.WriteHeader(http.StatusNoContent)

	case http.MethodPost:
		switch action {
		case "test":
			ws.handleEgressTest(w, id)
		case "enable", "disable":
			current, ok, err := ws.db.GetEgress(id)
			if err != nil {
				httpx.WriteJSONError(w, http.StatusInternalServerError, err.Error())
				return
			}
			if !ok {
				httpx.WriteJSONError(w, http.StatusNotFound, "egress not found")
				return
			}
			current.Enabled = action == "enable"
			if err := ws.db.SaveEgress(&current); err != nil {
				httpx.WriteJSONError(w, http.StatusInternalServerError, err.Error())
				return
			}
			if err := ws.refreshEgresses(); err != nil {
				httpx.WriteJSONError(w, http.StatusInternalServerError, err.Error())
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			httpx.WriteJSONError(w, http.StatusBadRequest, "expected /test, /enable or /disable")
		}

	default:
		httpx.WriteMethodNotAllowed(w, "DELETE, GET, POST, PUT")
	}
}

func (ws *WebServer) egressView(id string) (service.EgressView, bool) {
	views, err := ws.egressViews()
	if err != nil {
		return service.EgressView{}, false
	}
	for _, view := range views {
		if view.ID == id {
			return view, true
		}
	}
	return service.EgressView{}, false
}

type egressRequest struct {
	Kind       *string `json:"kind"`
	Name       *string `json:"name"`
	Country    *string `json:"country"`
	Address    *string `json:"address"`
	Username   *string `json:"username"`
	Password   *string `json:"password"`
	Config     *string `json:"config"`
	Resolve    *string `json:"resolve"`
	TestHost   *string `json:"test_host"`
	FailClosed *bool   `json:"fail_closed"`
	Enabled    *bool   `json:"enabled"`
}

func decodeEgressRequest(w http.ResponseWriter, r *http.Request) (egressRequest, bool) {
	var req egressRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRequestBody)).Decode(&req); err != nil {
		httpx.WriteJSONError(w, http.StatusBadRequest, "the request body could not be read as JSON")
		return req, false
	}
	return req, true
}

// egressFromRequest merges a request over an existing record. Secrets are only
// replaced when the field is present: nil keeps the stored value, an empty
// string clears it.
func (ws *WebServer) egressFromRequest(current database.Egress, req egressRequest, create bool) (database.Egress, error) {
	item := current
	if req.Kind != nil {
		item.Kind = strings.ToLower(strings.TrimSpace(*req.Kind))
	}
	if create {
		if item.Kind == "" {
			item.Kind = database.EgressKindSOCKS5
		}
		// Fail-closed is the safe default. The UI always sends the switch, but
		// an API client that omits it still gets the protective behaviour.
		item.Enabled = true
		item.FailClosed = true
	}
	if req.Name != nil {
		item.Name = strings.TrimSpace(*req.Name)
	}
	if req.Country != nil {
		item.Country = strings.ToUpper(strings.TrimSpace(*req.Country))
	}
	if req.Address != nil {
		item.Address = strings.TrimSpace(*req.Address)
	}
	if req.Username != nil {
		item.Username = strings.TrimSpace(*req.Username)
	}
	if req.Password != nil {
		item.Password = *req.Password
	}
	if req.Config != nil {
		item.Config = *req.Config
	}
	if req.Resolve != nil {
		item.Resolve = strings.ToLower(strings.TrimSpace(*req.Resolve))
	}
	if req.TestHost != nil {
		item.TestHost = strings.TrimSpace(*req.TestHost)
	}
	if req.FailClosed != nil {
		item.FailClosed = *req.FailClosed
	}
	if req.Enabled != nil {
		item.Enabled = *req.Enabled
	}

	switch item.Kind {
	case database.EgressKindSOCKS5:
		if item.Name == "" {
			return item, errors.New("a name is required")
		}
		host, port, err := net.SplitHostPort(item.Address)
		if err != nil || strings.TrimSpace(host) == "" || strings.TrimSpace(port) == "" {
			return item, errors.New("address must be host:port")
		}
		if item.Resolve == "" {
			item.Resolve = "remote"
		}
		if item.Resolve != "remote" && item.Resolve != "local" {
			return item, errors.New("resolve must be remote or local")
		}
	case database.EgressKindWireGuard:
		if item.Name == "" {
			return item, errors.New("a name is required")
		}
		if strings.TrimSpace(item.Config) == "" {
			return item, errors.New("a WireGuard configuration is required")
		}
	default:
		return item, errors.New("kind must be socks5 or wireguard")
	}
	if item.Country != "" && len(item.Country) != 2 {
		return item, errors.New("country must be a two-letter code")
	}
	return item, nil
}

func (ws *WebServer) handleEgressTest(w http.ResponseWriter, id string) {
	if ws.egressRegistry == nil {
		httpx.WriteJSONError(w, http.StatusServiceUnavailable, "egress testing is unavailable")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	latency, err := ws.egressRegistry.Test(ctx, id)
	if err != nil {
		writeEgressJSON(w, http.StatusOK, map[string]any{
			"ok":         false,
			"error":      err.Error(),
			"latency_ms": 0,
		})
		return
	}
	writeEgressJSON(w, http.StatusOK, map[string]any{
		"ok":         true,
		"latency_ms": float64(latency.Microseconds()) / 1000.0,
	})
}

func writeEgressJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
