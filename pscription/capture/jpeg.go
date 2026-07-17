package capture

import (
	"context"

	"github.com/chromedp/chromedp"
)

// JPEG captures the page as a full-page JPEG. It's much smaller than the PNG
// screenshot on photographic pages, at some quality loss and with no
// transparency; use the Screenshot (PNG) backend when lossless matters.
type JPEG struct{}

func (JPEG) Name() string { return "jpeg" }
func (JPEG) Ext() string  { return ".jpg" }

// jpegQuality trades size against artifacts; 85 is the usual "looks fine,
// notably smaller" sweet spot.
const jpegQuality = 85

func (JPEG) Action(snap *Snapshot) chromedp.ActionFunc {
	return func(ctx context.Context) error {
		// Fixed (parallax) backgrounds only paint in the first viewport of a
		// full-page screenshot, leaving lower sections blank.
		if err := normalizeBackgroundAttachment(ctx); err != nil {
			return err
		}
		var buf []byte
		// chromedp.FullScreenshot emits JPEG for any quality other than 100.
		if err := chromedp.FullScreenshot(&buf, jpegQuality).Do(ctx); err != nil {
			return err
		}
		snap.MIME = "image/jpeg"
		snap.Bytes = buf
		return nil
	}
}
