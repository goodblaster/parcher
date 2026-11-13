// Package render turns an MHT document into a single self-contained HTML
// file. Every subresource the document carries — images, stylesheets,
// scripts, fonts, nested frames — is inlined, so the result renders with no
// network access and no sidecar files.
//
// Stylesheets are inlined as <style> elements so that the url() references
// inside them can be rewritten in turn; everything else becomes a data: URI.
// References the document does not carry are left untouched, which keeps a
// partial snapshot usable rather than broken.
package render

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/goodblaster/errors"
	"github.com/goodblaster/go-mht/pkg/documents"
	"github.com/goodblaster/go-mht/pkg/sections"
	"github.com/goodblaster/parcher/parchmint/internal/convert/css"
	"github.com/goodblaster/parcher/parchmint/internal/convert/refs"
	"golang.org/x/net/html"
)

// Options controls how a document is rendered. The zero value is the normal
// choice: inline everything, warn about nothing.
type Options struct {
	// Root selects the section to render by Content-Location. When empty,
	// the document's own root section is used.
	Root string

	// OmitScripts drops <script src> references instead of inlining them,
	// for when a snapshot is wanted as a static artifact.
	OmitScripts bool

	// OnWarn, when set, is called for each reference that could not be
	// inlined, with a short explanation.
	OnWarn func(message string)
}

// Document renders doc as a self-contained HTML file.
func Document(doc *documents.Document, opts Options) ([]byte, error) {
	renderer := &renderer{
		opts:    opts,
		byLoc:   make(map[string]*sections.Section, len(doc.Sections)),
		byCID:   make(map[string]*sections.Section, len(doc.Sections)),
		inlined: make(map[*sections.Section]string),
		active:  make(map[*sections.Section]bool),
	}

	for i := range doc.Sections {
		section := &doc.Sections[i]
		if location := section.Headers.Get("Content-Location"); location != "" {
			// The first section wins: Blink can repeat a location, and the
			// earlier part is the one the page actually loaded.
			if _, seen := renderer.byLoc[location]; !seen {
				renderer.byLoc[location] = section
			}
		}
		if id := section.Headers.Get("Content-ID"); id != "" {
			renderer.byCID[trimCID(id)] = section
		}
	}

	root, err := rootSection(doc, opts.Root)
	if err != nil {
		return nil, err
	}

	return renderer.html(root)
}

// File renders the named MHT file as a self-contained HTML file.
func File(name string, opts Options) ([]byte, error) {
	doc, err := documents.ParseFile(name)
	if err != nil {
		return nil, err
	}
	return Document(doc, opts)
}

// rootSection picks the section that holds the top-level page: the one named
// by location if given, otherwise the one matching the document's
// Snapshot-Content-Location, otherwise the first HTML section.
func rootSection(doc *documents.Document, location string) (*sections.Section, error) {
	if location != "" {
		section := doc.LocIndex[location]
		if section == nil {
			return nil, errors.Newf("no section at %q", location)
		}
		return section, nil
	}

	if snapshot := doc.Headers.Get("Snapshot-Content-Location"); snapshot != "" {
		if section := doc.LocIndex[snapshot]; section != nil && isHTML(section) {
			return section, nil
		}
	}

	for i := range doc.Sections {
		if isHTML(&doc.Sections[i]) {
			return &doc.Sections[i], nil
		}
	}

	return nil, errors.New("document contains no HTML section")
}

type renderer struct {
	opts  Options
	byLoc map[string]*sections.Section
	byCID map[string]*sections.Section

	// inlined caches the data: URI produced for a section, so a resource
	// referenced many times is encoded once.
	inlined map[*sections.Section]string

	// active holds the sections currently being rendered, so that a frame
	// or stylesheet that refers back to its own ancestor stops instead of
	// recursing forever.
	active map[*sections.Section]bool
}

