// Package capture serializes a prepared page into a self-contained snapshot.
//
// A Backend decides the output format (SingleFile HTML, MHTML, screenshot);
// Capture runs a preparation recipe and then the backend in one browser
// session.
package capture

import (
	"context"

	"github.com/chromedp/chromedp"
	"github.com/goodblaster/errors"
	"github.com/goodblaster/pscription/pipeline"
	"github.com/goodblaster/pscription/runner"
)

// Snapshot is the result of capturing a page.
type Snapshot struct {
	URL   string
	Title string
	MIME  string
	Bytes []byte
}

// Backend serializes the current page state into a snapshot.
type Backend interface {
	// Name identifies the backend (e.g. "singlefile", "mht").
	Name() string

	// Ext is the conventional file extension for this backend's output,
	// including the dot (e.g. ".html").
	Ext() string

	// Action fills snap from the current browser state. It runs after the
	// preparation recipe in the same session.
	Action(snap *Snapshot) chromedp.ActionFunc
}

// Capture loads url, runs the preparation recipe, and serializes the page
// with the given backend, all in one browser session.
func Capture(ctx context.Context, url string, recipe pipeline.Recipe, backend Backend) (*Snapshot, error) {
	return CaptureWithConfig(ctx, url, runner.DefaultConfig(), recipe, backend)
}

// CaptureWithConfig is Capture with a custom runner configuration.
func CaptureWithConfig(ctx context.Context, url string, cfg runner.Config, recipe pipeline.Recipe, backend Backend) (*Snapshot, error) {
	snap := &Snapshot{URL: url}
	cfg.PreNavigate = append(cfg.PreNavigate, recipe.PreNavigateActions(url)...)
	err := runner.RunWithConfig(ctx, url, cfg,
		recipe.Action(),
		captureTitle(&snap.Title),
		backend.Action(snap),
	)
	if err != nil {
		return nil, errors.Wrapf(err, "%s capture failed", backend.Name())
	}
	return snap, nil
}

func captureTitle(title *string) chromedp.ActionFunc {
	return func(ctx context.Context) error {
		return chromedp.Title(title).Do(ctx)
	}
}

// normalizeBackgroundAttachment forces background-attachment:scroll. Chrome's
// print pipeline (PDF) and full-page screenshots don't paint fixed (parallax)
// backgrounds beyond the first viewport, so such sections come out blank —
// and any white-on-dark text in them becomes invisible. Parallax is
// meaningless in a static capture anyway. Used by the "flat" backends (PDF,
// PNG, JPEG); the HTML/MHT backends render in a real browser and don't need
// it. Background-attachment doesn't affect layout, so this triggers no
// reflow.
func normalizeBackgroundAttachment(ctx context.Context) error {
	return chromedp.Evaluate(
		`(() => { const s = document.createElement('style');
			s.textContent = '*{background-attachment:scroll !important}';
			(document.head || document.documentElement).appendChild(s); })()`,
		nil).Do(ctx)
}
