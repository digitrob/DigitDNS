//go:build linux

package dns

import "syscall"

// udpSocketBuffer is the receive buffer requested for the plain-DNS UDP
// listener. The kernel default is ~208 KB, which drops bursts at query peaks;
// each drop becomes a client retransmit and a visible latency spike. Linux caps
// the value at net.core.rmem_max, so this is a request, not a guarantee.
const udpSocketBuffer = 4 << 20

// udpBufferControl asks for a larger socket buffer before the listener starts.
// Every failure is ignored on purpose: a resolver that cannot raise its buffer
// must still serve DNS.
func udpBufferControl(_, _ string, c syscall.RawConn) error {
	_ = c.Control(func(fd uintptr) {
		_ = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_RCVBUF, udpSocketBuffer)
		_ = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_SNDBUF, udpSocketBuffer/2)
	})
	return nil
}
