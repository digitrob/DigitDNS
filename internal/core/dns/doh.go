package dns

import (
	"crypto/subtle"
	"encoding/base64"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"digitdns/internal/netutil"
	"github.com/miekg/dns"
)

type DoHHandler struct {
	dnsHandler *Handler
	// tokens, when non-empty, are the only values a DoH client may present as
	// ?token= to be served. Comparison is constant-time per entry. An empty
	// list means the endpoint is open to whoever the access layer allows.
	//
	// Guarded by mu: SetDoHTokens is called both at boot and live from the
	// dashboard's access card (POST /api/config/access), while tokenOK reads it
	// from every DoH request goroutine — an unguarded slice would be a data race
	// the moment the operator saved a token on a running server.
	mu     sync.RWMutex
	tokens []string
	// altSvc is the Alt-Svc value advertised to TCP DoH clients while the
	// HTTP/3 listener is up. Empty means DoH3 is disabled or failed to bind;
	// advertising a QUIC endpoint that is not listening only delays clients.
	altSvc string
	// doh3Requests counts HTTP/3 requests for the Prometheus endpoint. TCP DoH
	// already contributes to the query and protocol aggregates; this counter
	// answers "is anyone actually using the QUIC listener?".
	doh3Requests atomic.Uint64
	// mtlsRequire is the DoH/DoH3 half of the global mTLS gate. DoT enforces it
	// at the TLS handshake; DoH requests a certificate at the handshake so the
	// portal on the same listener remains reachable, and requires a verified
	// chain here for /dns-query.
	mtlsRequire atomic.Bool
	// enabled gates the TCP DoH endpoint. The listener also serves the
	// subscriber portal, so disabling DoH must not stop it. HTTP/3 is a separate
	// listener and is not affected: DoH3 keeps working when TCP DoH is off.
	enabled atomic.Bool
}

func NewDoHHandler(h *Handler) *DoHHandler {
	doh := &DoHHandler{dnsHandler: h}
	doh.enabled.Store(true)
	return doh
}

// SetEnabled turns the TCP DoH endpoint on or off. The HTTPS listener and the
// subscriber portal routes sharing its port stay up either way.
func (h *DoHHandler) SetEnabled(enabled bool) { h.enabled.Store(enabled) }

// SetDoHTokens installs the operator's DoH bearer tokens. An empty list
// disables token checking. Wired from the "access" record (dashboard card and
// config.json doh_tokens) — previously the card wrote the list and nothing
// ever read it (v2.1.0 B-07 remediation), and until v2.5 a live save persisted
// but never reached this method, so the gate stayed at its boot-time state
// until the next restart.
func (h *DoHHandler) SetDoHTokens(tokens []string) {
	// Copy so a later mutation of the caller's slice cannot reach into the live
	// gate behind the lock.
	cp := append([]string(nil), tokens...)
	h.mu.Lock()
	h.tokens = cp
	h.mu.Unlock()
}

// tokenOK reports whether the request's token is accepted. The token may be
// supplied as ?token= (the historical form for clients that cannot set headers)
// or as Authorization: Bearer. Constant-time per entry so a wrong guess reveals
// nothing about which prefix matched.
func (h *DoHHandler) tokenOK(r *http.Request) bool {
	h.mu.RLock()
	tokens := h.tokens
	h.mu.RUnlock()
	if len(tokens) == 0 {
		return true
	}
	given := r.URL.Query().Get("token")
	if given == "" {
		if auth := r.Header.Get("Authorization"); len(auth) > 7 && strings.EqualFold(auth[:7], "Bearer ") {
			given = strings.TrimSpace(auth[7:])
		}
	}
	if given == "" {
		return false
	}
	ok := false
	for _, t := range tokens {
		if subtle.ConstantTimeCompare([]byte(t), []byte(given)) == 1 {
			ok = true
		}
	}
	return ok
}

// SetAltSvc advertises the HTTP/3 endpoint on TCP DoH responses. The DNS
// server calls it when the DoH3 listener starts and clears it when it stops;
// an empty value means no h3 endpoint is listening.
func (h *DoHHandler) SetAltSvc(value string) {
	h.mu.Lock()
	h.altSvc = value
	h.mu.Unlock()
}

func (h *DoHHandler) altSvcValue() string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.altSvc
}

func (h *DoHHandler) DoH3Requests() uint64 { return h.doh3Requests.Load() }

// SetMTLSRequire turns the DoH/DoH3 client-certificate gate on or off. The
// listener must already have been configured to request a certificate.
func (h *DoHHandler) SetMTLSRequire(require bool) { h.mtlsRequire.Store(require) }

