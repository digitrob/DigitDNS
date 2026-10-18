package selfupdate

import (
	"crypto/ed25519"
	"encoding/base64"
	"net/http"
	"testing"
	"time"
)

// The release channel's trust comes from the signature, not the hash: both the
// binary and checksums.txt arrive from the same origin, so a bare SHA-256
// compare is an unauthenticated self-consistency test. These pin that the
// signature gate is the decisive check.
func TestChecksumsSignatureGate(t *testing.T) {
	body := []byte("abc123  digitdns-linux-amd64\n")
	goodPub, goodPriv, _ := ed25519.GenerateKey(nil)
	otherPub, _, _ := ed25519.GenerateKey(nil)

	sig := base64.StdEncoding.EncodeToString(ed25519.Sign(goodPriv, body))

	// Right key: accepted.
	if err := verifyChecksumsSignatureWith(body, sig, goodPub); err != nil {
		t.Fatalf("a correctly signed checksums.txt must verify: %v", err)
	}
	// Wrong key: refused.
	if err := verifyChecksumsSignatureWith(body, sig, otherPub); err == nil {
		t.Fatal("a signature from a foreign key must be refused")
	}
	// Tampered body: refused.
	if err := verifyChecksumsSignatureWith([]byte("deadbeef  digitdns-linux-amd64\n"), sig, goodPub); err == nil {
		t.Fatal("a signature over different bytes must be refused")
	}
	// Missing signature: refused — an unsigned bundle is never installable.
	if err := verifyChecksumsSignatureWith(body, "", goodPub); err == nil {
		t.Fatal("a missing signature must be refused")
	}
	// Garbage header: refused, not a panic.
	if err := verifyChecksumsSignatureWith(body, "!!!not-base64!!!", goodPub); err == nil {
		t.Fatal("an unparseable signature must be refused")
	}
	// Truncated signature: refused.
	if err := verifyChecksumsSignatureWith(body, sig[:20], goodPub); err == nil {
		t.Fatal("a truncated signature must be refused")
	}
}

func TestDefaultKeyIsUsable(t *testing.T) {
	if len(ReleasePubKey) != ed25519.PublicKeySize {
		t.Fatalf("ReleasePubKey is %d bytes, want %d", len(ReleasePubKey), ed25519.PublicKeySize)
	}
}

// Release selection must match the target version exactly. A substring test
// (strings.Contains(tag, "2.6.0")) accepts v2.6.0-beta.9, v2.6.01 and v12.6.0,
// so an operator who asked for a final build can receive a beta.
func TestTagMatchesTargetExact(t *testing.T) {
	cases := []struct {
		tag, target string
		want        bool
	}{
		{"v2.6.0", "2.6.0", true},
		{"v2.6.0", "v2.6.0", true},
		{"2.6.0", "2.6.0", true},
		{"v2.6.0-beta.1", "2.6.0-beta", true},   // channel name matches target's channel
		{"v2.6.0-beta.1", "2.6.0-beta.1", true}, // exact channel suffix
		{"v2.6.0", "2.6.0-beta", true},          // final satisfies a channel target
		{"v2.6.0-beta.1", "2.6.0", false},       // beta must NOT satisfy a final target
		{"v2.6.01", "2.6.0", false},             // superstring, not a match
		{"v12.6.0", "2.6.0", false},             // superstring, not a match
		{"v2.60.0", "2.6.0", false},             // numeric core differs
		{"v2.5.9", "2.6.0", false},              // older
		{"v3.0.0", "2.6.0", false},              // newer
		{"v2.7.0-rc.1", "2.6.0", false},         // version differs as well as channel
		{"", "2.6.0", false},
		{"v2.6.0-beta", "2.6.0-alpha", false}, // channel mismatch is not "compatible"
	}
	for _, c := range cases {
		if got := tagMatchesTarget(c.tag, c.target); got != c.want {
			t.Errorf("tagMatchesTarget(%q,%q) = %v, want %v", c.tag, c.target, got, c.want)
		}
	}
}

func TestChannelAndCoreHelpers(t *testing.T) {
	if got := targetNumericCore("2.6.0-beta.1"); got != "2.6.0" {
		t.Errorf("targetNumericCore = %q, want 2.6.0", got)
	}
	if got := targetNumericCore("2.6.0"); got != "2.6.0" {
		t.Errorf("targetNumericCore(final) = %q, want 2.6.0", got)
	}
	if got := targetChannel("2.6.0-beta.1"); got != "beta.1" {
		t.Errorf("targetChannel = %q, want beta.1", got)
	}
	if got := targetChannel("2.6.0"); got != "" {
		t.Errorf("targetChannel(final) = %q, want empty", got)
	}
}

// The update client must never honour HTTP(S)_PROXY: walking a root-executed
// update through a locally-trusted TLS-intercepting middlebox defeats the whole
// origin-authentication chain.
func TestUpdaterTransportIgnoresProxies(t *testing.T) {
	u := New("/nonexistent/digitdns", nil, "")
	tr, ok := u.client.Transport.(*http.Transport)
	if !ok || tr == nil {
		t.Fatalf("the update client must use an explicit *http.Transport, got %T", u.client.Transport)
	}
	if tr.Proxy != nil {
		t.Fatal("the update client must not honour a proxy")
	}
	if u.client.Timeout <= 0 || u.client.Timeout > 5*time.Minute {
		t.Fatalf("unexpected client timeout %v", u.client.Timeout)
	}
}
