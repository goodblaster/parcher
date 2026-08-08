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

	"github.com/goodblaster/errors"
	"github.com/goodblaster/parchmint/internal/ocr/std"
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
	var tessLangs []string
	for _, lang := range langs {
		tessLang := NormalizeLangCode(lang)
		if tessLang != "" {
			tessLangs = append(tessLangs, tessLang)
		}
	}

	if len(tessLangs) == 0 {
		tessLangs = []string{"eng"}
	}

	// Prepare the command to call Tesseract with TSV output
	cmd := exec.Command("tesseract", filename, "stdout", "-c", "tessedit_create_tsv=1", "--psm", "1", "-l", strings.Join(tessLangs, "+"), "tsv")

	// Capture the output
	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr

	// Run the command
	err := cmd.Run()
	if err != nil {
		return nil, errors.Wrap(err, "tesseract error")
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
		if strings.TrimSpace(word.Text) != "" {
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