func (h *DoHHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.ProtoMajor != 3 && !h.enabled.Load() {
		http.NotFound(w, r)
		return
	}
	// No CORS headers (v2.1.0 A-05 remediation): RFC 8484 clients — browsers via
	// the standard API, stub resolvers — issue no preflighted cross-origin reads
	// that need them, and an open wildcard let any third-party website use this
	// resolver through its visitors' browsers. The endpoint is a service for
	// this server's subscribers, not a public cross-origin resource.
	if r.Method == http.MethodOptions {
		w.Header().Set("Allow", "GET, POST")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if alt := h.altSvcValue(); alt != "" {
		w.Header().Set("Alt-Svc", alt)
	}

	if h.mtlsRequire.Load() {
		if r.TLS == nil || !verifiedClientCert(*r.TLS) {
			http.Error(w, "Client certificate required", http.StatusForbidden)
			return
		}
	}

	clientIP := netutil.ClientIP(r)

	if !h.tokenOK(r) {
		http.Error(w, "Missing or invalid DoH token", http.StatusUnauthorized)
		return
	}

	var reqMsg *dns.Msg
	switch r.Method {
	case http.MethodGet:
		dnsQueryParam := r.URL.Query().Get("dns")
		if dnsQueryParam == "" {
			http.Error(w, "Missing 'dns' query parameter", http.StatusBadRequest)
			return
		}
		raw, err := base64.RawURLEncoding.DecodeString(dnsQueryParam)
		if err != nil {
			raw, err = base64.URLEncoding.DecodeString(dnsQueryParam)
			if err != nil {
				http.Error(w, "Invalid base64 payload", http.StatusBadRequest)
				return
			}
		}
		// A DoH wire message is small; the POST branch caps the body at 4 KiB with
		// io.LimitReader, and the GET branch has to enforce the same ceiling or a
		// GET ?dns= is decoded and Unpack'd with no length bound at all — the same
		// per-request parse work the POST path refuses (v2.5 GET/POST parity fix).
		if len(raw) > 4096 {
			http.Error(w, "DNS query too large", http.StatusBadRequest)
			return
		}
		reqMsg = new(dns.Msg)
		if err := reqMsg.Unpack(raw); err != nil {
			http.Error(w, "Invalid DNS wire format", http.StatusBadRequest)
			return
		}

	case http.MethodPost:
		body, err := io.ReadAll(io.LimitReader(r.Body, 4096))
		if err != nil {
			http.Error(w, "Failed to read request body", http.StatusBadRequest)
			return
		}
		reqMsg = new(dns.Msg)
		if err := reqMsg.Unpack(body); err != nil {
			http.Error(w, "Invalid DNS wire format", http.StatusBadRequest)
			return
		}

	default:
		// RFC 8484 defines exactly these two methods for DoH, and RFC 9110 §15.5.6 asks a 405
		// to say so: a resolver client that guessed wrong is told which verb to retry with
		// instead of being left to conclude the endpoint is not DoH at all.
		w.Header().Set("Allow", "GET, POST")
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	// peerIP is the immediate TCP peer: the rate limiter's loopback exemption
	// keys on it, so a forwarded header can never claim the exemption.
	peerIP := netutil.PeerIP(r)
	// HTTP/3 requests arrive on the same handler as TCP DoH. r.ProtoMajor is
	// 3 for HTTP/3, which keeps the protocol breakdown honest without a second
	// handler or a second token gate.
	proto := "DoH"
	if r.ProtoMajor == 3 {
		proto = "DoH3"
		h.doh3Requests.Add(1)
	}
	resp := h.dnsHandler.ProcessQuery(reqMsg, clientIP, proto, peerIP)
	if resp == nil {
		http.Error(w, "DNS processing failure", http.StatusInternalServerError)
		return
	}

	packed, err := resp.Pack()
	if err != nil {
		http.Error(w, "Failed to pack DNS response", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/dns-message")
	// RFC 8484 §5.1: the HTTP freshness lifetime must not outlive the DNS TTL,
	// otherwise intermediary caches keep serving answers the resolver retired.
	if ttl := minAnswerTTL(resp); ttl > 0 {
		w.Header().Set("Cache-Control", "max-age="+strconv.FormatUint(uint64(ttl), 10))
	} else {
		w.Header().Set("Cache-Control", "no-store")
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(packed)
}

// minAnswerTTL is the smallest TTL across all non-OPT records, i.e. how long
// the whole message stays valid.
func minAnswerTTL(msg *dns.Msg) uint32 {
	var ttl uint32
	first := true
	for _, section := range [][]dns.RR{msg.Answer, msg.Ns, msg.Extra} {
		for _, rr := range section {
			h := rr.Header()
			if h.Rrtype == dns.TypeOPT {
				continue
			}
			if first || h.Ttl < ttl {
				ttl, first = h.Ttl, false
			}
		}
	}
	return ttl
}
