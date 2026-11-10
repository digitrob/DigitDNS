package web

import (
	"fmt"
	"strings"

	dnscryptlib "github.com/ameshkov/dnscrypt/v2"

	"digitdns/internal/database"
)

const defaultDNSCryptPort = 5443

// loadDNSCryptSettings returns the persisted key material, or a zero value when
// the setting has never been written. The keys are encrypted by the database
// layer; this helper never logs them.
func (ws *WebServer) loadDNSCryptSettings() database.DNSCryptSettings {
	var st database.DNSCryptSettings
	_ = ws.db.GetSetting("dnscrypt", &st)
	return st
}

// ensureDNSCrypt generates the provider/resolver keys on first enable and
// regenerates the signed certificate on every save. The provider public key is
// the stable identity in every client stamp; changing it requires a deliberate
// key rotation.
func (ws *WebServer) ensureDNSCrypt(providerName string) (database.DNSCryptSettings, *dnscryptlib.ResolverConfig, *dnscryptlib.Cert, error) {
	st := ws.loadDNSCryptSettings()
	providerName = normalizeDNSCryptProviderName(providerName)

	if st.PublicKey == "" || st.PrivateKey == "" || st.ResolverPk == "" || st.ResolverSk == "" {
		rc, err := dnscryptlib.GenerateResolverConfig(providerName, nil)
		if err != nil {
			return st, nil, nil, fmt.Errorf("generate DNSCrypt keys: %w", err)
		}
		st = database.DNSCryptSettings{
			ProviderName: rc.ProviderName,
			PublicKey:    rc.PublicKey,
			PrivateKey:   rc.PrivateKey,
			ResolverPk:   rc.ResolverPk,
			ResolverSk:   rc.ResolverSk,
			EsVersion:    int(rc.EsVersion),
		}
	}
	st.ProviderName = providerName

	rc := dnscryptlib.ResolverConfig{
		ProviderName: st.ProviderName,
		PublicKey:    st.PublicKey,
		PrivateKey:   st.PrivateKey,
		ResolverPk:   st.ResolverPk,
		ResolverSk:   st.ResolverSk,
		EsVersion:    dnscryptlib.CryptoConstruction(st.EsVersion),
	}
	cert, err := rc.CreateCert()
	if err != nil {
		return st, nil, nil, fmt.Errorf("generate DNSCrypt certificate: %w", err)
	}
	st.CertNotAfter = int64(cert.NotAfter)
	if err := ws.db.SetSetting("dnscrypt", st); err != nil {
		return st, nil, nil, fmt.Errorf("persist DNSCrypt keys: %w", err)
	}
	return st, &rc, cert, nil
}

// dnscryptRuntime loads the persisted keys without generating anything. It is
// used by the view and by rollback paths.
func (ws *WebServer) dnscryptRuntime() (*dnscryptlib.ResolverConfig, *dnscryptlib.Cert) {
	st := ws.loadDNSCryptSettings()
	if st.PublicKey == "" || st.PrivateKey == "" || st.ResolverPk == "" || st.ResolverSk == "" {
		return nil, nil
	}
	rc := &dnscryptlib.ResolverConfig{
		ProviderName: st.ProviderName,
		PublicKey:    st.PublicKey,
		PrivateKey:   st.PrivateKey,
		ResolverPk:   st.ResolverPk,
		ResolverSk:   st.ResolverSk,
		EsVersion:    dnscryptlib.CryptoConstruction(st.EsVersion),
	}
	cert, err := rc.CreateCert()
	if err != nil {
		return nil, nil
	}
	return rc, cert
}

func (ws *WebServer) dnscryptStamp(st database.DNSCryptSettings, port int) string {
	if st.PublicKey == "" || st.ProviderName == "" {
		return ""
	}
	rc := dnscryptlib.ResolverConfig{
		ProviderName: st.ProviderName,
		PublicKey:    st.PublicKey,
		PrivateKey:   st.PrivateKey,
		ResolverPk:   st.ResolverPk,
		ResolverSk:   st.ResolverSk,
		EsVersion:    dnscryptlib.CryptoConstruction(st.EsVersion),
	}
	host := ws.settings.GetPublicIP()
	if strings.TrimSpace(host) == "" {
		// A domain-only install still has a name clients can reach; a stamp
		// pointing at loopback would be unusable on every other device.
		host = ws.effectivePanelDomain()
	}
	if strings.TrimSpace(host) == "" {
		host = "127.0.0.1"
	}
	stamp, err := rc.CreateStamp(fmt.Sprintf("%s:%d", host, port))
	if err != nil {
		return ""
	}
	return stamp.String()
}

func normalizeDNSCryptProviderName(raw string) string {
	name := strings.ToLower(strings.TrimSpace(strings.TrimSuffix(raw, ".")))
	name = strings.TrimPrefix(name, "2.dnscrypt-cert.")
	if name == "" {
		name = "digitdns.local"
	}
	return "2.dnscrypt-cert." + name
}

// PrepareDNSCrypt generates or loads the key material and installs it on the DNS
// server before Start. It is called at boot when DNSCrypt is already enabled.
func (ws *WebServer) PrepareDNSCrypt(providerName string) error {
	_, rc, cert, err := ws.ensureDNSCrypt(providerName)
	if err != nil {
		return err
	}
	if ws.dnscryptApplier != nil {
		return ws.dnscryptApplier(rc, cert)
	}
	return nil
}
