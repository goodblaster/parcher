package actions

import (
	"context"

	"github.com/goodblaster/errors"
	"github.com/goodblaster/parcher/pscription/act"
	js "github.com/goodblaster/parcher/pscription/scripts"
)

func CaptureVideoFrame() act.Func {
	return func(cxt context.Context) error {
		err := js.CaptureVideoFrame.Action(nil).Do(cxt)
		if err != nil {
			return errors.Wrap(err, "failed to capture video frame")
		}
		return nil
	}
}
