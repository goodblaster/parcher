// Package capture serializes a prepared page into a self-contained snapshot.
//
// A Backend decides the output format (SingleFile HTML, MHTML, screenshot);
// Capture runs a preparation recipe and then the backend in one browser
// session.
package capture

import (
	"context"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/goodblaster/errors"
	"github.com/goodblaster/parchmint/internal/log"
	"github.com/goodblaster/parchmint/textlayer"
	"github.com/goodblaster/pscription/pipeline"
	"github.com/goodblaster/pscription/runner"
	js "github.com/goodblaster/pscription/scripts"
)

// maxCaptureAttempts bounds the retry loop for pages that come out of
// preparation in a detectably broken state (e.g. a carousel library that
// intermittently fails to initialize in headless, leaving slides stacked).
const maxCaptureAttempts = 3

// Snapshot is the result of capturing a page.
type Snapshot struct {
	URL   string
	Title string
	MIME  string
	Bytes []byte

	// ViewportWidth is the CSS-pixel layout width the page was rendered
	// at, from the session config. Backends that need the page's width
	// (PDF paper sizing) read it from here — never from a global.
	ViewportWidth int64

	// DevicePixelRatio is how many output pixels each CSS pixel became, from
	// the session config. Raster backends need it because encoder limits are
	// in DEVICE pixels while layout metrics are in CSS pixels. Zero means 1.
	DevicePixelRatio float64

	// PaperWidthIn and PaperHeightIn size the PDF sheet, in inches, and are
	// INDEPENDENT of ViewportWidth: the page keeps the layout it was
	// rendered at and the output is scaled onto the sheet, so asking for
	// letter paper does not reflow a desktop layout into a mobile one.
	// Zero width = derive the sheet from the layout width (the historical
	// behavior). Zero height = one tall page, or tall pagination when the
	// content will not fit.
	PaperWidthIn  float64
	PaperHeightIn float64

	// TextLayer is the composed parchmint-text/1 document, extracted from
	// the prepared page when Options.TextLayer is set; the SingleFile
	// backend embeds it. Nil when extraction was off or failed.
	TextLayer []byte

	// favicons are the page's icons, fetched and inlined as data URIs when
	// Options.Favicon is set. The HTML and MHT backends write them into the
	// serialized document (embedFaviconsInHTML); the live DOM is left
	// alone, since SingleFile drops icon links anyway.
	favicons []faviconEmbed
}

// Options control capture extras beyond the recipe and backend.
type Options struct {
	// TextLayer extracts the text layer (TEXTLAYER.md) from the prepared
	// page into Snapshot.TextLayer, for backends that can embed it.
	TextLayer bool

	// Favicon collects the page's icon(s) as data URIs for the HTML and
	// MHT backends to embed, so an archive keeps its identity in a tab
	// with no network. Icons are decoration: a slow or dead icon host is
	// given a short budget and then skipped, never allowed to hold up a
	// capture.
	Favicon bool

	// Highlight wraps every match of these phrases in
	// <mark data-parchmint> before serialization — same matcher as
	// `parch find` — so highlights appear in every backend's output.
	Highlight []string

	// Match selects how the Highlight phrases match (zero value = the
	// default phrase matcher).
	Match textlayer.MatchOptions

	// PaperWidthIn and PaperHeightIn size the PDF sheet (see Snapshot).
	// Ignored by every other backend.
	PaperWidthIn  float64
	PaperHeightIn float64
}

// Backend serializes the current page state into a snapshot.
type Backend interface {
	// Name identifies the backend (e.g. "singlefile", "mht").
	Name() string

	// Ext is the conventional file extension for this backend's output,
	// including the dot (e.g. ".html").
	Ext() string

	// Action fills snap from the current browser state. It runs after the
	// preparation recipe in the same session.
	Action(snap *Snapshot) chromedp.ActionFunc
}

// Capture loads url, runs the preparation recipe, and serializes the page
// with the given backend, all in one browser session.
func Capture(ctx context.Context, url string, recipe pipeline.Recipe, backend Backend) (*Snapshot, error) {
	return CaptureWithOptions(ctx, url, runner.DefaultConfig(), recipe, backend, Options{})
}

// CaptureWithConfig is Capture with a custom runner configuration.
func CaptureWithConfig(ctx context.Context, url string, cfg runner.Config, recipe pipeline.Recipe, backend Backend) (*Snapshot, error) {
	return CaptureWithOptions(ctx, url, cfg, recipe, backend, Options{})
}

