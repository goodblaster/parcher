package tesseract

import (
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/goodblaster/parcher/parchmint/internal/ocr/std"
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

// Regression: pure red text on a pale blue ground is BRIGHTER than the
// ground in the red channel, and tesseract's own binarization erased it. The
// darkest-channel copy must make saturated text dark and leave the ground,
// and transparent pixels (page background), light.
func TestDarkestChannel(t *testing.T) {
	src := image.NewNRGBA(image.Rect(0, 0, 3, 1))
	src.Set(0, 0, color.NRGBA{254, 0, 0, 255})     // the text
	src.Set(1, 0, color.NRGBA{197, 242, 245, 255}) // the ground
	src.Set(2, 0, color.NRGBA{0, 0, 0, 0})         // transparent
	g := darkestChannel(src)
	if g.Pix[0] > 10 || g.Pix[1] < 190 || g.Pix[2] < 250 {
		t.Fatalf("text %d (want dark), ground %d (want light), transparent %d (want white)", g.Pix[0], g.Pix[1], g.Pix[2])
	}
}

// A later pass adds only words in places no earlier pass read.
func TestMergeLines(t *testing.T) {
	w := func(text string, left float64) std.Word {
		return std.Word{Text: text, Left: left, Top: 0.1, Width: 0.1, Height: 0.1}
	}
	base := []std.Line{{Words: []std.Word{w("SERIES", 0.1)}}}
	more := []std.Line{{Words: []std.Word{w("SERlES", 0.11), w("12", 0.5)}}}
	got := mergeLines(base, more)
	if len(got) != 2 || got[1].LineText != "12" || len(got[1].Words) != 1 {
		t.Fatalf("want the re-read word dropped and the new one added, got %+v", got)
	}
	if !approx(got[1].Rect.Left, 0.5) {
		t.Errorf("added line's rect not recomputed: %+v", got[1].Rect)
	}
}
