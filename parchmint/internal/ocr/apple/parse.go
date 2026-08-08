//go:build darwin && cgo

package apple

/*
#cgo CFLAGS: -x objective-c -fmodules
#cgo LDFLAGS: -framework Foundation -framework Vision
#include <stdlib.h>
const char *performAppleVisionOCR(const void *imageBytes, size_t length, const char **langs, size_t langsCount);
*/
import "C"

import (
	"encoding/json"
	"io"
	"os"
	"unsafe"

	"github.com/goodblaster/errors"
	"github.com/goodblaster/parchmint/internal/ocr/std"
)

func (engine *Engine) ParseBytes(b []byte, langs []string) ([]std.Line, error) {
	// If langs is nil, use default "en-US"
	if langs == nil {
		langs = []string{"en-US"}
	}

	// Convert Go slice of strings to a slice of *C.char
	cLangArray := make([]*C.char, len(langs))
	for i, lang := range langs {
		cLangArray[i] = C.CString(lang)
	}
	// Ensure that allocated C strings are freed later.
	defer func() {
		for _, cStr := range cLangArray {
			C.free(unsafe.Pointer(cStr))
		}
	}()

	// Perform OCR using the converted array.
	result := C.performAppleVisionOCR(
		unsafe.Pointer(&b[0]),
		C.size_t(len(b)),
		(**C.char)(unsafe.Pointer(&cLangArray[0])),
		C.size_t(len(cLangArray)),
	)
	defer C.free(unsafe.Pointer(result))

	// Parse JSON result: an array of lines on success, {"error": …} from
	// the helper on failure (undecodable bytes, Vision errors).
	raw := []byte(C.GoString(result))
	var lines []std.Line
	if err := json.Unmarshal(raw, &lines); err != nil {
		var helperErr struct {
			Error string `json:"error"`
		}
		if jerr := json.Unmarshal(raw, &helperErr); jerr == nil && helperErr.Error != "" {
			return nil, errors.New("apple vision: " + helperErr.Error)
		}
		return nil, errors.Wrap(err, "failed to parse JSON")
	}

	return lines, nil
}

func (engine *Engine) ParseReader(r io.Reader, langs []string) ([]std.Line, error) {
	b, err := io.ReadAll(r)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read image reader")
	}

	return engine.ParseBytes(b, langs)
}

func (engine *Engine) ParseFile(imagePath string, langs []string) ([]std.Line, error) {
	b, err := os.ReadFile(imagePath)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read image file")
	}

	return engine.ParseBytes(b, langs)
}
