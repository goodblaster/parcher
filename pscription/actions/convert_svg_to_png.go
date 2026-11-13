package actions

import (
	"github.com/goodblaster/parcher/pscription/act"
	js "github.com/goodblaster/parcher/pscription/scripts"
)

func ConvertSvgToPng(selector string, png *string) act.Func {
	return js.ConvertSvgToPng.Action(png, selector)
}

func ConvertSvgToJpg(selector string, png *string) act.Func {
	return js.ConvertSvgToJpg.Action(png, selector)
}
