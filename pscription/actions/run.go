package actions

import (
	"context"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/chromedp"
)

func Run(ctx context.Context, actions ...chromedp.ActionFunc) error {
	tasks := chromedp.Tasks{}
	for _, action := range actions {
		tasks = append(tasks, action)
	}
	target := chromedp.FromContext(ctx).Target
	return tasks.Do(cdp.WithExecutor(ctx, target))
}
