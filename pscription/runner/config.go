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
