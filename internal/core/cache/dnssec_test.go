package cache

import (
	"net"
	"testing"
	"time"

	"github.com/miekg/dns"
)

func signatureFor(name string) dns.RR {
	return &dns.RRSIG{
		Hdr: dns.RR_Header{
			Name:   name,
			Rrtype: dns.TypeRRSIG,
			Class:  dns.ClassINET,
			Ttl:    300,
		},
		TypeCovered: dns.TypeA,
		Algorithm:   dns.RSASHA256,
		Labels:      2,
		OrigTtl:     300,
		Expiration:  2000000000,
		Inception:   1000000000,
		KeyTag:      1234,
		SignerName:  "example.",
		Signature:   "AAAA",
	}
}

func signedAnswerFor(name string) (dns.Question, *dns.Msg) {
	q, msg := answerFor(name)
	msg.Answer = append(msg.Answer, signatureFor(msg.Question[0].Name))
	msg.SetEdns0(4096, true)
	return q, msg
}

func TestCacheSeparatesDOAndEDNS(t *testing.T) {
	c := NewCache(1024, 30, 3600)
	defer c.Close()

	q, unsigned := answerFor("dnssec.example")
	_, signed := signedAnswerFor("dnssec.example")
	doOpts := QueryOptions{EDNS: true, DO: true}

	c.PutWithOptions(q, unsigned, QueryOptions{})
	c.PutWithOptions(q, signed, doOpts)

	plain := c.GetWithOptions(q, QueryOptions{})
	if plain == nil {
		t.Fatal("plain entry reported as a miss")
	}
	if len(plain.Answer) != 1 {
		t.Fatalf("plain answer carries %d records, want 1 — signatures leaked into a DO=0 entry", len(plain.Answer))
	}
	if plain.IsEdns0() != nil {
		t.Fatal("plain answer carries an OPT record it never asked for")
	}

	withDNSSEC := c.GetWithOptions(q, doOpts)
	if withDNSSEC == nil {
		t.Fatal("signed entry reported as a miss")
	}
	if len(withDNSSEC.Answer) != 2 {
		t.Fatalf("signed answer carries %d records, want the A record plus RRSIG", len(withDNSSEC.Answer))
	}
	opt := withDNSSEC.IsEdns0()
	if opt == nil || !opt.Do() {
		t.Fatal("signed answer lost its DO bit")
	}

	if got := c.GetWithOptions(q, QueryOptions{EDNS: true}); got != nil {
		t.Fatal("a DO=0 EDNS query was served the DO=1 entry")
	}
}

func TestCacheDoesNotOutliveSignature(t *testing.T) {
	// minTTL is 300, but the signature has 20 seconds left. The cache must not
	// extend the signed data to the operator's min_ttl.
	c := NewCache(1024, 300, 3600)
	defer c.Close()

	q, msg := signedAnswerFor("expiry.example")
	msg.Answer[0].Header().Ttl = 5
	msg.Answer[1].(*dns.RRSIG).Expiration = uint32(time.Now().Add(20 * time.Second).Unix())

	opts := QueryOptions{EDNS: true, DO: true}
	c.PutWithOptions(q, msg, opts)

	got := c.GetWithOptions(q, opts)
	if got == nil {
		t.Fatal("signed entry with a live signature was not cached")
	}
	for _, rr := range got.Answer {
		if ttl := rr.Header().Ttl; ttl > 20 {
			t.Fatalf("%s TTL = %d, want at most the signature's 20 seconds", dns.TypeToString[rr.Header().Rrtype], ttl)
		}
	}
}

func TestCacheRejectsExpiredSignature(t *testing.T) {
	c := NewCache(1024, 300, 3600)
	defer c.Close()

	q, msg := signedAnswerFor("expired.example")
	msg.Answer[1].(*dns.RRSIG).Expiration = uint32(time.Now().Add(-time.Second).Unix())

	opts := QueryOptions{EDNS: true, DO: true}
	c.PutWithOptions(q, msg, opts)
	if got := c.GetWithOptions(q, opts); got != nil {
		t.Fatal("an answer whose RRSIG has expired was cached")
	}
}

