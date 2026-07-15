package actions

import (
	"github.com/chromedp/chromedp"
	js "github.com/goodblaster/webrun/scripts"
)

func DismissPopups() chromedp.ActionFunc {
	return js.DismissPopups.Action(nil)
}
