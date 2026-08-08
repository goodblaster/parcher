package capture

import (
	"github.com/goodblaster/parcher/parchmint/textlayer"
)

// defaultOverlayColor fills the highlight behind matched OCR words in a
// PDF overlay when the caller does not pick one.
const defaultOverlayColor = "rgba(255, 220, 0, 0.45)"

// ocrWord is one overlay word: text plus an image-relative box (fractions
// of the displayed image), whether it matched a phrase, and which phrase
// (C indexes the per-term colors).
type ocrWord struct {
	T  string  `json:"t"`
	X  float64 `json:"x"`
	Y  float64 `json:"y"`
	W  float64 `json:"w"`
	H  float64 `json:"h"`
	Hl bool    `json:"hl"`
	C  int     `json:"c"`
}

// ocrOverlaySpecs turns the layer's OCR blocks into per-image overlay word
// lists (keyed by image hash), flagging words that match any phrase — and
// with which phrase, scanning in phrase order so an overlap resolves to
// the LAST phrase, like the DOM marks. The int returned is the number of
// phrase matches inside images.
func ocrOverlaySpecs(layer *textlayer.Layer, phrases []string, match textlayer.MatchOptions) (map[string][]ocrWord, int) {
	type termQuery struct {
		m    textlayer.Matcher
		term int
	}
	queries := make([]termQuery, 0, len(phrases))
	for ti, p := range phrases {
		if q, err := textlayer.Compile(p, match); err == nil {
			queries = append(queries, termQuery{m: q, term: ti})
		}
	}

	specs := map[string][]ocrWord{}
	matches := 0
	for i := range layer.Blocks {
		b := &layer.Blocks[i]
		if b.Source != "ocr" || b.Image == "" || b.Box[2] == 0 || b.Box[3] == 0 {
			continue
		}

		// Char ranges matched by any phrase, for flagging words.
		type termRange struct {
			r    [2]int
			term int
		}
		var ranges []termRange
		for _, q := range queries {
			for _, hit := range q.m.FindBlock(b) {
				matches++
				ranges = append(ranges, termRange{r: [2]int{hit.Start, hit.End}, term: q.term})
			}
		}

		fx, fy := float64(b.Box[0]), float64(b.Box[1])
		fw, fh := float64(b.Box[2]), float64(b.Box[3])
		for _, w := range b.Words {
			hl := false
			ci := 0
			for _, r := range ranges { // term order: last overlap wins
				if w.Start < r.r[1] && w.End > r.r[0] {
					hl = true
					ci = r.term
				}
			}
			specs[b.Image] = append(specs[b.Image], ocrWord{
				T:  textlayer.UTF16Slice(b.Text, w.Start, w.End),
				X:  (float64(w.Box[0]) - fx) / fw,
				Y:  (float64(w.Box[1]) - fy) / fh,
				W:  float64(w.Box[2]) / fw,
				H:  float64(w.Box[3]) / fh,
				Hl: hl,
				C:  ci,
			})
		}
	}
	return specs, matches
}
