package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"testing"
)

// The Actions secret has been documented both as the 64-byte private key and as
// its 32-byte seed. decodeKey must accept both, because a release that fails on
// the secret's encoding blocks every publish.
func TestDecodeKeyAcceptsPrivateKeyAndSeed(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	seed := priv.Seed()

	for name, raw := range map[string][]byte{
		"private key": priv,
		"seed":        seed,
	} {
		key, err := decodeKey(base64.StdEncoding.EncodeToString(raw))
		if err != nil {
			t.Fatalf("%s: decodeKey: %v", name, err)
		}
		if !key.Public().(ed25519.PublicKey).Equal(pub) {
			t.Fatalf("%s: decoded key does not match the generated keypair", name)
		}
		msg := []byte("checksums")
		if !ed25519.Verify(pub, msg, ed25519.Sign(key, msg)) {
			t.Fatalf("%s: signature does not verify", name)
		}
	}
}

func TestDecodeKeyRejectsWrongSizes(t *testing.T) {
	for _, n := range []int{0, 31, 33, 63, 65} {
		if _, err := decodeKey(base64.StdEncoding.EncodeToString(make([]byte, n))); err == nil {
			t.Errorf("%d bytes: decodeKey accepted a key of the wrong size", n)
		}
	}
	if _, err := decodeKey("not base64!"); err == nil {
		t.Error("decodeKey accepted invalid base64")
	}
}
