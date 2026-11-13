package actions

import (
	"context"

	"github.com/goodblaster/parcher/pscription/act"
	js "github.com/goodblaster/parcher/pscription/scripts"
)

func DismissPopups() act.Func {
	return js.DismissPopups.Action(nil)
}

// DismissTally records whether the dialog-clearing steps actually did
// anything, so a later step can skip work that only matters when they
// did. Steps run sequentially, so no locking is needed.
type DismissTally struct{ Acted bool }

// DismissCMPCounting is DismissCMP, recording whether it clicked or
// removed anything into tally.
func DismissCMPCounting(tally *DismissTally) act.Func {
	return func(ctx context.Context) error {
		var res struct {
			Clicked        []string `json:"clicked"`
			RemovedIframes int      `json:"removedIframes"`
		}
		err := js.DismissCMP.ActionNoError(&res).Do(ctx)
		if len(res.Clicked) > 0 || res.RemovedIframes > 0 {
			tally.Acted = true
		}
		return err
	}
}

// DismissPopupsCounting is DismissPopups, recording whether it clicked,
// removed, or closed anything into tally.
func DismissPopupsCounting(tally *DismissTally) act.Func {
	return func(ctx context.Context) error {
		var res struct {
			Clicked       int `json:"clicked"`
			Removed       int `json:"removed"`
			ClosedDialogs int `json:"closedDialogs"`
		}
		err := js.DismissPopups.ActionNoError(&res).Do(ctx)
		if res.Clicked > 0 || res.Removed > 0 || res.ClosedDialogs > 0 {
			tally.Acted = true
		}
		return err
	}
}
