package web

import "testing"

func TestEncryptedUpstreamsClassifiesThePool(t *testing.T) {
	list := []string{
		"1.1.1.1:53",
		"tcp://9.9.9.9:53",
		"tls://dns.google:853",
		"https://dns.google/dns-query",
		"quic://dns.adguard.com:853",
		"h3://dns.google/dns-query",
		"sdns://abc",
	}
	want := []string{
		"tls://dns.google:853",
		"https://dns.google/dns-query",
		"quic://dns.adguard.com:853",
		"h3://dns.google/dns-query",
		"sdns://abc",
	}
	got := encryptedUpstreams(list)
	if len(got) != len(want) {
		t.Fatalf("encryptedUpstreams = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("encryptedUpstreams = %v, want %v", got, want)
		}
	}
	if plain := encryptedUpstreams([]string{"1.1.1.1:53"}); len(plain) != 0 {
		t.Fatalf("plain-only pool reported encrypted resolvers: %v", plain)
	}
}

func TestCanonicalUpstreamsMatchThePoolForms(t *testing.T) {
	// The stats list reports the pool's canonical form. A settings record saved
	// before the pool appended the default port must be normalised to the same
	// strings, or the encrypted set and the pickers cannot annotate its rows.
	got := canonicalUpstreams([]string{"tls://one.one.one.one", "1.1.1.1", "https://dns.google/dns-query"})
	want := []string{"tls://one.one.one.one:853", "1.1.1.1:53", "https://dns.google/dns-query"}
	if len(got) != len(want) {
		t.Fatalf("canonicalUpstreams = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("canonicalUpstreams = %v, want %v", got, want)
		}
	}
	if canonicalUpstream("") != "" {
		t.Fatalf("empty default upstream must stay empty")
	}
}
