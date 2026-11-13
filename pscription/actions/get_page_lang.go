package actions

import (
	"github.com/goodblaster/parcher/pscription/act"
	js "github.com/goodblaster/parcher/pscription/scripts"
)

func GetPageLanguages(langs *[]string) act.Func {
	return js.DetectLanguage.Action(langs)
}
