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
