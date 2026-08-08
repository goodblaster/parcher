package capture

import (
	"slices"
	"testing"
)

// "html" must resolve to the native backend. This is the repository's only
// self-contained-HTML serializer now that the vendored third-party one is
// gone, so nothing else can quietly take the default format's place.
func TestHTMLBackendIsRegistered(t *testing.T) {
	backend := Lookup("html")
	if backend == nil {
		t.Fatal("no backend registered for \"html\"")
	}
	if _, ok := backend.(HTML); !ok {
		t.Errorf("-f html resolved to %T, want capture.HTML", backend)
	}
	if got := backend.Ext(); got != ".html" {
		t.Errorf("Ext() = %q, want .html", got)
	}
}

func TestLookupUnknownFormat(t *testing.T) {
	if backend := Lookup("nonesuch"); backend != nil {
		t.Errorf("Lookup(\"nonesuch\") = %T, want nil", backend)
	}
}

func TestJPGIsAnAliasNotAFormat(t *testing.T) {
	if _, ok := Lookup("jpg").(JPEG); !ok {
		t.Error("-f jpg did not resolve to the JPEG backend")
	}
	if slices.Contains(Formats(), "jpg") {
		t.Error("Formats() lists the jpg alias; it is a spelling, not a choice")
	}
}

// Only document containers can carry the text layer and inlined favicons.
// The CLI asks the backend rather than matching on format names, so this is
// what keeps `-f png --text` from silently promising a layer.
func TestCarriesLayer(t *testing.T) {
	for _, format := range []string{"html", "mht"} {
		if !CarriesLayer(Lookup(format)) {
			t.Errorf("%s should carry a text layer", format)
		}
	}
	for _, format := range []string{"pdf", "png", "jpeg", "webp"} {
		if CarriesLayer(Lookup(format)) {
			t.Errorf("%s should not claim to carry a text layer", format)
		}
	}
}

func TestFormatsAreSortedAndComplete(t *testing.T) {
	formats := Formats()
	if !slices.IsSorted(formats) {
		t.Errorf("Formats() is not sorted: %v", formats)
	}
	for _, want := range []string{"html", "jpeg", "mht", "pdf", "png", "webp"} {
		if !slices.Contains(formats, want) {
			t.Errorf("Formats() is missing %q: %v", want, formats)
		}
	}
}
