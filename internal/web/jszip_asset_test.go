package web

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"testing"

	webAssets "digitdns/web"
)

type vendoredAssetManifest struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Source  string `json:"source"`
	License string `json:"license"`
	Files   map[string]struct {
		SHA256 string `json:"sha256"`
		Bytes  int    `json:"bytes"`
	} `json:"files"`
}

func TestJSZipAssetMatchesManifest(t *testing.T) {
	raw, err := fs.ReadFile(webAssets.StaticFS, "vendor/jszip/manifest.json")
	if err != nil {
		t.Fatalf("read jszip manifest: %v", err)
	}
	var manifest vendoredAssetManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatalf("parse jszip manifest: %v", err)
	}
	if manifest.Name != "jszip" || manifest.Version != "3.10.1" {
		t.Fatalf("manifest names %s %s, want jszip 3.10.1", manifest.Name, manifest.Version)
	}
	if manifest.Source != "https://registry.npmjs.org/jszip/-/jszip-3.10.1.tgz" {
		t.Errorf("manifest source = %q, want the pinned npm tarball", manifest.Source)
	}
	for name, want := range manifest.Files {
		body, err := fs.ReadFile(webAssets.StaticFS, "vendor/jszip/"+name)
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
		if _, err := fs.ReadFile(webAssets.StaticFS, "vendor/jszip/"+name+".br"); err != nil {
			t.Errorf("%s has no brotli sidecar: %v", name, err)
		}
	}
}
