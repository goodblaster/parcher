package capture

import (
	"context"

	"github.com/chromedp/chromedp"
)

// Screenshot captures the page as a full-page PNG.
type Screenshot struct{}

func (Screenshot) Name() string { return "screenshot" }
func (Screenshot) Ext() string  { return ".png" }

func (Screenshot) Action(snap *Snapshot) chromedp.ActionFunc {
	return func(ctx context.Context) error {
		var buf []byte
		if err := chromedp.FullScreenshot(&buf, 100).Do(ctx); err != nil {
			return err
		}
		snap.MIME = "image/png"
		snap.Bytes = buf
		return nil
	}
}
