package dnssec

import (
	"context"
	"crypto"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/miekg/dns"
)

// fakeFetcher serves a pre-built zone. It is deliberately dumb: every record
// the validator asks for must have been planted, so a missing DNSKEY or NSEC is
// a test bug rather than something the network could paper over.
type fakeFetcher struct {
	answers map[string]*dns.Msg
}

func fetchKey(name string, qtype uint16) string {
	return canonicalName(name) + "/" + strconv.Itoa(int(qtype))
}

func (f *fakeFetcher) Fetch(_ context.Context, name string, qtype uint16) (*dns.Msg, error) {
	m, ok := f.answers[fetchKey(name, qtype)]
	if !ok {
		return nil, fmt.Errorf("fake fetcher has no %s/%s", name, dns.TypeToString[qtype])
	}
	return m.Copy(), nil
}

func (f *fakeFetcher) put(name string, qtype uint16, rcode int, answer, authority []dns.RR) {
	m := new(dns.Msg)
	m.SetQuestion(dns.Fqdn(name), qtype)
	m.Response = true
	m.Rcode = rcode
	m.Answer = answer
	m.Ns = authority
	f.answers[fetchKey(name, qtype)] = m
}

type testZone struct {
	name string
	key  *dns.DNSKEY
	priv crypto.Signer
	ds   *dns.DS
}

func newTestZone(t *testing.T, name string) *testZone {
	t.Helper()
	key := &dns.DNSKEY{
		Hdr:       dns.RR_Header{Name: dns.Fqdn(name), Rrtype: dns.TypeDNSKEY, Class: dns.ClassINET, Ttl: 3600},
		Flags:     257,
		Protocol:  3,
		Algorithm: dns.ECDSAP256SHA256,
	}
	priv, err := key.Generate(256)
	if err != nil {
		t.Fatalf("generate key for %s: %v", name, err)
	}
	signer, ok := priv.(crypto.Signer)
	if !ok {
		t.Fatalf("generated key for %s is not a crypto.Signer", name)
	}
	return &testZone{name: dns.Fqdn(name), key: key, priv: signer, ds: key.ToDS(dns.SHA256)}
}

func (z *testZone) sign(t *testing.T, rrset []dns.RR, signer string) []dns.RR {
	t.Helper()
	if len(rrset) == 0 {
		t.Fatal("cannot sign an empty RRset")
	}
	sig := &dns.RRSIG{
		Hdr:         dns.RR_Header{Name: rrset[0].Header().Name, Rrtype: dns.TypeRRSIG, Class: rrset[0].Header().Class, Ttl: rrset[0].Header().Ttl},
		TypeCovered: rrset[0].Header().Rrtype,
		Algorithm:   z.key.Algorithm,
		OrigTtl:     rrset[0].Header().Ttl,
		Expiration:  uint32(time.Now().Add(time.Hour).Unix()),
		Inception:   uint32(time.Now().Add(-time.Hour).Unix()),
		KeyTag:      z.key.KeyTag(),
		SignerName:  dns.Fqdn(signer),
	}
	if err := sig.Sign(z.priv, rrset); err != nil {
		t.Fatalf("sign %s/%s: %v", rrset[0].Header().Name, dns.TypeToString[rrset[0].Header().Rrtype], err)
	}
	return []dns.RR{sig}
}

func dnskeyRR(z *testZone) dns.RR { return dns.Copy(z.key) }

func dsRR(z *testZone) *dns.DS {
	ds := *z.ds
	ds.Hdr = dns.RR_Header{Name: z.name, Rrtype: dns.TypeDS, Class: dns.ClassINET, Ttl: 3600}
	return &ds
}

func mustRR(t *testing.T, text string) dns.RR {
	t.Helper()
	rr, err := dns.NewRR(text)
	if err != nil {
		t.Fatalf("parse %q: %v", text, err)
	}
	return rr
}

// chainFixture builds root -> test. -> example.test. plus an unsigned
// delegation insecure.test.
type chainFixture struct {
	fetcher *fakeFetcher
	root    *testZone
	test    *testZone
	example *testZone
	answer  *dns.Msg
}

