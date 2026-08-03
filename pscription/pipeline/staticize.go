package pipeline

import (
	"context"
	"fmt"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/goodblaster/pscription/actions"
	js "github.com/goodblaster/pscription/scripts"
)

// Staticize is the default page-preparation recipe: it turns a live page
// into something a capture backend can serialize as a self-contained,
// static snapshot. The goal is to capture what a human would see after
// waiting, clearing dialogs, and scrolling.
//
// Ordering constraints baked into this recipe:
//   - hide-overlays runs first so popup chrome never flashes into view.
//   - dismiss-popups runs several times: age gates need a click, then time
//     for the site's reaction, then another sweep for late popups.
//   - capture-video-frame and rasterize-font-icons run BEFORE the embed
//     steps, because the <img> elements they create (posters, YouTube
//     thumbnails) still point at external URLs that need embedding.
//   - Image/background embedding uses server-side fetching, which bypasses
//     CORS and preserves carousel/late-load images.
//
// A quiet-window is a FLOOR on what a wait costs: an already-quiet page
// still sits through it to prove it stayed quiet. The first check after
// load earns the full window; by the later re-checks the page has already
// demonstrated it can go quiet, so they use a shorter one. (The final
// pre-capture wait was already 300ms on the same reasoning.)
const (
	recheckQuiet   = 250 * time.Millisecond
	recheckQuietMS = 250
)

