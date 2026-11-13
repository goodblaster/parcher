package actions

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/goodblaster/errors"
	"github.com/goodblaster/parcher/pscription/act"
	"github.com/goodblaster/parcher/pscription/internal/log"
	js "github.com/goodblaster/parcher/pscription/scripts"
)

// ForceWebGLPreserve registers a script that runs on every new document
// BEFORE the page's own JavaScript, forcing preserveDrawingBuffer on every
// WebGL context the page creates so the pixels stay readable at capture
// time. Pre-navigation action; without it WebGL canvases read back as
// transparent black and would archive as blank rectangles.
func ForceWebGLPreserve() act.Func {
	return func(ctx context.Context) error {
		script, err := js.PreserveWebGLBuffer.Source()
		if err != nil {
			return errors.Wrap(err, "failed to read webgl preserve script")
		}
		// The script file is a parenthesized function; invoke it.
		source := fmt.Sprintf("(%s)();", script)
		if _, err := chromedp.Call(ctx, page.AddScriptToEvaluateOnNewDocument, page.AddScriptToEvaluateOnNewDocumentParams{Source: source}); err != nil {
			return errors.Wrap(err, "failed to register webgl preserve script")
		}
		return nil
	}
}

// RasterizeCanvases replaces every visible <canvas> with a static <img> of
// its current pixels. Most canvases are read in-page
// (rasterize_canvases.js); those page JS cannot read — tainted by
// cross-origin content or driven by an OffscreenCanvas — come back marked,
// and are captured here as CDP element screenshots, which see the
// compositor's pixels regardless of taint.
func RasterizeCanvases() act.Func {
	return func(ctx context.Context) error {
		var stats struct {
			Replaced    int   `json:"replaced"`
			Transparent int   `json:"transparent"`
			Skipped     int   `json:"skipped"`
			Unreadable  int   `json:"unreadable"`
			Marked      []int `json:"marked"`
		}
		if err := js.RasterizeCanvases.Action(&stats).Do(ctx); err != nil {
			return errors.Wrap(err, "failed to rasterize canvases")
		}

		shot := 0
		for _, id := range stats.Marked {
			if err := screenshotCanvas(ctx, id); err != nil {
				log.With("canvas", id).WithError(err).Warn("canvas screenshot fallback failed")
				continue
			}
			shot++
		}

		log.With("replaced", stats.Replaced).
			With("screenshot", shot).
			With("transparent", stats.Transparent).
			With("skipped", stats.Skipped).
			With("unreadable", stats.Unreadable).
			Debug("rasterized canvases")
		return nil
	}
}

// screenshotCanvas captures one marked canvas as a PNG element screenshot
// and swaps in the <img> in page context. Time-bounded: chromedp.Screenshot
// waits for visibility, and a canvas hidden since marking would otherwise
// stall the whole recipe.
func screenshotCanvas(ctx context.Context, id int) error {
	sel := fmt.Sprintf(`canvas[data-gb-canvas-shot="%d"]`, id)
	sctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	buf, err := chromedp.Run(sctx, chromedp.Screenshot(chromedp.CSS(sel)))
	if err != nil {
		return errors.Wrap(err, "element screenshot")
	}
	dataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf)

	var res struct {
		Replaced bool `json:"replaced"`
	}
	if err := js.ReplaceCanvasWithImage.Action(&res, id, dataURL).Do(sctx); err != nil {
		return errors.Wrap(err, "inject screenshot")
	}
	if !res.Replaced {
		return errors.New("marked canvas disappeared before replacement")
	}
	return nil
}
