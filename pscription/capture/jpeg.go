package capture

import (
	"context"

	"github.com/chromedp/cdproto/page"
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

// jpegMaxDim is JPEG's per-dimension limit (16 bits).
const jpegMaxDim = 65535

func (JPEG) Action(snap *Snapshot) chromedp.ActionFunc {
	return func(ctx context.Context) error {
		// Fixed (parallax) backgrounds only paint in the first viewport of a
		// full-page screenshot, leaving lower sections blank.
		if err := normalizeBackgroundAttachment(ctx); err != nil {
			return err
		}
		buf, err := captureFullImage(ctx, page.CaptureScreenshotFormatJpeg, jpegQuality, jpegMaxDim)
		if err != nil {
			return err
		}
		snap.MIME = "image/jpeg"
		snap.Bytes = buf
		return nil
	}
}
