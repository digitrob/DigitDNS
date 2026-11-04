package filterpresets

import (
	"net/url"
	"testing"
)

func TestPresetCatalogueIsCompleteAndUnique(t *testing.T) {
	all := All()
	if len(all) < 40 {
		t.Fatalf("only %d presets — the catalogue is incomplete", len(all))
	}
	ids := map[string]bool{}
	urls := map[string]bool{}
	bases := 0
	recommended := 0
	for _, p := range all {
		if p.ID == "" || p.Name == "" || p.Group == "" {
			t.Errorf("preset has an empty id/name/group: %#v", p)
		}
		if ids[p.ID] {
			t.Errorf("duplicate preset id %q", p.ID)
		}
		ids[p.ID] = true
		if urls[p.URL] {
			t.Errorf("duplicate preset URL %q", p.URL)
		}
		urls[p.URL] = true
		u, err := url.Parse(p.URL)
		if err != nil || u.Scheme != "https" || u.Host == "" {
			t.Errorf("%s has an invalid https URL: %q", p.ID, p.URL)
		}
		if p.Homepage == "" {
			t.Errorf("%s has no homepage", p.ID)
		} else if h, err := url.Parse(p.Homepage); err != nil || (h.Scheme != "https" && h.Scheme != "http") || h.Host == "" {
			t.Errorf("%s has an invalid homepage: %q", p.ID, p.Homepage)
		}
		if p.Base {
			bases++
		}
		if p.Recommended {
			recommended++
		}
	}
	if bases < 2 || recommended != 1 {
		t.Errorf("bases=%d recommended=%d, want at least 2 and exactly 1", bases, recommended)
	}
}

func TestPresetByID(t *testing.T) {
	if p, ok := ByID("hagezi-pro"); !ok || !p.Recommended || !p.Base {
		t.Fatalf("hagezi-pro = %#v/%v", p, ok)
	}
	if _, ok := ByID("does-not-exist"); ok {
		t.Fatal("unknown id resolved")
	}
}
