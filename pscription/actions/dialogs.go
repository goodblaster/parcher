package actions

import (
	"github.com/chromedp/chromedp"
	js "github.com/goodblaster/pscription/scripts"
)

func DismissPopups() chromedp.ActionFunc {
	return js.DismissPopups.Action(nil)
}
