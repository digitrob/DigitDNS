package web

import (
	"bytes"
	"encoding/xml"
	"errors"
	"digitdns/internal/database"
	"image/png"
	"io"
	"strconv"
	"strings"
)

// The upload standard for a custom policy group's icon (v2.7):
//
//   - SVG: at most maxGroupSVGBytes after re-encoding, and the re-encode is the
//     safety mechanism, not a filter on top of the original. The upload is
//     decoded as XML and re-encoded from a strict allowlist of elements and
//     attributes — anything outside the allowlist (scripts, event handlers,
//     foreignObject, external references, xlink) simply does not exist in the
//     stored bytes, because the stored bytes are the re-encode.
//   - PNG: at most maxGroupIconUploadBytes, magic-byte verified, dimensions
//     capped (a 10000×10000 "icon" is a canvas-decompression lever).
//
// One icon per group, stored in the database (so backup/restore carries it) and
// served only to authenticated dashboard sessions.
const (
	maxGroupSVGBytes       = 16 << 10
	maxGroupIconUploadByte = 64 << 10
	maxGroupIconDimension  = 512
)

// ErrBadGroupIcon covers every rejection the validator makes; the HTTP layer
// turns it into one 400 with a specific message carried alongside.
var ErrBadGroupIcon = errors.New("icon rejected")

func badIcon(why string) error {
	return errors.New(ErrBadGroupIcon.Error() + ": " + why)
}

// groupIconElementAllowlist is the full set of elements a stored SVG may contain.
// Everything shape- and gradient-related is in; every scripting, structuring and
// referencing element (<script>, <foreignObject>, <use>, <image>, <animate*>)
// is out by absence.
var groupIconElementAllowlist = map[string]bool{
	"svg": true, "g": true, "defs": true, "title": true, "desc": true,
	"circle": true, "ellipse": true, "line": true, "path": true,
	"polygon": true, "polyline": true, "rect": true,
	"linearGradient": true, "radialGradient": true, "stop": true,
}

// groupIconAttrAllowlist is the full set of attributes a stored SVG may carry.
// on* handlers and href/xlink:href are excluded by construction; style is
// excluded because it can smuggle url() references past the attribute check.
var groupIconAttrAllowlist = map[string]bool{
	"viewBox": true, "width": true, "height": true, "preserveAspectRatio": true, "version": true,
	"fill": true, "fill-opacity": true, "fill-rule": true, "stroke": true,
	"stroke-width": true, "stroke-linecap": true, "stroke-linejoin": true,
	"stroke-dasharray": true, "stroke-dashoffset": true, "stroke-opacity": true,
	"opacity": true, "d": true, "cx": true, "cy": true, "r": true, "rx": true, "ry": true,
	"x": true, "y": true, "x1": true, "x2": true, "y1": true, "y2": true,
	"points": true, "transform": true, "gradientUnits": true, "gradientTransform": true,
	"offset": true, "stop-color": true, "stop-opacity": true,
	"id": true, "class": true, "xmlns": true, "aria-label": true, "role": true,
}

// cssURLRefs returns the argument of every url( … ) in a CSS value: the target
// with surrounding quotes and whitespace stripped. Matching is case-insensitive
// because CSS keyword and function names are. A quoted internal reference such
// as url('#grad') yields "#grad"; an external one yields "https://…" (which the
// caller rejects). Only url() is inspected — the other attribute values are all
// numeric path coordinates or colours, and a keyword like `fill="url"` alone is
// not a reference.
func cssURLRefs(v string) []string {
	lower := strings.ToLower(v)
	var out []string
	for i := 0; ; {
		j := strings.Index(lower[i:], "url(")
		if j < 0 {
			break
		}
		start := i + j + len("url(")
		rest := v[start:]
		// An unterminated reference is malformed CSS; hand it back raw so the
		// caller's external-check sees something rather than nothing.
		closer := strings.IndexByte(rest, ')')
		if closer < 0 {
			out = append(out, strings.Trim(strings.TrimSpace(rest), `"'`))
			break
		}
		target := strings.Trim(strings.TrimSpace(rest[:closer]), `"'`)
		out = append(out, target)
		i = start + closer + 1
	}
	return out
}

