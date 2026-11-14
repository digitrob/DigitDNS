package web

import (
	"crypto/sha256"
	"encoding/base64"
	"io/fs"
	"regexp"

	webAssets "digitdns/web"
)

// The dashboard's Content-Security-Policy.
//
// script-src carries the SHA-256 of every inline <script> the served documents
// actually contain instead of 'unsafe-inline'. The two dashboard blocks are the
// pre-paint theme/language restore and the route hint; both have to run before
// the first paint, so moving them to a file would reintroduce the flash they
// exist to prevent. A hash permits exactly those bytes and nothing else, which
// is the part that matters against a stored-XSS payload in a domain name or a
// client note: an injected <script> has a different hash and is refused.
//
// login.html's main block is not static — the server splices the hidden admin
// namespace into it — so serveLogin computes that document's hashes per admin
// path and sets the header itself.
//
// style-src keeps 'unsafe-inline' for the operator's custom portal stylesheet,
// which is rendered as a <style> element by design.
//
// The directives after script-src live in cspRest so the dashboard and login
// builders cannot drift apart.
//
// inlineScriptRe captures a script's attributes and its body. </script> does not
// match the open: the slash precedes the name.
var inlineScriptRe = regexp.MustCompile(`(?s)<script\b([^>]*)>(.*?)</script>`)

// scriptSrcAttrRe distinguishes an external block from an inline one.
var scriptSrcAttrRe = regexp.MustCompile(`\bsrc\s*=`)

// inlineScriptHashes returns the CSP hash tokens for a document's inline
// scripts. A block with a src is external and needs no hash.
func inlineScriptHashes(doc []byte) []string {
	var hashes []string
	for _, m := range inlineScriptRe.FindAllSubmatch(doc, -1) {
		if scriptSrcAttrRe.Match(m[1]) {
			continue
		}
		if len(m[2]) == 0 {
			continue
		}
		sum := sha256.Sum256(m[2])
		hashes = append(hashes, "'sha256-"+base64.StdEncoding.EncodeToString(sum[:])+"'")
	}
	return hashes
}

// cspWithScriptHashes builds the policy with the given inline-script hashes.
func cspWithScriptHashes(hashes []string) string {
	script := "script-src 'self'"
	for _, h := range hashes {
		script += " " + h
	}
	return "default-src 'self'; " + script + "; " + cspRest()
}

// cspRest is the part of the policy after script-src, shared so the base text
// cannot drift between the two builders.
func cspRest() string {
	return "style-src 'self' 'unsafe-inline'; " +
		"img-src 'self' data: blob: https:; " +
		"font-src 'self' data:; " +
		"connect-src 'self' ws: wss:; " +
		"object-src 'none'; " +
		"base-uri 'self'; " +
		"form-action 'self'; " +
		"frame-ancestors 'none'"
}

// dashboardInlineScripts are the hashes of the embedded dashboard document. The
// prefix rewrite does not touch script bodies, so one set covers every admin
// namespace.
var dashboardInlineScripts = func() []string {
	// Hash the document the static server actually sends. index.html is minified
	// at build time and served under the canonical URL, and a minifier is free to
	// reflow whitespace inside an inline script; hashing the source would then
	// authorize bytes the browser never sees, and the pre-paint blocks would be
	// refused.
	for _, name := range []string{"index.min.html", "index.html"} {
		doc, err := fs.ReadFile(webAssets.StaticFS, name)
		if err == nil {
			return inlineScriptHashes(doc)
		}
	}
	return nil
}()

var contentSecurityPolicy = cspWithScriptHashes(dashboardInlineScripts)

// loginContentSecurityPolicy computes the policy for one rewritten login
// document. Called once per admin path, when the document itself is cached.
func loginContentSecurityPolicy(doc []byte) string {
	return cspWithScriptHashes(inlineScriptHashes(doc))
}
