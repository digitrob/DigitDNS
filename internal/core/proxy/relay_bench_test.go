package proxy

import (
	"io"
	"net"
	"testing"
)

// BenchmarkCopyIdle compares the two relay bulk-copy paths with a full TCP
// socket on each side. The unmetered path hands the sockets to the kernel; the
// metered path keeps the userspace buffer because the byte counter has to see
// every chunk. Run with -benchmem to see the allocation difference as well.
func BenchmarkCopyIdleUnmetered(b *testing.B) {
	benchmarkCopyIdle(b, false)
}

func BenchmarkCopyIdleMetered(b *testing.B) {
	benchmarkCopyIdle(b, true)
}

func benchmarkCopyIdle(b *testing.B, metered bool) {
	srcClient, srcServer := newRelayTCPPair(b)
	dstServer, dstClient := newRelayTCPPair(b)
	defer srcClient.Close()
	defer srcServer.Close()
	defer dstServer.Close()
	defer dstClient.Close()

	payload := make([]byte, relayBufSize)
	start := make(chan struct{})
	feedErr := make(chan error, 1)
	go func() {
		<-start
		for i := 0; i < b.N; i++ {
			if _, err := srcClient.Write(payload); err != nil {
				feedErr <- err
				return
			}
		}
		_ = srcClient.CloseWrite()
		feedErr <- nil
	}()
	drainErr := make(chan error, 1)
	go func() {
		<-start
		_, err := io.Copy(io.Discard, dstClient)
		drainErr <- err
	}()

	var report func(uint64)
	var meteredBytes uint64
	if metered {
		report = func(n uint64) { meteredBytes += n }
	}

	b.SetBytes(int64(len(payload)))
	b.ReportAllocs()
	b.ResetTimer()
	close(start)
	n := copyIdle(dstServer, srcServer, payload, 0, report)
	b.StopTimer()

	if err := <-feedErr; err != nil {
		b.Fatalf("feed source: %v", err)
	}
	_ = dstServer.CloseWrite()
	if err := <-drainErr; err != nil {
		b.Fatalf("drain destination: %v", err)
	}
	want := uint64(b.N) * uint64(len(payload))
	if n != want {
		b.Fatalf("copyIdle counted %d bytes, want %d", n, want)
	}
	if metered && meteredBytes != want {
		b.Fatalf("meter saw %d bytes, want %d", meteredBytes, want)
	}
}

func newRelayTCPPair(b *testing.B) (client, server *net.TCPConn) {
	b.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		b.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	accepted := make(chan *net.TCPConn, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		accepted <- conn.(*net.TCPConn)
	}()
	conn, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		b.Fatalf("dial: %v", err)
	}
	return conn.(*net.TCPConn), <-accepted
}