// html rewrites one HTML section into self-contained markup.
func (r *renderer) html(section *sections.Section) ([]byte, error) {
	if r.active[section] {
		return nil, errors.New("reference cycle")
	}
	r.active[section] = true
	defer delete(r.active, section)

	base := parseURL(section.Headers.Get("Content-Location"))

	var out bytes.Buffer
	tokenizer := html.NewTokenizer(strings.NewReader(section.Body))
	inStyle := false

	for {
		switch tokenizer.Next() {
		case html.ErrorToken:
			if err := tokenizer.Err(); !errors.Is(err, io.EOF) {
				return nil, errors.Wrap(err, "failed to tokenize HTML")
			}
			return out.Bytes(), nil

		case html.TextToken:
			raw := tokenizer.Raw()
			if !inStyle {
				out.Write(raw)
				continue
			}
			out.WriteString(css.Rewrite(string(raw), func(ref string) string {
				return r.inlineRef(ref, base)
			}))

		case html.StartTagToken, html.SelfClosingTagToken:
			raw := tokenizer.Raw()
			token := tokenizer.Token()

			if token.Data == "style" {
				inStyle = true
			}
			if token.Data == "base" {
				if href, ok := attr(token, "href"); ok {
					if resolved := resolveRef(base, href); resolved != nil {
						base = resolved
					}
				}
				out.Write(raw)
				continue
			}
			if replacement, ok := r.stylesheetLink(token, base); ok {
				out.WriteString(replacement)
				continue
			}
			// Every rewrite must run before the token is emitted, so none
			// short-circuits another. restoreHidden goes first: it puts the
			// element's original style attribute back, which rewriteAttrs
			// then inlines like any other.
			restored := restoreHidden(&token)
			rewritten := r.rewriteAttrs(&token, base)
			if reviveShadowRoot(&token) || rewritten || restored {
				out.WriteString(token.String())
				continue
			}
			out.Write(raw)

		case html.EndTagToken:
			if name, _ := tokenizer.TagName(); string(name) == "style" {
				inStyle = false
			}
			out.Write(tokenizer.Raw())

		default:
			out.Write(tokenizer.Raw())
		}
	}
}

// restoreHidden undoes pscription's preserve-hidden-attr step, reporting
// whether it changed anything.
//
// Chrome's MHTML writer drops any element with a `hidden` attribute, so the
// capture trades the attribute for data-parchmint-hidden (holding the
// original value), freezes the computed display inline, and stashes the
// element's own style attribute in data-parchmint-style. Here the element
// gets all three back exactly as the page had them: `hidden` with its value,
// the original style (or none), and no trace of the detour.
func restoreHidden(token *html.Token) bool {
	value, ok := attr(*token, "data-parchmint-hidden")
	if !ok {
		return false
	}
	style, hadStyle := attr(*token, "data-parchmint-style")

	kept := token.Attr[:0]
	for _, a := range token.Attr {
		switch strings.ToLower(a.Key) {
		case "data-parchmint-hidden", "data-parchmint-style", "style", "hidden":
		default:
			kept = append(kept, a)
		}
	}
	kept = append(kept, html.Attribute{Key: "hidden", Val: value})
	if hadStyle {
		kept = append(kept, html.Attribute{Key: "style", Val: style})
	}
	token.Attr = kept
	return true
}

// reviveShadowRoot renames Blink's `shadowmode` attribute to the standard
// `shadowrootmode`, reporting whether it changed anything.
//
// Chrome's MHTML writer serializes a shadow root — open AND closed, which no
// page script can do — as `<template shadowmode="open">`, but that spelling is
// honored only by Blink's own MHTML loader. In an ordinary HTML file the
// template is inert: the content is present in the markup and renders as
// nothing at all. The standard declarative-shadow-DOM attribute is what makes
// the parser attach it, and the two differ by nothing but the name.
func reviveShadowRoot(token *html.Token) bool {
	if token.Data != "template" {
		return false
	}
	for i := range token.Attr {
		if strings.ToLower(token.Attr[i].Key) != "shadowmode" {
			continue
		}
		token.Attr[i].Key = "shadowrootmode"
		return true
	}
	return false
}

