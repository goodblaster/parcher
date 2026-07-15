package actions

import (
	"github.com/chromedp/chromedp"
	js "github.com/goodblaster/pscription/scripts"
)

func GetPageLanguages(langs *[]string) chromedp.ActionFunc {
	return js.DetectLanguage.Action(langs)
}
