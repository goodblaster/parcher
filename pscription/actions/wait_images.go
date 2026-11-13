package actions

import (
	"context"
	"time"

	"github.com/goodblaster/parcher/pscription/act"
	"github.com/goodblaster/parcher/pscription/internal/log"
	js "github.com/goodblaster/parcher/pscription/scripts"
)

type ImageWaitResult struct {
	Total   int `json:"total"`
	Loaded  int `json:"loaded"`
	Elapsed int `json:"elapsed"`
}

// WaitForImages waits for images to load with early exit
// maxWait is the maximum time to wait (e.g., 10 seconds)
// Returns early if all images load before timeout
func WaitForImages(maxWait time.Duration) act.Func {
	var result ImageWaitResult
	return func(ctx context.Context) error {
		maxWaitMs := int(maxWait.Milliseconds())
		err := js.WaitForImages.Action(&result, maxWaitMs).Do(ctx)
		if err == nil {
			log.Default().
				With("total", result.Total).
				With("loaded", result.Loaded).
				With("elapsed_ms", result.Elapsed).
				Info("waited for images")
		}
		return err
	}
}
