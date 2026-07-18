package runner

import (
	"time"

	"github.com/chromedp/chromedp"
	"github.com/goodblaster/pscription/agents"
	"github.com/goodblaster/pscription/internal/config"
)

// Config holds configuration for the script runner
type Config struct {
	// Context timeout for entire operation
	Timeout time.Duration

	// Browser viewport width
	ViewportWidth int64

	// Browser user agent string
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
		Timeout:       3000 * time.Second,
		ViewportWidth: config.ViewWidth,
		UserAgent:     agents.Current,
		WaitFor:       "body",
		InitialWait:   1 * time.Second,
	}
}
