package ocr

import (
	"io"
	"runtime"

	"github.com/goodblaster/parchmint/internal/ocr/apple"
	"github.com/goodblaster/parchmint/internal/ocr/std"
	"github.com/goodblaster/parchmint/internal/ocr/tesseract"
	windowsocr "github.com/goodblaster/parchmint/internal/ocr/windows"
)

type Engine interface {
	ParseFile(filename string, langs []string) ([]std.Line, error)
	ParseBytes(b []byte, langs []string) ([]std.Line, error)
	ParseReader(r io.Reader, langs []string) ([]std.Line, error)
	String() string // engine name
}

const (
	AppleVision = "apple"
	Tesseract   = "tesseract"
	// WindowsOCR is scaffolding only — see internal/ocr/windows. It is
	// selectable by name so the wiring is real, but Default never picks
	// it and it recognizes nothing yet.
	WindowsOCR = "windows"
)

// Default picks the platform's best WORKING engine: Apple Vision on
// macOS (fast, accurate, no install), tesseract elsewhere (or when
// chosen explicitly).
//
// Windows deliberately still gets tesseract. Windows.Media.Ocr would
// need no install there, but internal/ocr/windows is a stub that
// recognizes nothing — switch this once it does, not before.
func Default() string {
	if runtime.GOOS == "darwin" {
		return AppleVision
	}
	return Tesseract
}

func New(engine string) Engine {
	switch engine {
	case AppleVision:
		return &apple.Engine{}
	case Tesseract:
		return &tesseract.Engine{}
	case WindowsOCR:
		return &windowsocr.Engine{}
	default:
		return nil
	}
}
