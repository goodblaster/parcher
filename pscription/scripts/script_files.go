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
	"github.com/goodblaster/pscription/internal/config"
)

type ScriptFile string

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

		ps, err := sf.prep(params...)
		if err != nil {
			return errors.Wrapf(err, "failed to prepare params for script file %q", sf)
		}

		// Check if script defines an async function. Leading // and /* */
		// comments are skipped so documented scripts detect correctly.
		scriptStr := string(script)
		isAsync := strings.HasPrefix(stripLeadingComments(scriptStr), "async function")

		if isAsync {
			// For async functions, use runtime.Evaluate with awaitPromise

			// Wrap the function call in an immediately invoked expression
			expression := fmt.Sprintf("(%s)(%s)", script, ps)

			// Evaluate with awaitPromise to wait for the Promise to resolve
			var v *runtime.RemoteObject
			var exceptionDetails *runtime.ExceptionDetails
			v, exceptionDetails, err = runtime.Evaluate(expression).
				WithAwaitPromise(true).
				WithReturnByValue(true). // Get the actual JSON value, not just a reference
				Do(ctx)

			if exceptionDetails != nil {
				return fmt.Errorf("script file %q threw exception: %v", sf, exceptionDetails)
			}

			if err != nil {
				return errors.Wrapf(err, "failed to evaluate async script file %q", sf)
			}

			// Unmarshal the result
			if res != nil && v.Value != nil {
				err = json.Unmarshal(v.Value, res)
				if err != nil {
					return errors.Wrapf(err, "failed to unmarshal result from script file %q", sf)
				}
			}
		} else {
			// For non-async, use regular chromedp.Evaluate
			evalStr := fmt.Sprintf("(%s)(%s)", script, ps)
			err = chromedp.Evaluate(evalStr, res).Do(ctx)
			if err != nil {
				return errors.Wrapf(err, "failed to evaluate script file %q", sf)
			}
		}

		return nil
	}
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
