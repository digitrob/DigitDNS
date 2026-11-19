package web

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"strings"
	"testing"

	webAssets "digitdns/web"
)

// sqliteWasmManifest is the pinned provenance record for the vendored SQLite
// build. A checksum test is not ceremony: the importer will run this binary in
// the operator's browser against a Pi-hole database, so a silent asset swap
// would be a supply-chain change to a parser that handles untrusted input.
type sqliteWasmManifest struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Source  string `json:"source"`
	License string `json:"license"`
	Files   map[string]struct {
		SHA256 string `json:"sha256"`
		Bytes  int    `json:"bytes"`
	} `json:"files"`
}

func TestSQLiteWasmAssetMatchesManifest(t *testing.T) {
	raw, err := fs.ReadFile(webAssets.StaticFS, "vendor/sql.js/manifest.json")
	if err != nil {
		t.Fatalf("read sql.js manifest: %v", err)
	}
	var manifest sqliteWasmManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatalf("parse sql.js manifest: %v", err)
	}
	if manifest.Name != "sql.js" || manifest.Version != "1.16.0" {
		t.Fatalf("manifest names %s %s, want sql.js 1.16.0", manifest.Name, manifest.Version)
	}
	if manifest.License != "MIT" {
		t.Errorf("manifest license = %q, want MIT", manifest.License)
	}
	if manifest.Source != "https://registry.npmjs.org/sql.js/-/sql.js-1.16.0.tgz" {
		t.Errorf("manifest source = %q, want the pinned npm tarball", manifest.Source)
	}
	if len(manifest.Files) != 2 {
		t.Fatalf("manifest has %d files, want sql-wasm.js and sql-wasm.wasm", len(manifest.Files))
	}

	for name, want := range manifest.Files {
		body, err := fs.ReadFile(webAssets.StaticFS, "vendor/sql.js/"+name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		sum := sha256.Sum256(body)
		if got := hex.EncodeToString(sum[:]); got != want.SHA256 {
			t.Errorf("%s sha256 = %s, want %s", name, got, want.SHA256)
		}
		if len(body) != want.Bytes {
			t.Errorf("%s has %d bytes, want %d", name, len(body), want.Bytes)
		}
		br, err := fs.ReadFile(webAssets.StaticFS, "vendor/sql.js/"+name+".br")
		if err != nil {
			t.Errorf("%s has no brotli sidecar: %v", name, err)
			continue
		}
		if len(br) >= len(body) {
			t.Errorf("%s brotli sidecar is %d bytes, not smaller than the %d-byte original", name, len(br), len(body))
		}
	}
}

func TestSQLiteWasmIsServedAsWebAssembly(t *testing.T) {
	server := newStaticServer(webAssets.StaticFS)
	asset := server.assets["vendor/sql.js/sql-wasm.wasm"]
	if asset == nil {
		t.Fatal("the static server does not carry sql-wasm.wasm")
	}
	if asset.contentType != "application/wasm" {
		t.Errorf("content type = %q, want application/wasm", asset.contentType)
	}
	if asset.br == nil {
		t.Error("the brotli sidecar was not attached to the served asset")
	}
	if !strings.Contains(contentSecurityPolicy, "'wasm-unsafe-eval'") {
		t.Error("the CSP does not permit WebAssembly compilation, so the in-panel SQLite importer cannot run")
	}
}
