package engines

import (
	"slices"
	"testing"

	"github.com/goodblaster/parcher/parchmint/internal/ocr"
	"github.com/goodblaster/parcher/parchmint/internal/ocr/googlevision"
	"github.com/goodblaster/parcher/parchmint/internal/ocr/tesseract"
	"github.com/goodblaster/parcher/parchmint/internal/ocr/textract"
)

func TestEnginesAreLinkedIn(t *testing.T) {
	names := ocr.Names()
	for _, want := range []string{tesseract.Name, textract.Name, googlevision.Name} {
		if !slices.Contains(names, want) {
			t.Errorf("%q is not registered (have %v)", want, names)
		}
	}
}

// The default must be an engine this build contains, and never one that
// sends images to a service.
func TestDefault(t *testing.T) {
	d := Default()
	if !ocr.Registered(d) {
		t.Fatalf("Default() = %q, which this build does not contain", d)
	}
	if d == textract.Name || d == googlevision.Name {
		t.Fatalf("Default() must never pick an engine that uploads images, got %q", d)
	}
}
