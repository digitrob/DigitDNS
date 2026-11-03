package web

import (
	"bytes"
	"strings"
	"testing"

	webAssets "digitdns/web"
)

// The dashboard brand mark is the Android launcher icon, served from the
// embedded asset tree. Two copies exist because the sidebar and the mobile
// header are separate document regions; both must point at the same file so
// the brand cannot drift between desktop and phone.
func TestDashboardBrandUsesTheAppIcon(t *testing.T) {
	doc := readAsset(t, "index.html")
	const src = `src="/img/digitdns-app-icon.png"`
	if got := strings.Count(doc, src); got != 2 {
		t.Fatalf("index.html carries %d dashboard logo references, want 2 (sidebar and mobile header)", got)
	}

	body, err := webAssets.StaticFS.ReadFile("img/digitdns-app-icon.png")
	if err != nil {
		t.Fatalf("embedded app icon is missing: %v", err)
	}
	if !bytes.HasPrefix(body, []byte("\x89PNG\r\n\x1a\n")) {
		t.Fatal("embedded app icon is not a PNG")
	}
}
