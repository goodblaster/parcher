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
	"github.com/goodblaster/pscription/internal/log"
	"github.com/goodblaster/pscription/pipeline"
	"github.com/goodblaster/pscription/runner"
	js "github.com/goodblaster/pscription/scripts"
)

// maxCaptureAttempts bounds the retry loop for pages that come out of
// preparation in a detectably broken state (e.g. a carousel library that
// intermittently fails to initialize in headless, leaving slides stacked).
const maxCaptureAttempts = 3

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

// CaptureWithConfig is Capture with a custom runner configuration. If a
// prepared page is detected to be in a broken layout state (a slider that
// failed to initialize), the whole capture is retried in a fresh session,
// since the failure is intermittent.
func CaptureWithConfig(ctx context.Context, url string, cfg runner.Config, recipe pipeline.Recipe, backend Backend) (*Snapshot, error) {
	cfg.PreNavigate = append(cfg.PreNavigate, recipe.PreNavigateActions(url)...)

	for attempt := 1; ; attempt++ {
		snap, defect, err := captureOnce(ctx, url, cfg, recipe, backend, attempt < maxCaptureAttempts)
		if err != nil {
			return nil, errors.Wrapf(err, "%s capture failed", backend.Name())
		}
		if !defect {
			return snap, nil
		}
		log.With("attempt", attempt).With("url", url).Warn("layout defect detected (slider not initialized); retrying capture")
	}
}

// captureOnce runs one prepare+capture pass. When retryable is true it checks
// for a layout defect BEFORE serializing (so a doomed attempt doesn't waste
// backend work) and, if found, returns defect=true with no snapshot.
func captureOnce(ctx context.Context, url string, cfg runner.Config, recipe pipeline.Recipe, backend Backend, retryable bool) (snap *Snapshot, defect bool, err error) {
	session, err := runner.Start(ctx, url, cfg)
	if err != nil {
		return nil, false, err
	}
	defer session.Close()

	snap = &Snapshot{URL: url}
	if err := session.Run(recipe.Action(), captureTitle(&snap.Title)); err != nil {
		return nil, false, err
	}

	if retryable {
		if session.Run(detectDefect(&defect)) == nil && defect {
			return nil, true, nil
		}
	}

	if err := session.Run(backend.Action(snap)); err != nil {
		return nil, false, err
	}
	return snap, false, nil
}

func detectDefect(defect *bool) chromedp.ActionFunc {
	return func(ctx context.Context) error {
		var res struct {
			Defect bool `json:"defect"`
		}
		if err := js.DetectSliderDefect.Action(&res)(ctx); err != nil {
			return err
		}
		*defect = res.Defect
		return nil
	}
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
