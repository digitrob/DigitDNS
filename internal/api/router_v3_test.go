package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"digitdns/internal/service"
)

// fakeV3Backend is a stand-in for the web-layer backend so the router's
// delegation contract can be tested without constructing a whole web server.
type fakeV3Backend struct {
	loginToken string
	loginTwoFA bool
	loginErr   error
	loggedOut  string
	ticket     string
}

func (f *fakeV3Backend) Login(r *http.Request, username, password, code string) (string, bool, error) {
	if f.loginErr != nil {
		return "", f.loginTwoFA, f.loginErr
	}
	return f.loginToken, false, nil
}
func (f *fakeV3Backend) Logout(token string)          { f.loggedOut = token }
func (f *fakeV3Backend) AuthSnapshot() map[string]any { return map[string]any{"totp_enabled": false} }
func (f *fakeV3Backend) Setup2FA(r *http.Request, u, p string) (map[string]any, error) {
	return map[string]any{"secret": "ABC", "otpauth_uri": "otpauth://x"}, nil
}
func (f *fakeV3Backend) Enable2FA(r *http.Request, u, p, c string) (map[string]any, error) {
	return map[string]any{"success": true}, nil
}
func (f *fakeV3Backend) Disable2FA(r *http.Request, u, p, c string) (map[string]any, error) {
	return map[string]any{"success": true}, nil
}
func (f *fakeV3Backend) Unlock(r *http.Request, ip string) (map[string]any, error) {
	return map[string]any{"success": true, "cleared": 1}, nil
}
func (f *fakeV3Backend) SSETicket(sessionKey string) (map[string]any, error) {
	return map[string]any{"ticket": f.ticket, "expires_in": 60}, nil
}
func (f *fakeV3Backend) TicketAuthorized(r *http.Request) bool {
	return r.URL.Query().Get("ticket") == f.ticket
}
func (f *fakeV3Backend) Config() map[string]any       { return map[string]any{"version": "x"} }
func (f *fakeV3Backend) AccessConfig() map[string]any { return map[string]any{"allow_all": true} }
func (f *fakeV3Backend) SaveAccess(r *http.Request, b map[string]any) (map[string]any, error) {
	return map[string]any{"success": true}, nil
}
func (f *fakeV3Backend) Rules() map[string]any { return map[string]any{"custom_proxied": []string{}} }
func (f *fakeV3Backend) SaveRules(r *http.Request, b map[string]any) (map[string]any, error) {
	return map[string]any{"success": true}, nil
}
func (f *fakeV3Backend) Settings() (map[string]any, error) {
	return map[string]any{"api_key": "k"}, nil
}
func (f *fakeV3Backend) SaveSettings(r *http.Request, b map[string]any) (map[string]any, error) {
	return map[string]any{"api_key": "k"}, nil
}
func (f *fakeV3Backend) RegenerateAPIKey(r *http.Request, p, c string) (map[string]any, error) {
	return map[string]any{"api_key": "new"}, nil
}
func (f *fakeV3Backend) RegenerateAdminPath(r *http.Request, c bool) (map[string]any, error) {
	return map[string]any{"admin_path": "deadbeef"}, nil
}
func (f *fakeV3Backend) Subscription() map[string]any { return map[string]any{"enabled": false} }
func (f *fakeV3Backend) SaveSubscription(r *http.Request, b map[string]any) (map[string]any, error) {
	return map[string]any{"success": true}, nil
}
func (f *fakeV3Backend) IssueTLS(r *http.Request, b map[string]any) (map[string]any, error) {
	return map[string]any{"success": true, "issuing": false}, nil
}
func (f *fakeV3Backend) TLSAutoRenew(r *http.Request, b map[string]any) (map[string]any, error) {
	return map[string]any{"success": true}, nil
}
func (f *fakeV3Backend) ACMEStatus() map[string]any { return map[string]any{"running": false} }
func (f *fakeV3Backend) DiagnosticsTargets() map[string]any {
	return map[string]any{"targets": []any{}}
}
func (f *fakeV3Backend) AddDiagnosticsTarget(r *http.Request, b map[string]any) (map[string]any, error) {
	return map[string]any{"targets": []any{}}, nil
}
func (f *fakeV3Backend) DeleteDiagnosticsTarget(r *http.Request, h string) (map[string]any, error) {
	return map[string]any{"targets": []any{}}, nil
}
func (f *fakeV3Backend) CustomGroups() ([]map[string]any, error) {
	return []map[string]any{{"id": "g1"}}, nil
}
func (f *fakeV3Backend) CreateCustomGroup(r *http.Request, b map[string]any) (map[string]any, error) {
	return map[string]any{"id": "g2"}, nil
}
func (f *fakeV3Backend) UpdateCustomGroup(r *http.Request, id string, b map[string]any) (map[string]any, error) {
	return map[string]any{"id": id}, nil
}
func (f *fakeV3Backend) DeleteCustomGroup(r *http.Request, id string) (map[string]any, error) {
	return map[string]any{"success": true}, nil
}
func (f *fakeV3Backend) UpdateCheck(r *http.Request) (map[string]any, error) {
	return map[string]any{"update_available": false}, nil
}
func (f *fakeV3Backend) UpdateApply(r *http.Request) (map[string]any, error) {
	return map[string]any{"started": true}, nil
}
func (f *fakeV3Backend) UpdateStatus() map[string]any { return map[string]any{"running": false} }
func (f *fakeV3Backend) Benchmark() (map[string]any, error) {
	return map[string]any{"started": true}, nil
}

