// Package refs identifies the subresource references in an HTML document:
// which element attributes point at fetchable content, how to read and write
// the composite srcset values that hold several at once, and which URL
// schemes are worth resolving at all.
//
// Hyperlink attributes (a href, form action) are deliberately absent: they
// point at other documents, not at content that belongs inside a snapshot.
package refs

import (
	"strings"
)

// resourceAttrs maps an element name to the attributes on it that reference
// a subresource.
var resourceAttrs = map[string][]string{
	"audio":  {"src"},
	"body":   {"background"},
	"embed":  {"src"},
	"frame":  {"src"},
	"iframe": {"src"},
	"image":  {"href", "xlink:href"}, // SVG
	"img":    {"src", "srcset"},
	"input":  {"src"},
	"link":   {"href", "imagesrcset"},
	"object": {"data"},
	"script": {"src"},
	"source": {"src", "srcset"},
	"table":  {"background"},
	"td":     {"background"},
	"th":     {"background"},
	"track":  {"src"},
	"use":    {"href", "xlink:href"}, // SVG
	"video":  {"poster", "src"},
}

// srcsetAttrs are the attributes whose value is a comma-separated candidate
// list rather than a single URL.
var srcsetAttrs = map[string]bool{
	"srcset":      true,
	"imagesrcset": true,
}

// Attrs returns the attributes of tag that reference a subresource. The
// result is nil for elements that reference none.
func Attrs(tag string) []string {
	return resourceAttrs[strings.ToLower(tag)]
}

// IsSrcset reports whether attr holds a srcset-style candidate list.
func IsSrcset(attr string) bool {
	return srcsetAttrs[strings.ToLower(attr)]
}

// linkRels are the <link> relations that pull in a subresource. A link with
// any other relation (canonical, alternate, dns-prefetch) is a pointer to
// another document and is left alone.
var linkRels = map[string]bool{
	"apple-touch-icon":             true,
	"apple-touch-icon-precomposed": true,
	"apple-touch-startup-image":    true,
	"icon":                         true,
	"mask-icon":                    true,
	"modulepreload":                true,
	"prefetch":                     true,
	"preload":                      true,
	"shortcut":                     true,
	"stylesheet":                   true,
}

// IsResourceLink reports whether a <link> element with the given rel
// attribute references a subresource.
func IsResourceLink(rel string) bool {
	for _, value := range strings.Fields(strings.ToLower(rel)) {
		if linkRels[value] {
			return true
		}
	}
	// Chrome writes bare <link href> for stylesheets in some snapshots.
	return strings.TrimSpace(rel) == ""
}

// IsStylesheet reports whether a <link> element with the given rel attribute
// is a stylesheet.
func IsStylesheet(rel string) bool {
	for _, value := range strings.Fields(strings.ToLower(rel)) {
		if value == "stylesheet" {
			return true
		}
	}
	return false
}

// Fetchable reports whether ref names content worth resolving. Fragment-only
// references, empty values, and non-retrievable schemes (javascript:, about:,
// mailto:, blob:) are not.
func Fetchable(ref string) bool {
	ref = strings.TrimSpace(ref)
	if ref == "" || strings.HasPrefix(ref, "#") {
		return false
	}

	scheme, _, found := strings.Cut(ref, ":")
	if !found || strings.ContainsAny(scheme, "/?#") {
		// No scheme at all: a relative reference, which is fetchable.
		return true
	}

	switch strings.ToLower(scheme) {
	case "javascript", "about", "mailto", "tel", "blob", "chrome", "chrome-extension":
		return false
	}
	return true
}

// Candidate is one entry in a srcset attribute: a URL plus its optional
// descriptor ("2x", "640w").
type Candidate struct {
	URL        string
	Descriptor string
}

// ParseSrcset splits a srcset attribute into its candidates. It follows the
// HTML rule that a candidate URL ends at the first whitespace, so URLs
// containing commas — data: URIs above all — survive intact.
func ParseSrcset(value string) []Candidate {
	var candidates []Candidate

	for i := 0; i < len(value); {
		// Skip leading whitespace and the commas separating candidates.
		for i < len(value) && (isSpace(value[i]) || value[i] == ',') {
			i++
		}
		if i >= len(value) {
			break
		}

		start := i
		for i < len(value) && !isSpace(value[i]) {
			i++
		}
		url := value[start:i]

		// A URL ending in commas takes no descriptor; the commas are
		// separators, not part of the URL.
		if trimmed := strings.TrimRight(url, ","); trimmed != url {
			candidates = append(candidates, Candidate{URL: trimmed})
			continue
		}

		// Everything up to the next comma is this candidate's descriptor.
		for i < len(value) && isSpace(value[i]) {
			i++
		}
		start = i
		for i < len(value) && value[i] != ',' {
			i++
		}
		candidates = append(candidates, Candidate{
			URL:        url,
			Descriptor: strings.TrimSpace(value[start:i]),
		})
	}

	return candidates
}

// FormatSrcset renders candidates back into a srcset attribute value.
func FormatSrcset(candidates []Candidate) string {
	parts := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.Descriptor == "" {
			parts = append(parts, candidate.URL)
			continue
		}
		parts = append(parts, candidate.URL+" "+candidate.Descriptor)
	}
	return strings.Join(parts, ", ")
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\r' || c == '\n' || c == '\f'
}