// CaptureWithOptions is the full form. If a prepared page is detected to
// be in a broken layout state (a slider that failed to initialize), the
// whole capture is retried in a fresh session, since the failure is
// intermittent.
func CaptureWithOptions(ctx context.Context, url string, cfg runner.Config, recipe pipeline.Recipe, backend Backend, opts Options) (*Snapshot, error) {
	cfg.PreNavigate = append(cfg.PreNavigate, recipe.PreNavigateActions(url)...)

	// Retrying is only justified while the evidence keeps changing. The
	// defect this guards against is intermittent by nature, so a second
	// attempt that measures EXACTLY what the first one did is not going to
	// come out differently — it just costs another full pass. Bail on a
	// repeat rather than burning the whole attempt budget.
	var lastSig string
	for attempt := 1; ; attempt++ {
		snap, sig, defect, err := captureOnce(ctx, url, cfg, recipe, backend, opts, attempt < maxCaptureAttempts)
		if err != nil {
			return nil, errors.Wrapf(err, "%s capture failed", backend.Name())
		}
		if !defect {
			return snap, nil
		}
		if sig != "" && sig == lastSig {
			log.With("attempt", attempt).With("url", url).With("evidence", sig).
				Warn("layout defect unchanged since the last attempt; not transient, capturing as-is")
			snap, _, _, err = captureOnce(ctx, url, cfg, recipe, backend, opts, false)
			if err != nil {
				return nil, errors.Wrapf(err, "%s capture failed", backend.Name())
			}
			return snap, nil
		}
		lastSig = sig
		log.With("attempt", attempt).With("url", url).With("evidence", sig).
			Warn("layout defect detected (slider not initialized); retrying capture")
	}
}

// captureOnce runs one prepare+capture pass. When retryable is true it checks
// for a layout defect BEFORE serializing (so a doomed attempt doesn't waste
// backend work) and, if found, returns defect=true with no snapshot.
func captureOnce(ctx context.Context, url string, cfg runner.Config, recipe pipeline.Recipe, backend Backend, opts Options, retryable bool) (snap *Snapshot, signature string, defect bool, err error) {
	session, err := runner.Start(ctx, url, cfg)
	if err != nil {
		return nil, signature, false, err
	}
	defer session.Close()

	snap = &Snapshot{
		URL:              url,
		ViewportWidth:    cfg.ViewportWidth,
		DevicePixelRatio: cfg.DeviceScaleFactor,
		PaperWidthIn:     opts.PaperWidthIn,
		PaperHeightIn:    opts.PaperHeightIn,
	}
	if err := session.Run(recipe.Action(), captureTitle(&snap.Title)); err != nil {
		return nil, signature, false, err
	}

	if retryable {
		_ = session.Run(detectDefect(&defect, &signature))
		if defect {
			// Cheap in-session recovery before escalating to a full
			// re-capture: sliders reliably initialize given the time the
			// later prep steps take, but occasionally one is still pending
			// here — give it a few more seconds in THIS session and re-check.
			// A full retry (fresh navigation) only happens if that fails.
			log.With("url", url).Debug("slider not yet initialized; recovering in-session")
			_ = session.Run(js.WaitForSliders.Action(nil, 5000))
			_ = session.Run(detectDefect(&defect, &signature))
		}
		if defect {
			return nil, signature, true, nil
		}
	}

	if opts.Favicon {
		_ = session.Run(func(ctx context.Context) error {
			embeds, n, err := captureFavicon(ctx)
			snap.favicons = embeds
			if err != nil {
				log.WithError(err).Warn("favicon capture had an issue; will embed fetched icons if available")
				return nil
			}
			if n > 0 {
				log.With("icons", n).Debug("captured favicon")
			}
			return nil
		})
	}

	// Text layer and capture-time highlighting, on the prepared page
	// before serialization: the extraction is needed for either, the
	// highlight marks must be in the DOM for every backend to render, and
	// the embedded layer uses the pre-mark payload (measure first — marks
	// are inline and layout-neutral, but the layer stays authoritative).
	// Failures here degrade to a warning: a capture without the extras
	// beats no capture.
	if opts.TextLayer || len(opts.Highlight) > 0 {
		_ = session.Run(func(ctx context.Context) error {
			payload, err := extractPayload(ctx, len(opts.Highlight) > 0)
			if err != nil {
				log.WithError(err).Warn("text layer extraction failed; archiving without it")
				return nil
			}
			if len(opts.Highlight) > 0 {
				if _, err := applyHighlights(ctx, payload, opts.Highlight, opts.Match, nil, ""); err != nil {
					log.WithError(err).Warn("highlighting failed; archiving without marks")
				}
			}
			if opts.TextLayer {
				layer, err := composeLayer(snap, payload)
				if err != nil {
					log.WithError(err).Warn("text layer composition failed; archiving without it")
					return nil
				}
				snap.TextLayer = layer
			}
			return nil
		})
	}

	if err := session.Run(backend.Action(snap)); err != nil {
		return nil, signature, false, err
	}
	return snap, signature, false, nil
}

