package runner

import (
	"time"

	"github.com/chromedp/chromedp"
	"github.com/goodblaster/parcher/pscription/internal/config"
)

// Config holds configuration for the script runner
type Config struct {
	// Context timeout for entire operation
	Timeout time.Duration

	// Browser viewport width, in CSS pixels. This is the LAYOUT width —
	// it decides which responsive breakpoint the page renders at.
	ViewportWidth int64

	// ViewportHeight is the CSS-pixel viewport height. It is as load-bearing
	// as the width: leaving it unset made Chrome keep its default window
	// height while the width was overridden, so pages laid out in a slot
	// like 1600x413 — an aspect ratio no responsive design anticipates, with
	// 100vh collapsed to a third of a screen. Carousels that size themselves
	// from the viewport, IntersectionObserver lazy-loading, and every 100vh
	// hero all mis-render there. Zero means the configured default.
	ViewportHeight int64

	// DeviceScaleFactor is the device pixel ratio: how many output pixels
	// each CSS pixel becomes. It is deliberately separate from
	// ViewportWidth, because layout width and output resolution are
	// independent — a 1024-wide layout at DPR 2 rasterizes to 2048 px
	// without changing a single breakpoint. Zero means 1.0.
	DeviceScaleFactor float64

	// UserAgent is the user agent string to announce. Empty — the default —
	// means the running browser's own, with the "Headless" marker removed
	// and matching client hints: the page sees the Chrome that is actually
	// rendering it, at its real version. A non-empty value is sent verbatim
	// with no client hints (agents has constants, including the Safari string
	// that used to be the default).
	UserAgent string

	// CSS selector to wait for after navigation
	WaitFor string

	// Duration to wait after initial page load
	InitialWait time.Duration

	// Actions executed after browser setup but before navigation
	// (cookie seeding, evaluate-on-new-document scripts, request blocking)
	PreNavigate []chromedp.Action

	// Headful launches a visible browser instead of headless. Required for
	// interactive steps like wait-for-user (manual login).
	Headful bool

	// UserDataDir points Chrome at a persistent profile directory instead of
	// a throwaway one. Logging in once (headful) leaves the session in this
	// dir, so later runs — even headless — reuse it until it expires. Empty
	// means an ephemeral profile (the default). The directory must not be in
	// use by another Chrome instance.
	UserDataDir string

	// AllowFileAccess lets a document loaded over file:// reach other local
	// files: reading a stylesheet's rules through the CSSOM, fetching a
	// sibling asset. Chrome otherwise gives every local file its own opaque
	// origin, so a saved page's own stylesheets are cross-origin to it and
	// sheet.cssRules throws SecurityError — which silently costs you every
	// rule in them, since the sheets still LOAD and render.
	//
	// This is for reading archives off disk, not for browsing: it relaxes a
	// real boundary, letting a local document read anything else the user can
	// read. Turn it on only for content already trusted enough to open, and
	// preferably alongside disabled script execution, so nothing in the page
	// can act on the access.
	AllowFileAccess bool

	// DiskCacheDir points Chrome's HTTP cache at a fixed directory that
	// survives across runs, even with an ephemeral profile. Repeat loads of
	// the same site then reuse cached CSS/JS/font/image responses (subject to
	// normal cache-control revalidation) instead of refetching, which speeds
	// up re-captures and archiving many pages from one site. Empty uses the
	// profile's default (throwaway) cache. Unlike UserDataDir it holds no
	// cookies or login state, so it's safe to share globally.
	DiskCacheDir string
}

// DefaultConfig returns sensible defaults
func DefaultConfig() Config {
	return Config{
		// A backstop only: callers bound the session with their own context.
		// It was 3000s, which left a library user with a fifty-minute hang.
		Timeout:           5 * time.Minute,
		ViewportWidth:     config.ViewWidth,
		ViewportHeight:    config.ViewHeight,
		DeviceScaleFactor: 1.0,
		WaitFor:           "body",
		InitialWait:       1 * time.Second,
	}
}