func Staticize() Recipe {
	// Shared by this recipe's dialog steps and the settle that follows
	// them: the pause exists to let a dismissal's reaction (which may be a
	// full reload) begin, so with nothing dismissed there is nothing to
	// wait for.
	var dismissed actions.DismissTally

	// Image bytes are fetched server-side; starting them as soon as the
	// URLs are known lets the downloads run THROUGH the steps that follow
	// (scrolling, layout settling, freezing iframes) instead of after
	// them. Purely a cache — see actions.PrefetchCache.
	prefetch := actions.NewPrefetchCache()

	return Recipe{
		Name: "staticize",

		// Layer 0 of dialog clearing: seed consent/age-gate state before
		// the page loads, so most banners decide they were already
		// answered and never render.
		PreNavigate: []PreNavFunc{
			// Site CSPs without data: in img-src silently block every data
			// URI the prep injects — broken-image placeholders in the
			// live-rendering backends (pdf/png/jpeg/webp).
			func(url string) chromedp.Action { return actions.BypassCSP() },
			func(url string) chromedp.Action { return actions.PreseedConsentCookies(url) },
			func(url string) chromedp.Action { return actions.PreseedConsentStorage() },
			// WebGL drawing buffers must be preserved from the first frame
			// or rasterize-canvases reads transparent black at capture time.
			func(url string) chromedp.Action { return actions.ForceWebGLPreserve() },
		},

		Steps: []Step{
			// Overlay-hiding CSS goes in before anything renders on top.
			stepIgnoreError("hide-overlays", actions.RunScriptIgnoreError(js.HideOverlays, nil)),

			// Let the initial load settle: network first, then DOM quiet.
			step("wait-network-idle", actions.WaitForNetworkIdle(5*time.Second)),
			stepIgnoreError("wait-quiet", actions.RunScriptIgnoreError(js.WaitForQuiet, nil, 500, 3000)),

			// Dialog clearing, strongest first: exact selectors for known
			// consent platforms, then text heuristics. Age gates / cookie
			// walls often need a click, a pause for the site's reaction —
			// which may be a full page reload — and another sweep.
			// wait-ready absorbs any navigation the click triggered; a
			// reloaded document also loses the injected overlay CSS, so
			// hide-overlays runs again afterwards.
			stepIgnoreError("dismiss-cmp", actions.DismissCMPCounting(&dismissed)),
			stepIgnoreError("dismiss-popups", actions.DismissPopupsCounting(&dismissed)),
			SleepIf("settle", 1500*time.Millisecond, func() bool { return dismissed.Acted }),
			step("wait-ready", actions.WaitForReady(10*time.Second)),
			step("wait-network-idle", actions.WaitForNetworkIdleQuiet(5*time.Second, recheckQuiet)),
			stepIgnoreError("wait-quiet", actions.RunScriptIgnoreError(js.WaitForQuiet, nil, recheckQuietMS, 3000)),
			stepIgnoreError("hide-overlays", actions.RunScriptIgnoreError(js.HideOverlays, nil)),
			stepIgnoreError("dismiss-cmp", actions.RunScriptIgnoreError(js.DismissCMP, nil)),
			stepIgnoreError("dismiss-popups", actions.DismissPopups()),
			step("press-escape", pressEscape()),

			// Defeat lazy loading: rewrite deferred attributes first, then
			// scroll the viewport through the page for observer-based
			// loaders, then wait until images actually complete.
			stepIgnoreError("force-image-loading", actions.RunScriptIgnoreError(js.ForceImageLoading, nil)),
			stepIgnoreError("prefetch-images", actions.PrefetchImages(prefetch)),
			stepIgnoreError("scroll-and-wait-images", actions.RunScriptIgnoreError(js.ScrollAndWaitImages, nil)),
			step("wait-network-idle", actions.WaitForNetworkIdleQuiet(5*time.Second, recheckQuiet)),
			stepIgnoreError("force-image-loading", actions.RunScriptIgnoreError(js.ForceImageLoading, nil)),
			step("wait-images", actions.WaitForImages(10*time.Second)),
			stepIgnoreError("prefetch-images", actions.PrefetchImages(prefetch)),

			// Let carousels/sliders/tab panels finish initializing before
			// we freeze media and capture: until their JS runs, their slides
			// render stacked (tripling page height) with a blank void where
			// the active slide belongs. Wait for page height to settle.
			stepIgnoreError("wait-stable-layout", actions.RunScriptIgnoreError(js.WaitForStableLayout, nil, 700, 8000)),

			// Anything that popped up during scrolling.
			stepIgnoreError("dismiss-popups", actions.DismissPopups()),
			step("press-escape", pressEscape()),

			// Freeze media first (creates <img> elements with external
			// URLs), then embed everything as data URIs. Canvas replacement
			// produces data URIs directly, so its position relative to the
			// embed steps doesn't matter — it lives here with the rest of
			// the media freezing.
			// warm-canvases first: rAF is throttled outside the viewport, so
			// an animated canvas may not have painted a single frame yet; a
			// canvas that never painted rasterizes and screenshots as an
			// empty box. freeze-iframes then stills cross-origin iframes,
			// whose pixels only exist while they are composited in-viewport
			// (offscreen OOPIFs are blank even in captureBeyondViewport).
			// remove-junk-iframes before freeze-iframes: trackers must not
			// eat the freeze step's screenshot budget, and every frame that
			// survives to serialization costs a downstream backend a full
			// recursive pass (see actions.RemoveJunkIframes).
			stepIgnoreError("warm-canvases", actions.WarmCanvases()),
			stepIgnoreError("remove-junk-iframes", actions.RemoveJunkIframes()),
			stepIgnoreError("freeze-iframes", actions.FreezeOpaqueIframes()),
			step("rasterize-canvases", actions.RasterizeCanvases()),
			step("capture-video-frame", actions.CaptureVideoFrame()),
			step("rasterize-font-icons", actions.RasterizeFontIcons()),
			step("fetch-images", actions.FetchAndInjectImagesCached(prefetch)),
			step("fetch-backgrounds", actions.FetchAndInjectBackgrounds()),
			// Note: web-font embedding is NOT here. Chrome's snapshot omits
			// fonts, so the backends built on it (HTML, MHT) embed them
			// themselves, at a point where they know they need them; a
			// serializer that inlines fonts on its own would double-embed
			// them if shared prep did it too.
			stepIgnoreError("wait-quiet", actions.RunScriptIgnoreError(js.WaitForQuiet, nil, recheckQuietMS, 2000)),

			// Late-loading popups, then a final sweep before capture:
			// heuristic dismissal, then geometry-based demolition of
			// anything still covering the page. A short quiet-wait lets the
			// last DOM edits (overlay removal) paint; it returns as soon as
			// things settle instead of always burning a fixed pause.
			step("wait-network-idle", actions.WaitForNetworkIdleQuiet(10*time.Second, recheckQuiet)),
			stepIgnoreError("dismiss-cmp", actions.RunScriptIgnoreError(js.DismissCMP, nil)),
			stepIgnoreError("dismiss-popups", actions.DismissPopups()),
			stepIgnoreError("remove-blocking-overlays", actions.RunScriptIgnoreError(js.RemoveBlockingOverlays, nil)),
			stepIgnoreError("wait-quiet", actions.RunScriptIgnoreError(js.WaitForQuiet, nil, 300, 1000)),

			// A cross-origin stylesheet renders but cannot be READ, so the
			// CSSOM-inspecting steps below (and the backends' font
			// extraction) are blind to it. Inlining its text makes it
			// readable. Before the state-materializing steps, since it adds
			// elements; after the layout work, since it changes no layout.
			stepIgnoreError("inline-crossorigin-css", actions.InlineCrossOriginStylesheets()),

			// Last: make runtime-only state visible to a serializer that
			// walks markup. These run at the very end because every step
			// above can still change the DOM and what gets written back has
			// to be the FINAL state. Both are layout-neutral (an attribute
			// the page already reflects, a stylesheet already in effect), so
			// nothing needs to settle afterwards.
			stepIgnoreError("materialize-form-state", actions.RunScriptIgnoreError(js.MaterializeFormState, nil)),
			stepIgnoreError("flatten-adopted-stylesheets", actions.RunScriptIgnoreError(js.FlattenAdoptedStylesheets, nil)),
			// After flatten-adopted-stylesheets, which deliberately leaves
			// shadow roots alone: this one owns them, and would otherwise
			// pick up its own output.
			stepIgnoreError("link-shadow-styles", actions.RunScriptIgnoreError(js.LinkShadowStyles, nil)),
		},
	}
}