func newChainFixture(t *testing.T) *chainFixture {
	t.Helper()
	f := &fakeFetcher{answers: make(map[string]*dns.Msg)}
	root := newTestZone(t, ".")
	testZone := newTestZone(t, "test.")
	example := newTestZone(t, "example.test.")

	// Root DNSKEY, self-signed.
	rootKeys := []dns.RR{dnskeyRR(root)}
	f.put(".", dns.TypeDNSKEY, dns.RcodeSuccess, append(rootKeys, root.sign(t, rootKeys, ".")...), nil)

	// test. is delegated with a DS signed by the root.
	testDS := []dns.RR{dsRR(testZone)}
	f.put("test.", dns.TypeDS, dns.RcodeSuccess, append(testDS, root.sign(t, testDS, ".")...), nil)
	testKeys := []dns.RR{dnskeyRR(testZone)}
	f.put("test.", dns.TypeDNSKEY, dns.RcodeSuccess, append(testKeys, testZone.sign(t, testKeys, "test.")...), nil)

	// example.test. is delegated with a DS signed by test.
	exampleDS := []dns.RR{dsRR(example)}
	f.put("example.test.", dns.TypeDS, dns.RcodeSuccess, append(exampleDS, testZone.sign(t, exampleDS, "test.")...), nil)
	exampleKeys := []dns.RR{dnskeyRR(example)}
	f.put("example.test.", dns.TypeDNSKEY, dns.RcodeSuccess, append(exampleKeys, example.sign(t, exampleKeys, "example.test.")...), nil)

	// www.example.test. is not a delegation: DS is NODATA, proved by an NSEC
	// whose owner is the name itself and whose bitmap has no DS.
	nsec := mustRR(t, "www.example.test. 300 IN NSEC zzz.example.test. A RRSIG NSEC")
	f.put("www.example.test.", dns.TypeDS, dns.RcodeSuccess, nil, append([]dns.RR{nsec}, example.sign(t, []dns.RR{nsec}, "example.test.")...))
	f.put("www.example.test.", dns.TypeNS, dns.RcodeSuccess, nil, nil)

	// The positive A answer, signed by example.test.
	a := mustRR(t, "www.example.test. 300 IN A 192.0.2.10")
	answer := new(dns.Msg)
	answer.SetQuestion("www.example.test.", dns.TypeA)
	answer.Response = true
	answer.Answer = append([]dns.RR{a}, example.sign(t, []dns.RR{a}, "example.test.")...)

	// insecure.test. is a delegation with no DS. The NSEC from test proves the
	// absence; the NS lookup below proves it is a delegation.
	insDSNSEC := mustRR(t, "insecure.test. 300 IN NSEC www.example.test. NS RRSIG NSEC")
	f.put("insecure.test.", dns.TypeDS, dns.RcodeSuccess, nil, append([]dns.RR{insDSNSEC}, testZone.sign(t, []dns.RR{insDSNSEC}, "test.")...))
	insNS := mustRR(t, "insecure.test. 300 IN NS ns.insecure.test.")
	insNSResp := new(dns.Msg)
	insNSResp.SetQuestion("insecure.test.", dns.TypeNS)
	insNSResp.Response = true
	insNSResp.Answer = []dns.RR{insNS}
	f.answers[fetchKey("insecure.test.", dns.TypeNS)] = insNSResp

	// An unsigned A answer below that delegation.
	insA := mustRR(t, "host.insecure.test. 300 IN A 192.0.2.20")
	insAnswer := new(dns.Msg)
	insAnswer.SetQuestion("host.insecure.test.", dns.TypeA)
	insAnswer.Response = true
	insAnswer.Answer = []dns.RR{insA}

	// NXDOMAIN for nosuch.example.test.: NSEC covering the name, and an NSEC
	// covering *.example.test.
	nxCover := mustRR(t, "www.example.test. 300 IN NSEC zzz.example.test. A RRSIG NSEC")
	nxWildcard := mustRR(t, "example.test. 300 IN NSEC www.example.test. NS SOA RRSIG DNSKEY NSEC3PARAM")
	nxAuthority := []dns.RR{
		mustRR(t, "example.test. 300 IN SOA ns.example.test. hostmaster.example.test. 1 2 3 4 300"),
	}
	nxAuthority = append(nxAuthority, nxCover)
	nxAuthority = append(nxAuthority, example.sign(t, []dns.RR{nxCover}, "example.test.")...)
	nxAuthority = append(nxAuthority, nxWildcard)
	nxAuthority = append(nxAuthority, example.sign(t, []dns.RR{nxWildcard}, "example.test.")...)
	f.put("nosuch.example.test.", dns.TypeA, dns.RcodeNameError, nil, nxAuthority)
	// Walking the chain to a nonexistent name first asks for its DS and NS; both
	// negative answers carry the same denial proof.
	f.put("nosuch.example.test.", dns.TypeDS, dns.RcodeNameError, nil, nxAuthority)
	f.put("nosuch.example.test.", dns.TypeNS, dns.RcodeNameError, nil, nxAuthority)

	return &chainFixture{fetcher: f, root: root, test: testZone, example: example, answer: answer}
}