func v3Request(mux *http.ServeMux, method, path, key string, body any) *httptest.ResponseRecorder {
	var rdr *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	} else {
		rdr = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, rdr)
	if key != "" {
		req.Header.Set("X-API-Key", key)
	}
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	return w
}

func TestV3_RequiresCredential(t *testing.T) {
	apiInst, _, _, cleanup := setupTestAPI(t)
	defer cleanup()
	mux := http.NewServeMux()
	apiInst.RegisterRoutes(mux)

	// No credential: 401.
	w := v3Request(mux, http.MethodGet, "/api/v3/clients", "", nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("v3 clients without credential = %d, want 401", w.Code)
	}
	// The master key works.
	w = v3Request(mux, http.MethodGet, "/api/v3/clients", "digitdns_live_testkey123", nil)
	if w.Code != http.StatusOK {
		t.Fatalf("v3 clients with key = %d, want 200: %s", w.Code, w.Body.String())
	}
}

func TestV3_VersionNoAuthLoopbackOnly(t *testing.T) {
	apiInst, _, _, cleanup := setupTestAPI(t)
	defer cleanup()
	mux := http.NewServeMux()
	apiInst.RegisterRoutes(mux)

	// Non-loopback caller on a localhost bind: 403.
	req := httptest.NewRequest(http.MethodGet, "/api/v3/version", nil)
	req.RemoteAddr = "203.0.113.9:5555"
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("v3 version from non-loopback = %d, want 403", w.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v3/version", nil)
	req.RemoteAddr = "127.0.0.1:5555"
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("v3 version from loopback = %d, want 200", w.Code)
	}
	var body map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body["api"] != "v3" {
		t.Errorf("api field = %v, want v3", body["api"])
	}
}

