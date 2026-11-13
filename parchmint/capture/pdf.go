package capture

import (
	"context"
	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/chromedp"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/page"
	"github.com/goodblaster/errors"
	"github.com/goodblaster/parcher/parchmint/internal/log"
	"github.com/goodblaster/parcher/pscription/act"
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
//     layout — unless Snapshot.PaperWidthIn asks for a real sheet size, in
//     which case the layout is kept and the output is scaled to fit
//   - zero margins (edge-to-edge, like the screen)
//   - a single tall page when the content fits under Chrome's page-size
//     limit (no mid-content page breaks); pagination for very long pages
type PDF struct{}

func init() {
	Register("pdf", func() Backend { return PDF{} })
}

func (PDF) Name() string { return "pdf" }
func (PDF) Ext() string  { return ".pdf" }

// Chrome/PDF cap each page dimension at ~200 inches. Stay just under it and
// paginate beyond that.
const maxPaperInches = 199.0

// tallPaperInches is the height used when content will not fit on one page.
// Chrome's own fallback is LETTER (11in), which shreds a 25-foot page into
// dozens of stubs; a tall sheet keeps it to a handful.
const tallPaperInches = 190.0

// Chrome's printToPDF accepts a scale in [0.1, 2.0].
const (
	minPrintScale = 0.1
	maxPrintScale = 2.0
)

const cssPxPerInch = 96.0

func (PDF) Action(snap *Snapshot) act.Func {
	return func(ctx context.Context) error {
		// Render screen styles, not the print stylesheet.
		if _, err := chromedp.Call(ctx, emulation.SetEmulatedMedia, emulation.SetEmulatedMediaParams{Media: "screen"}); err != nil {
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
		metrics, err := chromedp.Call(ctx, page.GetLayoutMetrics, cdp.Empty{})
		cssContentSize := metrics.CSSContentSize
		if err != nil {
			return errors.Wrap(err, "failed to get layout metrics")
		}

		// Layout width: the width the page was actually rendered at (the
		// session's viewport), falling back to the measured content width
		// for callers that built the Snapshot themselves.
		widthPx := float64(snap.ViewportWidth)
		if widthPx <= 0 {
			widthPx = cssContentSize.Width
		}
		heightPx := cssContentSize.Height + 4

		// Paper width is INDEPENDENT of layout width. By default the paper
		// is whatever the layout happens to measure (1600 css px → 16.67in),
		// which is why an unconfigured capture yields a very wide page. When
		// a paper width is requested, the layout is unchanged — no reflow,
		// no different breakpoint — and the rendered content is scaled to
		// fit the sheet instead.
		widthIn := widthPx / cssPxPerInch
		scale := 1.0
		if snap.PaperWidthIn > 0 {
			widthIn = snap.PaperWidthIn
			scale = (widthIn * cssPxPerInch) / widthPx
			if scale < minPrintScale || scale > maxPrintScale {
				clamped := min(max(scale, minPrintScale), maxPrintScale)
				log.With("scale", scale).With("clamped", clamped).
					With("paperWidthIn", widthIn).With("layoutWidthPx", widthPx).
					Warn("requested paper width needs a print scale outside Chrome's range; clamping (content will not fill the sheet)")
				scale = clamped
			}
		}

		params := page.PrintToPDFParams{
			PrintBackground:   new(true),
			PreferCSSPageSize: new(false),
			Scale:             scale,
			MarginTop:         new(0.0),
			MarginBottom:      new(0.0),
			MarginLeft:        new(0.0),
			MarginRight:       new(0.0),
			PaperWidth:        widthIn,
		}

		// Height, in three cases. The scale applies to height as well as
		// width, so it is folded in here — a scaled-down layout needs a
		// proportionally shorter sheet.
		contentIn := (heightPx * scale) / cssPxPerInch
		switch {
		case snap.PaperHeightIn > 0:
			// A real sheet was asked for (letter, a4, …): paginate onto it.
			params.PaperHeight = snap.PaperHeightIn
		case contentIn > 0 && contentIn <= maxPaperInches:
			// One tall page preserves the continuous layout — no
			// mid-content page breaks.
			params.PaperHeight = contentIn
		default:
			// Too long for one page. Chrome's own fallback is LETTER, which
			// turns a 25-foot page into dozens of stubs; paginate tall.
			log.With("contentIn", contentIn).With("paperHeightIn", tallPaperInches).
				Info("content exceeds the single-page limit; paginating at a tall page height")
			params.PaperHeight = tallPaperInches
		}

		printed, err := chromedp.Call(ctx, page.PrintToPDF, params)
		if err != nil {
			return errors.Wrap(err, "printToPDF failed")
		}
		data := printed.Data
		snap.MIME = "application/pdf"
		snap.Bytes = data
		return nil
	}
}
