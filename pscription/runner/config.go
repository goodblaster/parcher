package runner

import (
	"time"

	"github.com/chromedp/chromedp"
	"github.com/goodblaster/webrun/agents"
	"github.com/goodblaster/webrun/internal/config"
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
