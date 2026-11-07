package agent

import (
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
