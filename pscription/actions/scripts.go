package actions

import (
	"context"

	"github.com/goodblaster/parcher/pscription/act"
	js "github.com/goodblaster/parcher/pscription/scripts"
)

func RunScript(script js.ScriptFile, res any, params ...any) act.Func {
	return script.Action(res, params...)
}

func RunActions(actions ...act.Func) act.Func {
	return func(ctx context.Context) error {
		for _, action := range actions {
			err := action.Do(ctx)
			if err != nil {
				return err
			}
		}
		return nil
	}
}

func RunScriptIgnoreError(script js.ScriptFile, res any, params ...any) act.Func {
	return script.ActionNoError(res, params...)
}
