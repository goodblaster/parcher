package capture

import (
	"context"

	"github.com/chromedp/chromedp"
	"github.com/goodblaster/pscription/actions"
)

// MHT captures the page as an MHTML document (Chrome's Page.captureSnapshot).
//
// The output is used verbatim: MHTML is a MIME container whose HTML part is
// quoted-printable encoded, and every conformant reader (i.e. a browser)
// decodes that on open. We deliberately do NOT post-process it — an earlier
// "repair" pass that rewrote soft line breaks inside data URIs was both
// unnecessary (browsers decode QP themselves) and pathologically slow
// (O(n²) regex rebuilds of a multi-megabyte string turned a 77ms capture
// into an 8-minute one). SingleFile HTML remains the preferred backend;
// this exists for callers who specifically want MHTML.
type MHT struct{}

func (MHT) Name() string { return "mht" }
func (MHT) Ext() string  { return ".mht" }

func (MHT) Action(snap *Snapshot) chromedp.ActionFunc {
	return func(ctx context.Context) error {
		// Chrome's MHTML snapshot omits CSS web fonts, so embed the ones
		// the page actually loaded as data: URIs first — otherwise MHT
		// archives lose icon fonts (tofu carets) and web-font text. (The
		// SingleFile backend needs no such help; it inlines fonts itself.)
		if err := actions.FetchAndInjectFonts()(ctx); err != nil {
			return err
		}
		var content string
		if err := actions.SaveMhtToString(&content)(ctx); err != nil {
			return err
		}
		snap.MIME = "multipart/related"
		snap.Bytes = []byte(content)
		return nil
	}
}
