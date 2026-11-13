package ocr

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/goodblaster/parcher/parchmint/internal/ocr/std"
)

// Image is one image handed to a Recognizer.
type Image struct {
	// ID identifies the image within one Recognize call; the matching
	// Result carries the same ID. `parch ocr` uses the text layer's image
	// hash, so each distinct image is sent once however often it appears.
	ID string
	// MIME is the image's media type ("image/png", "image/jpeg", …).
	MIME string
	// Data is the encoded image file, exactly as embedded in the archive.
	Data []byte
}

// Result is what a Recognizer read in one Image.
type Result struct {
	ID string
	// Lines follow the std contract: word boxes are FRACTIONS of the image's
	// width and height (0..1, top-left origin), never pixels. A service that
	// reports pixels or polygons converts before returning.
	Lines []std.Line
	// Err is this image's failure. One bad image does not fail the batch.
	Err error
}

// Recognizer is the seam `parch ocr` works through: it hands over EVERY
// image of an archive in one call and waits for all of the results.
//
// It exists so OCR can be done somewhere else. The local engines (Apple
// Vision, tesseract) read one image at a time and are wrapped by PerImage;
// a remote service — AWS Textract, Google Vision, a queue with workers
// behind it — implements Recognize directly, and is free to upload the
// batch, fan out, poll, and collect however suits it. The call is
// synchronous from the caller's side: it returns when every image has a
// Result or ctx is done.
//
// Contract:
//   - One Result per Image, matched by ID; order does not matter. An image
//     with no Result is reported as failed.
//   - A per-image problem goes in Result.Err. The returned error is for the
//     whole batch (no credentials, service unreachable) and discards it.
//   - langs are BCP 47 hints from the page (`<html lang>`) or --lang. An
//     engine that detects language itself may ignore them.
//   - Honor ctx: `parch ocr --timeout` cancels it.
//
// Adding one is a single file: implement Recognizer, and Register it from
// that file's init. It is then selectable as `parch ocr --engine <name>`
// with no other edit. Configuration (credentials, region, endpoint) is the
// engine's own business — read it from the environment in the factory.
type Recognizer interface {
	Name() string
	Recognize(ctx context.Context, images []Image, langs []string) ([]Result, error)
}

var (
	registryMu sync.RWMutex
	registry   = map[string]func() (Recognizer, error){}
)

// Register makes a Recognizer selectable by name. The factory runs when the
// engine is chosen, not at startup, so it may fail on missing configuration
// with an error that says what to set.
func Register(name string, factory func() (Recognizer, error)) {
	registryMu.Lock()
	defer registryMu.Unlock()
	if _, dup := registry[name]; dup {
		panic("ocr: engine registered twice: " + name)
	}
	registry[name] = factory
}

// Names lists the registered engines, sorted.
func Names() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	names := make([]string, 0, len(registry))
	for n := range registry {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// NewRecognizer builds the named engine.
func NewRecognizer(name string) (Recognizer, error) {
	registryMu.RLock()
	factory, ok := registry[name]
	registryMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown OCR engine %q (have: %v)", name, Names())
	}
	return factory()
}

// PerImage adapts a one-image-at-a-time Engine to Recognizer. Images are
// read in order, one after another: the local engines are CPU-bound and
// Apple Vision already uses every core for a single image.
func PerImage(e Engine) Recognizer { return perImage{e} }

type perImage struct{ e Engine }

func (p perImage) Name() string { return p.e.String() }

func (p perImage) Recognize(ctx context.Context, images []Image, langs []string) ([]Result, error) {
	results := make([]Result, 0, len(images))
	for _, img := range images {
		if err := ctx.Err(); err != nil {
			results = append(results, Result{ID: img.ID, Err: err})
			continue
		}
		lines, err := p.e.ParseBytes(img.Data, langs)
		results = append(results, Result{ID: img.ID, Lines: lines, Err: err})
	}
	return results, nil
}

func init() {
	for _, name := range []string{AppleVision, Tesseract, WindowsOCR} {
		Register(name, func() (Recognizer, error) { return PerImage(New(name)), nil })
	}
}
