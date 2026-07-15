package actions

import (
	"context"

	"github.com/chromedp/chromedp"
	js "github.com/goodblaster/webrun/scripts"
)

func RunScript(script js.ScriptFile, res any, params ...any) chromedp.ActionFunc {
	return script.Action(res, params...)
}

func RunActions(actions ...chromedp.ActionFunc) chromedp.ActionFunc {
	return func(ctx context.Context) error {
		for _, action := range actions {
			err := action.Do(ctx)
			if err != nil {
				return err
			}
		}
		return nil
	}
}

func RunScriptIgnoreError(script js.ScriptFile, res any, params ...any) chromedp.ActionFunc {
	return script.ActionNoError(res, params...)
}