func (f *chainFixture) validator() *Validator {
	v := New(f.fetcher)
	v.SetTrustAnchors([]string{
		fmt.Sprintf(". 3600 IN DS %d %d %d %s", f.root.ds.KeyTag, f.root.ds.Algorithm, f.root.ds.DigestType, f.root.ds.Digest),
	})
	return v
}

func TestValidatePositiveAnswer(t *testing.T) {
	f := newChainFixture(t)
	v := f.validator()
	q := dns.Question{Name: "www.example.test.", Qtype: dns.TypeA, Qclass: dns.ClassINET}

	result := v.Validate(context.Background(), q, f.answer)
	if result.Status != Secure {
		t.Fatalf("valid answer = %s (%s), want secure", result.Status, result.Reason)
	}
}

func TestValidateDetectsTamperedAnswer(t *testing.T) {
	f := newChainFixture(t)
	v := f.validator()
	q := dns.Question{Name: "www.example.test.", Qtype: dns.TypeA, Qclass: dns.ClassINET}

	tampered := f.answer.Copy()
	tampered.Answer[0].(*dns.A).A = []byte{203, 0, 113, 99}
	result := v.Validate(context.Background(), q, tampered)
	if result.Status != Bogus {
		t.Fatalf("tampered answer = %s (%s), want bogus", result.Status, result.Reason)
	}
}

func TestUnsignedAnswerInSignedZoneIsBogus(t *testing.T) {
	f := newChainFixture(t)
	v := f.validator()
	q := dns.Question{Name: "www.example.test.", Qtype: dns.TypeA, Qclass: dns.ClassINET}

	resp := f.answer.Copy()
	resp.Answer = resp.Answer[:1] // drop the RRSIG
	result := v.Validate(context.Background(), q, resp)
	if result.Status != Bogus {
		t.Fatalf("unsigned answer in a signed zone = %s (%s), want bogus", result.Status, result.Reason)
	}
}

func TestForgedSOACannotDowngradeNegativeAnswer(t *testing.T) {
	f := newChainFixture(t)
	v := f.validator()
	q := dns.Question{Name: "www.example.test.", Qtype: dns.TypeA, Qclass: dns.ClassINET}

	// A forged NXDOMAIN whose SOA points at an unsigned zone must not make the
	// validator treat the signed name as insecure.
	resp := new(dns.Msg)
	resp.SetQuestion(q.Name, q.Qtype)
	resp.Response = true
	resp.Rcode = dns.RcodeNameError
	resp.Ns = []dns.RR{mustRR(t, "insecure.test. 300 IN SOA ns.insecure.test. hostmaster.insecure.test. 1 2 3 4 300")}

	result := v.Validate(context.Background(), q, resp)
	if result.Status != Bogus {
		t.Fatalf("forged SOA downgrade = %s (%s), want bogus", result.Status, result.Reason)
	}
}

func TestValidateWildcardAnswer(t *testing.T) {
	f := newChainFixture(t)
	v := f.validator()

	// A wildcard RRset is signed at *.example.test. and served with the expanded
	// owner. The RRSIG's Labels field is lower than the owner's label count, and
	// miekg/dns reconstructs the wildcard when verifying.
	wild := mustRR(t, "*.example.test. 300 IN A 192.0.2.30")
	wildSig := f.example.sign(t, []dns.RR{wild}, "example.test.")[0].(*dns.RRSIG)
	expanded := mustRR(t, "foo.example.test. 300 IN A 192.0.2.30")
	wildSig.Hdr.Name = "foo.example.test."

	noDS := mustRR(t, "foo.example.test. 300 IN NSEC zzz.example.test. A RRSIG NSEC")
	f.fetcher.put("foo.example.test.", dns.TypeDS, dns.RcodeSuccess, nil, append([]dns.RR{noDS}, f.example.sign(t, []dns.RR{noDS}, "example.test.")...))
	f.fetcher.put("foo.example.test.", dns.TypeNS, dns.RcodeSuccess, nil, nil)

	q := dns.Question{Name: "foo.example.test.", Qtype: dns.TypeA, Qclass: dns.ClassINET}
	resp := new(dns.Msg)
	resp.SetQuestion(q.Name, q.Qtype)
	resp.Response = true
	resp.Answer = []dns.RR{expanded, wildSig}

	result := v.Validate(context.Background(), q, resp)
	if result.Status != Secure {
		t.Fatalf("wildcard answer = %s (%s), want secure", result.Status, result.Reason)
	}
}

