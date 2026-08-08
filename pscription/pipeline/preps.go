package pipeline

import (
	"time"

	"github.com/chromedp/chromedp"
	"github.com/goodblaster/parcher/pscription/actions"
	js "github.com/goodblaster/parcher/pscription/scripts"
)

// Prep recipes are the reusable "get the page ready" phases a pscription
// runs before its own steps. They range from nothing to the full
// self-contained-snapshot preparation.

// None does no preparation — the page is used as it loaded. Fastest; fine
// for simple/static pages.
func None() Recipe {
	return Recipe{Name: "none"}
}

// Minimal readies a page for content extraction: clears dialogs, defeats
// lazy loading, waits for images — but does NOT embed resources or freeze
// media. This is the right default for scraping/extraction (pscribe), where you
// want the rendered content, not a self-contained artifact.
func Minimal() Recipe {
	// See Staticize: the monitor is what keeps a quiet window from being a
	// floor on every wait's cost.
	settle := actions.NewSettleMonitor()

	return Recipe{
		Name: "minimal",
		PreNavigate: []PreNavFunc{
			func(url string) chromedp.Action { return settle.Attach() },
			func(url string) chromedp.Action { return actions.PreseedConsentCookies(url) },
			func(url string) chromedp.Action { return actions.PreseedConsentStorage() },
		},
		Steps: []Step{
			stepIgnoreError("hide-overlays", actions.RunScriptIgnoreError(js.HideOverlays, nil)),
			step("wait-network-idle", settle.WaitForNetworkIdle(5*time.Second)),
			stepIgnoreError("wait-quiet", actions.RunScriptIgnoreError(js.WaitForQuiet, nil, 500, 3000)),
			stepIgnoreError("dismiss-cmp", actions.RunScriptIgnoreError(js.DismissCMP, nil)),
			stepIgnoreError("dismiss-popups", actions.DismissPopups()),
			stepIgnoreError("force-image-loading", actions.RunScriptIgnoreError(js.ForceImageLoading, nil)),
			stepIgnoreError("scroll-and-wait-images", actions.RunScriptIgnoreError(js.ScrollAndWaitImages, nil)),
			step("wait-images", actions.WaitForImages(8*time.Second)),
		},
	}
}

// PrepByName returns a prep recipe by name: "none", "minimal", or
// "staticize" (the full self-contained-snapshot preparation).
func PrepByName(name string) (Recipe, bool) {
	switch name {
	case "none", "":
		return None(), true
	case "minimal":
		return Minimal(), true
	case "staticize":
		return Staticize(), true
	default:
		return Recipe{}, false
	}
}