// detectDefect reports whether the prepared page is in a known-broken layout
// state, along with the SIGNATURE of the evidence behind that verdict — see
// the retry loop in Capture for why the evidence, not just the verdict,
// decides whether another attempt is worth paying for.
func detectDefect(defect *bool, signature *string) chromedp.ActionFunc {
	return func(ctx context.Context) error {
		var res struct {
			Defect    bool   `json:"defect"`
			Signature string `json:"signature"`
		}
		if err := js.DetectSliderDefect.Action(&res)(ctx); err != nil {
			return err
		}
		*defect = res.Defect
		*signature = res.Signature
		return nil
	}
}

func captureTitle(title *string) chromedp.ActionFunc {
	return func(ctx context.Context) error {
		return chromedp.Title(title).Do(ctx)
	}
}

// flattenFixedPositioning converts position:fixed elements to absolute.
// Chrome's print engine paints fixed elements on EVERY page, so a fixed nav
// bar becomes a header repeated on every PDF page. Absolute stops that repeat
// (only fixed repeats) while keeping the element out of normal flow — so,
// unlike converting to static, it adds no height and can't tip a borderline
// page into pagination. The element keeps its top/left placement and appears
// once. Only fixed is touched — sticky already prints once in flow. PDF-only:
// full-page screenshots aren't paginated, so fixed elements already appear
// once there.
func flattenFixedPositioning(ctx context.Context) error {
	return chromedp.Evaluate(
		`(() => {
			for (const el of document.querySelectorAll('body *')) {
				if (getComputedStyle(el).position === 'fixed') {
					el.style.setProperty('position', 'absolute', 'important');
				}
			}
		})()`,
		nil).Do(ctx)
}

// normalizeBackgroundAttachment forces background-attachment:scroll. Chrome's
// print pipeline (PDF) and full-page screenshots don't paint fixed (parallax)
// backgrounds beyond the first viewport, so such sections come out blank —
// and any white-on-dark text in them becomes invisible. Parallax is
// meaningless in a static capture anyway. Used by the "flat" backends (PDF,
// PNG, JPEG); the HTML/MHT backends render in a real browser and don't need
// it. Background-attachment doesn't affect layout, so this triggers no
// reflow.
func normalizeBackgroundAttachment(ctx context.Context) error {
	return chromedp.Evaluate(
		`(() => { const s = document.createElement('style');
			s.textContent = '*{background-attachment:scroll !important}';
			(document.head || document.documentElement).appendChild(s); })()`,
		nil).Do(ctx)
}

// captureFullImage takes a full-page screenshot in the given lossy format,
// clamping the captured height to the format's per-dimension encoder limit
// (WebP: 16383, JPEG: 65535). Chrome doesn't error past the limit — it
// silently returns ZERO bytes — so tall pages must be cut off at the
// bottom to produce anything at all.
//
// dpr is the device pixel ratio the page is rendering at. It is load-bearing:
// layout metrics are in CSS pixels but the encoder limit is in DEVICE pixels,
// so at dpr 2 a 9000-css-px page is an 18000-px image and blows the WebP
// limit even though the CSS height looks safe. Zero means 1.
func captureFullImage(ctx context.Context, format page.CaptureScreenshotFormat, quality, maxDim int64, dpr float64) ([]byte, error) {
	_, _, _, _, _, cssContentSize, err := page.GetLayoutMetrics().Do(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get layout metrics")
	}
	if cssContentSize == nil {
		return nil, errors.New("layout metrics returned no content size")
	}
	if dpr <= 0 {
		dpr = 1
	}
	// The budget the clip must respect, expressed back in CSS pixels.
	maxCSS := float64(maxDim) / dpr
	width, height := cssContentSize.Width, cssContentSize.Height
	if width > maxCSS {
		// Cropping horizontally would cut into every line of the page;
		// unlike height there is no sensible partial capture.
		return nil, errors.Newf("page width %.0f css px (%.0f device px at dpr %g) exceeds the %s encoder limit %d",
			width, width*dpr, dpr, format, maxDim)
	}
	if height > maxCSS {
		log.With("height", int64(height)).With("dpr", dpr).
			With("deviceHeight", int64(height*dpr)).With("limit", maxDim).
			With("format", string(format)).
			Warn("page taller than the encoder limit; capture is cut off at the bottom (use the png backend for full length, or a lower -dpr)")
		height = maxCSS
	}

	buf, err := page.CaptureScreenshot().
		WithCaptureBeyondViewport(true).
		WithFromSurface(true).
		WithFormat(format).
		WithQuality(quality).
		WithClip(&page.Viewport{X: 0, Y: 0, Width: width, Height: height, Scale: 1}).
		Do(ctx)
	if err != nil {
		return nil, err
	}
	if len(buf) == 0 {
		return nil, errors.Newf("Chrome returned an empty %s screenshot", string(format))
	}
	return buf, nil
}
