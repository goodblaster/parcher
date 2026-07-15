package actions

import (
	"github.com/chromedp/chromedp"
	js "github.com/goodblaster/webrun/scripts"
)

func InjectCss(cssId string, css string) chromedp.ActionFunc {
	return js.InjectCss.Action(nil, cssId, css)
}
