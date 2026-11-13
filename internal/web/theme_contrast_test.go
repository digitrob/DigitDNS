package web

import (
	"math"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// The dashboard ships three themes that share one token vocabulary: :root/[data-theme="dark"]
// defines every --c-* channel token and the grey and light blocks override a subset, so a
// token a theme does not restate is inherited. This file recomputes the WCAG contrast ratio
// for the foreground/background pairs the panel actually paints, in all three themes, and
// fails when one drops below the AA floor.
//
// It exists because a contrast failure is invisible in every way that normally catches a
// regression: the page still renders, the tests still pass, and the only signal is a user who
// cannot read the text. The grey theme's primary button is the case in point — its neutral
// accent ramp made --c-cyan-500 a mid-grey, and the near-black label on it measured 4.12:1
// while the same button cleared 10.8:1 in dark. Nothing else noticed.
//
// The inputs are read from the embedded assets, so the test measures the bytes a browser is
// served rather than the working tree.

// cssRule returns the declaration block of the first rule whose selector is exactly the
// given string, matched at the start of a line so a descendant rule such as
// `[data-theme="grey"] .panel-btn-primary` is not mistaken for the theme block itself.
// Comments are already stripped by readCSSRules, so a selector named in prose cannot match.
func cssRule(t *testing.T, css, selector string) string {
	t.Helper()
	re := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(selector) + `\s*\{`)
	loc := re.FindStringIndex(css)
	if loc == nil {
		t.Fatalf("css/style.css has no %q rule", selector)
	}
	open := loc[1] - 1 // the index of the '{'
	depth := 0
	for i := open; i < len(css); i++ {
		switch css[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return css[open+1 : i]
			}
		}
	}
	t.Fatalf("css/style.css has an unterminated %q rule", selector)
	return ""
}

// cssRGBTokenRe matches a channel token: `--c-slate-900: 15 23 42;`.
var cssRGBTokenRe = regexp.MustCompile(`--c-([a-z0-9-]+):\s*(\d+)\s+(\d+)\s+(\d+)\s*;`)

func cssRGBTokens(t *testing.T, block string) map[string][3]int {
	t.Helper()
	out := map[string][3]int{}
	for _, m := range cssRGBTokenRe.FindAllStringSubmatch(block, -1) {
		r, err1 := strconv.Atoi(m[2])
		g, err2 := strconv.Atoi(m[3])
		b, err3 := strconv.Atoi(m[4])
		if err1 != nil || err2 != nil || err3 != nil {
			t.Fatalf("token --c-%s has a non-numeric channel: %v", m[1], m[2:])
		}
		out[m[1]] = [3]int{r, g, b}
	}
	return out
}

// mergeTokens layers a theme's overrides on top of the shared dark vocabulary, which is how
// the cascade resolves a token the theme does not restate.
func mergeTokens(base, over map[string][3]int) map[string][3]int {
	out := make(map[string][3]int, len(base)+len(over))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range over {
		out[k] = v
	}
	return out
}

// cssBackgroundVarRe pulls the token name out of `background: rgb(var(--c-cyan-500) / 1)`.
var cssBackgroundVarRe = regexp.MustCompile(`background:\s*rgb\(var\(--c-([a-z0-9-]+)\)`)

// primaryFillToken reports the --c-* token a rule paints as its background fill.
func primaryFillToken(t *testing.T, css, selector string) string {
	t.Helper()
	m := cssBackgroundVarRe.FindStringSubmatch(cssRule(t, css, selector))
	if m == nil {
		t.Fatalf("%s has no `background: rgb(var(--c-…))` fill to measure", selector)
	}
	return m[1]
}

