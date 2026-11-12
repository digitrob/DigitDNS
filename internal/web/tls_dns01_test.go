package web

import (
	"net/http"
	"testing"
)

func TestCertificateNameCoversWildcard(t *testing.T) {
	cases := []struct {
		certName, domain string
		want             bool
	}{
		{"dns.example.com", "dns.example.com", true},
		{"*.example.com", "dns.example.com", true},
		{"*.example.com", "a.b.example.com", false},
		{"*.example.com", "example.com", false},
		{"*.example.com", "other.example.net", false},
	}
	for _, c := range cases {
		if got := certificateNameCovers(c.certName, c.domain); got != c.want {
			t.Errorf("certificateNameCovers(%q, %q) = %v, want %v", c.certName, c.domain, got, c.want)
		}
	}
}

func TestValidACMECertNameAllowsOnlyFullWildcard(t *testing.T) {
	if got, err := validACMECertName("*.Example.com"); err != nil || got != "*.example.com" {
		t.Fatalf("validACMECertName wildcard = %q, %v", got, err)
	}
	if _, err := validACMECertName("foo.*.example.com"); err == nil {
		t.Fatal("a wildcard in the middle must be refused")
	}
}

func TestHandleTLSIssueRequiresDNS01ForWildcard(t *testing.T) {
	ws, _, cleanup := setupTestWebServer(t)
	defer cleanup()
	ws.tlsSettings.Domain = "panel.example"
	ws.tlsSettings.DoTDomain = "dot.example"
	token := loginTestSession(t, ws)

	w := postJSON(t, ws.buildAdminHandler(), "/api/tls/issue",
		`{"purpose":"dot","domain":"dot.example","challenge":"http-01","cert_name":"*.example"}`,
		token)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body %s", w.Code, w.Body.String())
	}
}

func TestHandleTLSIssueRequiresDNS01Token(t *testing.T) {
	ws, _, cleanup := setupTestWebServer(t)
	defer cleanup()
	ws.tlsSettings.Domain = "panel.example"
	ws.tlsSettings.DoTDomain = "dot.example"
	token := loginTestSession(t, ws)

	w := postJSON(t, ws.buildAdminHandler(), "/api/tls/issue",
		`{"purpose":"dot","domain":"dot.example","challenge":"dns-01","cert_name":"*.example"}`,
		token)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body %s", w.Code, w.Body.String())
	}
}
