package tesseract

import (
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	"bytes"
	"fmt"
	_ "golang.org/x/image/webp"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"

	"github.com/goodblaster/errors"
	"github.com/goodblaster/parcher/parchmint/internal/ocr/std"
)

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
	return engine.ParseFile(tmpFile.Name(), langs)
}

func (engine *Engine) ParseReader(r io.Reader, langs []string) ([]std.Line, error) {
	tmpFile, err := os.CreateTemp("", "tesseract-*")
	if err != nil {
		return nil, errors.Wrap(err, "failed to create temporary file")
	}
	defer os.Remove(tmpFile.Name())

	_, err = io.Copy(tmpFile, r)
	if err != nil {
		return nil, errors.Wrap(err, "failed to write to temporary file")
	}
	_ = tmpFile.Close()
	return engine.ParseFile(tmpFile.Name(), langs)
}

func (engine *Engine) ParseFile(filename string, langs []string) ([]std.Line, error) {
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
	args := []string{filename, "stdout", "--psm", strconv.Itoa(psm()), "-l", strings.Join(tessLangs, "+"), "tsv"}
	cmd := exec.Command("tesseract", args...)

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

	// Parse the TSV output
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

// installed lists the language packs this tesseract has, asked once per
// process. It is also where a missing tesseract is reported, with what to do
// about it, instead of once per image as a bare exec error.
var installed = sync.OnceValues(func() (map[string]bool, error) {
	out, err := exec.Command("tesseract", "--list-langs").CombinedOutput()
	if err != nil {
		return nil, errors.New("the tesseract OCR engine needs the `tesseract` program on PATH " +
			"(Debian/Ubuntu: apt install tesseract-ocr; Fedora: dnf install tesseract; macOS: brew install tesseract)")
	}
	langs := map[string]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.Contains(line, " ") { // the header line
			continue
		}
		langs[line] = true
	}
	return langs, nil
})

var warnedMissing sync.Map

// resolveLangs turns page language hints (BCP 47, as in <html lang>) into
// the tesseract language list for one run.
//
// A language whose pack is not installed is dropped with a one-time note
// naming the package to install — passing it through makes tesseract fail on
// EVERY image, and a stock Linux install carries English only. English is
// always included when present: whatever language a page is written in, its
// images are full of Latin-script brand names, units and UI words, and a
// single-language run reads those as garbage.
func resolveLangs(hints []string, inst map[string]bool) []string {
	var out []string
	add := func(l string) {
		for _, have := range out {
			if have == l {
				return
			}
		}
		out = append(out, l)
	}
	for _, hint := range hints {
		tl := NormalizeLangCode(hint)
		if tl == "" {
			continue
		}
		if !inst[tl] {
			if _, seen := warnedMissing.LoadOrStore(tl, true); !seen {
				fmt.Fprintf(os.Stderr, "parch: tesseract has no %q language pack for page language %q; "+
					"text in that language will be misread (Debian/Ubuntu: apt install tesseract-ocr-%s)\n",
					tl, hint, strings.ReplaceAll(tl, "_", "-"))
			}
			continue
		}
		add(tl)
	}
	if inst["eng"] {
		add("eng")
	}
	if len(out) == 0 {
		// Nothing usable is installed; let tesseract say so itself.
		out = []string{"eng"}
	}
	return out
}

// parseTSV parses the TSV output from Tesseract and organizes it hierarchically
func (engine *Engine) parseTSV(tsv string, pageWidth int, pageHeight int) ([]std.Line, error) {
	var lines []std.Line
	var currentLine std.Line

	rows, err := ParseTabDelimitedString(tsv)
	if err != nil {
		return lines, fmt.Errorf("failed to read TSV: %v", err)
	}

	// TSV columns: level page_num block_num par_num line_num word_num
	// left top width height conf text. Words are level 5; a line's
	// identity is the (block, par, line) triple.
	lastLineKey := ""
	var confSum float64
	var confN int
	flush := func() {
		if len(currentLine.Words) > 0 {
			if confN > 0 {
				currentLine.Confidence = confSum / float64(confN) / 100.0
			}
			lines = append(lines, currentLine)
		}
		currentLine = std.Line{}
		confSum, confN = 0, 0
	}
	for i, row := range rows {
		if i == 0 || len(row) < 12 {
			// Header or incomplete row.
			continue
		}

		level, _ := strconv.Atoi(row[0])
		if level != 5 {
			continue
		}
		lineKey := row[2] + "/" + row[3] + "/" + row[4]
		left, _ := strconv.Atoi(row[6])
		top, _ := strconv.Atoi(row[7])
		width, _ := strconv.Atoi(row[8])
		height, _ := strconv.Atoi(row[9])
		conf, _ := strconv.ParseFloat(row[10], 64)

		word := std.Word{
			Text:   row[11],
			Top:    toPct(top, pageHeight),
			Left:   toPct(left, pageWidth),
			Width:  toPct(width, pageWidth),
			Height: toPct(height, pageHeight),
		}

		if lineKey != lastLineKey {
			flush()
			lastLineKey = lineKey
		}
		if strings.TrimSpace(word.Text) != "" && conf >= minConf() {
			currentLine.Words = append(currentLine.Words, word)
			if conf >= 0 {
				confSum += conf
				confN++
			}
		}
	}
	flush()

	for i, line := range lines {
		var rect std.Rect
		var words []string
		for _, word := range line.Words {
			text := strings.TrimSpace(word.Text)
			if text == "" {
				continue
			}

			words = append(words, text)
			if rect.Width == 0 || rect.Height == 0 {
				rect.Left = word.Left
				rect.Top = word.Top
				rect.Width = word.Width
				rect.Height = word.Height
				continue
			}
			if word.Left < rect.Left {
				rect.Left = word.Left
			}
			if word.Top < rect.Top {
				rect.Top = word.Top
			}
			if word.Left+word.Width > rect.Left+rect.Width {
				rect.Width = word.Left + word.Width - rect.Left
			}
			if word.Top+word.Height > rect.Top+rect.Height {
				rect.Height = word.Top + word.Height - rect.Top
			}
		}
		lines[i].Rect = rect
		lines[i].LineText = strings.Join(words, " ")
	}

	return lines, nil
}

func ParseTabDelimitedString(tsv string) ([][]string, error) {
	var rows [][]string

	// Split the input string into lines
	lines := strings.Split(tsv, "\n")

	for _, line := range lines {
		columns := strings.Split(line, "\t")
		rows = append(rows, columns)
	}

	return rows, nil
}

func toPct(d int, max int) float64 {
	return float64(d) / float64(max)
}

// imageSize reads the pixel dimensions of an image file (png/jpeg/gif/webp).
func imageSize(filename string) (int, int, error) {
	f, err := os.Open(filename)
	if err != nil {
		return 0, 0, err
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return 0, 0, err
	}
	return cfg.Width, cfg.Height, nil
}