func TestCacheClearsDOBitForDO0Entry(t *testing.T) {
	c := NewCache(1024, 30, 3600)
	defer c.Close()

	q, signed := signedAnswerFor("do-bit.example")
	// Stored under a DO=0 key even though the message still carried the DO bit
	// from the upstream query the resolver had to make to validate it.
	c.PutWithOptions(q, signed, QueryOptions{EDNS: true})

	got := c.GetWithOptions(q, QueryOptions{EDNS: true})
	if got == nil {
		t.Fatal("DO=0 entry reported as a miss")
	}
	if len(got.Answer) != 1 {
		t.Fatalf("DO=0 entry kept %d records, want only the A record", len(got.Answer))
	}
	opt := got.IsEdns0()
	if opt == nil {
		t.Fatal("EDNS entry lost its OPT record")
	}
	if opt.Do() {
		t.Fatal("DO=0 entry still advertises the DO bit")
	}
}

func TestCacheSeparatesCDAndClearsAD(t *testing.T) {
	c := NewCache(1024, 30, 3600)
	defer c.Close()

	q, normal := answerFor("checking.example")
	normal.AuthenticatedData = true

	cd := normal.Copy()
	cd.Answer[0].(*dns.A).A = net.IPv4(203, 0, 113, 9)
	cd.AuthenticatedData = true

	c.PutWithOptions(q, normal, QueryOptions{})
	c.PutWithOptions(q, cd, QueryOptions{CD: true})

	gotNormal := c.GetWithOptions(q, QueryOptions{})
	if gotNormal == nil || !gotNormal.AuthenticatedData {
		t.Fatal("a validated answer lost its AD bit")
	}
	if gotNormal.CheckingDisabled {
		t.Fatal("a normal answer came back with CD set")
	}
	if ip := gotNormal.Answer[0].(*dns.A).A.String(); ip != "198.51.100.7" {
		t.Fatalf("normal answer = %s, want the normal entry", ip)
	}

	gotCD := c.GetWithOptions(q, QueryOptions{CD: true})
	if gotCD == nil {
		t.Fatal("checking-disabled entry reported as a miss")
	}
	if gotCD.AuthenticatedData {
		t.Fatal("a CD=1 answer still claimed AD")
	}
	if !gotCD.CheckingDisabled {
		t.Fatal("a CD=1 answer did not preserve its CD bit")
	}
	if ip := gotCD.Answer[0].(*dns.A).A.String(); ip != "203.0.113.9" {
		t.Fatalf("CD answer = %s, want the CD entry", ip)
	}
}

func TestLookupRefreshesWithTheEntryOptions(t *testing.T) {
	c := NewCache(1024, 30, 3600)
	defer c.Close()

	q, signed := signedAnswerFor("refresh-dnssec.example")
	opts := QueryOptions{EDNS: true, DO: true}
	c.PutWithOptions(q, signed, opts)

	seen := make(chan QueryOptions, 1)
	c.SetRefresherWithOptions(func(_ dns.Question, got QueryOptions) (*dns.Msg, error) {
		seen <- got
		return signed, nil
	})

	// Move the entry into the stale window so Lookup starts a refresh.
	shard := c.getShard(c.cacheKeyFor(q, opts))
	shard.mu.Lock()
	entry := shard.entries[c.cacheKeyFor(q, opts)]
	entry.cachedAt = time.Now().Add(-2 * time.Minute)
	entry.expireAt = time.Now().Add(-time.Second)
	shard.mu.Unlock()

	if got, stale := c.LookupWithOptions(q, opts); got == nil || !stale {
		t.Fatal("stale signed entry was not served")
	}

	select {
	case got := <-seen:
		if !got.EDNS || !got.DO || got.CD {
			t.Fatalf("refresh options = %+v, want EDNS+DO and no CD", got)
		}
	case <-time.After(time.Second):
		t.Fatal("refresh did not run with the entry's EDNS/DO options")
	}
}
