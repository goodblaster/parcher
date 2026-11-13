package actions

import (
	"github.com/goodblaster/parcher/pscription/act"
	js "github.com/goodblaster/parcher/pscription/scripts"
)

func FindAllSVG(selectors *[]string) act.Func {
	return js.LocateSvgElements.Action(selectors)
}
