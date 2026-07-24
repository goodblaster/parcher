package tesseract

import (
	"math"
	"testing"
)

// A tesseract TSV, columns:
// level page_num block_num par_num line_num word_num left top width height conf text
const sampleTSV = "level\tpage_num\tblock_num\tpar_num\tline_num\tword_num\tleft\ttop\twidth\theight\tconf\ttext\n" +
	"5\t1\t1\t1\t1\t1\t10\t5\t20\t10\t96\tThe\n" +
	"5\t1\t1\t1\t1\t2\t35\t5\t25\t10\t94\tquick\n" +
	"5\t1\t1\t1\t2\t1\t10\t20\t30\t10\t90\tbrown\n" +
	"5\t1\t1\t1\t2\t2\t45\t20\t20\t10\t92\tfox\n"

func approx(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func TestParseTSV(t *testing.T) {
	e := &Engine{}
	lines, err := e.parseTSV(sampleTSV, 100, 50)
	if err != nil {
		t.Fatal(err)
	}

	// Regression: the final line must be flushed (an earlier version
	// dropped it), and lines must group by (block,par,line) identity, not
	// be merged — so exactly two lines here.
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2", len(lines))
	}
	if lines[0].LineText != "The quick" || lines[1].LineText != "brown fox" {
		t.Fatalf("line text: %q / %q", lines[0].LineText, lines[1].LineText)
	}

	// Regression: TSV columns are left/top/WIDTH/HEIGHT, not x2/y2. "The"
	// at left=10 width=20 on a 100px page → left 0.10, width 0.20.
	w := lines[0].Words[0]
	if !approx(w.Left, 0.10) || !approx(w.Width, 0.20) || !approx(w.Top, 0.10) || !approx(w.Height, 0.20) {
		t.Errorf("word box normalized wrong: %+v", w)
	}

	// Line confidence is the mean word conf, 0..1.
	if !approx(lines[0].Confidence, (96.0+94.0)/2/100) {
		t.Errorf("confidence %v, want %v", lines[0].Confidence, 0.95)
	}
}
