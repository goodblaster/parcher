package capture

import (
	"context"
	"encoding/json"
	"time"

	"github.com/goodblaster/errors"
	"github.com/goodblaster/parchmint/internal/log"
	"github.com/goodblaster/parchmint/textlayer"
	js "github.com/goodblaster/pscription/scripts"
)

// TextLayerType is the script-element type identifying the embedded text
// layer in an HTML archive. See TEXTLAYER.md for the format contract.
const TextLayerType = textlayer.ScriptType

// textLayerPayload is what the extraction script returns; the header
// fields (format/url/capturedAt/generator) are composed Go-side.
type textLayerPayload struct {
	Viewport json.RawMessage `json:"viewport"`
	Stats    json.RawMessage `json:"stats"`
	Blocks   json.RawMessage `json:"blocks"`
	Images   json.RawMessage `json:"images"`
}

// extractPayload runs the text-layer extraction against the live,
// prepared page. marks, when non-nil, additionally wraps the given ranges
// in <mark> elements during the walk (capture-time highlighting).
func extractPayload(ctx context.Context, marks map[int]markEntry) (*textLayerPayload, error) {
	var payload textLayerPayload
	var err error
	if marks == nil {
		err = js.ExtractTextLayer.Action(&payload).Do(ctx)
	} else {
		err = js.ExtractTextLayer.Action(&payload, marks).Do(ctx)
	}
	if err != nil {
		return nil, errors.Wrap(err, "text layer extraction")
	}

	var st struct {
		Blocks           int      `json:"blocks"`
		Words            int      `json:"words"`
		OracleMismatches int      `json:"oracleMismatches"`
		PageSimilarity   *float64 `json:"pageSimilarity"`
		Marked           int      `json:"marked"`
		MarkSkipped      int      `json:"markSkipped"`
	}
	_ = json.Unmarshal(payload.Stats, &st)
	l := log.With("blocks", st.Blocks).With("words", st.Words).With("oracleMismatches", st.OracleMismatches)
	if st.PageSimilarity != nil {
		l = l.With("pageSimilarity", *st.PageSimilarity)
	}
	if marks != nil {
		l = l.With("marked", st.Marked).With("markSkipped", st.MarkSkipped)
	}
	l.Debug("extracted text layer")
	return &payload, nil
}

// markEntry mirrors the marks parameter of text_layer.js: the block's
// expected text length (a mutation guard) plus the ranges to wrap.
type markEntry struct {
	Len    int      `json:"len"`
	Ranges [][2]int `json:"ranges"`
}

// applyHighlights matches the phrases against an extracted payload and
// runs a second walk that wraps every match in <mark data-parchmint>.
// Matching uses the same Go matcher as `parch find`, so capture-time
// highlighting and archive search agree by construction. The marks land
// in the DOM before serialization, so every backend shows them — yellow
// in screenshots and PDFs, <mark> elements in HTML.
func applyHighlights(ctx context.Context, payload *textLayerPayload, phrases []string) (int, error) {
	var blocks []textlayer.Block
	if err := json.Unmarshal(payload.Blocks, &blocks); err != nil {
		return 0, errors.Wrap(err, "parse blocks")
	}

	var hits []textlayer.Hit
	for _, phrase := range phrases {
		q, err := textlayer.ParseQuery(phrase)
		if err != nil {
			return 0, err
		}
		for i := range blocks {
			hits = append(hits, q.FindBlock(&blocks[i])...)
		}
	}
	log.With("phrases", len(phrases)).With("matches", len(hits)).Info("highlighting matches")
	if len(hits) == 0 {
		return 0, nil
	}

	blockText := map[int]string{}
	for i := range blocks {
		blockText[blocks[i].ID] = blocks[i].Text
	}
	marks := map[int]markEntry{}
	for id, ranges := range textlayer.MergeHitRanges(hits) {
		marks[id] = markEntry{Len: utf16Len(blockText[id]), Ranges: ranges}
	}

	// The second walk re-derives identical block ids (deterministic walk,
	// unchanged DOM) and wraps as it goes.
	_, err := extractPayload(ctx, marks)
	return len(hits), err
}

// utf16Len is the length of s in UTF-16 code units — JavaScript's
// String.length, the unit the extractor's offsets live in.
func utf16Len(s string) int {
	n := 0
	for _, r := range s {
		if r > 0xFFFF {
			n += 2
		} else {
			n++
		}
	}
	return n
}

// composeLayer wraps an extraction payload with the archive header,
// producing the complete parchmint-text/1 document for embedding.
func composeLayer(snap *Snapshot, payload *textLayerPayload) ([]byte, error) {
	layer := struct {
		Format     string          `json:"format"`
		URL        string          `json:"url"`
		CapturedAt string          `json:"capturedAt"`
		Generator  string          `json:"generator"`
		Viewport   json.RawMessage `json:"viewport"`
		Stats      json.RawMessage `json:"stats"`
		Blocks     json.RawMessage `json:"blocks"`
		Images     json.RawMessage `json:"images,omitempty"`
	}{
		Format:     textlayer.Format,
		URL:        snap.URL,
		CapturedAt: time.Now().UTC().Format(time.RFC3339),
		Generator:  "parch",
		Viewport:   payload.Viewport,
		Stats:      payload.Stats,
		Blocks:     payload.Blocks,
		Images:     payload.Images,
	}
	out, err := json.Marshal(layer)
	if err != nil {
		return nil, errors.Wrap(err, "marshal text layer")
	}
	return out, nil
}

// spliceTextLayer embeds the layer in the serialized archive (see
// textlayer.EmbedInHTML for the mechanics).
func spliceTextLayer(html, layer []byte) []byte {
	return textlayer.EmbedInHTML(html, layer)
}
