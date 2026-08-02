package actions

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	"image/png"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/goodblaster/errors"
	"github.com/goodblaster/pscription/internal/log"
	js "github.com/goodblaster/pscription/scripts"
)

// Dwell budget for a cross-origin iframe brought into the viewport: time
// for its rAF loops to resume and paint after being throttled offscreen.
// The wait is adaptive — most frames (ads, maps, static embeds) finish
// painting well inside the budget, and a page with a dozen frames paid
// the full worst case for every one of them, sequentially.
//
//   - nothing is captured before opaqueFrameMinDwell, so a frame that has
//     not started painting is never mistaken for a finished one;
//   - after that the crop is sampled every opaqueFramePoll, and accepted
//     as soon as two consecutive samples are identical AND not a uniform
//     blank (an unpainted frame is stable-blank, which would otherwise
//     look "settled" and bake an empty box into the archive);
//   - a frame that never settles (animation, video, spinner) still gets
//     the full opaqueFrameDwell, so this is never worse than before.
const (
	opaqueFrameDwell    = 1500 * time.Millisecond
	opaqueFrameMinDwell = 400 * time.Millisecond
	opaqueFramePoll     = 250 * time.Millisecond
)

// junkFrameMinSize is the smallest edge, in CSS pixels, a frame can have
// and still be considered content. Ad-sync and analytics frames are 0x0
// or 1x1; the smallest genuine embeds (compact video players, map insets)
// are comfortably larger. Deliberately well below freeze-iframes'
// minSize of 80: this step DELETES, so its bar for "not content" is
// higher than that step's bar for "worth screenshotting".
const junkFrameMinSize = 32

// junkFrameHosts serve iframes that are never page content: ad exchanges,
// tag managers, analytics and identity-sync endpoints. They are removed
// regardless of size, because some (reCAPTCHA anchors, ad slots) do
// occupy real layout space while being inert in a static archive.
//
// Suffix-matched in the script, so an entry covers its subdomains.
var junkFrameHosts = []string{
	"adnxs.com",
	"adsrvr.org",
	"amazon-adsystem.com",
	"bat.bing.com",
	"bidswitch.net",
	"casalemedia.com",
	"criteo.com",
	"criteo.net",
	"doubleclick.net",
	"google-analytics.com",
	"googlesyndication.com",
	"googletagmanager.com",
	"googletagservices.com",
	"id5-sync.com",
	"imasdk.googleapis.com",
	"onetag-sys.com",
	"openx.net",
	"pubmatic.com",
	"quantserve.com",
	"recaptcha.net",
	"rubiconproject.com",
	"scorecardresearch.com",
	"taboola.com",
	"www.google.com/recaptcha",
	"yieldmo.com",
}

// RemoveJunkIframes deletes tracker, ad and zero-area iframes before the
// serialization backends see them. It is a performance step: a serializer
// that walks frames recurses into every survivor with its own resource
// fetching, and on ad-heavy pages that is where nearly all of an HTML
// capture's time goes (5.0s of 5.4s on test-site-6) for a
// content contribution measured in kilobytes.
//
// Runs before FreezeOpaqueIframes so that step's screenshot budget goes
// to real embeds instead of trackers.
func RemoveJunkIframes() chromedp.ActionFunc {
	return func(ctx context.Context) error {
		var stats struct {
			Removed int `json:"removed"`
			Hidden  int `json:"hidden"`
			Tiny    int `json:"tiny"`
			Tracker int `json:"tracker"`
			Kept    int `json:"kept"`
		}
		if err := js.RemoveJunkIframes.Action(&stats, junkFrameMinSize, junkFrameHosts).Do(ctx); err != nil {
			return errors.Wrap(err, "failed to remove junk iframes")
		}
		log.With("removed", stats.Removed).
			With("hidden", stats.Hidden).
			With("tiny", stats.Tiny).
			With("tracker", stats.Tracker).
			With("kept", stats.Kept).
			Debug("removed junk iframes")
		return nil
	}
}

// restoreOccluders un-hides the fixed/sticky elements the LAST
// focus_opaque_iframe.js call hid for its screenshot; the record format
// matches what that script stores in window.__gbFrameShotHidden.
const restoreOccluders = `(function () {
	for (const it of (window.__gbFrameShotHidden || [])) {
		try {
			if (it.prev) it.el.style.setProperty('visibility', it.prev, it.prio);
			else it.el.style.removeProperty('visibility');
		} catch (e) { /* element may be gone */ }
	}
	window.__gbFrameShotHidden = [];
})()`

