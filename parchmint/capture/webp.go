package capture

import (
	"context"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

// WebP captures the page as a full-page WebP image. WebP is typically smaller
// than both the PNG and JPEG screenshots at comparable quality, so it's the
// best default when a raster snapshot is wanted and file size matters. Like
// the JPEG backend it's lossy and has no transparency; use Screenshot (PNG)
// when lossless matters.
type WebP struct{}

func (WebP) Name() string { return "webp" }
func (WebP) Ext() string  { return ".webp" }

// webpQuality trades size against artifacts; 80 is a good "looks fine, clearly
// smaller than JPEG" point for WebP.
const webpQuality = 80

// webpMaxDim is WebP's hard per-dimension limit (14 bits); Chrome silently
// returns zero bytes for anything larger.
const webpMaxDim = 16383

func (WebP) Action(snap *Snapshot) chromedp.ActionFunc {
	return func(ctx context.Context) error {
		// Fixed (parallax) backgrounds only paint in the first viewport of a
		// full-page screenshot, leaving lower sections blank.
		if err := normalizeBackgroundAttachment(ctx); err != nil {
			return err
		}
		buf, err := captureFullImage(ctx, page.CaptureScreenshotFormatWebp, webpQuality, webpMaxDim)
		if err != nil {
			return err
		}
		snap.MIME = "image/webp"
		snap.Bytes = buf
		return nil
	}
}
