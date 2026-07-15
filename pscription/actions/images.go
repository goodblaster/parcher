package actions

import (
	"context"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/chromedp"
	"github.com/goodblaster/webrun/internal/config"
	"github.com/goodblaster/webrun/internal/log"
	js "github.com/goodblaster/webrun/scripts"
)

type EmbedResult struct {
	Total     int `json:"total"`
	Processed int `json:"processed"`
	Inlined   int `json:"inlined"`
}

func ConvertToGrayscale() chromedp.ActionFunc {
	return js.ConvertToGrayscale.Action(nil)
}

func EmbedImages() chromedp.ActionFunc {
	var result EmbedResult
	return func(ctx context.Context) error {
		err := js.EmbedImages.Action(&result).Do(ctx)
		if err == nil {
			log.Default().With("total", result.Total).
				With("processed", result.Processed).
				With("inlined", result.Inlined).
				Info("embedded images")
		}
		return err
	}
}

func EmbedBackgroundImages() chromedp.ActionFunc {
	var result EmbedResult
	return func(ctx context.Context) error {
		err := js.EmbedBackgroundImages.Action(&result).Do(ctx)
		if err == nil {
			log.Default().With("total", result.Total).
				With("processed", result.Processed).
				With("inlined", result.Inlined).
				Info("embedded background images")
		}
		return err
	}
}

func NormalizeViewportUnits() chromedp.ActionFunc {
	return js.NormalizeViewportUnits.Action(nil)
}

func MakeFullHeight() chromedp.ActionFunc {
	return func(ctx context.Context) error {
		var pageHeight int64
		_ = chromedp.Evaluate(`document.documentElement.scrollHeight`, &pageHeight).Do(ctx)
		_ = emulation.SetPageScaleFactor(1.0).Do(ctx)
		_ = emulation.SetDeviceMetricsOverride(config.ViewWidth, pageHeight, 1.0, false).Do(ctx)
		return nil
	}
}
