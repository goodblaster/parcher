package tesseract

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/goodblaster/parcher/parchmint/internal/ocr/std"
)

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

	for i := range lines {
		lines[i].SetBounds()
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
