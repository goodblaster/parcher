package actions

import (
	"github.com/chromedp/chromedp"
	js "github.com/goodblaster/parcher/pscription/scripts"
)

func FindAllSVG(selectors *[]string) chromedp.ActionFunc {
	return js.LocateSvgElements.Action(selectors)
}
