package actions

import (
	"github.com/chromedp/chromedp"
	js "github.com/goodblaster/pscription/scripts"
)

func ConvertSvgToPng(selector string, png *string) chromedp.ActionFunc {
	return js.ConvertSvgToPng.Action(png, selector)
}

func ConvertSvgToJpg(selector string, png *string) chromedp.ActionFunc {
	return js.ConvertSvgToJpg.Action(png, selector)
}
