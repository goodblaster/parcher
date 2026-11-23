package actions

import (
	"context"

	"github.com/goodblaster/parcher/pscription/act"
	"github.com/goodblaster/parcher/pscription/imageopt"
	"github.com/goodblaster/parcher/pscription/internal/log"
	js "github.com/goodblaster/parcher/pscription/scripts"
)

// SizeImages records painted image sizes and optionally reduces embedded
// bitmaps. Run after prep and before text extraction (which hashes the sources).
func SizeImages(preserve bool) act.Func {
	return func(ctx context.Context) error {
		var targets map[string]struct{ Width, Height int }
		if err := js.SizeImages.Action(&targets)(ctx); err != nil {
			return err
		}
		replacements := map[string]struct {
			Src   string `json:"src"`
			Width int    `json:"width"`
		}{}
		if !preserve {
			for src, target := range targets {
				if err := ctx.Err(); err != nil {
					return err
				}
				uri, width, _ := imageopt.Resize(src, target.Width, target.Height)
				if uri != "" {
					replacements[src] = struct {
						Src   string `json:"src"`
						Width int    `json:"width"`
					}{uri, width}
				}
			}
		}
		var stats struct{ Changed int }
		if err := js.SizeImages.Action(&stats, replacements)(ctx); err != nil {
			return err
		}
		log.With("images", stats.Changed).Debug("downsized embedded images")
		return nil
	}
}
