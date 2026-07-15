package actions

import (
	"github.com/chromedp/chromedp"
	js "github.com/goodblaster/webrun/scripts"
)

func GetPageLanguages(langs *[]string) chromedp.ActionFunc {
	return js.DetectLanguage.Action(langs)
}
