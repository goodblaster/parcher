package actions

import (
	"github.com/chromedp/chromedp"
	js "github.com/goodblaster/webrun/scripts"
)

func ExtractLinks(links *[]string) chromedp.ActionFunc {
	return js.ExtractLinks.Action(links)
}
