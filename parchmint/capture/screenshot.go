package capture

import (
	"context"

	"github.com/chromedp/chromedp"
	"github.com/goodblaster/parcher/pscription/act"
)

// Screenshot captures the page as a full-page PNG.
type Screenshot struct{}

func init() {
	Register("png", func() Backend { return Screenshot{} })
}

func (Screenshot) Name() string { return "screenshot" }
func (Screenshot) Ext() string  { return ".png" }

func (Screenshot) Action(snap *Snapshot) act.Func {
	return func(ctx context.Context) error {
		// Fixed (parallax) backgrounds only paint in the first viewport of a
		// full-page screenshot, leaving lower sections blank.
		if err := normalizeBackgroundAttachment(ctx); err != nil {
			return err
		}
		var buf []byte
		buf, err := chromedp.Run(ctx, chromedp.FullScreenshot(100))
		if err != nil {
			return err
		}
		snap.MIME = "image/png"
		snap.Bytes = buf
		return nil
	}
}
