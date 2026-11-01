package web

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestDOMSelectorsRemainValid(t *testing.T) {
	wd, _ := os.Getwd()
	var htmlPath, jsPath string
	candidates := []string{
		filepath.Join(wd, "..", "..", "web", "index.html"),
		filepath.Join(wd, "web", "index.html"),
		"/Users/sergei/Downloads/Project/web/index.html",
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			htmlPath = p
			break
		}
	}
	candidatesJS := []string{
		filepath.Join(wd, "..", "..", "web", "js", "app.js"),
		filepath.Join(wd, "web", "js", "app.js"),
		"/Users/sergei/Downloads/Project/web/js/app.js",
	}
	for _, p := range candidatesJS {
		if _, err := os.Stat(p); err == nil {
			jsPath = p
			break
		}
	}
	if htmlPath == "" || jsPath == "" {
		t.Fatalf("could not locate web files: html=%s js=%s", htmlPath, jsPath)
	}

	htmlBytes, err := os.ReadFile(htmlPath)
	if err != nil {
		t.Fatalf("read index.html: %v", err)
	}
	html := string(htmlBytes)

	jsBytes, err := os.ReadFile(jsPath)
	if err != nil {
		t.Fatalf("read js/app.js: %v", err)
	}
	js := string(jsBytes)

	reGetById := regexp.MustCompile(`getElementById\(['"]([^'"]+)['"]\)`)
	matches := reGetById.FindAllStringSubmatch(js, -1)
	if len(matches) == 0 {
		t.Fatal("no getElementById calls found")
	}

	for _, m := range matches {
		id := m[1]
		pattern := regexp.MustCompile(`id=["']` + regexp.QuoteMeta(id) + `["']`)
		if !pattern.MatchString(html) {
			t.Errorf("getElementById('%s') has no matching id in index.html", id)
		}
	}

	criticalClasses := []string{
		"tab-content",
		"domain-filter-chip",
		"sidebar-nav-item",
		"mobile-nav-item",
	}
	for _, cls := range criticalClasses {
		if !strings.Contains(html, cls) && !strings.Contains(js, cls) {
			t.Errorf("critical class '%s' not found", cls)
		}
	}
}
