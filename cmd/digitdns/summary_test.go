package main

import (
	"strings"
	"testing"

	"digitdns/internal/database"
)

func TestSummaryLinesPrefersPanelDomain(t *testing.T) {
	srv := &database.ServerSettings{BindHost: "127.0.0.1", WebPort: 8080, AdminPath: "abc123de45"}
	dns := &database.DNSSettings{Port: 53, DoTPort: 853, DoHPort: 8443}
	sni := &database.SNIProxySettings{}
	tls := &database.TLSSettings{PanelHTTPS: true}

	tls.Domain = "docs.example.com"

	lines := summaryLines(srv, dns, sni, tls)
	joined := strings.Join(lines, "\n")

	for _, want := range []string{
		"https://docs.example.com:8080/abc123de45/dash/",
		" Standard DNS       : docs.example.com:53 (UDP/TCP)",
		" DNS-over-TLS (DoT) : docs.example.com:853 (TCP/TLS)",
		"https://docs.example.com:8443/dns-query",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("summary lost %q:\n%s", want, joined)
		}
	}
	if strings.Contains(joined, "0.0.0.0") {
		t.Errorf("summary advertised the bind wildcard while a domain is set:\n%s", joined)
	}
}

func TestSummaryLinesHonoursDedicatedDoTDomain(t *testing.T) {
	srv := &database.ServerSettings{PublicIP: "192.0.2.10", WebPort: 8080, AdminPath: "abc123de45"}
	dns := &database.DNSSettings{Port: 53, DoTPort: 853, DoHPort: 8443}
	sni := &database.SNIProxySettings{}
	tls := &database.TLSSettings{}

	tls.Domain = "docs.example.com"
	tls.DoTDomain = "tls.example.org"

	joined := strings.Join(summaryLines(srv, dns, sni, tls), "\n")

	if !strings.Contains(joined, " DNS-over-TLS (DoT) : tls.example.org:853 (TCP/TLS)") {
		t.Errorf("dedicated DoT domain did not outrank the panel host:\n%s", joined)
	}
	if !strings.Contains(joined, "https://tls.example.org:8443/dns-query") {
		t.Errorf("DoH line did not ride the dedicated domain:\n%s", joined)
	}
	if !strings.Contains(joined, " Standard DNS       : docs.example.com:53 (UDP/TCP)") {
		t.Errorf("standard DNS should keep the panel host preference:\n%s", joined)
	}
	if !strings.Contains(joined, " DigitDNS Dashboard : http://docs.example.com:8080/abc123de45/dash/") {
		t.Errorf("dashboard with a domain and plain HTTP:\n%s", joined)
	}
}

func TestSummaryLinesFallsBackToPublicIPThenBind(t *testing.T) {
	sni := &database.SNIProxySettings{}
	dns := &database.DNSSettings{Port: 53, DoTPort: 853, DoHPort: 8443}

	srv := &database.ServerSettings{PublicIP: "192.0.2.10", WebPort: 27586, AdminPath: "abc123de45"}
	joined := strings.Join(summaryLines(srv, dns, sni, &database.TLSSettings{}), "\n")
	if !strings.Contains(joined, " DigitDNS Dashboard : http://192.0.2.10:27586/abc123de45/dash/") {
		t.Errorf("blank-domain install should show its public IP:\n%s", joined)
	}

	srv = &database.ServerSettings{BindHost: "127.0.0.1", WebPort: 8080, AdminPath: "abc123de45"}
	joined = strings.Join(summaryLines(srv, dns, sni, &database.TLSSettings{}), "\n")
	if !strings.Contains(joined, " DigitDNS Dashboard : http://127.0.0.1:8080/abc123de45/dash/") {
		t.Errorf("loopback install should show the bind host:\n%s", joined)
	}
}

func TestSummaryLinesSuppressesDefaultDoHPort(t *testing.T) {
	srv := &database.ServerSettings{PublicIP: "192.0.2.10", WebPort: 27586, AdminPath: "abc123de45"}
	dns := &database.DNSSettings{Port: 53, DoTPort: 853, DoHPort: 443}
	joined := strings.Join(summaryLines(srv, dns, &database.SNIProxySettings{}, &database.TLSSettings{}), "\n")
	if !strings.Contains(joined, "https://192.0.2.10/dns-query") {
		t.Errorf("default DoH port should be suppressed:\n%s", joined)
	}
}

func TestSummaryLinesShowsSNIState(t *testing.T) {
	srv := &database.ServerSettings{PublicIP: "192.0.2.10", WebPort: 8080, AdminPath: "a"}
	dns := &database.DNSSettings{Port: 53, DoTPort: 853, DoHPort: 8443}

	on := strings.Join(summaryLines(srv, dns, &database.SNIProxySettings{Enabled: true, HTTPPort: 80, HTTPSPort: 443}, &database.TLSSettings{}), "\n")
	if !strings.Contains(on, " SNI Proxy Relays   : Ports 80 (HTTP) and 443 (HTTPS)") {
		t.Errorf("enabled SNI relays not shown:\n%s", on)
	}

	off := strings.Join(summaryLines(srv, dns, &database.SNIProxySettings{}, &database.TLSSettings{}), "\n")
	if !strings.Contains(off, " SNI Proxy Relays   : disabled") {
		t.Errorf("disabled SNI relays not shown:\n%s", off)
	}
}

