// Package windows is a PLACEHOLDER for OCR via the Windows.Media.Ocr
// API — the engine Windows ships with, so `parch index` there would need
// no install, exactly as Apple Vision needs none on macOS. It is not
// implemented; every call returns an error naming tesseract instead, and
// ocr.Default() still picks tesseract on Windows. Nothing selects this
// engine unless a user asks for it by name.
//
// # Why it is worth doing
//
// Windows.Media.Ocr is present on Windows 10/11 (and 8.1) with nothing to
// install, and it reports WORD-LEVEL boxes — OcrResult → OcrLine →
// OcrWord.BoundingRect — which is the geometry the text layer is built
// on. It also tends to read screen-rendered text (most of an archived
// page) better than tesseract, which is tuned for scanned documents.
//
// # What implementing it looks like
//
// Mirror internal/ocr/apple, which solves the same problem against a
// platform framework:
//
//   - engine.go with `//go:build windows && cgo` holding the real
//     implementation, and stub.go with `//go:build !windows || !cgo`
//     keeping other platforms compiling. Splitting this file that way is
//     the first step; today it is one file so the package is inert.
//   - Call OcrEngine.TryCreateFromUserProfileLanguages() (or
//     TryCreateFromLanguage for an explicit -lang), run RecognizeAsync
//     over a SoftwareBitmap decoded from the image bytes, then walk
//     Lines/Words.
//   - Convert boxes to the std contract: Top/Left/Width/Height as
//     FRACTIONS of image width and height (0..1, top-left origin), NOT
//     pixels. Every engine normalizes; capture/index.go multiplies back
//     up by the image's recorded box.
//   - Confidence: Windows.Media.Ocr does not expose a per-line score.
//     Leave Line.Confidence zero rather than inventing one — the field
//     is written to the text layer and read back by `parch find`.
//
// # Decisions to make when picking this up
//
//   - Interop route. cgo + C++/WinRT mirrors the Apple engine most
//     closely but makes a full-featured Windows build need cgo and MSVC;
//     today GOOS=windows cross-compiles from anywhere with CGO_ENABLED=0
//     and would keep doing so (falling back to the stub). Alternatives
//     are WinRT activation through go-ole/syscall (no MSVC, fiddly COM
//     marshalling) or shelling out to PowerShell (no build complexity,
//     but a process per image).
//   - UNVERIFIED: whether TryCreateFromUserProfileLanguages returns a
//     usable engine on a stock install with no extra language packs.
//     That answer decides whether this is genuinely zero-install or just
//     a different install. Check it before committing to the work.
//   - Flip ocr.Default() to return WindowsOCR on windows only once the
//     engine actually recognizes text, and say so in the README's
//     requirements table.
package windows

import (
	"io"

	"github.com/goodblaster/errors"
	"github.com/goodblaster/parchmint/internal/ocr/std"
)

// Engine is the not-yet-implemented Windows.Media.Ocr engine. It satisfies
// ocr.Engine so the plumbing (engine selection, error reporting) is real
// and exercised; only recognition is missing.
type Engine struct{}

func (engine *Engine) String() string { return "windows" }

// errNotImplemented names the working alternative, because a user only
// ever sees this by explicitly asking for `-engine windows`.
func errNotImplemented() error {
	return errors.New("the windows OCR engine (Windows.Media.Ocr) is not implemented yet; use -engine tesseract")
}

func (engine *Engine) ParseBytes(b []byte, langs []string) ([]std.Line, error) {
	return nil, errNotImplemented()
}

func (engine *Engine) ParseReader(r io.Reader, langs []string) ([]std.Line, error) {
	return nil, errNotImplemented()
}

func (engine *Engine) ParseFile(filename string, langs []string) ([]std.Line, error) {
	return nil, errNotImplemented()
}
