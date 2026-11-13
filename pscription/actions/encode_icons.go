package actions

import (
	"context"

	"github.com/goodblaster/parcher/pscription/act"
	js "github.com/goodblaster/parcher/pscription/scripts"
)

func RasterizeFontIcons() act.Func {
	return func(ctx context.Context) error {
		return js.RasterizeFontIcons.Action(nil).Do(ctx)
	}
}
