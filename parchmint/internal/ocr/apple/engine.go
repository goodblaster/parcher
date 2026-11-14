//go:build darwin && cgo

package apple

import "github.com/goodblaster/parcher/parchmint/internal/ocr"

type Engine struct{}

func (engine *Engine) Name() string { return Name }

func init() {
	// One image at a time: Vision already uses every core for one image.
	ocr.Register(Name, func() (ocr.Recognizer, error) { return ocr.PerImage(&Engine{}, 1), nil })
}
