package render

import (
	"strings"
	"testing"
)

// Chrome writes a serialized shadow root as <template shadowmode="...">, a
// spelling only its own MHTML loader honors. Converted to an HTML file it
// must come out as the standard declarative-shadow-DOM attribute, or the
// content is present in the markup and renders as nothing.
func TestDocumentRevivesShadowRoots(t *testing.T) {
	doc := mht(t, htmlPart(
		`<div id="open"><template shadowmode="open"><b>OPEN</b></template></div>`+
			`<div id="closed"><template shadowmode="closed"><b>CLOSED</b></template></div>`))

	out, err := Document(doc, Options{})
	if err != nil {
		t.Fatalf("Document: %v", err)
	}
	got := string(out)

	if strings.Contains(got, "shadowmode=") {
		t.Errorf("legacy shadowmode attribute survived:\n%s", got)
	}
	for _, want := range []string{`shadowrootmode="open"`, `shadowrootmode="closed"`} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %s in:\n%s", want, got)
		}
	}
	// The content itself must be untouched — closed roots included, which is
	// the case no page-script serializer can reach at all.
	for _, want := range []string{"<b>OPEN</b>", "<b>CLOSED</b>"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %s in:\n%s", want, got)
		}
	}
}

// An ordinary <template> is not a shadow root and must pass through as-is,
// as must a template that already uses the standard spelling.
func TestDocumentLeavesOtherTemplatesAlone(t *testing.T) {
	doc := mht(t, htmlPart(
		`<template id="row"><td>cell</td></template>`+
			`<div><template shadowrootmode="open"><b>ALREADY</b></template></div>`))

	out, err := Document(doc, Options{})
	if err != nil {
		t.Fatalf("Document: %v", err)
	}
	got := string(out)

	if !strings.Contains(got, `<template id="row">`) {
		t.Errorf("plain template was rewritten:\n%s", got)
	}
	if strings.Count(got, "shadowrootmode=") != 1 {
		t.Errorf("expected exactly one shadowrootmode attribute:\n%s", got)
	}
}
