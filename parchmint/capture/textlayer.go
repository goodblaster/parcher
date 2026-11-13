package capture

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/goodblaster/errors"
	"github.com/goodblaster/parcher/parchmint/internal/log"
	"github.com/goodblaster/parcher/parchmint/textlayer"
	js "github.com/goodblaster/parcher/pscription/scripts"
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
	Hidden   json.RawMessage `json:"hidden"`
	Images   json.RawMessage `json:"images"`
}

// extractPayload runs the text-layer extraction against the live,
// prepared page. When cacheRuns is set, the walk stashes each block's
// text-node map on window.__pmRunCache so a following applyTextMarks can
// wrap ranges against this exact extraction (see applyHighlights).
func extractPayload(ctx context.Context, cacheRuns bool) (*textLayerPayload, error) {
	var payload textLayerPayload
	if err := js.ExtractTextLayer.Action(&payload, cacheRuns).Do(ctx); err != nil {
		return nil, errors.Wrap(err, "text layer extraction")
	}

	var st struct {
		Blocks           int      `json:"blocks"`
		Words            int      `json:"words"`
		HiddenBlocks     int      `json:"hiddenBlocks"`
		OracleMismatches int      `json:"oracleMismatches"`
		PageSimilarity   *float64 `json:"pageSimilarity"`
	}
	_ = json.Unmarshal(payload.Stats, &st)
	l := log.With("blocks", st.Blocks).With("words", st.Words).
		With("hiddenBlocks", st.HiddenBlocks).With("oracleMismatches", st.OracleMismatches)
	if st.PageSimilarity != nil {
		l = l.With("pageSimilarity", *st.PageSimilarity)
	}
	l.Debug("extracted text layer")
	return &payload, nil
}

// markEntry mirrors the marks parameter of apply_text_marks.js: the
// UTF-16 ranges to wrap in a block, each tagged with the query term that
// owns it (for per-term colors).
type markEntry struct {
	Ranges [][3]int `json:"ranges"`
}

// MarkCounts reports what a marking pass matched.
type MarkCounts struct {
	// Visible is matches in text the page shows, all of them marked.
	Visible int
	// Hidden is matches in hidden text that were marked AND revealed — only
	// ever non-zero when the caller both asked for hidden text
	// (MatchOptions.Hidden) and allowed revealing.
	Hidden int
	// HiddenUnmarked is matches sitting in hidden text that were left alone:
	// not asked for, or not revealable here. Reported so the caller can say
	// "N more in hidden text" instead of silently under-counting.
	HiddenUnmarked int
}