// stylesheetLink replaces a <link rel=stylesheet> with a <style> element
// holding the stylesheet's rewritten text. It reports false when the element
// is not a stylesheet link, or when the CSS cannot be inlined safely — in
// which case the ordinary attribute rewrite applies instead.
func (r *renderer) stylesheetLink(token html.Token, base *url.URL) (string, bool) {
	if token.Data != "link" {
		return "", false
	}
	rel, _ := attr(token, "rel")
	if !refs.IsStylesheet(rel) {
		return "", false
	}
	href, ok := attr(token, "href")
	if !ok || !refs.Fetchable(href) {
		return "", false
	}
	section := r.lookup(href, base)
	if section == nil {
		return "", false
	}

	text, err := r.stylesheet(section, base)
	if err != nil {
		r.warn("stylesheet %s: %v", href, err)
		return "", false
	}

	// A stylesheet containing "</style" would end the element early. Fall
	// back to the data: URI form, which has no such hazard.
	if strings.Contains(strings.ToLower(text), "</style") {
		return "", false
	}

	style := html.Token{Type: html.StartTagToken, Data: "style"}
	for _, a := range token.Attr {
		// rel and href describe the link, not the stylesheet; media, type,
		// and title carry over unchanged.
		switch strings.ToLower(a.Key) {
		case "rel", "href", "as", "crossorigin", "integrity", "referrerpolicy":
		default:
			style.Attr = append(style.Attr, a)
		}
	}

	return style.String() + text + "</style>", true
}

// rewriteAttrs inlines every subresource attribute on token, reporting
// whether any of them changed.
func (r *renderer) rewriteAttrs(token *html.Token, base *url.URL) bool {
	changed := false

	for i := range token.Attr {
		attribute := &token.Attr[i]
		key := strings.ToLower(attribute.Key)

		// A style attribute is a CSS declaration block, not a URL.
		if key == "style" {
			rewritten := css.Rewrite(attribute.Val, func(ref string) string {
				return r.inlineRef(ref, base)
			})
			if rewritten != attribute.Val {
				attribute.Val = rewritten
				changed = true
			}
			continue
		}

		if !r.isResourceAttr(*token, key) {
			continue
		}

		if refs.IsSrcset(key) {
			candidates := refs.ParseSrcset(attribute.Val)
			for j := range candidates {
				candidates[j].URL = r.inlineRef(candidates[j].URL, base)
			}
			if rewritten := refs.FormatSrcset(candidates); rewritten != attribute.Val {
				attribute.Val = rewritten
				changed = true
			}
			continue
		}

		if token.Data == "script" && r.opts.OmitScripts {
			attribute.Val = "data:text/javascript;base64,"
			changed = true
			continue
		}

		if rewritten := r.inlineRef(attribute.Val, base); rewritten != attribute.Val {
			attribute.Val = rewritten
			changed = true
		}
	}

	return changed
}

// isResourceAttr reports whether key on token names a subresource. The href
// of a <link> counts only for the relations that actually load content.
func (r *renderer) isResourceAttr(token html.Token, key string) bool {
	for _, candidate := range refs.Attrs(token.Data) {
		if candidate != key {
			continue
		}
		if token.Data == "link" && key == "href" {
			rel, _ := attr(token, "rel")
			return refs.IsResourceLink(rel)
		}
		return true
	}
	return false
}

// inlineRef converts one reference into a data: URI, returning it unchanged
// when the document carries no matching section.
func (r *renderer) inlineRef(ref string, base *url.URL) string {
	if !refs.Fetchable(ref) || strings.HasPrefix(ref, "data:") {
		return ref
	}

	section := r.lookup(ref, base)
	if section == nil {
		r.warn("no section for %s", ref)
		return ref
	}

	uri, err := r.dataURI(section, base)
	if err != nil {
		r.warn("inline %s: %v", ref, err)
		return ref
	}

	return uri
}

