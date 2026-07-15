package actions

import (
	"context"
	"time"

	"github.com/chromedp/chromedp"
)

// WaitFunc - This is intended to obviously wait up until some timeout.
// It exists only for clarity of action.
func WaitFunc(wait func(ctx context.Context, timeout time.Duration) error, timeout time.Duration) chromedp.ActionFunc {
	return func(ctx context.Context) error {
		return wait(ctx, timeout)
	}
}
