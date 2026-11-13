package actions

import (
	"context"
	"sync"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/goodblaster/errors"
	"github.com/goodblaster/parcher/pscription/act"
	"github.com/goodblaster/parcher/pscription/internal/log"
	js "github.com/goodblaster/parcher/pscription/scripts"
)

// SettleMonitor records page activity CONTINUOUSLY — network requests from
// before the first navigation, DOM mutations from document start — so that a
// wait can answer "has this page been quiet for N ms?" from HISTORY instead
// of buying an N-ms window to prove it prospectively.
//
// A wait that starts its own observer can never return faster than its quiet
// window, so a page that finished rendering seconds ago pays exactly what one
// still mid-render pays. Measured: the four network-idle and four DOM-quiet
// waits in Staticize, plus the layout window, cost a FLAT ~3.5s on every
// capture — 4.6s of example.com's 5.1s total, for a twelve-node document with
// no script. With the monitor attached those same waits return on their first
// check for a page that is already done (example.com 5.1s -> 2.8s, test-site-1
// 8.3s -> 6.9s, both byte-identical output), while a page that is genuinely
// still working (music.youtube.com) waits as long as it ever did.
//
// This is deliberately NOT a "detect pre-rendered pages" classifier. Neither
// served-HTML-vs-DOM size nor framework hydration markers separate the cases
// — test-site-1 carries no framework markers at all, and music.youtube.com
// ships a large server-rendered shell — but a page's own recorded mutation
// history separates them exactly, per page, with no site list to maintain.
//
// The zero value is a valid unattached monitor: its waits fall back to the
// prospective implementations, so a recipe whose PreNavigate actions were
// dropped still behaves correctly, only slower.
type SettleMonitor struct {
	mu           sync.Mutex
	attached     bool
	lastActivity time.Time
	// mainFrame is the target's own frame. Requests belonging to other
	// frames are not counted — see staleAfter.
	mainFrame cdp.FrameID
	// open maps each in-flight request to when it started, so a request
	// whose completion never arrives can be aged out instead of counted
	// forever.
	open map[network.RequestID]time.Time
	// Request ids whose completion must be ignored too, see
	// nonRenderingRequest.
	ignored map[network.RequestID]bool
}

// staleAfter is how long a request may stay in flight before it stops being
// evidence that the page is still working.
//
// A COUNTER OF IN-FLIGHT REQUESTS ONLY WORKS IF EVERY REQUEST IS SEEN TO
// END, and on a page target that is not true: a cross-origin iframe's own
// document request is reported here when it STARTS (the parent initiated it)
// but completes on the iframe's out-of-process target, which nothing is
// listening to. Measured on test-site-2: fourteen such requests —
// twelve YouTube embeds, a youtube-nocookie embed and Google's IMA SDK
// bridge — sat "in flight" for the whole session, ages climbing past 42s, so
// the page could never be idle and all four network waits burned their full
// timeout: 25s of pure waiting, a capture that took 26s before the monitor
// existed taking 49s after it. The per-wait monitors this replaced never saw
// it because each one started counting from zero and those frames were
// created before the wait began.
//
// Subframe documents are therefore not counted at all (the precise fix), and
// this is the net under everything else that can leak the same way — an XHR
// abandoned when its iframe was removed, a connection that hangs. It is
// deliberately longer than the recheck windows, so a genuinely slow resource
// still counts as work in progress, and short enough that one leak costs at
// most one wait rather than every wait.
const staleAfter = 5 * time.Second

// NewSettleMonitor returns a monitor ready to Attach.
func NewSettleMonitor() *SettleMonitor {
	return &SettleMonitor{
		open:    map[network.RequestID]time.Time{},
		ignored: map[network.RequestID]bool{},
	}
}

