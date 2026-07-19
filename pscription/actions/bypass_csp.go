package actions

import (
	"context"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/goodblaster/errors"
)

// BypassCSP disables the page's Content-Security-Policy enforcement for
// the session. Pre-navigation action.
//
// The prep pipeline rewrites the page with data: URIs (embedded images and
// backgrounds, canvas and iframe stills). A site CSP whose img-src (or
// default-src) omits data: silently blocks every injected image: the <img>
// reports complete=true but naturalWidth=0 and renders as a broken-image
// placeholder. That breaks the live-rendering backends (pdf, png, jpeg,
// webp); the DOM backends escape only because the saved file carries no
// CSP header, so the same data URIs decode fine on reopen. test-site-4
// (default-src 'self') is the reference case.
func BypassCSP() chromedp.ActionFunc {
	return func(ctx context.Context) error {
		if err := page.SetBypassCSP(true).Do(ctx); err != nil {
			return errors.Wrap(err, "failed to bypass CSP")
		}
		return nil
	}
}
