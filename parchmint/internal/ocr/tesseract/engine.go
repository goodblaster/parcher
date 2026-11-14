package tesseract

import (
	"bytes"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	"github.com/goodblaster/errors"
	"github.com/goodblaster/parcher/parchmint/internal/ocr"
	"github.com/goodblaster/parcher/parchmint/internal/ocr/std"
)

// Name is the engine's name on the command line.
const Name = "tesseract"

// Engine runs the `tesseract` program, once per pass per image.
type Engine struct{}

func (engine *Engine) Name() string { return Name }

func init() {
	// Several images at once: each is its own tesseract processes, so the
	// work spreads across cores, and results keep their input order. Every
	// process is held to ONE thread (see pass) — tesseract's own OpenMP
	// threading would otherwise put four threads behind each of them.
	ocr.Register(Name, func() (ocr.Recognizer, error) {
		return ocr.PerImage(&Engine{}, ocr.Concurrency(runtime.NumCPU())), nil
	})
}

func (engine *Engine) ParseBytes(b []byte, langs []string) ([]std.Line, error) {
	tmpFile, err := os.CreateTemp("", "tesseract-*")
	if err != nil {
		return nil, errors.Wrap(err, "failed to create temporary file")
	}
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.Write(b)
	if err != nil {
		return nil, errors.Wrap(err, "failed to write to temporary file")
	}
	_ = tmpFile.Close()
	return engine.parseFile(tmpFile.Name(), langs)
}

// parseFile reads one image file: the first pass over it as given, then the
// derived copies.
func (engine *Engine) parseFile(filename string, langs []string) ([]std.Line, error) {
	inst, err := installed()
	if err != nil {
		return nil, err
	}
	tessLangs := resolveLangs(langs, inst)

	// --psm 11 (sparse text) with a per-word confidence floor. Web images are
	// logos, banners, product shots and UI — scattered words, not a page of
	// prose — and the page-layout modes discard most of it looking for
	// columns. Sparse mode alone finds more but hallucinates words in photos,
	// so low-confidence words are dropped. Measured against Apple Vision on
	// two reference archives (share of Vision's words found / share of
	// tesseract's words Vision agrees with):
	//
	//	psm 1, no floor (the old setting)   0.35 / 0.79    0.07 / 0.50
	//	psm 11, no floor                    0.45 / 0.38    0.16 / 0.12
	//	psm 11, conf >= 70                  0.40 / 0.91    0.12 / 0.50
	//
	// More found, and no more noise than before. Enlarging the image 2x or 3x
	// first was tried and made both numbers worse. Mode 1 also needs the
	// separate orientation model (osd), which a minimal install lacks.
	// PARCH_TESSERACT_PSM and PARCH_TESSERACT_MIN_CONF override, for tuning.
	lines, err := engine.pass(filename, tessLangs)
	if err != nil {
		return nil, err
	}

	// Further passes over derived copies of the image (see passImages), each
	// contributing only words in places no earlier pass read. A failure here
	// costs the extra words, never the first pass's.
	derived, cleanup := passImages(filename)
	defer cleanup()
	for _, f := range derived {
		more, err := engine.pass(f, tessLangs)
		if err != nil {
			continue
		}
		lines = mergeLines(lines, more)
	}
	return lines, nil
}

// pass runs tesseract once over one image file.
func (engine *Engine) pass(filename string, tessLangs []string) ([]std.Line, error) {
	args := []string{filename, "stdout", "--psm", strconv.Itoa(psm()), "-l", strings.Join(tessLangs, "+"), "tsv"}
	cmd := exec.Command("tesseract", args...)
	// One thread per process. Images are already read in parallel, and
	// tesseract's OpenMP threads mostly spin waiting on each other: with
	// them on, parallel runs fight over the same cores.
	cmd.Env = append(os.Environ(), "OMP_THREAD_LIMIT=1")

	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// tesseract explains itself on stderr; "exit status 1" alone does not.
		detail := strings.TrimSpace(stderr.String())
		if len(detail) > 300 {
			detail = detail[:300]
		}
		return nil, errors.Wrapf(err, "tesseract error: %s", detail)
	}

	// We need page width and height to normalize boxes to 0..1.
	pageWidth, pageHeight, err := imageSize(filename)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get image size")
	}

	lines, err := engine.parseTSV(out.String(), pageWidth, pageHeight)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse tesseract TSV output")
	}
	return lines, nil
}

func minConf() float64 {
	if v, err := strconv.ParseFloat(os.Getenv("PARCH_TESSERACT_MIN_CONF"), 64); err == nil {
		return v
	}
	return 70
}

func psm() int {
	if v, err := strconv.Atoi(os.Getenv("PARCH_TESSERACT_PSM")); err == nil && v >= 0 && v <= 13 {
		return v
	}
	return 11
}
