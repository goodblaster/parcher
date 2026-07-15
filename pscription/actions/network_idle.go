package actions

import (
	"context"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"github.com/goodblaster/webrun/internal/log"
)

// WaitForNetworkIdle waits for network activity to stop
// Useful before capturing MHTML to ensure all resources are loaded
func WaitForNetworkIdle(timeout time.Duration) chromedp.ActionFunc {
	return func(ctx context.Context) error {
		// Enable network tracking
		if err := network.Enable().Do(ctx); err != nil {
			return err
		}
		defer network.Disable().Do(ctx)

		startTime := time.Now()
		lastActivity := time.Now()
		activeRequests := 0

		// Listen for network events
		chromedp.ListenTarget(ctx, func(ev interface{}) {
			switch ev.(type) {
			case *network.EventRequestWillBeSent:
				activeRequests++
				lastActivity = time.Now()
			case *network.EventLoadingFinished, *network.EventLoadingFailed:
				activeRequests--
				if activeRequests < 0 {
					activeRequests = 0
				}
				lastActivity = time.Now()
			}
		})

		// Wait for network to be idle (no activity for 500ms)
		idleThreshold := 500 * time.Millisecond

		for {
			elapsed := time.Since(startTime)
			if elapsed > timeout {
				log.Default().
					With("elapsed_ms", elapsed.Milliseconds()).
					With("active_requests", activeRequests).
					Warn("network idle timeout")
				return nil // Don't fail, just continue
			}

			timeSinceActivity := time.Since(lastActivity)
			if activeRequests == 0 && timeSinceActivity > idleThreshold {
				log.Default().
					With("elapsed_ms", elapsed.Milliseconds()).
					Info("network idle")
				return nil
			}

			time.Sleep(100 * time.Millisecond)
		}
	}
}
