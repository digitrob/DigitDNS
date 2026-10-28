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
	loginToken      string
	loginTwoFA      bool
	loginErr        error
	loggedOut       string
	ticket          string
	persisted       [][]string
	resolution      map[string]any
	savedResolution map[string]any
}

func (f *fakeV3Backend) Login(r *http.Request, username, password, code string) (string, bool, error) {
	if f.loginErr != nil {
		return "", f.loginTwoFA, f.loginErr
	}
	return f.loginToken, false, nil
}
func (f *fakeV3Backend) Logout(token string) { f.loggedOut = token }
func (f *fakeV3Backend) PersistUpstreams(list []string) error {
	f.persisted = append(f.persisted, append([]string(nil), list...))
	return nil
}

func (f *fakeV3Backend) ResolutionSettings() (map[string]any, error) {
	if f.resolution == nil {
		return map[string]any{"transport": "any"}, nil
	}
	return f.resolution, nil
}

func (f *fakeV3Backend) SaveResolution(r *http.Request, body map[string]any) (map[string]any, error) {
	f.savedResolution = body
	return f.resolution, nil
}
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
func (f *fakeV3Backend) GeoIPStatus() (map[string]any, error) {
	return map[string]any{"available": true, "country": "NL", "database_date": "2026-10-22"}, nil
}
func (f *fakeV3Backend) GeoIPUpdate(r *http.Request) (map[string]any, error) {
	return map[string]any{"available": true, "country": "NL"}, nil
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
	if body["api_version"] != "3.1" {
		t.Errorf("api_version = %v, want 3.1", body["api_version"])
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

// TestV3_ClientsContract31 pins the additive API 3.1 surface: absolute expiry
// on create and PATCH (including the zero-timestamp "lifetime" sentinel), an
// editable UUID, and the usage figures the web client card has always shown.
func TestV3_ClientsContract31(t *testing.T) {
	apiInst, _, _, cleanup := setupTestAPI(t)
	defer cleanup()
	mux := http.NewServeMux()
	apiInst.RegisterRoutes(mux)
	key := "digitdns_live_testkey123"

	expiry := time.Date(2031, 3, 4, 5, 6, 7, 0, time.UTC)
	w := v3Request(mux, http.MethodPost, "/api/v3/clients", key, map[string]any{
		"name": "api31", "expires_at": expiry.Format(time.RFC3339), "traffic_reset_cycle": "monthly",
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create = %d, want 201: %s", w.Code, w.Body.String())
	}
	var created v3ClientDTO
	_ = json.Unmarshal(w.Body.Bytes(), &created)
	if created.ExpiresAt != expiry.Format(timeFormatRFC3339) {
		t.Errorf("expires_at = %q, want %q", created.ExpiresAt, expiry.Format(timeFormatRFC3339))
	}
	if created.NextTrafficReset == "" {
		t.Error("next_traffic_reset empty for a monthly cycle")
	}

	// An absolute expiry wins over validity_days, exactly as the service documents.
	w = v3Request(mux, http.MethodPost, "/api/v3/clients", key, map[string]any{
		"name": "both", "validity_days": 1, "expires_at": expiry.Format(time.RFC3339),
	})
	if w.Code != http.StatusCreated {
		t.Fatalf("create with both = %d: %s", w.Code, w.Body.String())
	}
	var both v3ClientDTO
	_ = json.Unmarshal(w.Body.Bytes(), &both)
	if both.ExpiresAt != expiry.Format(timeFormatRFC3339) {
		t.Errorf("absolute expiry lost to validity_days: %q", both.ExpiresAt)
	}

	// UUID edit.
	w = v3Request(mux, http.MethodPatch, "/api/v3/clients/"+created.ID, key, map[string]any{
		"uuid": "11111111-2222-3333-4444-555555555555",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("patch uuid = %d, want 200: %s", w.Code, w.Body.String())
	}
	var patched v3ClientDTO
	_ = json.Unmarshal(w.Body.Bytes(), &patched)
	if patched.UUID != "11111111-2222-3333-4444-555555555555" {
		t.Errorf("uuid = %q, want the patched value", patched.UUID)
	}

	// Zero timestamp clears the expiry (lifetime).
	w = v3Request(mux, http.MethodPatch, "/api/v3/clients/"+created.ID, key, map[string]any{
		"expires_at": "0001-01-01T00:00:00Z",
	})
	if w.Code != http.StatusOK {
		t.Fatalf("clear expiry = %d, want 200: %s", w.Code, w.Body.String())
	}
	var cleared v3ClientDTO
	_ = json.Unmarshal(w.Body.Bytes(), &cleared)
	if cleared.ExpiresAt != "" {
		t.Errorf("expires_at = %q after clear, want empty", cleared.ExpiresAt)
	}

	// A malformed timestamp is a 400, not a silent lifetime account.
	w = v3Request(mux, http.MethodPatch, "/api/v3/clients/"+created.ID, key, map[string]any{
		"expires_at": "not-a-date",
	})
	if w.Code != http.StatusBadRequest {
		t.Errorf("bad expires_at = %d, want 400", w.Code)
	}

	// The list carries the new usage fields; a fresh account has no page views.
	w = v3Request(mux, http.MethodGet, "/api/v3/clients", key, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("list = %d", w.Code)
	}
	var page struct {
		Items []v3ClientDTO `json:"items"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &page)
	if len(page.Items) == 0 {
		t.Fatal("list is empty")
	}
	for _, item := range page.Items {
		if item.ID == created.ID && item.TotalQueries != 0 {
			t.Errorf("total_queries = %d for a fresh account, want 0", item.TotalQueries)
		}
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
	recorder := &fakeV3Backend{}
	apiInst.SetV3Backend(recorder)
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
	// Remove the added upstream.
	w = v3Request(mux, http.MethodPost, "/api/v3/upstreams/delete", key, map[string]any{"address": "8.8.8.8"})
	if w.Code != http.StatusOK {
		t.Fatalf("delete upstream = %d, want 200: %s", w.Code, w.Body.String())
	}
	var after struct {
		Items []string `json:"items"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &after)
	for _, u := range after.Items {
		if u == "8.8.8.8:53" || u == "8.8.8.8" {
			t.Fatalf("deleted upstream still present: %v", after.Items)
		}
	}
	// Removing it again is a 404: it is no longer in the pool.
	if w := v3Request(mux, http.MethodPost, "/api/v3/upstreams/delete", key, map[string]any{"address": "8.8.8.8"}); w.Code != http.StatusNotFound {
		t.Fatalf("re-delete = %d, want 404", w.Code)
	}

	// Every mutation must reach the settings record, or the resolution card's
	// upstream list and encrypted gate never see a resolver added from Android.
	if len(recorder.persisted) < 3 {
		t.Fatalf("upstream mutations were not persisted: %v", recorder.persisted)
	}
	found := false
	for _, u := range recorder.persisted[0] {
		if u == "8.8.8.8:53" {
			found = true
		}
	}
	if !found {
		t.Fatalf("added resolver missing from the persisted list: %v", recorder.persisted[0])
	}
}

// A companion app may authenticate with the master API key. The resolution card
// used to live on the panel mux, whose session-only gate answered 401, leaving
// the card's dropdowns stuck on their defaults. The v3 route must accept the key.
func TestV3_ResolutionSettingsAcceptsAPIKey(t *testing.T) {
	apiInst, _, _, cleanup := setupTestAPI(t)
	defer cleanup()
	backend := &fakeV3Backend{resolution: map[string]any{
		"transport":           "encrypted",
		"encrypted_available": true,
		"upstreams":           []string{"tls://dns.google:853"},
	}}
	apiInst.SetV3Backend(backend)
	mux := http.NewServeMux()
	apiInst.RegisterRoutes(mux)
	key := "digitdns_live_testkey123"

	w := v3Request(mux, http.MethodGet, "/api/v3/settings/resolution", key, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("GET resolution = %d, want 200: %s", w.Code, w.Body.String())
	}
	var got map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if got["transport"] != "encrypted" {
		t.Fatalf("GET resolution transport = %v, want encrypted", got["transport"])
	}

	w = v3Request(mux, http.MethodPost, "/api/v3/settings/resolution", key,
		map[string]any{"transport": "regular", "profiles": map[string]any{}})
	if w.Code != http.StatusOK {
		t.Fatalf("POST resolution = %d, want 200: %s", w.Code, w.Body.String())
	}
	if backend.savedResolution["transport"] != "regular" {
		t.Fatalf("save body transport = %v, want regular", backend.savedResolution["transport"])
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

// A v3 problem must name the v3 route it came from: writeProblem used to
// hardcode instance "/api/v2", so a v3 401 claimed to be a v2 problem.
func TestV3_ProblemInstancePointsAtTheV3Route(t *testing.T) {
	apiInst, _, _, cleanup := setupTestAPI(t)
	defer cleanup()
	mux := http.NewServeMux()
	apiInst.RegisterRoutes(mux)

	// Unauthenticated /api/v3/clients => 401 with the request path as instance.
	w := v3Request(mux, http.MethodGet, "/api/v3/clients", "", nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", w.Code)
	}
	var prob map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &prob); err != nil {
		t.Fatalf("problem is not JSON: %v", err)
	}
	if prob["instance"] != "/api/v3/clients" {
		t.Errorf("instance = %v, want /api/v3/clients", prob["instance"])
	}

	// The v2 route still reports its own prefix.
	// A v2 writeProblem route still reports its own prefix (the fix left the
	// shared helper's v2 default in place). A bad limit with a valid key gives a
	// v2 400 built by writeProblem.
	req := httptest.NewRequest(http.MethodGet, "/api/v2/clients?limit=0", nil)
	req.Header.Set("X-API-Key", "digitdns_live_testkey123")
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	var v2prob map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &v2prob); err != nil {
		t.Fatalf("v2 problem is not JSON (%d): %v", rec.Code, err)
	}
	if v2prob["instance"] != "/api/v2" {
		t.Errorf("v2 instance = %v, want /api/v2", v2prob["instance"])
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
