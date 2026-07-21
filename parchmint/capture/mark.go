package capture

import (
	"context"

	"github.com/chromedp/chromedp"
	"github.com/goodblaster/errors"
	"github.com/goodblaster/parchmint/internal/log"
	"github.com/goodblaster/parchmint/textlayer"
	"github.com/goodblaster/pscription/actions"
	"github.com/goodblaster/pscription/runner"
	js "github.com/goodblaster/pscription/scripts"
)

// MarkOptions style the highlights `parch mark` applies to an archive.
type MarkOptions struct {
	// Grayscale mutes images that contain hits so the highlight pops.
	Grayscale bool
	// Color fills highlight rectangles baked into images.
	Color string
	// Stroke outlines them (empty = no outline).
	Stroke string
}

// MarkResult reports what MarkArchive did.
type MarkResult struct {
	HTML         []byte
	DOMMatches   int
	OCRMatches   int
	ImagesMarked int
}

// MarkArchive loads an existing archive (file:// URL) at its recorded
// viewport and produces a marked copy: DOM text matches wrapped in
// <mark data-parchmint> via the same deterministic walk used at capture,
// and OCR matches (from `parch index`) BAKED into their images as
// translucent rectangles — image-relative coordinates, so the highlight
// is correct at any viewer size, in any renderer, with no scripts.
// layer is the archive's embedded layer (for the OCR blocks); DOM
// matching runs against a FRESH extraction of the loaded document, so
// marking is self-consistent even if the serialized DOM walks slightly
// differently than the original page did.
func MarkArchive(ctx context.Context, cfg runner.Config, fileURL string, phrases []string, layer *textlayer.Layer, opts MarkOptions) (*MarkResult, error) {
	if layer.Viewport.Width > 0 {
		cfg.ViewportWidth = int64(layer.Viewport.Width)
	}
	cfg.PreNavigate = append(cfg.PreNavigate, actions.BypassCSP())

	session, err := runner.Start(ctx, fileURL, cfg)
	if err != nil {
		return nil, err
	}
	defer session.Close()

	res := &MarkResult{}
	err = session.Run(func(ctx context.Context) error {
		// Fonts settle geometry; the walk measures.
		if err := js.EvalSource(ctx, `(async () => { await document.fonts.ready; })`, nil); err != nil {
			log.WithError(err).Debug("fonts.ready wait failed; continuing")
		}

		// DOM text: fresh extraction, match, second walk wraps.
		payload, err := extractPayload(ctx, nil)
		if err != nil {
			return err
		}
		n, err := applyHighlights(ctx, payload, phrases)
		if err != nil {
			return err
		}
		res.DOMMatches = n

		// OCR text: matched against the EMBEDDED layer's ocr blocks (a
		// fresh walk cannot see inside images), baked via canvas.
		specs, ocrMatches, err := ocrMarkSpecs(layer, phrases)
		if err != nil {
			return err
		}
		res.OCRMatches = ocrMatches
		if len(specs) > 0 {
			var stats struct {
				Marked int `json:"marked"`
				Missed int `json:"missed"`
			}
			if err := js.BakeImageMarks.Action(&stats, specs, map[string]any{
				"grayscale": opts.Grayscale,
				"color":     opts.Color,
				"stroke":    opts.Stroke,
			}).Do(ctx); err != nil {
				return errors.Wrap(err, "bake image marks")
			}
			res.ImagesMarked = stats.Marked
			if stats.Missed > 0 {
				log.With("missed", stats.Missed).Warn("some images could not be marked")
			}
		}

		// Inline linked stylesheets before serializing: an MHT source
		// keeps its CSS in cid: parts, which die in a standalone HTML
		// copy (unstyled page, hidden text visible). The CSSOM has the
		// rules regardless of where they came from; HTML sources have no
		// <link> sheets left, so this is a no-op there.
		if err := chromedp.Evaluate(inlineLinkedStylesheets, nil).Do(ctx); err != nil {
			log.WithError(err).Warn("could not inline stylesheets; marked copy may lose styling")
		}

		var html string
		if err := chromedp.Evaluate(`'<!DOCTYPE html>' + document.documentElement.outerHTML`, &html).Do(ctx); err != nil {
			return errors.Wrap(err, "serialize marked document")
		}
		res.HTML = []byte(html)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}

// inlineLinkedStylesheets replaces every <link rel=stylesheet> with a
// <style> holding its rendered rules, read from the CSSOM.
const inlineLinkedStylesheets = `(() => {
	for (const sheet of Array.from(document.styleSheets)) {
		const node = sheet.ownerNode;
		if (!node || node.tagName !== 'LINK') continue;
		let css = '';
		try {
			for (const r of sheet.cssRules) css += r.cssText + '\n';
		} catch (e) { continue; }
		const style = document.createElement('style');
		style.textContent = css;
		node.replaceWith(style);
	}
})()`

// ocrMarkSpecs matches phrases against the layer's OCR blocks and returns
// bake specs: image hash → highlight rects as fractions of the image box
// (block.Box IS the source image's recorded box, so fractions transfer to
// natural resolution unchanged).
func ocrMarkSpecs(layer *textlayer.Layer, phrases []string) (map[string][][4]float64, int, error) {
	specs := map[string][][4]float64{}
	matches := 0
	for _, phrase := range phrases {
		q, err := textlayer.ParseQuery(phrase)
		if err != nil {
			return nil, 0, err
		}
		for i := range layer.Blocks {
			b := &layer.Blocks[i]
			if b.Source != "ocr" || b.Image == "" || b.Box[2] == 0 || b.Box[3] == 0 {
				continue
			}
			for _, hit := range q.FindBlock(b) {
				matches++
				for _, r := range mergeLineRects(hit.Words()) {
					specs[b.Image] = append(specs[b.Image], [4]float64{
						float64(r[0]-b.Box[0]) / float64(b.Box[2]),
						float64(r[1]-b.Box[1]) / float64(b.Box[3]),
						float64(r[2]) / float64(b.Box[2]),
						float64(r[3]) / float64(b.Box[3]),
					})
				}
			}
		}
	}
	return specs, matches, nil
}

// mergeLineRects joins a hit's word boxes into per-line rectangles, so a
// phrase highlights as one continuous band (spaces included) instead of
// per-word confetti — the image-side analogue of the DOM marks' contiguous
// wrapping. Words share a line when they overlap vertically; the merge
// bridges the inter-word gap.
func mergeLineRects(words []textlayer.Word) [][4]int {
	var out [][4]int
	for _, w := range words {
		x, y, ww, wh := w.Box[0], w.Box[1], w.Box[2], w.Box[3]
		merged := false
		for i := range out {
			r := &out[i]
			sameLine := y < r[1]+r[3] && y+wh > r[1]
			adjacent := x <= r[0]+r[2]+wh && x+ww >= r[0]-wh
			if sameLine && adjacent {
				x2 := max(r[0]+r[2], x+ww)
				y2 := max(r[1]+r[3], y+wh)
				if x < r[0] {
					r[0] = x
				}
				if y < r[1] {
					r[1] = y
				}
				r[2] = x2 - r[0]
				r[3] = y2 - r[1]
				merged = true
				break
			}
		}
		if !merged {
			out = append(out, [4]int{x, y, ww, wh})
		}
	}
	return out
}
