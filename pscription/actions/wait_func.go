package actions

import (
	"context"
	"time"

	"github.com/goodblaster/parcher/pscription/act"
)

// WaitFunc - This is intended to obviously wait up until some timeout.
// It exists only for clarity of action.
func WaitFunc(wait func(ctx context.Context, timeout time.Duration) error, timeout time.Duration) act.Func {
	return func(ctx context.Context) error {
		return wait(ctx, timeout)
	}
}
