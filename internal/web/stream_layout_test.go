package web

import (
	"regexp"
	"strings"
	"testing"
)

// The live log renders into #tab-stream. Its full-height layout depends on
// #tab-stream being a flex column, but the pre-paint boot rule also targets the
// same id:
//
//	html[data-boot-tab="stream"] #tab-stream { display: block; }
//
// One id, one attribute and two elements out-specifies a bare
// `#tab-stream:not(.hidden)` (one id, one class). The v5.2.9 panel shipped with
// the bare form, so the section stayed a block, the glass panel had no definite
// height, .stream-viewport's flex:1 resolved to zero pixels, and the operator
// saw an empty table while a hundred rows sat in the DOM behind a 0px scroller.
// The stream itself was fine: the bug only existed in the cascade.
func TestStreamTabFlexRuleOutranksBootTabRule(t *testing.T) {
	css := regexp.MustCompile(`(?s)/\*.*?\*/`).ReplaceAllString(readAsset(t, "css/style.css"), "")

	// The boot rules share one selector list, so locate the #tab-stream entry
	// inside it rather than expecting a rule of its own.
	const bootSel = `html[data-boot-tab="stream"] #tab-stream`
	at := strings.Index(css, bootSel)
	if at < 0 {
		t.Fatal("the pre-paint boot rule for #tab-stream is missing; update this test if the rule moved")
	}
	open := strings.Index(css[at:], "{")
	close := strings.Index(css[at:], "}")
	if open < 0 || close < 0 || close < open {
		t.Fatal("could not read the boot rule's declaration block")
	}
	bootBody := css[at+open+1 : at+close]
	if !strings.Contains(bootBody, "display: block") {
		t.Fatalf("the boot rule no longer sets display:block (%s); the specificity conflict changed", strings.TrimSpace(bootBody))
	}

	// Find the rule that gives the tab its flex column, whatever its selector.
	ruleRe := regexp.MustCompile(`(?m)([^{}]+)\{([^}]*)\}`)
	var flexSel, flexBody string
	for _, m := range ruleRe.FindAllStringSubmatch(css, -1) {
		sel, body := strings.TrimSpace(m[1]), m[2]
		if strings.Contains(sel, "#tab-stream") && !strings.Contains(sel, ">") && strings.Contains(body, "display: flex") {
			flexSel, flexBody = sel, body
		}
	}
	if flexSel == "" {
		t.Fatal("no #tab-stream rule sets display:flex; the live log will collapse to an empty box")
	}
	if !strings.Contains(flexBody, "flex-direction: column") {
		t.Fatalf("#tab-stream flex rule lost flex-direction: column: %s", flexSel)
	}

	bootSpec := selectorSpecificity(bootSel)
	flexSpec := selectorSpecificity(flexSel)
	// Compare id, then class/attribute, then element, as CSS does.
	if bootSpec[0] > flexSpec[0] ||
		(bootSpec[0] == flexSpec[0] && bootSpec[1] > flexSpec[1]) ||
		(bootSpec[0] == flexSpec[0] && bootSpec[1] == flexSpec[1] && bootSpec[2] >= flexSpec[2]) {
		t.Fatalf("#tab-stream flex rule %q (specificity %v) does not out-rank the boot rule %q (%v); "+
			"the live log will render inside a zero-height viewport",
			flexSel, flexSpec, bootSel, bootSpec)
	}
}

// selectorSpecificity counts the id, class/attribute and element components of
// a selector. :not() takes the specificity of its argument, and the selectors
// this test compares contain no other pseudo-classes.
func selectorSpecificity(sel string) [3]int {
	var spec [3]int
	notRe := regexp.MustCompile(`:not\(([^)]*)\)`)
	if m := notRe.FindStringSubmatch(sel); m != nil {
		inner := selectorSpecificity(m[1])
		spec[1] += inner[1]
		spec[2] += inner[2]
	}
	sel = notRe.ReplaceAllString(sel, "")

	spec[0] = strings.Count(sel, "#")
	spec[1] += strings.Count(sel, ".") + strings.Count(sel, "[")

	// Elements are the bare tag tokens left after the id/class/attribute parts.
	cleaned := regexp.MustCompile(`#[A-Za-z0-9_-]+`).ReplaceAllString(sel, " ")
	cleaned = regexp.MustCompile(`\.[A-Za-z0-9_-]+`).ReplaceAllString(cleaned, " ")
	cleaned = regexp.MustCompile(`\[[^\]]*\]`).ReplaceAllString(cleaned, " ")
	for _, tag := range strings.Fields(cleaned) {
		if tag != "*" && tag != ">" && tag != "+" && tag != "~" {
			spec[2]++
		}
	}
	return spec
}
