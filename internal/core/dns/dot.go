package dns

import (
	"context"
	"crypto/tls"
	"net"
	"time"
)

// sniTokenListener accepts TLS connections, completes the handshake, and closes
// the connection before it reaches dns.Server when the SNI is not a configured
// token. Doing the check here keeps the query path unchanged and means an
// invalid token costs one handshake and no DNS work.
type sniTokenListener struct {
	net.Listener
	check   func(serverName string) bool
	timeout time.Duration
}

func (l *sniTokenListener) Accept() (net.Conn, error) {
	for {
		conn, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}
		tlsConn, ok := conn.(*tls.Conn)
		if !ok {
			_ = conn.Close()
			continue
		}
		timeout := l.timeout
		if timeout <= 0 {
			timeout = 5 * time.Second
		}
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		err = tlsConn.HandshakeContext(ctx)
		cancel()
		if err != nil {
			_ = conn.Close()
			continue
		}
		if l.check != nil && !l.check(tlsConn.ConnectionState().ServerName) {
			_ = conn.Close()
			continue
		}
		return tlsConn, nil
	}
}