// applyHighlights matches the phrases against an extracted payload and
// wraps every match in <mark data-parchmint>. Matching uses the same Go
// matcher as `parch find`, so highlighting and archive search agree by
// construction. The marks are applied by apply_text_marks.js against the
// SAME extraction (its cached run maps) — not a second walk — so every
// match lands even when fonts/layout shift block boundaries between
// calls. Requires the preceding extractPayload to have run with
// cacheRuns=true. Marks land in the DOM before serialization, so every
// backend shows them — yellow pixels in screenshots and PDFs, <mark>
// elements in HTML.
//
// colors gives phrase i its own highlight color (nil / missing entries =
// the default <mark> look); style picks the mark presentation (bg,
// underline, box, bold — empty = bg). Ranges are flattened Go-side to
// DISJOINT per-block spans with "last term wins" on overlap, because the
// JS marker must wrap everything in one back-to-front pass against the
// cached extraction (a second pass would see nodes the first one split).
//
// Hidden text: a <mark> inside a collapsed section is a highlight nobody can
// see, and counting it as a match is a lie. So hidden blocks are matched
// separately and marked only when BOTH match.Hidden asks for them and reveal
// is true — in which case reveal_hidden.js first un-hides each matched block
// (labelling it as revealed) and the marks then land on visible text.
// reveal is false at capture time: an archive is the page as it was, and
// only a marked COPY may show what the page did not. Text hidden by parch's
// own prep (popup chrome) is never marked or revealed.
func applyHighlights(ctx context.Context, payload *textLayerPayload, phrases []string, match textlayer.MatchOptions, colors []string, style string, reveal bool) (MarkCounts, error) {
	var counts MarkCounts
	var blocks, hidden []textlayer.Block
	if err := json.Unmarshal(payload.Blocks, &blocks); err != nil {
		return counts, errors.Wrap(err, "parse blocks")
	}
	if len(payload.Hidden) > 0 {
		if err := json.Unmarshal(payload.Hidden, &hidden); err != nil {
			return counts, errors.Wrap(err, "parse hidden blocks")
		}
	}

	// Three views of the same query: visible text, the hidden text that was
	// asked for, and all hidden text (to count what was left out).
	visOpts := match
	visOpts.Hidden, visOpts.HiddenOnly = nil, false
	wantOpts := match
	wantOpts.HiddenOnly = true
	anyOpts := match
	anyOpts.Hidden, anyOpts.HiddenOnly = []string{textlayer.HiddenAll}, true
	wantHidden := reveal && len(match.Hidden) > 0

	hitsByTerm := make([][]textlayer.Hit, len(phrases))
	revealIDs := map[int]bool{}
	for ti, phrase := range phrases {
		vis, err := textlayer.Compile(phrase, visOpts)
		if err != nil {
			return counts, err
		}
		if !match.HiddenOnly {
			for i := range blocks {
				hits := vis.FindBlock(&blocks[i])
				hitsByTerm[ti] = append(hitsByTerm[ti], hits...)
				counts.Visible += len(hits)
			}
		}

		anyM, err := textlayer.Compile(phrase, anyOpts)
		if err != nil {
			return counts, err
		}
		var wantM textlayer.Matcher
		if wantHidden {
			if wantM, err = textlayer.Compile(phrase, wantOpts); err != nil {
				return counts, err
			}
		}
		for i := range hidden {
			b := &hidden[i]
			if b.Why == textlayer.HiddenPrep {
				continue
			}
			if wantM != nil {
				if hits := wantM.FindBlock(b); len(hits) > 0 {
					hitsByTerm[ti] = append(hitsByTerm[ti], hits...)
					counts.Hidden += len(hits)
					revealIDs[b.ID] = true
					continue
				}
			}
			counts.HiddenUnmarked += len(anyM.FindBlock(b))
		}
	}

	total := counts.Visible + counts.Hidden
	l := log.With("phrases", len(phrases)).With("matches", total)
	if counts.Hidden+counts.HiddenUnmarked > 0 {
		l = l.With("hiddenMarked", counts.Hidden).With("hiddenUnmarked", counts.HiddenUnmarked)
	}
	if total == 0 {
		l.Info("highlighting matches")
		return counts, nil
	}

	if len(revealIDs) > 0 {
		ids := make([]int, 0, len(revealIDs))
		for id := range revealIDs {
			ids = append(ids, id)
		}
		sort.Ints(ids)
		var st struct {
			Revealed   int `json:"revealed"`
			Unresolved int `json:"unresolved"`
		}
		if err := js.RevealHidden.Action(&st, ids).Do(ctx); err != nil {
			return counts, errors.Wrap(err, "reveal hidden text")
		}
		if st.Unresolved > 0 {
			log.With("blocks", st.Unresolved).Warn("some hidden text could not be made visible; its marks are in the document but may not show")
		}
		l = l.With("revealed", st.Revealed)
	}

	marks := map[int]markEntry{}
	for id, ranges := range textlayer.MergeTermRanges(hitsByTerm) {
		e := markEntry{Ranges: make([][3]int, 0, len(ranges))}
		for _, r := range ranges {
			e.Ranges = append(e.Ranges, [3]int{r.Start, r.End, r.Term})
		}
		marks[id] = e
	}

	var st struct {
		Marked  int `json:"marked"`
		Skipped int `json:"skipped"`
	}
	if err := js.ApplyTextMarks.Action(&st, marks, map[string]any{"colors": colors, "style": style}).Do(ctx); err != nil {
		return counts, errors.Wrap(err, "apply text marks")
	}
	l.With("marked", st.Marked).With("skipped", st.Skipped).Info("highlighting matches")
	return counts, nil
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
		Hidden     json.RawMessage `json:"hidden,omitempty"`
		Images     json.RawMessage `json:"images,omitempty"`
	}{
		Format:     textlayer.Format,
		URL:        snap.URL,
		CapturedAt: time.Now().UTC().Format(time.RFC3339),
		Generator:  "parch",
		Viewport:   payload.Viewport,
		Stats:      payload.Stats,
		Blocks:     payload.Blocks,
		Hidden:     nonEmptyArray(payload.Hidden),
		Images:     payload.Images,
	}
	out, err := json.Marshal(layer)
	if err != nil {
		return nil, errors.Wrap(err, "marshal text layer")
	}
	return out, nil
}

// nonEmptyArray drops a JSON array that is absent or empty, so the layer
// omits the field instead of carrying "hidden": [].
func nonEmptyArray(raw json.RawMessage) json.RawMessage {
	if t := strings.TrimSpace(string(raw)); t == "" || t == "[]" || t == "null" {
		return nil
	}
	return raw
}

// spliceTextLayer embeds the layer in the serialized archive (see
// textlayer.EmbedInHTML for the mechanics).
func spliceTextLayer(html, layer []byte) []byte {
	return textlayer.EmbedInHTML(html, layer)
}
