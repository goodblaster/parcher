package actions

import (
	"context"
	"time"

	"github.com/goodblaster/errors"
	"github.com/goodblaster/parcher/pscription/act"
)

// WaitForReady waits until the current document is fully loaded and has
// content. Its real purpose is absorbing navigations triggered by earlier
// steps (an age-gate click often reloads the page): evaluation errors while
// the old document is being torn down are treated as "not ready yet" and
// retried until the new document settles.
func WaitForReady(timeout time.Duration) act.Func {
	return func(ctx context.Context) error {
		deadline := time.Now().Add(timeout)
		for {
			var ready bool
			err := act.Evaluate(
				`document.readyState === 'complete' && !!document.body && document.body.children.length > 0`,
				&ready).Do(ctx)
			if err == nil && ready {
				return nil
			}
			if time.Now().After(deadline) {
				if err != nil {
					return errors.Wrap(err, "page not ready before timeout")
				}
				return errors.New("page not ready before timeout")
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(200 * time.Millisecond):
			}
		}
	}
}
