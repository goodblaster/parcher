//go:build !darwin || !cgo

package apple

import (
	"io"

	"github.com/goodblaster/errors"
	"github.com/goodblaster/parchmint/internal/ocr/std"
)

// Engine is the Apple Vision OCR engine; it requires macOS (the Vision
// framework via cgo). This stub keeps other platforms compiling.
type Engine struct{}

func (engine *Engine) String() string { return "apple" }

func (engine *Engine) ParseBytes(b []byte, langs []string) ([]std.Line, error) {
	return nil, errors.New("the apple OCR engine requires macOS; use -engine tesseract")
}

func (engine *Engine) ParseReader(r io.Reader, langs []string) ([]std.Line, error) {
	return nil, errors.New("the apple OCR engine requires macOS; use -engine tesseract")
}

func (engine *Engine) ParseFile(filename string, langs []string) ([]std.Line, error) {
	return nil, errors.New("the apple OCR engine requires macOS; use -engine tesseract")
}