func TestV3_LoginMintsSessionAndAcceptsIt(t *testing.T) {
	apiInst, _, _, cleanup := setupTestAPI(t)
	defer cleanup()
	// Wire a real session manager so a minted session validates, exactly as the
	// web build does. The fake's Login returns a token this manager already
	// holds, standing in for the web layer creating the session itself.
	mgr := service.NewSessionManager(time.Hour)
	apiInst.sessions = mgr
	token := mgr.Create()
	fake := &fakeV3Backend{loginToken: token}
	apiInst.SetV3Backend(fake)
	mux := http.NewServeMux()
	apiInst.RegisterRoutes(mux)

	w := v3Request(mux, http.MethodPost, "/api/v3/auth/login", "", map[string]any{
		"username": "admin", "password": "pw",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("login = %d, want 200: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["token"] != token {
		t.Fatalf("login token = %v, want %q", resp["token"], token)
	}

	// The minted session authenticates a v3 route.
	if w := v3Request(mux, http.MethodGet, "/api/v3/auth/me", token, nil); w.Code != http.StatusOK {
		t.Fatalf("me with session = %d, want 200: %s", w.Code, w.Body.String())
	}

	// Logout revokes it: the second call with the same token is 401.
	w = v3Request(mux, http.MethodPost, "/api/v3/auth/logout", token, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("logout = %d, want 200: %s", w.Code, w.Body.String())
	}
	if mgr.Validate(token) {
		t.Error("logout did not revoke the session")
	}
}

func TestV3_LoginTwoFactorRequired(t *testing.T) {
	apiInst, _, _, cleanup := setupTestAPI(t)
	defer cleanup()
	apiInst.SetV3Backend(&fakeV3Backend{loginTwoFA: true, loginErr: V3Error(ErrV3Unauthorized, "invalid credentials")})
	mux := http.NewServeMux()
	apiInst.RegisterRoutes(mux)

	w := v3Request(mux, http.MethodPost, "/api/v3/auth/login", "", map[string]any{
		"username": "admin", "password": "pw",
	})
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("2FA-required login = %d, want 401", w.Code)
	}
	var body map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	if body["twofactor_required"] != true {
		t.Errorf("twofactor_required = %v, want true", body["twofactor_required"])
	}
}

func TestV3_LoginStrictDecode(t *testing.T) {
	apiInst, _, _, cleanup := setupTestAPI(t)
	defer cleanup()
	apiInst.SetV3Backend(&fakeV3Backend{loginToken: "x"})
	mux := http.NewServeMux()
	apiInst.RegisterRoutes(mux)

	w := v3Request(mux, http.MethodPost, "/api/v3/auth/login", "", map[string]any{
		"username": "admin", "password": "pw", "typo_field": 1,
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("login with unknown field = %d, want 400", w.Code)
	}
}

func TestV3_BackendDependentRoutesUnavailableWithoutBackend(t *testing.T) {
	apiInst, _, _, cleanup := setupTestAPI(t)
	defer cleanup()
	mux := http.NewServeMux()
	apiInst.RegisterRoutes(mux)

	routes := []struct {
		method, path string
		body         any
	}{
		{http.MethodGet, "/api/v3/config", nil},
		{http.MethodGet, "/api/v3/access", nil},
		{http.MethodGet, "/api/v3/rules", nil},
		{http.MethodGet, "/api/v3/settings", nil},
		{http.MethodGet, "/api/v3/tls/acme/status", nil},
		{http.MethodGet, "/api/v3/diagnostics/targets", nil},
		{http.MethodGet, "/api/v3/custom-groups", nil},
		{http.MethodGet, "/api/v3/update/status", nil},
	}
	for _, rt := range routes {
		w := v3Request(mux, rt.method, rt.path, "digitdns_live_testkey123", rt.body)
		if w.Code != http.StatusServiceUnavailable {
			t.Errorf("%s %s without backend = %d, want 503", rt.method, rt.path, w.Code)
		}
	}
}

func TestV3_DelegatedRoutesUseBackend(t *testing.T) {
	apiInst, _, _, cleanup := setupTestAPI(t)
	defer cleanup()
	fake := &fakeV3Backend{ticket: "tick123"}
	apiInst.SetV3Backend(fake)
	mux := http.NewServeMux()
	apiInst.RegisterRoutes(mux)

	key := "digitdns_live_testkey123"

	if w := v3Request(mux, http.MethodGet, "/api/v3/custom-groups", key, nil); w.Code != http.StatusOK {
		t.Fatalf("custom-groups = %d, want 200: %s", w.Code, w.Body.String())
	}
	if w := v3Request(mux, http.MethodPost, "/api/v3/custom-groups", key, map[string]any{"name": "g"}); w.Code != http.StatusCreated {
		t.Fatalf("create custom-group = %d, want 201", w.Code)
	}
	if w := v3Request(mux, http.MethodPut, "/api/v3/custom-groups/g1", key, map[string]any{"name": "g"}); w.Code != http.StatusOK {
		t.Fatalf("update custom-group = %d, want 200", w.Code)
	}
	if w := v3Request(mux, http.MethodDelete, "/api/v3/custom-groups/g1", key, nil); w.Code != http.StatusOK {
		t.Fatalf("delete custom-group = %d, want 200", w.Code)
	}
	if w := v3Request(mux, http.MethodPost, "/api/v3/tls/issue", key, map[string]any{"domain": "x.example.com"}); w.Code != http.StatusOK {
		t.Fatalf("tls issue = %d, want 200", w.Code)
	}
	if w := v3Request(mux, http.MethodGet, "/api/v3/update/check", key, nil); w.Code != http.StatusOK {
		t.Fatalf("update check = %d, want 200", w.Code)
	}

	// SSE ticket: POST mints, then the stream route accepts ?ticket= (but streams
	// forever, so we only check the ticket mint here).
	w := v3Request(mux, http.MethodPost, "/api/v3/auth/sse-ticket", key, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("sse-ticket = %d, want 200: %s", w.Code, w.Body.String())
	}
	var tk map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &tk)
	if tk["ticket"] != "tick123" {
		t.Errorf("ticket = %v, want tick123", tk["ticket"])
	}
}

func TestV3_ClientCRUD(t *testing.T) {
	apiInst, _, _, cleanup := setupTestAPI(t)
	defer cleanup()
	mux := http.NewServeMux()
	apiInst.RegisterRoutes(mux)
	key := "digitdns_live_testkey123"

	w := v3Request(mux, http.MethodPost, "/api/v3/clients", key, map[string]any{
		"name": "v3-sub", "validity_days": 30, "allowed_ips": []string{"198.51.100.5"},
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create client = %d, want 201: %s", w.Code, w.Body.String())
	}
	var created v3ClientDTO
	_ = json.Unmarshal(w.Body.Bytes(), &created)
	if created.ID == "" {
		t.Fatal("created client has no id")
	}
	if len(created.AllowedIPs) != 1 {
		t.Errorf("allowed_ips = %v, want one entry", created.AllowedIPs)
	}

	// PATCH rename.
	w = v3Request(mux, http.MethodPatch, "/api/v3/clients/"+created.ID, key, map[string]any{"name": "renamed"})
	if w.Code != http.StatusOK {
		t.Fatalf("patch client = %d, want 200: %s", w.Code, w.Body.String())
	}
	var patched v3ClientDTO
	_ = json.Unmarshal(w.Body.Bytes(), &patched)
	if patched.DisplayName != "renamed" {
		t.Errorf("name = %q, want renamed", patched.DisplayName)
	}

	// Actions.
	if w := v3Request(mux, http.MethodPost, "/api/v3/clients/"+created.ID+"/actions/renew", key, map[string]any{"extend_days": 10}); w.Code != http.StatusOK {
		t.Fatalf("renew = %d, want 200: %s", w.Code, w.Body.String())
	}
	if w := v3Request(mux, http.MethodPost, "/api/v3/clients/"+created.ID+"/actions/toggle", key, map[string]any{"enabled": false}); w.Code != http.StatusOK {
		t.Fatalf("toggle = %d, want 200", w.Code)
	}
	if w := v3Request(mux, http.MethodPost, "/api/v3/clients/"+created.ID+"/actions/add_ip", key, map[string]any{"ip": "203.0.113.5"}); w.Code != http.StatusOK {
		t.Fatalf("add_ip = %d, want 200", w.Code)
	}
	if w := v3Request(mux, http.MethodPost, "/api/v3/clients/"+created.ID+"/actions/remove_ip", key, map[string]any{"ip": "203.0.113.5"}); w.Code != http.StatusOK {
		t.Fatalf("remove_ip = %d, want 200", w.Code)
	}
	if w := v3Request(mux, http.MethodPost, "/api/v3/clients/"+created.ID+"/actions/regenerate-uuid", key, nil); w.Code != http.StatusOK {
		t.Fatalf("regen-uuid = %d, want 200", w.Code)
	}
	if w := v3Request(mux, http.MethodPost, "/api/v3/clients/"+created.ID+"/actions/bogus", key, nil); w.Code != http.StatusNotFound {
		t.Fatalf("unknown action = %d, want 404", w.Code)
	}

	// Delete.
	if w := v3Request(mux, http.MethodDelete, "/api/v3/clients/"+created.ID, key, nil); w.Code != http.StatusNoContent {
		t.Fatalf("delete = %d, want 204: %s", w.Code, w.Body.String())
	}
}

func TestV3_ClientsListPagination(t *testing.T) {
	apiInst, _, _, cleanup := setupTestAPI(t)
	defer cleanup()
	mux := http.NewServeMux()
	apiInst.RegisterRoutes(mux)
	key := "digitdns_live_testkey123"

	for i := 0; i < 3; i++ {
		w := v3Request(mux, http.MethodPost, "/api/v3/clients", key, map[string]any{"name": "c"})
		if w.Code != http.StatusCreated {
			t.Fatalf("seed create = %d", w.Code)
		}
	}
	w := v3Request(mux, http.MethodGet, "/api/v3/clients?limit=2", key, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("list = %d", w.Code)
	}
	var page struct {
		Items      []v3ClientDTO `json:"items"`
		NextCursor string        `json:"next_cursor"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &page)
	if len(page.Items) != 2 {
		t.Errorf("page size = %d, want 2", len(page.Items))
	}
	if page.NextCursor == "" {
		t.Errorf("next_cursor empty, want a cursor")
	}

	// Bad limit.
	w = v3Request(mux, http.MethodGet, "/api/v3/clients?limit=0", key, nil)
	if w.Code != http.StatusBadRequest {
		t.Errorf("limit=0 = %d, want 400", w.Code)
	}
}

func TestV3_UpstreamsAddReorderRemove(t *testing.T) {
	apiInst, _, _, cleanup := setupTestAPI(t)
	defer cleanup()
	mux := http.NewServeMux()
	apiInst.RegisterRoutes(mux)
	key := "digitdns_live_testkey123"

	w := v3Request(mux, http.MethodPost, "/api/v3/upstreams", key, map[string]any{"address": "8.8.8.8"})
	if w.Code != http.StatusCreated {
		t.Fatalf("add upstream = %d, want 201: %s", w.Code, w.Body.String())
	}
	// Duplicate.
	if w := v3Request(mux, http.MethodPost, "/api/v3/upstreams", key, map[string]any{"address": "8.8.8.8"}); w.Code != http.StatusConflict {
		t.Fatalf("dup upstream = %d, want 409", w.Code)
	}
	// Reorder with the exact set.
	w = v3Request(mux, http.MethodGet, "/api/v3/upstreams", key, nil)
	var list struct {
		Items []string `json:"items"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &list)
	if w := v3Request(mux, http.MethodPost, "/api/v3/upstreams/reorder", key, map[string]any{"order": list.Items}); w.Code != http.StatusOK {
		t.Fatalf("reorder = %d, want 200: %s", w.Code, w.Body.String())
	}
	// Reorder with a wrong set.
	if w := v3Request(mux, http.MethodPost, "/api/v3/upstreams/reorder", key, map[string]any{"order": []string{"8.8.8.8"}}); w.Code != http.StatusBadRequest {
		t.Fatalf("bad reorder = %d, want 400", w.Code)
	}
}

func TestV3_PoliciesAndAccess(t *testing.T) {
	apiInst, _, _, cleanup := setupTestAPI(t)
	defer cleanup()
	fake := &fakeV3Backend{}
	apiInst.SetV3Backend(fake)
	mux := http.NewServeMux()
	apiInst.RegisterRoutes(mux)
	key := "digitdns_live_testkey123"

	if w := v3Request(mux, http.MethodGet, "/api/v3/policies", key, nil); w.Code != http.StatusOK {
		t.Fatalf("policies list = %d, want 200", w.Code)
	}
	if w := v3Request(mux, http.MethodPost, "/api/v3/policies", key, map[string]any{"key": "custom_proxied"}); w.Code != http.StatusCreated {
		t.Fatalf("policies post = %d, want 201: %s", w.Code, w.Body.String())
	}
	if w := v3Request(mux, http.MethodPost, "/api/v3/access/mode", key, map[string]any{"allow_all": false}); w.Code != http.StatusOK {
		t.Fatalf("access mode = %d, want 200: %s", w.Code, w.Body.String())
	}
	if w := v3Request(mux, http.MethodGet, "/api/v3/access", key, nil); w.Code != http.StatusOK {
		t.Fatalf("access get = %d, want 200", w.Code)
	}
}

func TestV3_OpenAPIServed(t *testing.T) {
	apiInst, _, _, cleanup := setupTestAPI(t)
	defer cleanup()
	mux := http.NewServeMux()
	apiInst.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v3/openapi.json", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("openapi = %d, want 200", w.Code)
	}
	var spec map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &spec); err != nil {
		t.Fatalf("openapi is not JSON: %v", err)
	}
	if spec["openapi"] == "" {
		t.Errorf("openapi spec missing version key")
	}
	paths, _ := spec["paths"].(map[string]any)
	for _, want := range []string{"/api/v3/auth/login", "/api/v3/clients", "/api/v3/settings", "/api/v3/tls/issue"} {
		if _, ok := paths[want]; !ok {
			t.Errorf("openapi spec missing path %s", want)
		}
	}
}

func TestV3_DocsServed(t *testing.T) {
	apiInst, _, _, cleanup := setupTestAPI(t)
	defer cleanup()
	mux := http.NewServeMux()
	apiInst.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v3/docs", nil)
	req.RemoteAddr = "127.0.0.1:1234"
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("docs = %d, want 200", w.Code)
	}
	if !bytes.Contains(w.Body.Bytes(), []byte("/api/v3/openapi.json")) {
		t.Errorf("docs page does not reference the v3 spec")
	}
}
