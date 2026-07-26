package capture

import (
	"context"

	"github.com/chromedp/cdproto/emulation"

	"github.com/chromedp/chromedp"
	"github.com/goodblaster/errors"
	"github.com/goodblaster/parchmint/internal/log"
	"github.com/goodblaster/parchmint/textlayer"
	"github.com/goodblaster/pscription/actions"
	"github.com/goodblaster/pscription/runner"
	js "github.com/goodblaster/pscription/scripts"
)

// RenderOptions control RenderArchive: which phrases get marked, how the
// marks look, and what the archive is rendered INTO. `parch mark` and
// `parch render` are the same operation with and without phrases.
type RenderOptions struct {
	// Grayscale mutes images that contain hits so the highlight pops.
	Grayscale bool
	// Color fills highlight rectangles baked into images.
	Color string
	// Stroke outlines them (empty = no outline).
	Stroke string
	// Match selects how the phrases match (zero value = the default
	// phrase matcher).
	Match textlayer.MatchOptions
	// Colors gives phrase i its own highlight color, DOM marks and baked
	// image rectangles alike (missing entries fall back to the default /
	// Color). Where different-color highlights overlap, the LAST phrase
	// wins — the one deterministic rule.
	Colors []string
	// Style picks the mark presentation: bg (default), underline, box,
	// or bold. Baked image highlights render box as an outline; the
	// other styles fill (pixels can't underline).
	Style string

	// ViewportWidth re-renders the archive at this CSS-pixel layout width
	// instead of the width it was captured at (0 = the captured width).
	// Re-rendering REFLOWS the archive — a different responsive breakpoint
	// may apply. DOM marks survive that (they are wrapped inline against a
	// fresh extraction) and so do baked image marks (their rectangles are
	// image-relative fractions, which are scale-invariant), but the
	// archive's embedded layer still describes the CAPTURE geometry, so
	// its page-space boxes no longer match what was rendered.
	ViewportWidth int

	// DPR is the device pixel ratio to render at (0 = 1.0). Independent of
	// ViewportWidth: layout width picks the breakpoint, DPR picks the
	// output resolution.
	DPR float64

	// Backend renders the page instead of serializing it as an HTML
	// document: the raster backends (Screenshot/JPEG/WebP) photograph it,
	// so both kinds of mark come out as pixels; PDF{} additionally gets
	// invisible OCR overlay text and the text layer as an attachment. Nil
	// serializes the marked DOM (the default).
	Backend Backend

	// PaperWidthIn and PaperHeightIn size the sheet when Backend is PDF{};
	// see Snapshot for the semantics (they never reflow the layout).
	PaperWidthIn  float64
	PaperHeightIn float64

	// OverlayColor fills the highlight behind matched OCR words in the PDF
	// overlay (empty = a default yellow).
	OverlayColor string
}

// RenderResult reports what RenderArchive produced.
type RenderResult struct {
	// Bytes is the output: the serialized HTML document by default, or
	// whatever Options.Backend produced.
	Bytes []byte
	// MIME identifies Bytes ("text/html" for the document path).
	MIME string

	DOMMatches   int
	OCRMatches   int
	ImagesMarked int

	// PDF only: invisible selectable text placed over image words.
	ImagesOverlaid int
	OverlayWords   int
}

