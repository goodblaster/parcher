package ocr

import (
	"io"
	"runtime"

	"github.com/goodblaster/parchmint/internal/ocr/apple"
	"github.com/goodblaster/parchmint/internal/ocr/std"
	"github.com/goodblaster/parchmint/internal/ocr/tesseract"
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
)

// Default picks the platform's best engine: Apple Vision on macOS (fast,
// accurate, no install), tesseract elsewhere (or when chosen explicitly).
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
	default:
		return nil
	}
}
