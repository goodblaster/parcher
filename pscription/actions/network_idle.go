package actions

import (
	"context"
	"sync"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"github.com/goodblaster/pscription/internal/log"
)

// idleInFlight is the number of still-open requests we tolerate and still
// call the network "idle". Zero (the strict definition) never happens on
// sites with a persistent connection — analytics beacons, long-polls,
// websockets — so a strict wait always burns its full timeout on them. Two
// is the well-worn threshold (Puppeteer's "networkidle2"): enough to ignore
// background chatter, low enough that a page actively pulling resources
// still counts as busy.
const idleInFlight = 2

// idleQuiet is how long the network must stay quiet (no request started or
// finished) before we call it idle.
const idleQuiet = 500 * time.Millisecond

// WaitForNetworkIdle returns once the network has been "almost idle" (at most
// idleInFlight requests open, no start/finish activity for idleQuiet), or the
// timeout elapses. It never fails — a timeout just means "proceed anyway".
func WaitForNetworkIdle(timeout time.Duration) chromedp.ActionFunc {
	return WaitForNetworkIdleQuiet(timeout, idleQuiet)
}

// nonRenderingRequest reports request kinds that cannot affect what the
// page looks like: telemetry beacons (navigator.sendBeacon), server-sent
// event streams and websockets (which stay open by design), and CSP
// violation reports. Deliberately conservative — XHR/Fetch still count,
// because those routinely deliver content.
func nonRenderingRequest(t network.ResourceType) bool {
	switch t {
	case network.ResourceTypePing,
		network.ResourceTypeEventSource,
		network.ResourceTypeWebSocket,
		network.ResourceTypeCSPViolationReport:
		return true
	}
	return false
}

// WaitForNetworkIdleQuiet is WaitForNetworkIdle with an explicit quiet
// window. The window is a floor on the call's cost — an already-idle page
// still waits it out to prove it stayed idle — so a recipe that checks
// repeatedly can spend the full window once and less on the re-checks.
func WaitForNetworkIdleQuiet(timeout, quiet time.Duration) chromedp.ActionFunc {
	return func(ctx context.Context) error {
		if err := network.Enable().Do(ctx); err != nil {
			return err
		}
		defer network.Disable().Do(ctx)

		var (
			mu           sync.Mutex
			active       int
			lastActivity = time.Now()
		)

		// Scope the listener to a child context so it is removed when this
		// wait returns, instead of accumulating one per call on the session.
		lctx, lcancel := context.WithCancel(ctx)
		defer lcancel()
		// Requests that cannot change a pixel are ignored entirely —
		// counting them means a page with a heartbeat (analytics beacon on
		// a timer, an open EventSource) is NEVER "idle" and every wait
		// burns its full timeout, even though the page finished painting
		// long ago. Their completion events must be ignored too, so the
		// ids are remembered.
		ignored := map[network.RequestID]bool{}
		chromedp.ListenTarget(lctx, func(ev interface{}) {
			mu.Lock()
			defer mu.Unlock()
			switch e := ev.(type) {
			case *network.EventRequestWillBeSent:
				if nonRenderingRequest(e.Type) {
					ignored[e.RequestID] = true
					return
				}
				active++
				lastActivity = time.Now()
			case *network.EventLoadingFinished:
				if ignored[e.RequestID] {
					delete(ignored, e.RequestID)
					return
				}
				if active > 0 {
					active--
				}
				lastActivity = time.Now()
			case *network.EventLoadingFailed:
				if ignored[e.RequestID] {
					delete(ignored, e.RequestID)
					return
				}
				if active > 0 {
					active--
				}
				lastActivity = time.Now()
			}
		})

		start := time.Now()
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-ticker.C:
			}

			mu.Lock()
			idle := active <= idleInFlight && time.Since(lastActivity) > quiet
			openReqs := active
			mu.Unlock()

			elapsed := time.Since(start)
			if idle {
				log.Default().With("elapsed_ms", elapsed.Milliseconds()).Info("network idle")
				return nil
			}
			if elapsed > timeout {
				log.Default().
					With("elapsed_ms", elapsed.Milliseconds()).
					With("active_requests", openReqs).
					Warn("network idle timeout")
				return nil
			}
		}
	}
}
