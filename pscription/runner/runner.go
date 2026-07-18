package runner

import (
	"context"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/goodblaster/errors"
)

// Run loads a URL in a headless browser and executes actions with default configuration
func Run(ctx context.Context, url string, actions ...chromedp.ActionFunc) error {
	return RunWithConfig(ctx, url, DefaultConfig(), actions...)
}

// RunWithConfig loads a URL and executes actions, then closes the browser.
func RunWithConfig(ctx context.Context, url string, cfg Config, actions ...chromedp.ActionFunc) error {
	session, err := Start(ctx, url, cfg)
	if err != nil {
		return err
	}
	defer session.Close()
	return session.Run(actions...)
}

// Session is a live browser context: navigation and setup have completed and
// further actions can be run against the same page. Callers control its
// lifetime, which is what makes headful "leave it open" and step-by-step
// execution possible.
type Session struct {
	ctx    context.Context
	cancel context.CancelFunc
}

// Context exposes the browser context for advanced use (e.g. attaching event
// listeners).
func (s *Session) Context() context.Context { return s.ctx }

// Start launches a browser, applies setup and pre-navigation actions,
// navigates to url, and waits for the page to be ready. The returned Session
// is live until Close.
func Start(ctx context.Context, url string, cfg Config) (*Session, error) {
	ctx, cancel := createBrowserContext(ctx, cfg)

	// Auto-accept JavaScript dialogs (alert/confirm/beforeunload).
	chromedp.ListenTarget(ctx, func(ev interface{}) {
		if _, ok := ev.(*page.EventJavascriptDialogOpening); ok {
			_ = chromedp.Run(ctx, page.HandleJavaScriptDialog(true))
		}
	})

	tasks := []chromedp.Action{
		emulation.SetUserAgentOverride(cfg.UserAgent),
		emulation.SetDeviceMetricsOverride(cfg.ViewportWidth, 0, 1.0, false),
	}
	tasks = append(tasks, cfg.PreNavigate...)
	tasks = append(tasks,
		chromedp.Navigate(url),
		chromedp.WaitReady(cfg.WaitFor),
		chromedp.Sleep(cfg.InitialWait),
	)

	if err := chromedp.Run(ctx, tasks...); err != nil {
		cancel()
		return nil, errors.Wrap(err, "failed to start browser session")
	}
	return &Session{ctx: ctx, cancel: cancel}, nil
}

// Run executes actions against the live session.
func (s *Session) Run(actions ...chromedp.ActionFunc) error {
	tasks := make([]chromedp.Action, len(actions))
	for i, a := range actions {
		tasks[i] = a
	}
	if err := chromedp.Run(s.ctx, tasks...); err != nil {
		return errors.Wrap(err, "failed to run browser tasks")
	}
	return nil
}

// Close tears down the browser. It first asks Chrome to close gracefully so
// a persistent profile flushes cookies and localStorage to disk (an abrupt
// kill loses unwritten session state, defeating -profile); then it releases
// the allocator and timeout contexts.
func (s *Session) Close() {
	_ = chromedp.Cancel(s.ctx)
	s.cancel()
}

// createBrowserContext creates a chromedp context with timeout and headless
// (or headful) configuration.
func createBrowserContext(ctx context.Context, cfg Config) (context.Context, context.CancelFunc) {
	ctx, cancel1 := context.WithTimeout(ctx, cfg.Timeout)

	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", !cfg.Headful),
	)
	if cfg.Headful {
		// A visible window the operator can actually use.
		opts = append(opts, chromedp.Flag("start-maximized", true))
	}
	if cfg.UserDataDir != "" {
		// A persistent profile: session cookies and localStorage survive
		// across runs, so a one-time login can be reused.
		opts = append(opts, chromedp.UserDataDir(cfg.UserDataDir))
	}
	if cfg.DiskCacheDir != "" {
		// A shared HTTP cache that outlives the (possibly throwaway) profile,
		// so repeat loads reuse cached resources.
		opts = append(opts, chromedp.Flag("disk-cache-dir", cfg.DiskCacheDir))
	}

	ctx, cancel2 := chromedp.NewExecAllocator(ctx, opts...)
	ctx, cancel3 := chromedp.NewContext(ctx)

	return ctx, func() {
		cancel3()
		cancel2()
		cancel1()
	}
}
