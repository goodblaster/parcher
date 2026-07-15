package actions

import (
	"context"

	"github.com/chromedp/chromedp"
	js "github.com/goodblaster/webrun/scripts"
)

func RasterizeFontIcons() chromedp.ActionFunc {
	return func(ctx context.Context) error {
		return js.RasterizeFontIcons.Action(nil).Do(ctx)
	}
}