// dataURI renders section as a data: URI, recursing into stylesheets and
// frames so their own references are inlined too.
func (r *renderer) dataURI(section *sections.Section, parent *url.URL) (string, error) {
	if cached, ok := r.inlined[section]; ok {
		return cached, nil
	}

	// A media type may carry parameters ("text/javascript; charset=utf-8"),
	// but a URL may not contain spaces, so the whitespace has to go.
	contentType := strings.Join(strings.Fields(section.Headers.Get("Content-Type")), "")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	var (
		data []byte
		err  error
	)

	switch {
	case isCSS(section):
		var text string
		text, err = r.stylesheet(section, parent)
		data = []byte(text)
	case isHTML(section):
		data, err = r.html(section)
	default:
		data, err = content(section)
	}
	if err != nil {
		return "", err
	}

	uri := "data:" + contentType + ";base64," + base64.StdEncoding.EncodeToString(data)
	r.inlined[section] = uri

	return uri, nil
}

// stylesheet returns the section's CSS with its own references inlined.
//
// parent is the base of whatever referred to this sheet. It matters for one
// kind of part: Blink stores an inline <style> as a part whose
// Content-Location is a cid: URI, and a relative url() cannot resolve against
// that — the sheet lived IN the document, so the document's base is its base.
// Resolving against the cid: left every relative url() in an inline <style>
// dangling in the HTML output while the .mht of the same page carried the
// resource (measured: a background-image the part list held, pointing at
// nothing).
func (r *renderer) stylesheet(section *sections.Section, parent *url.URL) (string, error) {
	if r.active[section] {
		return "", errors.New("reference cycle")
	}
	r.active[section] = true
	defer delete(r.active, section)

	data, err := content(section)
	if err != nil {
		return "", err
	}

	base := parseURL(section.Headers.Get("Content-Location"))
	if base == nil || base.Scheme == "cid" {
		base = parent
	}

	return css.Rewrite(string(data), func(ref string) string {
		return r.inlineRef(ref, base)
	}), nil
}

// lookup finds the section a reference points at, by Content-ID for cid:
// references and by absolute Content-Location for everything else.
func (r *renderer) lookup(ref string, base *url.URL) *sections.Section {
	ref = strings.TrimSpace(ref)

	// Blink writes stylesheet parts with the cid: URI in Content-Location
	// rather than Content-ID, so a cid: reference has to try both.
	if rest, found := strings.CutPrefix(ref, "cid:"); found {
		if section := r.byCID[trimCID(rest)]; section != nil {
			return section
		}
		return r.byLoc[ref]
	}
	if section := r.byLoc[ref]; section != nil {
		return section
	}
	if resolved := resolveRef(base, ref); resolved != nil {
		return r.byLoc[resolved.String()]
	}

	return nil
}

func (r *renderer) warn(format string, args ...any) {
	if r.opts.OnWarn != nil {
		r.opts.OnWarn(fmt.Sprintf(format, args...))
	}
}

// content returns a section's body as raw bytes, undoing base64 where the
// parser left the body encoded.
func content(section *sections.Section) ([]byte, error) {
	if section.IsBase64() {
		data, err := base64.StdEncoding.DecodeString(section.Body)
		if err != nil {
			return nil, errors.Wrap(err, "failed to decode base64 body")
		}
		return data, nil
	}
	return []byte(section.Body), nil
}

func isHTML(section *sections.Section) bool {
	return strings.HasPrefix(section.Headers.Get("Content-Type"), "text/html")
}

func isCSS(section *sections.Section) bool {
	return strings.HasPrefix(section.Headers.Get("Content-Type"), "text/css")
}

// trimCID strips the angle brackets around a Content-ID value.
func trimCID(id string) string {
	return strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(id), "<"), ">")
}

func parseURL(raw string) *url.URL {
	parsed, err := url.Parse(raw)
	if err != nil {
		return nil
	}
	return parsed
}

// resolveRef resolves ref against base, returning nil when the result is not
// a usable absolute reference.
func resolveRef(base *url.URL, ref string) *url.URL {
	parsed, err := url.Parse(strings.TrimSpace(ref))
	if err != nil {
		return nil
	}
	if base == nil {
		if parsed.IsAbs() {
			return parsed
		}
		return nil
	}
	return base.ResolveReference(parsed)
}

// attr returns the value of the named attribute on token.
func attr(token html.Token, name string) (string, bool) {
	for _, a := range token.Attr {
		if strings.EqualFold(a.Key, name) {
			return a.Val, true
		}
	}
	return "", false
}
