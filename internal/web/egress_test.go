package web

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"digitdns/internal/database"
)

// createEgress exercises the panel create flow and returns the stored id.
func createEgress(t *testing.T, ws *WebServer, body string) string {
	t.Helper()
	rec := doJSON(t, ws.handleEgresses, http.MethodPost, "/api/egresses", body, nil)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create egress: %d %s", rec.Code, rec.Body.String())
	}
	var created struct {
		Egress struct {
			ID string `json:"id"`
		} `json:"egress"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	if created.Egress.ID == "" {
		t.Fatalf("create response has no id: %s", rec.Body.String())
	}
	return created.Egress.ID
}

func TestEgressCreateDefaultsAndSecretMasking(t *testing.T) {
	ws := newAgentWebServer(t)
	id := createEgress(t, ws, `{
		"kind":"socks5","name":"NL proxy","country":"NL",
		"address":"10.0.0.5:1080","username":"alice","password":"hunter2"
	}`)

	stored, ok, err := ws.db.GetEgress(id)
	if err != nil || !ok {
		t.Fatalf("stored egress: ok=%v err=%v", ok, err)
	}
	// Fail-closed is the protective default for an API client that omits it.
	if !stored.FailClosed || !stored.Enabled {
		t.Fatalf("defaults = fail_closed:%v enabled:%v, want both true", stored.FailClosed, stored.Enabled)
	}
	if stored.Password != "hunter2" {
		t.Fatal("password was not stored")
	}

	rec := doJSON(t, ws.handleEgressByID, http.MethodGet, "/api/egresses/"+id, "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("get egress: %d %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Contains(body, "hunter2") {
		t.Fatalf("response leaked the SOCKS5 password: %s", body)
	}
	var view struct {
		Egress struct {
			PasswordSet bool   `json:"password_set"`
			FailClosed  bool   `json:"fail_closed"`
			Resolve     string `json:"resolve"`
		} `json:"egress"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if !view.Egress.PasswordSet || !view.Egress.FailClosed {
		t.Fatalf("view = %+v, want password_set and fail_closed", view.Egress)
	}
	if view.Egress.Resolve != "remote" {
		t.Fatalf("resolve = %q, want remote by default", view.Egress.Resolve)
	}
}

func TestEgressUpdateKeepsSecretsUnlessReplaced(t *testing.T) {
	ws := newAgentWebServer(t)
	id := createEgress(t, ws, `{
		"kind":"socks5","name":"NL proxy","address":"10.0.0.5:1080","password":"first"
	}`)

	// Omit password: the stored one must survive.
	rec := doJSON(t, ws.handleEgressByID, http.MethodPut, "/api/egresses/"+id,
		`{"name":"NL proxy 2","address":"10.0.0.6:1080"}`, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("update egress: %d %s", rec.Code, rec.Body.String())
	}
	stored, _, err := ws.db.GetEgress(id)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Password != "first" {
		t.Fatalf("password = %q, want the saved value", stored.Password)
	}
	if stored.Name != "NL proxy 2" || stored.Address != "10.0.0.6:1080" {
		t.Fatalf("update did not apply: %+v", stored)
	}

	// Send a new password: it replaces the old one.
	rec = doJSON(t, ws.handleEgressByID, http.MethodPut, "/api/egresses/"+id,
		`{"password":"second"}`, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("update password: %d %s", rec.Code, rec.Body.String())
	}
	stored, _, err = ws.db.GetEgress(id)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Password != "second" {
		t.Fatalf("password = %q, want the replacement", stored.Password)
	}
}

func TestEgressWireGuardConfigIsWriteOnly(t *testing.T) {
	ws := newAgentWebServer(t)
	id := createEgress(t, ws, `{
		"kind":"wireguard","name":"Home","country":"DE",
		"config":"[Interface]\nPrivateKey = aGVsbG8=\nAddress = 10.9.0.2/32\n"
	}`)

	stored, _, err := ws.db.GetEgress(id)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stored.Config, "PrivateKey") {
		t.Fatal("WireGuard config was not stored")
	}
	rec := doJSON(t, ws.handleEgressByID, http.MethodGet, "/api/egresses/"+id, "", nil)
	if strings.Contains(rec.Body.String(), "aGVsbG8=") || strings.Contains(rec.Body.String(), "PrivateKey") {
		t.Fatalf("response leaked the WireGuard configuration: %s", rec.Body.String())
	}
	var view struct {
		Egress struct {
			ConfigSet bool `json:"config_set"`
		} `json:"egress"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if !view.Egress.ConfigSet {
		t.Fatal("config_set is false after saving a configuration")
	}
}

func TestEgressValidationAndDelete(t *testing.T) {
	ws := newAgentWebServer(t)
	for _, body := range []string{
		`{"kind":"socks5","name":"bad","address":"not-a-host-port"}`,
		`{"kind":"wireguard","name":"no config"}`,
		`{"kind":"openvpn","name":"nope","address":"10.0.0.1:1"}`,
		`{"kind":"socks5","address":"10.0.0.1:1"}`,
	} {
		rec := doJSON(t, ws.handleEgresses, http.MethodPost, "/api/egresses", body, nil)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("body %s = %d, want 400 (%s)", body, rec.Code, rec.Body.String())
		}
	}

	id := createEgress(t, ws, `{"kind":"socks5","name":"ok","address":"10.0.0.1:1080"}`)
	rec := doJSON(t, ws.handleEgressByID, http.MethodDelete, "/api/egresses/"+id, "", nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete = %d, want 204 (%s)", rec.Code, rec.Body.String())
	}
	if _, ok, err := ws.db.GetEgress(id); err != nil || ok {
		t.Fatalf("egress still present after delete: ok=%v err=%v", ok, err)
	}
}

func TestEgressListNeverCarriesSecrets(t *testing.T) {
	ws := newAgentWebServer(t)
	createEgress(t, ws, `{"kind":"socks5","name":"A","address":"10.0.0.1:1080","password":"secret-one"}`)
	createEgress(t, ws, `{"kind":"wireguard","name":"B","config":"[Interface]\nPrivateKey = c2VjcmV0\nAddress = 10.9.0.2/32\n"}`)

	rec := doJSON(t, ws.handleEgresses, http.MethodGet, "/api/egresses", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("list = %d %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, secret := range []string{"secret-one", "c2VjcmV0", "PrivateKey"} {
		if strings.Contains(body, secret) {
			t.Fatalf("list leaked %q: %s", secret, body)
		}
	}
	if !strings.Contains(body, `"password_set":true`) || !strings.Contains(body, `"config_set":true`) {
		t.Fatalf("list did not report that secrets are set: %s", body)
	}
	if !strings.Contains(body, database.EgressKindSOCKS5) || !strings.Contains(body, database.EgressKindWireGuard) {
		t.Fatalf("list is missing a kind: %s", body)
	}
}
