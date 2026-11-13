package actions

import (
	"context"

	"github.com/goodblaster/parcher/pscription/act"
	"github.com/goodblaster/parcher/pscription/internal/log"
	js "github.com/goodblaster/parcher/pscription/scripts"
)

type EmbedResult struct {
	Total     int `json:"total"`
	Processed int `json:"processed"`
	Inlined   int `json:"inlined"`
}

func ConvertToGrayscale() act.Func {
	return js.ConvertToGrayscale.Action(nil)
}

func EmbedImages() act.Func {
	var result EmbedResult
	return func(ctx context.Context) error {
		err := js.EmbedImages.Action(&result).Do(ctx)
		if err == nil {
			log.Default().With("total", result.Total).
				With("processed", result.Processed).
				With("inlined", result.Inlined).
				Info("embedded images")
		}
		return err
	}
}

func EmbedBackgroundImages() act.Func {
	var result EmbedResult
	return func(ctx context.Context) error {
		err := js.EmbedBackgroundImages.Action(&result).Do(ctx)
		if err == nil {
			log.Default().With("total", result.Total).
				With("processed", result.Processed).
				With("inlined", result.Inlined).
				Info("embedded background images")
		}
		return err
	}
}

func NormalizeViewportUnits() act.Func {
	return js.NormalizeViewportUnits.Action(nil)
}