// RewriteLinksStep applies a link policy ("new-tab" or "disable") as a
// final prep step. The default policy — links left exactly as the page had
// them — needs no step at all; append this only when asked:
//
//	recipe := pipeline.Staticize().Append(pipeline.RewriteLinksStep("new-tab"))
func RewriteLinksStep(mode string) Step {
	return stepIgnoreError("rewrite-links", actions.RunScriptIgnoreError(js.RewriteLinks, nil, mode))
}

func step(name string, action chromedp.ActionFunc) Step {
	return Step{Name: name, Action: action}
}

func stepIgnoreError(name string, action chromedp.ActionFunc) Step {
	return Step{Name: name, Action: action, ContinueOnError: true}
}

func pressEscape() chromedp.ActionFunc {
	return func(ctx context.Context) error {
		return chromedp.KeyEvent(fmt.Sprintf("%c", rune(27))).Do(ctx)
	}
}

// The primitive steps register under their names with default parameters,
// so a future data-defined recipe can reference them directly.
func init() {
	Register("hide-overlays", func() Step {
		return stepIgnoreError("hide-overlays", actions.RunScriptIgnoreError(js.HideOverlays, nil))
	})
	Register("wait-network-idle", func() Step { return step("wait-network-idle", actions.WaitForNetworkIdle(5*time.Second)) })
	Register("wait-ready", func() Step { return step("wait-ready", actions.WaitForReady(10*time.Second)) })
	Register("wait-quiet", func() Step {
		return stepIgnoreError("wait-quiet", actions.RunScriptIgnoreError(js.WaitForQuiet, nil, 500, 3000))
	})
	Register("dismiss-cmp", func() Step {
		return stepIgnoreError("dismiss-cmp", actions.RunScriptIgnoreError(js.DismissCMP, nil))
	})
	Register("dismiss-popups", func() Step { return stepIgnoreError("dismiss-popups", actions.DismissPopups()) })
	Register("remove-blocking-overlays", func() Step {
		return stepIgnoreError("remove-blocking-overlays", actions.RunScriptIgnoreError(js.RemoveBlockingOverlays, nil))
	})
	Register("press-escape", func() Step { return step("press-escape", pressEscape()) })
	Register("force-image-loading", func() Step {
		return stepIgnoreError("force-image-loading", actions.RunScriptIgnoreError(js.ForceImageLoading, nil))
	})
	Register("scroll-and-wait-images", func() Step {
		return stepIgnoreError("scroll-and-wait-images", actions.RunScriptIgnoreError(js.ScrollAndWaitImages, nil))
	})
	Register("wait-images", func() Step { return step("wait-images", actions.WaitForImages(10*time.Second)) })
	Register("wait-stable-layout", func() Step {
		return stepIgnoreError("wait-stable-layout", actions.RunScriptIgnoreError(js.WaitForStableLayout, nil, 700, 8000))
	})
	Register("wait-sliders", func() Step {
		return stepIgnoreError("wait-sliders", actions.RunScriptIgnoreError(js.WaitForSliders, nil, 6000))
	})
	Register("fetch-images", func() Step { return step("fetch-images", actions.FetchAndInjectImages()) })
	Register("fetch-backgrounds", func() Step { return step("fetch-backgrounds", actions.FetchAndInjectBackgrounds()) })
	Register("fetch-fonts", func() Step { return step("fetch-fonts", actions.FetchAndInjectFonts()) })
	Register("rasterize-font-icons", func() Step { return step("rasterize-font-icons", actions.RasterizeFontIcons()) })
	Register("capture-video-frame", func() Step { return step("capture-video-frame", actions.CaptureVideoFrame()) })
	Register("warm-canvases", func() Step { return stepIgnoreError("warm-canvases", actions.WarmCanvases()) })
	Register("remove-junk-iframes", func() Step {
		return stepIgnoreError("remove-junk-iframes", actions.RemoveJunkIframes())
	})
	Register("freeze-iframes", func() Step { return stepIgnoreError("freeze-iframes", actions.FreezeOpaqueIframes()) })
	Register("rasterize-canvases", func() Step { return step("rasterize-canvases", actions.RasterizeCanvases()) })
	Register("inline-crossorigin-css", func() Step {
		return stepIgnoreError("inline-crossorigin-css", actions.InlineCrossOriginStylesheets())
	})
	Register("materialize-form-state", func() Step {
		return stepIgnoreError("materialize-form-state", actions.RunScriptIgnoreError(js.MaterializeFormState, nil))
	})
	Register("flatten-adopted-stylesheets", func() Step {
		return stepIgnoreError("flatten-adopted-stylesheets", actions.RunScriptIgnoreError(js.FlattenAdoptedStylesheets, nil))
	})
	Register("link-shadow-styles", func() Step {
		return stepIgnoreError("link-shadow-styles", actions.RunScriptIgnoreError(js.LinkShadowStyles, nil))
	})
	Register("rewrite-links-new-tab", func() Step { return RewriteLinksStep("new-tab") })
	Register("rewrite-links-disable", func() Step { return RewriteLinksStep("disable") })
}
