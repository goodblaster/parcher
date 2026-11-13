package actions

import (
	"github.com/goodblaster/parcher/pscription/act"
	js "github.com/goodblaster/parcher/pscription/scripts"
)

func InjectCss(cssId string, css string) act.Func {
	return js.InjectCss.Action(nil, cssId, css)
}
