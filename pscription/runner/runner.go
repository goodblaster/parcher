package runner

import (
	"context"
	"time"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/goodblaster/errors"
)

// Run loads a URL in a headless browser and executes actions with default configuration
func Run(ctx context.Context, url string, actions ...chromedp.ActionFunc) error {
	return RunWithConfig(ctx, url, DefaultConfig(), actions...)
}

// RunWithConfig loads a URL in a headless browser and executes actions with custom configuration
func RunWithConfig(ctx context.Context, url string, cfg Config, actions ...chromedp.ActionFunc) error {
	// Create context with timeout and browser options
	ctx, cancel := createBrowserContext(ctx, cfg.Timeout)
	defer cancel()

	// Setup JavaScript dialog handler (auto-accept)
	chromedp.ListenTarget(ctx, func(ev interface{}) {
		if _, ok := ev.(*page.EventJavascriptDialogOpening); ok {
			_ = chromedp.Run(ctx, page.HandleJavaScriptDialog(true))
		}
	})

	// Build complete task list - setup + pre-navigation + user actions
	allTasks := []chromedp.Action{
		// Browser setup
		emulation.SetUserAgentOverride(cfg.UserAgent),
		emulation.SetDeviceMetricsOverride(cfg.ViewportWidth, 0, 1.0, false),
	}

	// Pre-navigation actions run before the page exists: cookie seeding,
	// scripts registered to evaluate on new documents, request blocking.
	allTasks = append(allTasks, cfg.PreNavigate...)

	allTasks = append(allTasks,
		// Navigation
		chromedp.Navigate(url),
		chromedp.WaitReady(cfg.WaitFor),

		// Initial wait
		chromedp.Sleep(cfg.InitialWait),
	)

	// Convert ActionFuncs to Actions and append
	for _, actionFunc := range actions {
		allTasks = append(allTasks, actionFunc)
	}

	// Execute everything in a single Run call
	err := chromedp.Run(ctx, allTasks...)
	if err != nil {
		return errors.Wrap(err, "failed to run browser tasks")
	}

	return nil
}

// createBrowserContext creates a chromedp context with timeout and headless configuration
func createBrowserContext(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	// Create timeout context
	ctx, cancel1 := context.WithTimeout(ctx, timeout)

	// Setup browser allocator options
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", true),
	)

	ctx, cancel2 := chromedp.NewExecAllocator(ctx, opts...)
	ctx, cancel3 := chromedp.NewContext(ctx)

	// Return combined cancel function
	return ctx, func() {
		cancel3()
		cancel2()
		cancel1()
	}
}
