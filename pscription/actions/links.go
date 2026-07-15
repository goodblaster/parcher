package actions

import (
	"github.com/chromedp/chromedp"
	js "github.com/goodblaster/pscription/scripts"
)

func ExtractLinks(links *[]string) chromedp.ActionFunc {
	return js.ExtractLinks.Action(links)
}
