package capture

import (
	"context"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/goodblaster/errors"
)

// PDF captures the page as a PDF via Chrome's Page.printToPDF. Unlike the
// screenshot backend, PDF text stays selectable/searchable (vector, not
// pixels) and fonts embed natively — so a PDF archive is Ctrl-F searchable
// with no OCR, and there's no tofu-glyph problem.
//
// Defaults aim to match what parch captures on screen rather than a site's
// print stylesheet:
//   - screen media emulation (not @media print, which often hides nav and
//     strips layout)
//   - backgrounds printed (dark sections stay dark)
//   - paper width = our desktop viewport width, so responsive sites render
//     their desktop layout instead of reflowing to a narrow "paper" mobile
//     layout
//   - zero margins (edge-to-edge, like the screen)
//   - a single tall page when the content fits under Chrome's page-size
//     limit (no mid-content page breaks); natural pagination as a fallback
//     for very long pages
type PDF struct{}

func (PDF) Name() string { return "pdf" }
func (PDF) Ext() string  { return ".pdf" }

// Chrome/PDF cap each page dimension at ~200 inches. Stay just under it and
// paginate beyond that.
const maxPaperInches = 199.0

const cssPxPerInch = 96.0

func (PDF) Action(snap *Snapshot) chromedp.ActionFunc {
	return func(ctx context.Context) error {
		// Render screen styles, not the print stylesheet.
		if err := emulation.SetEmulatedMedia().WithMedia("screen").Do(ctx); err != nil {
			return errors.Wrap(err, "failed to emulate screen media")
		}

		// Fixed (parallax) backgrounds don't render in the print pipeline —
		// the section prints blank and its white-on-dark text vanishes.
		if err := normalizeBackgroundAttachment(ctx); err != nil {
			return errors.Wrap(err, "failed to normalize background attachment")
		}
		// Fixed elements repeat on every printed page (a fixed nav becomes a
		// header on each page); flatten them so they appear once.
		if err := flattenFixedPositioning(ctx); err != nil {
			return errors.Wrap(err, "failed to flatten fixed positioning")
		}

		// Measure content height from Chrome's own layout metrics
		// (cssContentSize is the authoritative scrollable-area height in CSS
		// px, and matches what printToPDF lays out — the DOM's scrollHeight
		// can under-report, e.g. a collapsed footer, spilling content onto a
		// spurious extra page). Small buffer absorbs sub-pixel rounding.
		_, _, _, _, _, cssContentSize, err := page.GetLayoutMetrics().Do(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get layout metrics")
		}

		// Paper width = the width the page was actually laid out at: the
		// session's viewport width, falling back to the measured content
		// width for callers that built the Snapshot themselves.
		widthPx := float64(snap.ViewportWidth)
		if widthPx <= 0 {
			widthPx = cssContentSize.Width
		}
		widthIn := widthPx / cssPxPerInch
		heightPx := cssContentSize.Height + 4

		params := page.PrintToPDF().
			WithPrintBackground(true).
			WithPreferCSSPageSize(false).
			WithScale(1).
			WithMarginTop(0).WithMarginBottom(0).WithMarginLeft(0).WithMarginRight(0).
			WithPaperWidth(widthIn)

		// One tall page preserves the continuous layout; only fall back to
		// pagination (default paper height) when the page is too long for a
		// single PDF page.
		if heightIn := heightPx / cssPxPerInch; heightIn > 0 && heightIn <= maxPaperInches {
			params = params.WithPaperHeight(heightIn)
		}

		data, _, err := params.Do(ctx)
		if err != nil {
			return errors.Wrap(err, "printToPDF failed")
		}
		snap.MIME = "application/pdf"
		snap.Bytes = data
		return nil
	}
}
