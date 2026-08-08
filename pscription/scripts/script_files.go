package scripts

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"github.com/goodblaster/errors"
	"github.com/goodblaster/logos"
	"github.com/goodblaster/parcher/pscription/internal/config"
)

type ScriptFile string

// Source returns the script's JavaScript source, honoring dev-mode
// (gb_env=dev) reload from disk.
func (sf ScriptFile) Source() (string, error) {
	if os.Getenv("gb_env") == "dev" {
		b, err := os.ReadFile(filepath.Join("scripts", string(sf)))
		if err != nil {
			return "", errors.Wrapf(err, "failed to read script file %q", sf)
		}
		return string(b), nil
	}
	b, err := Files.ReadFile(string(sf))
	if err != nil {
		return "", errors.Wrapf(err, "failed to read script file %q", sf)
	}
	return string(b), nil
}

func (sf ScriptFile) Action(res any, params ...any) chromedp.ActionFunc {
	return func(ctx context.Context) error {
		if config.ProfileJson {
			start := time.Now()
			defer func() {
				logos.With("script", sf).With("duration", time.Since(start)).Info("script execution time")
			}()
		}

		var script []byte
		var err error

		if os.Getenv("gb_env") == "dev" {
			// Read from disk, instead.
			script, err = os.ReadFile(filepath.Join("scripts", string(sf)))
		} else {
			script, err = Files.ReadFile(string(sf))
		}

		if err != nil {
			return errors.Wrapf(err, "failed to read script file %q", sf)
		}

		if err := EvalSource(ctx, string(script), res, params...); err != nil {
			return errors.Wrapf(err, "script file %q", sf)
		}
		return nil
	}
}

// EvalSource evaluates a JavaScript source string in the page context and
// unmarshals its return value into res (may be nil). The source must be a
// parenthesized function expression — `(() => {...})`, `((a) => {...})`, or
// the async forms — exactly like the embedded scripts. params are
// JSON-encoded and passed as arguments. Async functions (first code token
// `async function` / `async (`) are awaited.
//
// This is the shared execution core: embedded ScriptFiles and user-supplied
// script files both run through it, so external scripts behave identically.
func EvalSource(ctx context.Context, source string, res any, params ...any) error {
	args, err := encodeParams(params...)
	if err != nil {
		return err
	}

	head := stripLeadingComments(source)
	isAsync := strings.HasPrefix(head, "async function") || strings.HasPrefix(head, "async (") || strings.HasPrefix(head, "async(")

	expression := fmt.Sprintf("(%s)(%s)", source, args)

	if isAsync {
		v, exceptionDetails, err := runtime.Evaluate(expression).
			WithAwaitPromise(true).
			WithReturnByValue(true).
			Do(ctx)
		if exceptionDetails != nil {
			return fmt.Errorf("script threw exception: %v", exceptionDetails)
		}
		if err != nil {
			return errors.Wrap(err, "failed to evaluate async script")
		}
		if res != nil && v.Value != nil {
			if err := json.Unmarshal(v.Value, res); err != nil {
				return errors.Wrap(err, "failed to unmarshal script result")
			}
		}
		return nil
	}

	if err := chromedp.Evaluate(expression, res).Do(ctx); err != nil {
		return errors.Wrap(err, "failed to evaluate script")
	}
	return nil
}

func encodeParams(params ...any) (string, error) {
	var ps []string
	for _, p := range params {
		b, err := json.Marshal(p)
		if err != nil {
			return "", err
		}
		ps = append(ps, string(b))
	}
	return strings.Join(ps, ", "), nil
}

// stripLeadingComments returns the script with leading whitespace, // line
// comments, and /* */ block comments removed, so the first code token can
// be inspected.
func stripLeadingComments(s string) string {
	for {
		s = strings.TrimSpace(s)
		switch {
		case strings.HasPrefix(s, "//"):
			if i := strings.IndexByte(s, '\n'); i >= 0 {
				s = s[i+1:]
			} else {
				return ""
			}
		case strings.HasPrefix(s, "/*"):
			if i := strings.Index(s, "*/"); i >= 0 {
				s = s[i+2:]
			} else {
				return ""
			}
		default:
			return s
		}
	}
}

func (sf ScriptFile) prep(params ...any) (string, error) {
	return encodeParams(params...)
}

// ActionNoError runs the script like Action (including async handling and
// dev-mode reload) but logs failures instead of returning them.
func (sf ScriptFile) ActionNoError(res any, params ...any) chromedp.ActionFunc {
	return func(ctx context.Context) error {
		if err := sf.Action(res, params...)(ctx); err != nil {
			logos.With("error", err).With("script", sf).Error("failed to evaluate script")
		}
		return nil
	}
}
