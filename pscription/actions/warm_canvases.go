package actions

import (
	"context"

	"github.com/goodblaster/errors"
	"github.com/goodblaster/parcher/pscription/act"
	"github.com/goodblaster/parcher/pscription/internal/log"
	js "github.com/goodblaster/parcher/pscription/scripts"
)

// WarmCanvases scrolls blank canvases and unreachable iframes into the
// viewport so throttled requestAnimationFrame content paints its first
// frames before capture. Must run BEFORE RasterizeCanvases and the
// screenshot backends — a canvas that never painted captures as an empty
// box everywhere.
func WarmCanvases() act.Func {
	return func(ctx context.Context) error {
		var stats struct {
			Bands      int `json:"bands"`
			Dwelled    int `json:"dwelled"`
			Warmed     int `json:"warmed"`
			StillBlank int `json:"stillBlank"`
		}
		if err := js.WarmCanvases.Action(&stats).Do(ctx); err != nil {
			return errors.Wrap(err, "failed to warm canvases")
		}
		log.With("bands", stats.Bands).
			With("dwelled", stats.Dwelled).
			With("warmed", stats.Warmed).
			With("stillBlank", stats.StillBlank).
			Debug("warmed canvases")
		return nil
	}
}
