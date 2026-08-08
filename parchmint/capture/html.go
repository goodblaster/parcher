package capture

import (
	"context"
	"strings"

	"github.com/chromedp/chromedp"
	"github.com/goodblaster/errors"
	"github.com/goodblaster/go-mht/pkg/documents"
	"github.com/goodblaster/parchmint/internal/convert/render"
	"github.com/goodblaster/parchmint/internal/log"
	"github.com/goodblaster/pscription/actions"
)

func init() {
	Register("html", func() Backend { return HTML{} })
}

// HTML captures the page as a self-contained HTML document, using Chrome's
// own DOM serializer plus this repo's inliner. It is the default backend and
// the only one that produces a single HTML file; all of its code is original
// or Chrome's, so parch carries no third-party license obligation.
//
// The two halves:
//
//  1. Page.captureSnapshot — Chrome writes the CURRENT DOM and every
//     subresource it holds into an MHTML container. The resource graph comes
//     out of the browser's own cache, so nothing is re-fetched, nothing is
//     subject to CORS, and no reference has to be re-resolved by hand.
//  2. internal/convert/render — the MIME parts are inlined into one document
//     (stylesheets as <style> so their url()s can be rewritten in turn,
//     everything else as data: URIs).
//
// Measured on test-site-1, step 2 costs ~0.1s against an 8.4 MB snapshot, and
// the result was pixel-equivalent to the vendored SingleFile backend this
// replaced (0.06% of pixels, one region, across two independent live loads).
//
// Two properties of Chrome's serializer are load-bearing here. It emits NO
// <script> elements at all, so a snapshot cannot re-run the site's JS when it
// is opened — there is none to run, and no option has to be passed to make
// that true. And it walks shadow roots including CLOSED ones, which
// page-script serializers structurally cannot reach;
// render.reviveShadowRoot converts Blink's `shadowmode` spelling to the
// standard one so they actually attach on load.
//
// What Chrome does NOT record is runtime-only state that lives outside the
// markup: typed form values, and constructed stylesheets. Both are written
// back into the DOM by prep (pscription's materialize-form-state and
// flatten-adopted-stylesheets), which is where they belong — every backend
// benefits, and this one stays a serializer rather than a fidelity engine.
type HTML struct{}

func (HTML) Name() string  { return "html" }
func (HTML) Ext() string   { return ".html" }
func (HTML) CarriesLayer() {}

func (HTML) Action(snap *Snapshot) chromedp.ActionFunc {
	return func(ctx context.Context) error {
		// Chrome's snapshot omits CSS web fonts, so the ones the page
		// actually loaded are embedded as data: URIs first — otherwise the
		// archive loses icon fonts (tofu carets) and web-font text. Same
		// reason and same FromArchive guard as the MHT backend: a converted
		// archive already has its fonts inlined, and the extraction cannot
		// run there because RenderArchive disables page scripts.
		if !snap.FromArchive {
			if err := actions.FetchAndInjectFonts()(ctx); err != nil {
				return err
			}
		}

		var content string
		if err := actions.SaveMhtToString(&content)(ctx); err != nil {
			return err
		}

		doc, err := documents.Parse(strings.NewReader(content))
		if err != nil {
			return errors.Wrap(err, "failed to parse the page snapshot")
		}

		// A reference the snapshot does not carry is left as it was, which
		// keeps a partial capture usable; it is worth knowing about but never
		// worth failing over, so it is a debug line and a count rather than a
		// warning per reference (a big page can produce many).
		var unresolved int
		out, err := render.Document(doc, render.Options{
			// Chrome emits no <script> elements today. Asking for them to be
			// omitted anyway costs nothing and keeps the guarantee ours
			// rather than a property of whichever Chrome is installed.
			OmitScripts: true,
			OnWarn: func(message string) {
				unresolved++
				if unresolved == 1 {
					log.With("detail", message).Debug("snapshot reference could not be inlined")
				}
			},
		})
		if err != nil {
			return errors.Wrap(err, "failed to inline the page snapshot")
		}
		if unresolved > 0 {
			log.With("references", unresolved).Debug("some references were left as-is")
		}

		snap.MIME = "text/html"
		snap.Bytes = embedFaviconsInHTML(out, snap.favicons)
		if snap.TextLayer != nil {
			snap.Bytes = spliceTextLayer(snap.Bytes, snap.TextLayer)
		}
		return nil
	}
}