// FreezeOpaqueIframes replaces cross-origin/sandboxed iframes with <img>
// stills of their current pixels. Their documents are unreachable from
// page JS, unserializable by the DOM backends, and — crucially — not
// rasterized by full-page screenshots when they sit outside the live
// viewport (captureBeyondViewport included). So each frame is scrolled
// into view, given a moment to paint, and screenshotted while it is
// actually composited; the still then survives in every backend.
func FreezeOpaqueIframes() chromedp.ActionFunc {
	return func(ctx context.Context) error {
		var stats struct {
			Marked  []int `json:"marked"`
			Skipped int   `json:"skipped"`
			Tall    int   `json:"tall"`
		}
		if err := js.LocateOpaqueIframes.Action(&stats).Do(ctx); err != nil {
			return errors.Wrap(err, "failed to locate opaque iframes")
		}

		frozen := 0
		for _, id := range stats.Marked {
			if err := freezeOneIframe(ctx, id); err != nil {
				log.With("iframe", id).WithError(err).Warn("failed to freeze iframe")
				continue
			}
			frozen++
		}
		if len(stats.Marked) > 0 {
			// Restore anything the last focus call hid for its shot, and
			// leave the page where the other steps expect it.
			_ = chromedp.Evaluate(restoreOccluders, nil).Do(ctx)
			_ = chromedp.Evaluate(`window.scrollTo(0, 0)`, nil).Do(ctx)
		}

		log.With("frozen", frozen).
			With("marked", len(stats.Marked)).
			With("skipped", stats.Skipped).
			With("tall", stats.Tall).
			Debug("froze opaque iframes")
		return nil
	}
}

// freezeOneIframe scrolls one marked iframe into view, waits for it to
// paint, screenshots the viewport, crops the frame's box, and swaps the
// iframe for the resulting image.
func freezeOneIframe(ctx context.Context, id int) error {
	sctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	var box struct {
		Found bool    `json:"found"`
		X     float64 `json:"x"`
		Y     float64 `json:"y"`
		W     float64 `json:"w"`
		H     float64 `json:"h"`
	}
	if err := js.FocusOpaqueIframe.Action(&box, id).Do(sctx); err != nil {
		return errors.Wrap(err, "focus")
	}
	if !box.Found {
		return errors.New("marked iframe disappeared")
	}

	cropped, err := dwellUntilPainted(sctx, box.X, box.Y, box.W, box.H)
	if err != nil {
		return err
	}

	dataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(cropped)
	var res struct {
		Replaced bool `json:"replaced"`
	}
	if err := js.ReplaceIframeWithImage.Action(&res, id, dataURL).Do(sctx); err != nil {
		return errors.Wrap(err, "inject still")
	}
	if !res.Replaced {
		return errors.New("marked iframe disappeared before replacement")
	}
	return nil
}

// dwellUntilPainted holds the frame in the viewport until its pixels stop
// changing, then returns the cropped still. See the dwell constants for
// why it is not a flat sleep.
func dwellUntilPainted(ctx context.Context, x, y, w, h float64) ([]byte, error) {
	deadline := time.Now().Add(opaqueFrameDwell)
	if err := chromedp.Sleep(opaqueFrameMinDwell).Do(ctx); err != nil {
		return nil, err
	}

	var prev, cur []byte
	for {
		shot, err := page.CaptureScreenshot().WithFromSurface(true).Do(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "viewport screenshot")
		}
		cur, err = cropPNG(shot, x, y, w, h)
		if err != nil {
			return nil, errors.Wrap(err, "crop")
		}
		if prev != nil && bytes.Equal(prev, cur) && !isUniform(cur) {
			return cur, nil
		}
		if !time.Now().Before(deadline) {
			return cur, nil
		}
		prev = cur
		if err := chromedp.Sleep(opaqueFramePoll).Do(ctx); err != nil {
			return nil, err
		}
	}
}

// isUniform reports whether every pixel is the same colour — the
// signature of a frame that has not painted yet (or a blank placeholder),
// which must not be accepted as "settled".
func isUniform(pngBytes []byte) bool {
	img, err := png.Decode(bytes.NewReader(pngBytes))
	if err != nil {
		return false
	}
	b := img.Bounds()
	if b.Empty() {
		return true
	}
	r0, g0, b0, a0 := img.At(b.Min.X, b.Min.Y).RGBA()
	for yy := b.Min.Y; yy < b.Max.Y; yy++ {
		for xx := b.Min.X; xx < b.Max.X; xx++ {
			r, g, bl, a := img.At(xx, yy).RGBA()
			if r != r0 || g != g0 || bl != b0 || a != a0 {
				return false
			}
		}
	}
	return true
}

// cropPNG cuts the given box (CSS pixels; device scale is 1 in our
// sessions) out of a viewport PNG, clamped to the image bounds.
func cropPNG(data []byte, x, y, w, h float64) ([]byte, error) {
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, errors.Wrap(err, "decode")
	}
	bounds := img.Bounds()
	crop := image.Rect(int(x), int(y), int(x+w), int(y+h)).Intersect(bounds)
	if crop.Empty() {
		return nil, errors.New("iframe box is outside the viewport")
	}
	sub, ok := img.(interface {
		SubImage(image.Rectangle) image.Image
	})
	if !ok {
		return nil, errors.New("screenshot image does not support cropping")
	}
	var out bytes.Buffer
	if err := png.Encode(&out, sub.SubImage(crop)); err != nil {
		return nil, errors.Wrap(err, "encode")
	}
	return out.Bytes(), nil
}
