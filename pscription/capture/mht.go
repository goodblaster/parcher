package capture

import (
	"context"
	"regexp"

	"github.com/chromedp/chromedp"
	"github.com/goodblaster/webrun/actions"
)

// MHT captures the page as an MHTML document (Chrome's Page.captureSnapshot).
//
// Note: MHTML's quoted-printable encoding inserts soft line breaks that can
// corrupt long data URIs. The backend repairs the known breakage patterns,
// but SingleFile HTML avoids the problem entirely and is the preferred
// default.
type MHT struct{}

func (MHT) Name() string { return "mht" }
func (MHT) Ext() string  { return ".mht" }

func (MHT) Action(snap *Snapshot) chromedp.ActionFunc {
	return func(ctx context.Context) error {
		var content string
		if err := actions.SaveMhtToString(&content)(ctx); err != nil {
			return err
		}
		snap.MIME = "multipart/related"
		snap.Bytes = []byte(repairQuotedPrintableDataURIs(content))
		return nil
	}
}

var (
	// Soft line breaks (=\n) inside a data URI: data:image/png;base64,ABC=\nDEF
	qpSoftBreak = regexp.MustCompile(`(data:[^"'\s)]+)=\s*\n([^"'\s)]+)`)
	// Broken "data:" prefix, e.g. "dat=\na:" or "data=\n:"
	qpDataPrefix = regexp.MustCompile(`(?:dat|data)=\s*\n([a:])(image|application|text|video|audio|font)`)
	// Broken MIME type, e.g. "ima=\nge/png"
	qpMimeType = regexp.MustCompile(`data:([a-z]+)=\s*\n([a-z]+)(/[a-z0-9+.-]+)`)
)

// repairQuotedPrintableDataURIs fixes data URIs broken by MHTML's
// quoted-printable soft line breaks.
func repairQuotedPrintableDataURIs(content string) string {
	for qpSoftBreak.MatchString(content) {
		content = qpSoftBreak.ReplaceAllString(content, `$1$2`)
	}
	content = qpDataPrefix.ReplaceAllString(content, `data:$2`)
	content = qpMimeType.ReplaceAllString(content, `data:$1$2$3`)
	return content
}
