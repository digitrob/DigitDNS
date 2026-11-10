package upstream

// PresetProvider is one public DNS provider in the resolver browser. The
// addresses are stored in the same canonical form the pool persists, so the
// client can compare them against its configured list without re-parsing.
type PresetProvider struct {
	ID      string         `json:"id"`
	Name    string         `json:"name"`
	Note    string         `json:"note,omitempty"`
	Servers []PresetServer `json:"servers"`
}

// PresetServer is one selectable endpoint under a provider. Protocol is the
// display name the panel already uses for the DNS transport (UDP, TCP, DoT,
// DoH, DoQ, DoH3, DNSCrypt); clients colour that text with their existing
// protocol palette.
type PresetServer struct {
	Address  string `json:"address"`
	Label    string `json:"label"`
	Protocol string `json:"protocol"`
}

// presetSpec is the hand-maintained half of the catalogue. Addresses are
// normalised once in buildPresetCatalog so a typo fails the catalogue test
// instead of reaching a client as an address the add handler rejects.
type presetSpec struct {
	id      string
	name    string
	note    string
	servers []presetServerSpec
}

type presetServerSpec struct {
	address string
	label   string
}

// presetSpecs is the curated top-ten list. Only each provider's standard,
// unfiltered-or-default service is included: no family, ad-blocking or other
// named profiles. Providers that do not offer a generic encrypted endpoint
// (NextDNS) keep only the endpoints that work without a per-user profile.
var presetSpecs = []presetSpec{
	{
		id:   "cloudflare",
		name: "Cloudflare",
		servers: []presetServerSpec{
			{"1.1.1.1", "1.1.1.1"},
			{"1.0.0.1", "1.0.0.1"},
			{"tls://one.one.one.one", "one.one.one.one"},
			{"https://cloudflare-dns.com/dns-query", "cloudflare-dns.com"},
			{"h3://cloudflare-dns.com/dns-query", "cloudflare-dns.com"},
		},
	},
	{
		id:   "google",
		name: "Google Public DNS",
		servers: []presetServerSpec{
			{"8.8.8.8", "8.8.8.8"},
			{"8.8.4.4", "8.8.4.4"},
			{"tls://dns.google", "dns.google"},
			{"https://dns.google/dns-query", "dns.google"},
			{"h3://dns.google/dns-query", "dns.google"},
		},
	},
	{
		id:   "quad9",
		name: "Quad9",
		servers: []presetServerSpec{
			{"9.9.9.9", "9.9.9.9"},
			{"149.112.112.112", "149.112.112.112"},
			{"tls://dns.quad9.net", "dns.quad9.net"},
			{"https://dns.quad9.net/dns-query", "dns.quad9.net"},
			{"quic://dns.quad9.net", "dns.quad9.net"},
			{"h3://dns.quad9.net/dns-query", "dns.quad9.net"},
			{"sdns://AQMAAAAAAAAADDkuOS45Ljk6ODQ0MyBnyEe4yHWM0SAkVUO-dWdG3zTfHYTAC4xHA2jfgh2GPhkyLmRuc2NyeXB0LWNlcnQucXVhZDkubmV0", "sdns://…"},
		},
	},
	{
		id:   "opendns",
		name: "OpenDNS",
		servers: []presetServerSpec{
			{"208.67.222.222", "208.67.222.222"},
			{"208.67.220.220", "208.67.220.220"},
			{"tls://dns.opendns.com", "dns.opendns.com"},
			{"https://doh.opendns.com/dns-query", "doh.opendns.com"},
			{"sdns://AQAAAAAAAAAADjIwOC42Ny4yMjAuMjIwILc1EUAgbyJdPivYItf9aR6hwzzI1maNDL4Ev6vKQ_t5GzIuZG5zY3J5cHQtY2VydC5vcGVuZG5zLmNvbQ", "sdns://…"},
		},
	},
	{
		id:   "adguard",
		name: "AdGuard DNS",
		servers: []presetServerSpec{
			{"94.140.14.14", "94.140.14.14"},
			{"94.140.15.15", "94.140.15.15"},
			{"tls://dns.adguard-dns.com", "dns.adguard-dns.com"},
			{"https://dns.adguard-dns.com/dns-query", "dns.adguard-dns.com"},
			{"quic://dns.adguard-dns.com", "dns.adguard-dns.com"},
			{"sdns://AQIAAAAAAAAAETk0LjE0MC4xNC4xNDo1NDQzINErR_JS3PLCu_iZEIbq95zkSV2LFsigxDIuUso_OQhzIjIuZG5zY3J5cHQuZGVmYXVsdC5uczEuYWRndWFyZC5jb20", "sdns://…"},
		},
	},
	{
		id:   "controld",
		name: "Control D",
		servers: []presetServerSpec{
			{"76.76.2.0", "76.76.2.0"},
			{"76.76.10.0", "76.76.10.0"},
			{"tls://p0.freedns.controld.com", "p0.freedns.controld.com"},
			{"https://freedns.controld.com/p0", "freedns.controld.com/p0"},
			{"quic://p0.freedns.controld.com", "p0.freedns.controld.com"},
		},
	},
	{
		id:   "dnssb",
		name: "DNS.SB",
		servers: []presetServerSpec{
			{"185.222.222.222", "185.222.222.222"},
			{"45.11.45.11", "45.11.45.11"},
			{"tls://dot.sb", "dot.sb"},
			{"https://doh.dns.sb/dns-query", "doh.dns.sb"},
		},
	},
	{
		id:   "yandex",
		name: "Yandex DNS",
		servers: []presetServerSpec{
			{"77.88.8.8", "77.88.8.8"},
			{"77.88.8.1", "77.88.8.1"},
			{"tls://common.dot.dns.yandex.net", "common.dot.dns.yandex.net"},
			{"https://common.dot.dns.yandex.net/dns-query", "common.dot.dns.yandex.net"},
		},
	},
	{
		id:   "nextdns",
		name: "NextDNS",
		note: "Plain anycast endpoints work without an account. DoH and DoT need a NextDNS configuration ID.",
		servers: []presetServerSpec{
			{"45.90.28.0", "45.90.28.0"},
			{"45.90.30.0", "45.90.30.0"},
		},
	},
	{
		id:   "cleanbrowsing",
		name: "CleanBrowsing",
		note: "Security filter — blocks phishing, spam and malicious domains.",
		servers: []presetServerSpec{
			{"185.228.168.9", "185.228.168.9"},
			{"185.228.169.9", "185.228.169.9"},
			{"tls://security-filter-dns.cleanbrowsing.org", "security-filter-dns.cleanbrowsing.org"},
			{"https://doh.cleanbrowsing.org/doh/security-filter/", "doh.cleanbrowsing.org"},
		},
	},
}

var presetCatalog = buildPresetCatalog()

func buildPresetCatalog() []PresetProvider {
	out := make([]PresetProvider, 0, len(presetSpecs))
	for _, spec := range presetSpecs {
		provider := PresetProvider{ID: spec.id, Name: spec.name, Note: spec.note}
		for _, s := range spec.servers {
			address, ok := NormalizeUpstream(s.address)
			if !ok {
				continue
			}
			provider.Servers = append(provider.Servers, PresetServer{
				Address:  address,
				Label:    s.label,
				Protocol: ProtocolOf(address),
			})
		}
		out = append(out, provider)
	}
	return out
}

// PresetProviders returns the curated public-resolver catalogue. The slice and
// its members are read-only; callers only encode them into an API response.
func PresetProviders() []PresetProvider {
	return presetCatalog
}
