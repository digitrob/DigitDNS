package proxy

import (
	"context"
	"errors"
	"io"
	"net"
	"net/netip"
	"testing"
	"time"
)

func TestDialVettedWithUsesFirstSuccessAndCancelsLosers(t *testing.T) {
	slowStarted := make(chan struct{})
	cancelled := make(chan struct{})
	winnerServer, winnerClient := net.Pipe()
	defer winnerServer.Close()
	defer winnerClient.Close()

	addrs := []netip.Addr{
		netip.MustParseAddr("192.0.2.1"),
		netip.MustParseAddr("192.0.2.2"),
	}
	dial := func(ctx context.Context, network, address string) (net.Conn, error) {
		if network != "tcp" {
			return nil, errors.New("wrong network: " + network)
		}
		if address == dialAddress(addrs[0], 443) {
			close(slowStarted)
			<-ctx.Done()
			close(cancelled)
			return nil, ctx.Err()
		}
		return winnerServer, nil
	}

	conn, err := dialVettedWith(context.Background(), addrs, 443, dial)
	if err != nil {
		t.Fatalf("dialVettedWith: %v", err)
	}
	if conn != winnerServer {
		t.Fatalf("winner = %p, want the second address's connection", conn)
	}
	select {
	case <-slowStarted:
	default:
		t.Fatal("the first address was never dialled")
	}
	select {
	case <-cancelled:
	case <-time.After(2 * time.Second):
		t.Fatal("the losing dial was not cancelled")
	}
}

func TestDialVettedWithClosesLateConnection(t *testing.T) {
	winnerServer, winnerClient := net.Pipe()
	defer winnerServer.Close()
	defer winnerClient.Close()
	lateServer, lateClient := net.Pipe()
	defer lateClient.Close()

	addrs := []netip.Addr{
		netip.MustParseAddr("192.0.2.1"),
		netip.MustParseAddr("192.0.2.2"),
	}
	dial := func(ctx context.Context, _, address string) (net.Conn, error) {
		if address == dialAddress(addrs[0], 443) {
			return winnerServer, nil
		}
		// This dial finishes only after the winner has been chosen, simulating
		// a second TCP handshake that completes just too late to matter.
		<-ctx.Done()
		return lateServer, nil
	}

	conn, err := dialVettedWith(context.Background(), addrs, 443, dial)
	if err != nil {
		t.Fatalf("dialVettedWith: %v", err)
	}
	if conn != winnerServer {
		t.Fatalf("winner = %p, want the first address's connection", conn)
	}
	_ = lateClient.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := lateClient.Read(make([]byte, 1)); !errors.Is(err, io.EOF) {
		t.Fatalf("late connection read = %v, want EOF (it must be closed)", err)
	}
}

func TestDialVettedWithReportsFirstCandidateError(t *testing.T) {
	errFirst := errors.New("first failed")
	errSecond := errors.New("second failed")
	addrs := []netip.Addr{
		netip.MustParseAddr("192.0.2.1"),
		netip.MustParseAddr("192.0.2.2"),
	}
	dial := func(_ context.Context, _, address string) (net.Conn, error) {
		if address == dialAddress(addrs[0], 443) {
			return nil, errFirst
		}
		return nil, errSecond
	}

	_, err := dialVettedWith(context.Background(), addrs, 443, dial)
	if !errors.Is(err, errFirst) {
		t.Fatalf("error = %v, want the first candidate's error", err)
	}
}

func TestDialVettedWithNoAddresses(t *testing.T) {
	_, err := dialVettedWith(context.Background(), nil, 443, func(context.Context, string, string) (net.Conn, error) {
		t.Fatal("dial called with no addresses")
		return nil, nil
	})
	if err == nil || err.Error() != "proxy: no target addresses to dial" {
		t.Fatalf("error = %v, want no-target error", err)
	}
}
