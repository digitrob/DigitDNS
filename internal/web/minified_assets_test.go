package web

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"strings"
	"testing"

	webAssets "digitdns/web"
)

// The browser is served the minified copy, but the source is what the tests and
// future maintainers read. The banner is the contract between the two: editing
// app.js or style.css without running scripts/build-web-assets.sh makes this test
// fail instead of silently shipping stale bytes.
func TestMinifiedAssetsMatchSource(t *testing.T) {
	cases := []struct {
		source string
		min    string
	}{
		{"js/app.js", "js/app.min.js"},
		{"css/style.css", "css/style.min.css"},
	}
	for _, tc := range cases {
		source, err := fs.ReadFile(webAssets.StaticFS, tc.source)
		if err != nil {
			t.Fatalf("read %s: %v", tc.source, err)
		}
		minified, err := fs.ReadFile(webAssets.StaticFS, tc.min)
		if err != nil {
			t.Fatalf("read %s: %v", tc.min, err)
		}
		sum := sha256.Sum256(source)
		want := hex.EncodeToString(sum[:])
		firstLine, _, _ := strings.Cut(string(minified), "\n")
		if !strings.Contains(firstLine, want) {
			t.Errorf("%s is stale: banner %q does not contain sha256(%s)=%s; run scripts/build-web-assets.sh",
				tc.min, firstLine, tc.source, want)
		}
		if len(minified) >= len(source) {
			t.Errorf("%s is %d bytes, not smaller than %s (%d)", tc.min, len(minified), tc.source, len(source))
		}
	}
}

// The served canonical paths must carry the minified bytes, not just the source
// file that happens to share the name.
func TestStaticServerServesMinifiedAliases(t *testing.T) {
	s := newStaticServer(webAssets.StaticFS)
	for _, name := range []string{"js/app.js", "css/style.css"} {
		asset := s.assets[name]
		if asset == nil {
			t.Fatalf("%s is not in the static server", name)
		}
		if !strings.Contains(string(asset.body), "digitdns-minified:sha256=") {
			t.Errorf("%s was not replaced by its minified copy", name)
		}
	}
}
