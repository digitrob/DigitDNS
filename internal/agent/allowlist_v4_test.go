package agent

import (
	"net"
	"testing"
)

func TestClientListMatchesLoopbackLiteral(t *testing.T) {
	// Regression: net.ParseIP returns a 16-byte form for IPv4, and pairing that
	// with a 32-bit mask made IPNet.Contains never match. A client list holding
	// "127.0.0.1" refused localhost, which is the first thing anyone tests.
	c := NewClientList([]string{"127.0.0.1"})
	if !c.Allows(net.ParseIP("127.0.0.1")) {
		t.Fatal("a literal IPv4 entry must match that IPv4 address")
	}
}

func TestClientListIPv4CIDRMatchesIPv4(t *testing.T) {
	c := NewClientList([]string{"192.0.2.0/24"})
	if !c.Allows(net.ParseIP("192.0.2.77")) {
		t.Fatal("IPv4 CIDR must match an address inside it")
	}
	if c.Allows(net.ParseIP("192.0.3.77")) {
		t.Fatal("IPv4 CIDR must not match an address outside it")
	}
}

func TestClientListIPv6StillWorks(t *testing.T) {
	c := NewClientList([]string{"2001:db8::1"})
	if !c.Allows(net.ParseIP("2001:db8::1")) {
		t.Fatal("a literal IPv6 entry must match that address")
	}
	if c.Allows(net.ParseIP("2001:db8::2")) {
		t.Fatal("a literal IPv6 entry must not match a neighbour")
	}
}
