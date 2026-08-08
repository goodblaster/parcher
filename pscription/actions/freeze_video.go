package actions

import (
	"context"

	"github.com/chromedp/chromedp"
	"github.com/goodblaster/errors"
	js "github.com/goodblaster/parcher/pscription/scripts"
)

func CaptureVideoFrame() chromedp.ActionFunc {
	return func(cxt context.Context) error {
		err := js.CaptureVideoFrame.Action(nil).Do(cxt)
		if err != nil {
			return errors.Wrap(err, "failed to capture video frame")
		}
		return nil
	}
}
