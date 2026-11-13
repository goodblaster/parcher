package runner

import (
	"context"
	"os"
	goruntime "runtime"
	"strings"

	"github.com/chromedp/cdproto/browser"
	"github.com/chromedp/cdproto/cdp"
	"github.com/goodblaster/parcher/pscription/agents"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/goodblaster/errors"
	"github.com/goodblaster/parcher/pscription/act"
	"github.com/goodblaster/parcher/pscription/internal/config"
)

// Run loads a URL in a headless browser and executes actions with default configuration
func Run(ctx context.Context, url string, actions ...act.Func) error {
	return RunWithConfig(ctx, url, DefaultConfig(), actions...)
}

// RunWithConfig loads a URL and executes actions, then closes the browser.
func RunWithConfig(ctx context.Context, url string, cfg Config, actions ...act.Func) error {
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

	// Auto-accept JavaScript dialogs (alert/confirm/beforeunload). The
	// answer goes out from its own goroutine: listeners run on the goroutine
	// that delivers CDP responses, so a synchronous Run here never gets its
	// reply — an alert() during load hung the session until the timeout.
	if err := chromedp.Listen(ctx, func(ev any) {
		if _, ok := ev.(*page.EventJavascriptDialogOpening); ok {
			go func() {
				_, _ = chromedp.Call(ctx, page.HandleJavaScriptDialog, page.HandleJavaScriptDialogParams{Accept: true})
			}()
		}
	}); err != nil {
		cancel()
		return nil, errors.Wrap(err, "failed to listen for dialogs")
	}

	// A zero DeviceScaleFactor is not a valid metric override — callers
	// that build a Config literally (rather than via DefaultConfig) would
	// otherwise render at 0x.
	dpr := cfg.DeviceScaleFactor
	if dpr <= 0 {
		dpr = 1.0
	}
	// A zero height must NOT be passed through: Chrome then keeps its own
	// default window height while honouring the width override, producing a
	// viewport no real browser presents (see Config.ViewportHeight).
	h := cfg.ViewportHeight
	if h <= 0 {
		h = config.ViewHeight
	}
	tasks := []act.Action{
		userAgentAction(cfg.UserAgent),
		act.Func(func(ctx context.Context) error {
			_, err := chromedp.Call(ctx, emulation.SetDeviceMetricsOverride, emulation.SetDeviceMetricsOverrideParams{
				Width: cfg.ViewportWidth, Height: h, DeviceScaleFactor: dpr, Mobile: false,
			})
			return err
		}),
	}
	tasks = append(tasks, cfg.PreNavigate...)
	tasks = append(tasks,
		act.Func(func(ctx context.Context) error {
			return chromedp.Do(ctx, chromedp.Navigate(url), chromedp.WaitReady(cfg.WaitFor))
		}),
		act.Sleep(cfg.InitialWait),
	)

	if err := act.Run(ctx, tasks...); err != nil {
		cancel()
		return nil, errors.Wrap(err, "failed to start browser session")
	}
	return &Session{ctx: ctx, cancel: cancel}, nil
}

// Run executes actions against the live session.
func (s *Session) Run(actions ...act.Func) error {
	tasks := make([]act.Action, len(actions))
	for i, a := range actions {
		tasks[i] = a
	}
	if err := act.Run(s.ctx, tasks...); err != nil {
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

	opts := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
	if cfg.Headful {
		// A visible window the operator can actually use.
		opts = append(opts, chromedp.VisibleWindow)
	}
	if os.Getenv("PSCRIPTION_NO_SANDBOX") != "" {
		// Chrome refuses to start where it cannot build its sandbox: as
		// root in a container, or on a distribution that restricts
		// unprivileged user namespaces (Ubuntu 23.10+, which is every hosted
		// CI runner). Opt-in only — the sandbox is what stands between a
		// hostile page and the machine, so it is never dropped silently.
		opts = append(opts, chromedp.NoSandbox)
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
	if cfg.AllowFileAccess {
		// Local files share one origin, so a page opened from disk can read
		// its own stylesheets and assets. See the field's documentation for
		// what this gives up.
		opts = append(opts, chromedp.Flag("allow-file-access-from-files", true))
	}

	ctx, cancel2 := chromedp.NewExecAllocator(ctx, opts...)
	ctx, cancel3 := chromedp.NewContext(ctx)

	return ctx, func() {
		cancel3()
		cancel2()
		cancel1()
	}
}

// userAgentAction sets the session's user agent. An explicit one is sent as
// given. Otherwise the browser's own is used with the headless marker
// removed, plus client hints that agree with it: headless Chrome announces
// itself as "HeadlessChrome" in both, which sites treat as a bot, and an
// override that fixes only the string leaves sec-ch-ua contradicting it.
func userAgentAction(explicit string) act.Action {
	set := func(ctx context.Context, params emulation.SetUserAgentOverrideParams) error {
		_, err := chromedp.Call(ctx, emulation.SetUserAgentOverride, params)
		return err
	}
	return act.Func(func(ctx context.Context) error {
		if explicit != "" {
			return set(ctx, emulation.SetUserAgentOverrideParams{UserAgent: explicit})
		}
		version, err := chromedp.CallBrowser(ctx, browser.GetVersion, cdp.Empty{})
		product, ua := version.Product, version.UserAgent
		if err != nil || ua == "" {
			return set(ctx, emulation.SetUserAgentOverrideParams{UserAgent: agents.Current})
		}
		ua = strings.ReplaceAll(ua, "HeadlessChrome", "Chrome")

		// product is "Chrome/141.0.7390.55" (or "HeadlessChrome/…").
		full := product
		if i := strings.IndexByte(product, '/'); i >= 0 {
			full = product[i+1:]
		}
		major, _, _ := strings.Cut(full, ".")
		platform, platformVersion, arch := "Linux", "", "x86"
		switch {
		case strings.Contains(ua, "Macintosh"):
			platform, platformVersion = "macOS", "15.0.0"
		case strings.Contains(ua, "Windows"):
			platform, platformVersion = "Windows", "15.0.0"
		}
		if goruntime.GOARCH == "arm64" {
			arch = "arm"
		}
		brands := []*emulation.UserAgentBrandVersion{
			{Brand: "Chromium", Version: major},
			{Brand: "Google Chrome", Version: major},
			{Brand: "Not?A_Brand", Version: "99"},
		}
		return set(ctx, emulation.SetUserAgentOverrideParams{
			UserAgent: ua,
			UserAgentMetadata: &emulation.UserAgentMetadata{
				Brands: brands,
				FullVersionList: []*emulation.UserAgentBrandVersion{
					{Brand: "Chromium", Version: full},
					{Brand: "Google Chrome", Version: full},
					{Brand: "Not?A_Brand", Version: "99.0.0.0"},
				},
				Platform:        platform,
				PlatformVersion: platformVersion,
				Architecture:    arch,
				Bitness:         "64",
				Mobile:          false,
			},
		})
	})
}