// Attach is a PRE-NAVIGATION action: it registers the DOM-mutation recorder
// as a new-document script and starts the network listener before the first
// request is made. Both halves must be in place before navigation or the
// monitor's history starts mid-load, which is the whole thing it exists to
// avoid.
//
// The network domain stays enabled for the rest of the session. That is the
// cost of a continuous record, and it is why WaitForNetworkIdleQuiet no
// longer disables the domain on its way out — doing so would silence this
// listener.
func (m *SettleMonitor) Attach() act.Func {
	return func(ctx context.Context) error {
		source, err := js.SettleMonitor.Source()
		if err != nil {
			return errors.Wrap(err, "failed to read settle monitor script")
		}
		if _, err := chromedp.Call(ctx, page.AddScriptToEvaluateOnNewDocument, page.AddScriptToEvaluateOnNewDocumentParams{Source: source}); err != nil {
			return errors.Wrap(err, "failed to register settle monitor script")
		}
		if _, err := chromedp.Call(ctx, network.Enable, network.EnableParams{}); err != nil {
			return errors.Wrap(err, "failed to enable network domain")
		}

		// The target's own frame id, taken before navigation: it is stable
		// for the target's lifetime, so it stays valid across every
		// navigation and reload this capture performs.
		frames, err := chromedp.Call(ctx, page.GetFrameTree, cdp.Empty{})
		if err != nil {
			return errors.Wrap(err, "failed to read frame tree")
		}
		tree := frames.FrameTree

		m.mu.Lock()
		m.attached = true
		m.lastActivity = time.Now()
		m.mainFrame = tree.Frame.ID
		if m.open == nil {
			m.open = map[network.RequestID]time.Time{}
		}
		if m.ignored == nil {
			m.ignored = map[network.RequestID]bool{}
		}
		m.mu.Unlock()

		return chromedp.Listen(ctx, m.handle)
	}
}

func (m *SettleMonitor) handle(ev any) {
	m.mu.Lock()
	defer m.mu.Unlock()
	switch e := ev.(type) {
	case *network.EventRequestWillBeSent:
		// A subframe's own document request completes on that frame's
		// target, not this one, so counting it here means counting
		// something that can never be seen to finish. See staleAfter.
		if nonRenderingRequest(e.Type) ||
			(e.Type == network.ResourceTypeDocument && e.FrameID != m.mainFrame) {
			m.ignored[e.RequestID] = true
			return
		}
		m.open[e.RequestID] = time.Now()
		m.lastActivity = time.Now()
	case *network.EventLoadingFinished:
		m.finish(e.RequestID)
	case *network.EventLoadingFailed:
		m.finish(e.RequestID)
	}
}

// finish must be called with m.mu held.
func (m *SettleMonitor) finish(id network.RequestID) {
	if m.ignored[id] {
		delete(m.ignored, id)
		return
	}
	delete(m.open, id)
	m.lastActivity = time.Now()
}

// inFlight counts requests still plausibly loading, dropping any that have
// been open past staleAfter — and forgetting them, so the map cannot grow
// for the whole session. Must be called with m.mu held.
func (m *SettleMonitor) inFlight() int {
	now := time.Now()
	n := 0
	for id, started := range m.open {
		if now.Sub(started) > staleAfter {
			delete(m.open, id)
			continue
		}
		n++
	}
	return n
}

// WaitForNetworkIdle returns once the network has been quiet for the default
// window, or the timeout elapses. Unlike the standalone action of the same
// name, quiet that happened BEFORE the call counts.
func (m *SettleMonitor) WaitForNetworkIdle(timeout time.Duration) act.Func {
	return m.WaitForNetworkIdleQuiet(timeout, idleQuiet)
}

// WaitForNetworkIdleQuiet is WaitForNetworkIdle with an explicit quiet
// window. With the monitor attached the window is no longer a floor on the
// call's cost: a page whose last request finished a second ago satisfies a
// 250ms window immediately.
func (m *SettleMonitor) WaitForNetworkIdleQuiet(timeout, quiet time.Duration) act.Func {
	return func(ctx context.Context) error {
		m.mu.Lock()
		attached := m.attached
		m.mu.Unlock()
		if !attached {
			// No history to consult — behave exactly as before.
			return WaitForNetworkIdleQuiet(timeout, quiet)(ctx)
		}

		start := time.Now()
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()

		for {
			m.mu.Lock()
			openReqs := m.inFlight()
			idle := openReqs <= idleInFlight && time.Since(m.lastActivity) > quiet
			m.mu.Unlock()

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

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-ticker.C:
			}
		}
	}
}