// relativeLuminance is the WCAG 2.x definition: sRGB channels linearised, then weighted.
func relativeLuminance(c [3]int) float64 {
	lin := func(v int) float64 {
		s := float64(v) / 255
		if s <= 0.04045 {
			return s / 12.92
		}
		return math.Pow((s+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(c[0]) + 0.7152*lin(c[1]) + 0.0722*lin(c[2])
}

// contrastRatio is (lighter + 0.05) / (darker + 0.05), the WCAG contrast ratio.
func contrastRatio(a, b [3]int) float64 {
	la, lb := relativeLuminance(a), relativeLuminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

// minContrastAA is the WCAG AA floor for normal-size text. The panel's smallest tier — the
// 9-12px metadata that uses slate-500 — is normal text, so it is held to this, not to the
// 3:1 large-text allowance.
const minContrastAA = 4.5

func TestThemeTextContrast(t *testing.T) {
	css := readCSSRules(t, "css/style.css")
	root := cssRGBTokens(t, cssRule(t, css, `[data-theme="dark"]`))
	themes := []struct {
		name  string
		token map[string][3]int
	}{
		{"dark", root},
		{"grey", mergeTokens(root, cssRGBTokens(t, cssRule(t, css, `[data-theme="grey"]`)))},
		{"light", mergeTokens(root, cssRGBTokens(t, cssRule(t, css, `[data-theme="light"]`)))},
	}

	// Every foreground the panel paints on a page or card surface, with the surface it sits
	// on. Adding a tier to the ramp means adding it here.
	pairs := []struct{ fg, bg, why string }{
		{"slate-100", "slate-900", "body text on the page"},
		{"slate-100", "slate-800", "body text on a raised surface"},
		{"slate-300", "slate-900", "secondary text on the page"},
		{"slate-400", "slate-900", "muted text on the page"},
		{"slate-400", "slate-800", "muted text on a card"},
		{"slate-500", "slate-900", "faint metadata on the page"},
		{"slate-500", "slate-800", "faint metadata on a card"},
		{"cyan-400", "slate-900", "accent text on the page"},
		{"cyan-300", "slate-900", "accent-strong text on the page"},
		{"emerald-300", "slate-900", "success text"},
		{"emerald-400", "slate-900", "success text (alt)"},
		{"red-300", "slate-900", "danger text"},
		{"red-400", "slate-900", "danger text (alt)"},
		{"amber-300", "slate-900", "warning text"},
		{"amber-400", "slate-900", "warning text (alt)"},
		{"purple-300", "slate-900", "purple text"},
		{"orange-400", "slate-900", "orange text"},
	}
	for _, th := range themes {
		for _, p := range pairs {
			fg, okF := th.token[p.fg]
			bg, okB := th.token[p.bg]
			if !okF || !okB {
				t.Fatalf("%s theme is missing --c-%s or --c-%s (%s)", th.name, p.fg, p.bg, p.why)
			}
			if r := contrastRatio(fg, bg); r < minContrastAA {
				t.Errorf("%s theme: %s (--c-%s on --c-%s) is %.2f:1, below the %.1f:1 AA floor",
					th.name, p.why, p.fg, p.bg, r, minContrastAA)
			}
		}
	}

	// The primary button is a near-black label on a coloured fill, and the fill is a token
	// rather than a literal. Dark and light paint --c-cyan-500; the grey theme's cyan ramp is
	// neutral there, so the "Grey theme buttons" block further down repaints this button to
	// --c-slate-200 with a slate-950 label. Resolve whatever token each theme's rule names,
	// so moving or weakening that repaint fails here exactly as it would on screen.
	fills := map[string]string{
		"dark":  primaryFillToken(t, css, ".panel-btn-primary"),
		"light": primaryFillToken(t, css, ".panel-btn-primary"),
		"grey":  primaryFillToken(t, css, `[data-theme="grey"] .panel-btn-primary`),
	}
	for _, th := range themes {
		label, okL := th.token["slate-950"]
		fill, okF := th.token[fills[th.name]]
		if !okL || !okF {
			t.Fatalf("%s theme is missing --c-slate-950 or the primary fill --c-%s", th.name, fills[th.name])
		}
		if r := contrastRatio(label, fill); r < minContrastAA {
			t.Errorf("%s theme: primary button label (slate-950 on --c-%s) is %.2f:1, below the %.1f:1 AA floor",
				th.name, fills[th.name], r, minContrastAA)
		}
	}

	// text-onfill is a literal white that does NOT invert with the ramp (unlike the --c-white
	// token, which is near-black on the light page), so it is measured directly. It labels the
	// solid danger fill.
	white := [3]int{255, 255, 255}
	for _, th := range themes {
		fill, ok := th.token["red-600"]
		if !ok {
			t.Fatalf("%s theme is missing --c-red-600", th.name)
		}
		if r := contrastRatio(white, fill); r < minContrastAA {
			t.Errorf("%s theme: on-fill label (white on --c-red-600) is %.2f:1, below the %.1f:1 AA floor",
				th.name, r, minContrastAA)
		}
	}
}

// These two small captions sit over component backgrounds rather than the simple
// slate-900/slate-800 surfaces above. Their colors are checked structurally here
// so they stay on the theme's text tokens without parent opacity reducing contrast.
func TestDashboardCaptionsKeepThemeContrast(t *testing.T) {
	css := readCSSRules(t, "css/style.css")

	sidebar := cssRule(t, css, ".sidebar-group-label")
	if !regexp.MustCompile(`color:\s*var\(--text-muted\)\s*;`).MatchString(sidebar) {
		t.Error(".sidebar-group-label must use the theme's contrast-tested --text-muted color")
	}
	if regexp.MustCompile(`opacity\s*:`).MatchString(sidebar) {
		t.Error(".sidebar-group-label must not reduce its already-muted text color with opacity")
	}

	onboarding := cssRule(t, css, ".onboarding-step-desc")
	if !regexp.MustCompile(`color:\s*var\(--text-main\)\s*;`).MatchString(onboarding) {
		t.Error(".onboarding-step-desc must use --text-main to remain readable on its tinted step background")
	}
}

// The resolver strategy/transport chips and the two compact dashboard actions
// were 17-20px tall: smaller than WCAG 2.2 AA's 24px pointer-target minimum.
// Keep them comfortably tappable without imposing full-size form-control height
// on these dense, grouped controls.
func TestCompactDashboardButtonsMeetPointerTargetFloor(t *testing.T) {
	css := readCSSRules(t, "css/style.css")
	for _, selector := range []string{
		".resolver-chip",
		".global-notice-btn",
		".history-top-provider-toggle",
	} {
		rule := cssRule(t, css, selector)
		m := regexp.MustCompile(`min-height:\s*(\d+)px\s*;`).FindStringSubmatch(rule)
		if m == nil {
			t.Errorf("%s has no explicit minimum target height", selector)
			continue
		}
		height, err := strconv.Atoi(m[1])
		if err != nil || height < 24 {
			t.Errorf("%s minimum target height is %qpx; want at least 24px", selector, m[1])
		}
	}
}

func TestSystemHighContrastPreferenceStrengthensControls(t *testing.T) {
	css := readCSSRules(t, "css/style.css")
	rule := cssRule(t, css, "@media (prefers-contrast: more)")
	for _, declaration := range []string{
		"--border-color: rgb(var(--c-slate-500) / 1);",
		"--track-border: rgb(var(--c-slate-500) / 1);",
		"--disabled-opacity: 0.65;",
		"outline: 3px solid var(--accent-cyan) !important;",
		".switch input:focus-visible + .slider",
	} {
		if !strings.Contains(rule, declaration) {
			t.Errorf("high-contrast preference is missing %q", declaration)
		}
	}
}