// RenderArchive loads an existing archive (file:// URL) at its recorded
// viewport, optionally marks it, and renders it into the requested format.
// It is the one path behind both `parch mark` (phrases given) and
// `parch render` (none) — marking is an option on rendering, not a
// separate operation.
//
// Marking: DOM text matches are wrapped in <mark data-parchmint>, and OCR
// matches (from `parch ocr`) are BAKED into their images as translucent
// rectangles — image-relative coordinates, so the highlight is correct at
// any viewer size, in any renderer, with no scripts. layer is the
// archive's embedded layer (for the OCR blocks); DOM matching runs against
// a FRESH extraction of the loaded document, and the marks are wrapped
// against that same extraction's cached run maps, so every match lands
// even though a bare archive's fonts/layout are still settling (which
// would shift block boundaries between two separate walks).
//
// With no phrases every marking step is skipped and this is a pure format
// conversion.
func RenderArchive(ctx context.Context, cfg runner.Config, fileURL string, phrases []string, layer *textlayer.Layer, opts RenderOptions) (*RenderResult, error) {
	// An explicit width wins; otherwise reproduce the capture geometry, so
	// the default stays "render it the way it was captured".
	if opts.ViewportWidth > 0 {
		cfg.ViewportWidth = int64(opts.ViewportWidth)
	} else if layer.Viewport.Width > 0 {
		cfg.ViewportWidth = int64(layer.Viewport.Width)
	}
	if opts.DPR > 0 {
		cfg.DeviceScaleFactor = opts.DPR
	}
	// BypassCSP lets our injected data URIs load — but it also disarms the
	// archive's script-blocking meta CSP, so any residual script in an
	// (old or third-party) archive could re-run and mutate the page
	// mid-mark. Disable page script execution outright; CDP evaluate is
	// unaffected, and archives are static documents that need no JS.
	cfg.PreNavigate = append(cfg.PreNavigate,
		actions.BypassCSP(),
		emulation.SetScriptExecutionDisabled(true))

	session, err := runner.Start(ctx, fileURL, cfg)
	if err != nil {
		return nil, err
	}
	defer session.Close()

	// Matches inside same-origin iframes cannot be highlighted in the copy:
	// re-opened from file://, Chrome treats subframes as cross-origin, so
	// the walk can't reach their content to wrap marks (and outerHTML would
	// not serialize a live iframe document anyway). Rare — the capture
	// pipeline freezes most iframes to images — but honestly reported.
	if n := frameBlockMatches(layer, phrases, opts.Match); n > 0 {
		log.With("matches", n).Warn("matches inside same-origin iframes are not highlighted in the marked copy")
	}

	res := &RenderResult{}
	err = session.Run(func(ctx context.Context) error {
		// Fonts settle geometry; the walk measures.
		if err := js.EvalSource(ctx, `(async () => { await document.fonts.ready; })`, nil); err != nil {
			log.WithError(err).Debug("fonts.ready wait failed; continuing")
		}

		// DOM text: fresh extraction (caching run maps), match, then wrap
		// against that same extraction — no second walk to diverge.
		if len(phrases) > 0 {
			payload, err := extractPayload(ctx, true)
			if err != nil {
				return err
			}
			n, err := applyHighlights(ctx, payload, phrases, opts.Match, opts.Colors, opts.Style)
			if err != nil {
				return err
			}
			res.DOMMatches = n
		}

		// OCR text is matched against the EMBEDDED layer's ocr blocks — a
		// fresh walk cannot see inside images. How it is applied depends on
		// the target: PDF gets positioned overlay text (invisible, so image
		// text becomes selectable and searchable in any viewer, with a
		// highlight behind matched words), everything else gets the marks
		// baked into the image pixels so they survive any reflow or viewer.
		_, toPDF := opts.Backend.(PDF)
		switch {
		case toPDF:
			specs, ocrMatches := ocrOverlaySpecs(layer, phrases, opts.Match)
			res.OCRMatches = ocrMatches
			if len(specs) > 0 {
				var stats struct {
					Images int `json:"images"`
					Words  int `json:"words"`
					Missed int `json:"missed"`
				}
				color := opts.OverlayColor
				if color == "" {
					color = defaultOverlayColor
				}
				if err := js.OverlayOcrText.Action(&stats, specs, map[string]any{
					"color":  color,
					"colors": opts.Colors,
					"style":  opts.Style,
				}).Do(ctx); err != nil {
					return errors.Wrap(err, "overlay ocr text")
				}
				res.ImagesOverlaid, res.OverlayWords = stats.Images, stats.Words
				if stats.Missed > 0 {
					log.With("missed", stats.Missed).Warn("some images could not be overlaid")
				}
			}
		case len(phrases) > 0:
			specs, ocrMatches, err := ocrMarkSpecs(layer, phrases, opts.Match)
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
					"colors":    opts.Colors,
					"style":     opts.Style,
				}).Do(ctx); err != nil {
					return errors.Wrap(err, "bake image marks")
				}
				res.ImagesMarked = stats.Marked
				if stats.Missed > 0 {
					log.With("missed", stats.Missed).Warn("some images could not be marked")
				}
			}
		}

		// A backend renders the page as it stands — no serialization, so
		// none of the document-copy concerns below apply. The marks are
		// already in the DOM and in the image pixels (or the overlay), so
		// they simply get rendered along with everything else.
		if opts.Backend != nil {
			snap := &Snapshot{
				URL:              layer.URL,
				ViewportWidth:    cfg.ViewportWidth,
				DevicePixelRatio: cfg.DeviceScaleFactor,
				PaperWidthIn:     opts.PaperWidthIn,
				PaperHeightIn:    opts.PaperHeightIn,
			}
			if err := opts.Backend.Action(snap).Do(ctx); err != nil {
				return errors.Wrapf(err, "%s backend", opts.Backend.Name())
			}
			res.Bytes, res.MIME = snap.Bytes, snap.MIME

			// Attach the layer so `parch text`/`find` work on the .pdf
			// exactly as they do on the source archive.
			if toPDF {
				layerJSON, err := layer.Marshal()
				if err != nil {
					return errors.Wrap(err, "marshal text layer")
				}
				withLayer, err := textlayer.EmbedInPDF(snap.Bytes, layerJSON)
				if err != nil {
					log.WithError(err).Warn("could not attach text layer to pdf; the pdf is still searchable natively")
				} else {
					res.Bytes = withLayer
				}
			}
			return nil
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
		res.Bytes, res.MIME = []byte(html), "text/html"
		return nil
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}

// frameBlockMatches counts phrase matches that fall inside same-origin
// iframe blocks of the layer (Frame set) — the matches a marked copy
// cannot highlight.
func frameBlockMatches(layer *textlayer.Layer, phrases []string, match textlayer.MatchOptions) int {
	n := 0
	for _, phrase := range phrases {
		m, err := textlayer.Compile(phrase, match)
		if err != nil {
			continue
		}
		for i := range layer.Blocks {
			if layer.Blocks[i].Frame == "" {
				continue
			}
			n += len(m.FindBlock(&layer.Blocks[i]))
		}
	}
	return n
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
// natural resolution unchanged). Each rect's fifth element is the phrase
// index, for per-term colors; rects paint in phrase order, so overlaps
// resolve to "last term wins" like the DOM marks.
func ocrMarkSpecs(layer *textlayer.Layer, phrases []string, match textlayer.MatchOptions) (map[string][][5]float64, int, error) {
	specs := map[string][][5]float64{}
	matches := 0
	for term, phrase := range phrases {
		q, err := textlayer.Compile(phrase, match)
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
					specs[b.Image] = append(specs[b.Image], [5]float64{
						float64(r[0]-b.Box[0]) / float64(b.Box[2]),
						float64(r[1]-b.Box[1]) / float64(b.Box[3]),
						float64(r[2]) / float64(b.Box[2]),
						float64(r[3]) / float64(b.Box[3]),
						float64(term),
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
