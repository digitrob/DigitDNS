package dns

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"strings"
)

// mtlsPool parses the operator's CA bundle. mTLS is an additional gate on the
// encrypted client listeners; the panel certificate and the subscriber portal
// are not affected.
func (s *Server) mtlsPool() (*x509.CertPool, error) {
	if !s.transportCfg.MTLSEnabled {
		return nil, nil
	}
	pem := strings.TrimSpace(s.transportCfg.MTLSCAPEM)
	if pem == "" {
		return nil, errors.New("mTLS is enabled but no client CA is configured")
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM([]byte(pem)) {
		return nil, errors.New("mTLS client CA bundle contains no valid certificates")
	}
	return pool, nil
}

// applyMTLS configures client-certificate verification. require is used by DoT,
// which has no portal sharing its listener. DoH and DoQ request a certificate
// here and enforce it in their handlers, so the portal on the DoH port stays
// reachable without one.
func (s *Server) applyMTLS(cfg *tls.Config, require bool) error {
	pool, err := s.mtlsPool()
	if err != nil {
		return err
	}
	if pool == nil {
		return nil
	}
	cfg.ClientCAs = pool
	if require {
		cfg.ClientAuth = tls.RequireAndVerifyClientCert
	} else {
		// Request a certificate when one is offered and verify it, but do not
		// require one: the portal shares the DoH listener and must stay
		// reachable. RequestClientCert would leave VerifiedChains empty even
		// for a valid certificate, so the handler could never accept one.
		cfg.ClientAuth = tls.VerifyClientCertIfGiven
	}
	return nil
}

func verifiedClientCert(state tls.ConnectionState) bool {
	return len(state.VerifiedChains) > 0
}
