package agent

import (
	"context"
	"net"
	"reflect"
	"testing"
	"time"
)

func TestListenAddressesIPv4Only(t *testing.T) {
	cases := []struct {
		name string
		in   string
		on   bool
		want []string
	}{
		{"dual stack default", ":80,:443", true, []string{":80", ":443"}},
		{"ipv4 only wildcard", ":80,:443", false, []string{"0.0.0.0:80", "0.0.0.0:443"}},
		{"explicit ipv6 dropped", ":80,[2001:db8::1]:443", false, []string{"0.0.0.0:80"}},
		{"explicit ipv4 kept", "127.0.0.1:18080,0.0.0.0:18443", false, []string{"127.0.0.1:18080", "0.0.0.0:18443"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := listenAddresses(tc.in, tc.on); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("listenAddresses(%q, %v) = %v, want %v", tc.in, tc.on, got, tc.want)
			}
		})
	}
}

func TestRefusalLogThrottlesAndReportsSuppressed(t *testing.T) {
	now := time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)
	rl := newRefusalLog(time.Minute, func() time.Time { return now })

	ok, suppressed := rl.allow("ip|203.0.113.5")
	if !ok || suppressed != 0 {
		t.Fatalf("first event = (%v, %d), want (true, 0)", ok, suppressed)
	}
	for i := 0; i < 10; i++ {
		ok, suppressed = rl.allow("ip|203.0.113.5")
		if ok || suppressed != 0 {
			t.Fatalf("suppressed event %d = (%v, %d), want (false, 0)", i, ok, suppressed)
		}
	}
	now = now.Add(time.Minute)
	ok, suppressed = rl.allow("ip|203.0.113.5")
	if !ok || suppressed != 10 {
		t.Fatalf("after interval = (%v, %d), want (true, 10)", ok, suppressed)
	}
}

func TestRefusalLogKeysAreIndependent(t *testing.T) {
	now := time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)
	rl := newRefusalLog(time.Minute, func() time.Time { return now })

	if ok, _ := rl.allow("ip|one"); !ok {
		t.Fatal("first key's first event was suppressed")
	}
	if ok, _ := rl.allow("ip|two"); !ok {
		t.Fatal("second key's first event was suppressed")
	}
	if ok, _ := rl.allow("ip|one"); ok {
		t.Fatal("repeated first key was not suppressed")
	}
}

func TestTargetDNSCacheHonoursTTLAndCopies(t *testing.T) {
	s := NewServer(Config{})
	ip := net.ParseIP("203.0.113.9")
	s.storeIPs("example.test", []net.IP{ip})

	got, ok := s.cachedIPs("example.test")
	if !ok || len(got) != 1 || !got[0].Equal(ip) {
		t.Fatalf("cache hit = %v, %v", got, ok)
	}
	got[0][len(got[0])-1] ^= 0xff
	again, _ := s.cachedIPs("example.test")
	if !again[0].Equal(ip) {
		t.Fatal("caller mutation reached the cached slice")
	}

	s.dnsMu.Lock()
	entry := s.dnsCache["example.test"]
	entry.expires = time.Now().Add(-time.Second)
	s.dnsCache["example.test"] = entry
	s.dnsMu.Unlock()
	if _, ok := s.cachedIPs("example.test"); ok {
		t.Fatal("expired cache entry was served")
	}
}

func TestAgentMalformedPenaltyBox(t *testing.T) {
	s := NewServer(Config{})
	now := time.Now()
	const ip = "198.51.100.10"
	for i := 0; i < malformedThreshold; i++ {
		s.noteMalformed(ip, now)
	}
	if !s.sourceBlocked(ip, now.Add(time.Second)) {
		t.Fatal("source was not blocked after the threshold")
	}
	if s.sourceBlocked("198.51.100.11", now.Add(time.Second)) {
		t.Fatal("unrelated source was blocked")
	}
	if s.sourceBlocked(ip, now.Add(floodCooldown+time.Second)) {
		t.Fatal("cooldown did not expire")
	}
}

func TestDialVettedFallsBackToReachableAddress(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port
	// 127.0.0.2 is loopback but nothing is listening on that port, so the first
	// address refuses and the second must win.
	addrs := []net.IP{net.ParseIP("127.0.0.2"), net.ParseIP("127.0.0.1")}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := dialVetted(ctx, addrs, port)
	if err != nil {
		t.Fatalf("dialVetted: %v", err)
	}
	_ = conn.Close()
}
