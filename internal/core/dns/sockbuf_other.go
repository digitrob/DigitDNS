//go:build !linux

package dns

import "syscall"

// udpBufferControl is a no-op outside Linux: the socket-buffer knobs used on
// Linux are not portable, and the default is left untouched.
func udpBufferControl(_, _ string, _ syscall.RawConn) error { return nil }
