// Package engines links every OCR engine into the binary and picks the
// default one. Importing it is what makes the engines selectable: each
// registers itself with internal/ocr from its own package's init.
package engines

import (
	"github.com/goodblaster/parcher/parchmint/internal/ocr"
	"github.com/goodblaster/parcher/parchmint/internal/ocr/apple"
	_ "github.com/goodblaster/parcher/parchmint/internal/ocr/googlevision"
	"github.com/goodblaster/parcher/parchmint/internal/ocr/tesseract"
	_ "github.com/goodblaster/parcher/parchmint/internal/ocr/textract"
	"github.com/goodblaster/parcher/parchmint/internal/ocr/windows"
)

// Default picks the best engine THIS BUILD contains that needs nothing
// installed: Apple Vision on macOS, Windows.Media.Ocr on Windows, and
// tesseract everywhere else.
//
// "Contains" is the registry's answer, not the operating system's: a macOS
// binary cross-compiled without cgo has no Vision in it, and defaulting to
// an engine that is not there would make `parch ocr` useless.
//
// It never returns an engine that sends images to a service; those run
// only when named.
func Default() string {
	for _, name := range []string{apple.Name, windows.Name} {
		if ocr.Registered(name) {
			return name
		}
	}
	return tesseract.Name
}
