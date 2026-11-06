package dnssec

import "github.com/miekg/dns"

// rootAnchors are the IANA root key-signing-key DS records current at the time
// of writing: KSK-2017 (tag 20326) and KSK-2024 (tag 38696). Keeping both means
// a root KSK rollover does not require a release on the same day.
//
// Source: https://data.iana.org/root-anchors/root-anchors.xml
const (
	rootAnchor2017 = ". 172800 IN DS 20326 8 2 E06D44B80B8F1D39A95C0B0D7C65D08458E880409BBC683457104237C7F8EC8D"
	rootAnchor2024 = ". 172800 IN DS 38696 8 2 683D2D0ACB8C9B712A1948B27F741219298D0A450D612C483AF444A4C0FB2B16"
)

// DefaultTrustAnchorRecords returns the built-in anchors in zone-file form.
// The settings UI shows them as the default when the stored list is empty.
func DefaultTrustAnchorRecords() []string {
	return []string{rootAnchor2017, rootAnchor2024}
}

func defaultTrustAnchors() []*dns.DS {
	out := make([]*dns.DS, 0, 2)
	for _, text := range DefaultTrustAnchorRecords() {
		rr, err := dns.NewRR(text)
		if err != nil {
			// A malformed constant is a programming error, not runtime input.
			panic("dnssec: invalid built-in root anchor: " + err.Error())
		}
		out = append(out, rr.(*dns.DS))
	}
	return out
}
