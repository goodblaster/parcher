package actions

import (
	"github.com/goodblaster/parcher/pscription/act"
	js "github.com/goodblaster/parcher/pscription/scripts"
)

func ExtractLinks(links *[]string) act.Func {
	return js.ExtractLinks.Action(links)
}