// sanitizeGroupSVG re-encodes an uploaded SVG through the allowlists. The
// output is guaranteed well-formed XML containing nothing but allowlisted
// elements and attributes; on any decode error the upload is rejected.
func sanitizeGroupSVG(raw []byte) ([]byte, error) {
	if len(raw) > maxGroupSVGBytes {
		return nil, badIcon("the SVG exceeds the 16 KiB limit")
	}
	dec := xml.NewDecoder(bytes.NewReader(raw))
	dec.Strict = true

	var out bytes.Buffer
	enc := xml.NewEncoder(&out)
	depth := 0
	rootSeen := false
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, badIcon("the file is not well-formed SVG XML")
		}
		switch t := tok.(type) {
		case xml.StartElement:
			name := t.Name.Local
			if !groupIconElementAllowlist[name] {
				return nil, badIcon("the SVG uses a disallowed <" + name + "> element")
			}
			if depth == 0 {
				if name != "svg" || rootSeen {
					return nil, badIcon("the document root must be a single <svg>")
				}
				rootSeen = true
			}
			clean := xml.StartElement{Name: xml.Name{Local: name}}
			for _, a := range t.Attr {
				an := a.Name.Local
				if !groupIconAttrAllowlist[an] {
					// Unknown namespaces and handlers die here silently-by-design:
					// the allowlist, not a blocklist of the week, decides.
					return nil, badIcon("the SVG carries a disallowed attribute " + an)
				}
				// Every url() in the value must point at an internal fragment.
				// The check is case-insensitive (CSS is), covers EVERY
				// occurrence, tolerates quotes/whitespace, and runs on the
				// post-decode value — so an entity encoding such as &#76;
				// cannot smuggle "URL(" past it. The previous guard was a
				// case-sensitive prefix test on the whole value, which is how
				// fill="URL(https://…)" and a second url() after an internal
				// one both slipped through (audit finding #5).
				for _, ref := range cssURLRefs(a.Value) {
					if !strings.HasPrefix(ref, "#") {
						return nil, badIcon("the SVG references an external resource via url()")
					}
				}
				if strings.ContainsAny(a.Value, "<>") {
					return nil, badIcon("the SVG attribute value contains markup characters")
				}
				clean.Attr = append(clean.Attr, xml.Attr{Name: xml.Name{Local: an}, Value: a.Value})
			}
			if err := enc.EncodeToken(clean); err != nil {
				return nil, badIcon("the SVG could not be re-encoded")
			}
			depth++
		case xml.EndElement:
			// Re-encoded Local-only, exactly like the StartElement above: the
			// decoder resolves element names into the xmlns default namespace,
			// and an end token carrying that space would not match the
			// namespace-less start we emitted.
			if err := enc.EncodeToken(xml.EndElement{Name: xml.Name{Local: t.Name.Local}}); err != nil {
				return nil, badIcon("the SVG could not be re-encoded")
			}
			depth--
			if depth < 0 {
				return nil, badIcon("the SVG is not well-formed")
			}
		case xml.CharData:
			// Text is only meaningful inside <title>/<desc>; the encoder escapes
			// whatever arrives, so it can never close a tag.
			if err := enc.EncodeToken(t); err != nil {
				return nil, badIcon("the SVG could not be re-encoded")
			}
		case xml.ProcInst, xml.Directive:
			// <?xml …?> declarations and directives are dropped: the stored icon
			// is embedded in a page the server already framed correctly.
		case xml.Comment:
			// Comments are dropped — pure bytes, no meaning an icon needs.
		}
	}
	if !rootSeen {
		return nil, badIcon("the document contains no <svg> root")
	}
	if err := enc.Flush(); err != nil {
		return nil, badIcon("the SVG could not be re-encoded")
	}
	if out.Len() > maxGroupSVGBytes {
		return nil, badIcon("the SVG exceeds the 16 KiB limit")
	}
	return out.Bytes(), nil
}

// validateGroupPNG checks the PNG magic, parses the header, and enforces the
// dimension ceiling. The bytes are stored and served exactly as uploaded.
func validateGroupPNG(raw []byte) error {
	if len(raw) > maxGroupIconUploadByte {
		return badIcon("the PNG exceeds the 64 KiB limit")
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return badIcon("the file is not a valid PNG")
	}
	if cfg.Width > maxGroupIconDimension || cfg.Height > maxGroupIconDimension {
		return badIcon("the PNG must be at most 512×512")
	}
	return nil
}

// detectGroupIcon sniffs the upload: PNG by magic bytes, SVG by a leading "<svg"
// or an XML prolog followed by one. Anything else is rejected.
func detectGroupIcon(raw []byte) (kind string, err error) {
	if len(raw) >= 8 && bytes.Equal(raw[:8], []byte("\x89PNG\r\n\x1a\n")) {
		return "png", validateGroupPNG(raw)
	}
	trimmed := bytes.TrimLeft(raw, " \t\r\n")
	if bytes.HasPrefix(trimmed, []byte("<?xml")) {
		if i := bytes.IndexByte(trimmed, '>'); i >= 0 {
			after := bytes.TrimLeft(trimmed[i+1:], " \t\r\n")
			if bytes.HasPrefix(after, []byte("<svg")) {
				return "svg", nil
			}
		}
		return "", badIcon("the upload must be an SVG or PNG image")
	}
	if bytes.HasPrefix(trimmed, []byte("<svg")) {
		return "svg", nil
	}
	return "", badIcon("the upload must be an SVG or PNG image")
}

// processGroupIconUpload validates a raw upload and returns the stored form:
// PNG bytes verbatim, SVG re-encoded through the sanitizer.
func processGroupIconUpload(raw []byte) (database.GroupIcon, error) {
	kind, err := detectGroupIcon(raw)
	if err != nil {
		return database.GroupIcon{}, err
	}
	if kind == "svg" {
		clean, err := sanitizeGroupSVG(raw)
		if err != nil {
			return database.GroupIcon{}, err
		}
		return database.GroupIcon{Kind: "svg", Data: clean}, nil
	}
	return database.GroupIcon{Kind: "png", Data: raw}, nil
}

// groupIconContentType maps the stored kind to its HTTP content type.
func groupIconContentType(kind string) string {
	if kind == "png" {
		return "image/png"
	}
	return "image/svg+xml"
}

// groupIconETag gives the browser a cheap revalidation key (size+kind is enough:
// an upload that replaces one of the same size is a corner the 60s cache covers).
func groupIconETag(icon database.GroupIcon) string {
	return `"` + icon.Kind + "-" + strconv.Itoa(len(icon.Data)) + `"`
}
