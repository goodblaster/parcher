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
func Staticize() Recipe {
	return Recipe{
		Name: "staticize",

		// Layer 0 of dialog clearing: seed consent/age-gate state before
		// the page loads, so most banners decide they were already
		// answered and never render.
		PreNavigate: []PreNavFunc{
			func(url string) chromedp.Action { return actions.PreseedConsentCookies(url) },
			func(url string) chromedp.Action { return actions.PreseedConsentStorage() },
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
			stepIgnoreError("dismiss-cmp", actions.RunScriptIgnoreError(js.DismissCMP, nil)),
			stepIgnoreError("dismiss-popups", actions.DismissPopups()),
			Sleep("settle", 1500*time.Millisecond),
			step("wait-ready", actions.WaitForReady(10*time.Second)),
			step("wait-network-idle", actions.WaitForNetworkIdle(5*time.Second)),
			stepIgnoreError("wait-quiet", actions.RunScriptIgnoreError(js.WaitForQuiet, nil, 500, 3000)),
			stepIgnoreError("hide-overlays", actions.RunScriptIgnoreError(js.HideOverlays, nil)),
			stepIgnoreError("dismiss-cmp", actions.RunScriptIgnoreError(js.DismissCMP, nil)),
			stepIgnoreError("dismiss-popups", actions.DismissPopups()),
			step("press-escape", pressEscape()),

			// Defeat lazy loading: rewrite deferred attributes first, then
			// scroll the viewport through the page for observer-based
			// loaders, then wait until images actually complete.
			stepIgnoreError("force-image-loading", actions.RunScriptIgnoreError(js.ForceImageLoading, nil)),
			stepIgnoreError("scroll-and-wait-images", actions.RunScriptIgnoreError(js.ScrollAndWaitImages, nil)),
			step("wait-network-idle", actions.WaitForNetworkIdle(5*time.Second)),
			stepIgnoreError("force-image-loading", actions.RunScriptIgnoreError(js.ForceImageLoading, nil)),
			step("wait-images", actions.WaitForImages(10*time.Second)),

			// Anything that popped up during scrolling.
			stepIgnoreError("dismiss-popups", actions.DismissPopups()),
			step("press-escape", pressEscape()),

			// Freeze media first (creates <img> elements with external
			// URLs), then embed everything as data URIs.
			step("capture-video-frame", actions.CaptureVideoFrame()),
			step("rasterize-font-icons", actions.RasterizeFontIcons()),
			step("fetch-images", actions.FetchAndInjectImages()),
			step("fetch-backgrounds", actions.FetchAndInjectBackgrounds()),
			stepIgnoreError("wait-quiet", actions.RunScriptIgnoreError(js.WaitForQuiet, nil, 500, 2000)),

			// Late-loading popups, then a final sweep before capture:
			// heuristic dismissal, then geometry-based demolition of
			// anything still covering the page.
			step("wait-network-idle", actions.WaitForNetworkIdle(10*time.Second)),
			stepIgnoreError("dismiss-cmp", actions.RunScriptIgnoreError(js.DismissCMP, nil)),
			stepIgnoreError("dismiss-popups", actions.DismissPopups()),
			stepIgnoreError("remove-blocking-overlays", actions.RunScriptIgnoreError(js.RemoveBlockingOverlays, nil)),
			Sleep("settle", 500*time.Millisecond),
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
	Register("fetch-images", func() Step { return step("fetch-images", actions.FetchAndInjectImages()) })
	Register("fetch-backgrounds", func() Step { return step("fetch-backgrounds", actions.FetchAndInjectBackgrounds()) })
	Register("rasterize-font-icons", func() Step { return step("rasterize-font-icons", actions.RasterizeFontIcons()) })
	Register("capture-video-frame", func() Step { return step("capture-video-frame", actions.CaptureVideoFrame()) })
	Register("rewrite-links-new-tab", func() Step { return RewriteLinksStep("new-tab") })
	Register("rewrite-links-disable", func() Step { return RewriteLinksStep("disable") })
}
