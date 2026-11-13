package actions

import (
	"context"

	"github.com/goodblaster/parcher/pscription/act"
)

// Run executes actions in order against the session in ctx.
func Run(ctx context.Context, actions ...act.Func) error {
	for _, action := range actions {
		if err := action(ctx); err != nil {
			return err
		}
	}
	return nil
}
