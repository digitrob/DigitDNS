package dns

import (
	"fmt"
	"net"
	"testing"
	"time"

	dnscryptlib "github.com/ameshkov/dnscrypt/v2"
	"github.com/miekg/dns"
)

// freeDNSCryptPort reserves one port on both UDP and TCP. DNSCrypt uses the same
// number for both, so a UDP-only probe is not enough.
func freeDNSCryptPort(t *testing.T) int {
	t.Helper()
	for attempt := 0; attempt < 20; attempt++ {
		pc, err := net.ListenPacket("udp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("reserve UDP port: %v", err)
		}
		port := pc.LocalAddr().(*net.UDPAddr).Port
		_ = pc.Close()
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
		if err == nil {
			_ = ln.Close()
			return port
		}
	}
	t.Fatal("could not find a port free on both UDP and TCP")
	return 0
}

func TestApplyTransportsStartsDNSCryptUDPAndTCP(t *testing.T) {
	port := freeDNSCryptPort(t)
	s := &Server{
		handler:      testDoQHandler(t),
		bindHost:     "127.0.0.1",
		transportCfg: TransportConfig{DNSCryptEnabled: true, DNSCryptPort: port},
	}
	rc, err := dnscryptlib.GenerateResolverConfig("2.dnscrypt-cert.test.local", nil)
	if err != nil {
		t.Fatalf("GenerateResolverConfig: %v", err)
	}
	cert, err := rc.CreateCert()
	if err != nil {
		t.Fatalf("CreateCert: %v", err)
	}
	s.SetDNSCryptConfig(&rc, cert)
	if err := s.ApplyTransports(TransportConfig{DNSCryptEnabled: true, DNSCryptPort: port}); err != nil {
		t.Fatalf("start DNSCrypt: %v", err)
	}
	t.Cleanup(func() { _ = s.ApplyTransports(TransportConfig{DNSCryptPort: port}) })

	if listening, certNotAfter, listenerErr := s.DNSCryptStatus(); !listening || listenerErr != "" || certNotAfter == 0 {
		t.Fatalf("DNSCryptStatus = (%v, %d, %q)", listening, certNotAfter, listenerErr)
	}

	stamp, err := rc.CreateStamp(fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		t.Fatalf("CreateStamp: %v", err)
	}
	stampStr := stamp.String()
	if stampStr == "" {
		t.Fatal("empty DNSCrypt stamp")
	}

	q := new(dns.Msg)
	q.SetQuestion("pin.internal.test.", dns.TypeA)

	for _, network := range []string{"udp", "tcp"} {
		client := &dnscryptlib.Client{Net: network, Timeout: 3 * time.Second}
		info, err := client.Dial(stampStr)
		if err != nil {
			t.Fatalf("%s Dial: %v", network, err)
		}
		resp, err := client.Exchange(q, info)
		if err != nil {
			t.Fatalf("%s Exchange: %v", network, err)
		}
		if len(resp.Answer) != 1 {
			t.Fatalf("%s answers = %d, want 1", network, len(resp.Answer))
		}
		if a, ok := resp.Answer[0].(*dns.A); !ok || a.A.String() != "203.0.113.77" {
			t.Fatalf("%s answer = %#v", network, resp.Answer[0])
		}
	}

	m := s.TransportMetrics()
	if m.DNSCryptQueries < 2 {
		t.Errorf("DNSCrypt queries = %d, want at least 2", m.DNSCryptQueries)
	}
}