func TestValidateInsecureDelegation(t *testing.T) {
	f := newChainFixture(t)
	v := f.validator()
	q := dns.Question{Name: "host.insecure.test.", Qtype: dns.TypeA, Qclass: dns.ClassINET}

	resp := new(dns.Msg)
	resp.SetQuestion(q.Name, q.Qtype)
	resp.Response = true
	resp.Answer = []dns.RR{mustRR(t, "host.insecure.test. 300 IN A 192.0.2.20")}

	result := v.Validate(context.Background(), q, resp)
	if result.Status != Insecure {
		t.Fatalf("unsigned delegation = %s (%s), want insecure", result.Status, result.Reason)
	}
}

func TestValidateNXDOMAINNSEC(t *testing.T) {
	f := newChainFixture(t)
	v := f.validator()
	q := dns.Question{Name: "nosuch.example.test.", Qtype: dns.TypeA, Qclass: dns.ClassINET}

	resp := new(dns.Msg)
	resp.SetQuestion(q.Name, q.Qtype)
	resp.Response = true
	resp.Rcode = dns.RcodeNameError
	resp.Ns = f.fetcher.answers[fetchKey(q.Name, q.Qtype)].Ns

	result := v.Validate(context.Background(), q, resp)
	if result.Status != Secure {
		t.Fatalf("NSEC NXDOMAIN = %s (%s), want secure", result.Status, result.Reason)
	}
}

func TestNODATANSEC3(t *testing.T) {
	// example. is the zone, and these records prove that www.example. exists but
	// has no AAAA. The NSEC3 owner is the hashed name, as it is on the wire.
	zone := "example."
	salt := "AABBCCDD"
	name := "www.example."
	hash := dns.HashName(name, dns.SHA1, 1, salt)
	next := "FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF"
	nsec3 := &dns.NSEC3{
		Hdr:        dns.RR_Header{Name: hash + "." + zone, Rrtype: dns.TypeNSEC3, Class: dns.ClassINET, Ttl: 300},
		Hash:       dns.SHA1,
		Flags:      0,
		Iterations: 1,
		SaltLength: uint8(len(salt) / 2),
		Salt:       salt,
		HashLength: 20,
		NextDomain: next,
		TypeBitMap: []uint16{dns.TypeA, dns.TypeRRSIG},
	}
	if !nsec3.Match(name) {
		t.Fatal("fixture NSEC3 does not match its own name")
	}
	if nsec3NoData([]*dns.NSEC3{nsec3}, name, zone, dns.TypeAAAA) != true {
		t.Fatal("NSEC3 exact-match NODATA was not accepted")
	}
	if nsec3NoData([]*dns.NSEC3{nsec3}, name, zone, dns.TypeA) {
		t.Fatal("NSEC3 NODATA accepted a type the bitmap says exists")
	}
}

func TestCanonicalOrderAndNSECCover(t *testing.T) {
	ordered := []string{
		"example.", "*.example.", "a.example.", "b.example.", "www.example.",
	}
	for i := 1; i < len(ordered); i++ {
		if !canonicalLess(ordered[i-1], ordered[i]) {
			t.Fatalf("%q should sort before %q", ordered[i-1], ordered[i])
		}
	}
	nsec := mustRR(t, "a.example. 300 IN NSEC b.example. A RRSIG NSEC").(*dns.NSEC)
	if !nsecCovers(nsec, "ab.example.") {
		t.Fatal("NSEC did not cover a name between its owner and next")
	}
	if nsecCovers(nsec, "a.example.") || nsecCovers(nsec, "b.example.") {
		t.Fatal("NSEC covered its own owner or next name")
	}
}
