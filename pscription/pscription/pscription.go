// Package pscription models and runs a pscription: an ordered list of
// preparation steps and scripts executed against a live web page, producing
// a JSON report of what the scripts returned.
//
// A pscription is the recipe; running it (via the pscribe CLI or this package) is
// filling it. Steps come in a few kinds:
//
//   - a built-in pipeline step by name        e.g. dismiss-popups, wait-quiet
//   - a built-in extraction script "@name"    e.g. @article, @links
//   - a user script file "path.js"            your own page-context JS
//   - a wait step "wait:user" / "wait:2s"
//
// Script steps may capture their return value under a name ("script.js:out"),
// which lands in the report's Outputs. Every script receives the pscription's
// vars object as its first argument — the mechanism that keeps credentials
// (a separate secrets file) apart from the logic that applies them.
package pscription

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/goodblaster/errors"
	"github.com/goodblaster/parcher/pscription/actions"
	"github.com/goodblaster/parcher/pscription/pipeline"
	"github.com/goodblaster/parcher/pscription/runner"
	js "github.com/goodblaster/parcher/pscription/scripts"
)

// StepKind distinguishes how a step's Ref is resolved.
type StepKind int

const (
	KindBuiltin       StepKind = iota // registered pipeline step, by name
	KindBuiltinScript                 // embedded extraction script, "@name"
	KindScriptFile                    // user script file on disk
	KindWait                          // "wait:user" or "wait:<duration>"
)

// Step is one entry in a pscription.
type Step struct {
	Kind StepKind
	Ref  string // step name, @name, file path, or wait spec
	Out  string // capture the script's return value under this key (script kinds)
}

// Pscription is a runnable program.
type Pscription struct {
	Prep    string         // "none" | "minimal" | "staticize" (default minimal)
	Headful bool           // launch a visible browser (needed for wait:user)
	Profile string         // persistent Chrome user-data dir (reuse a login)
	Vars    map[string]any // passed as the first argument to every script
	Steps   []Step
}

// Options controls a run.
type Options struct {
	Timeout time.Duration
	// Profile overrides the pscription's Profile (persistent user-data dir).
	Profile string
}

// StepResult records how one step went.
type StepResult struct {
	Name  string `json:"name"`
	Ms    int64  `json:"ms"`
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

// Report is the JSON result of a run.
type Report struct {
	URL     string         `json:"url"`
	Title   string         `json:"title"`
	Outputs map[string]any `json:"outputs"`
	Steps   []StepResult   `json:"steps"`
	OK      bool           `json:"ok"`
}

// Run executes the pscription against url and returns a report. It returns a
// non-nil error only for setup failures (bad browser, bad prep name); step
// failures are recorded in the report and reflected in Report.OK.
func (p *Pscription) Run(ctx context.Context, url string, opts Options) (*Report, error) {
	prepName := p.Prep
	if prepName == "" {
		prepName = "minimal"
	}
	prep, ok := pipeline.PrepByName(prepName)
	if !ok {
		return nil, errors.Newf("unknown prep %q (want none, minimal, or staticize)", prepName)
	}

	cfg := runner.DefaultConfig()
	if opts.Timeout > 0 {
		cfg.Timeout = opts.Timeout
	}
	cfg.Headful = p.Headful
	cfg.UserDataDir = firstNonEmpty(opts.Profile, p.Profile)
	cfg.PreNavigate = prep.PreNavigateActions(url)

	session, err := runner.Start(ctx, url, cfg)
	if err != nil {
		return nil, err
	}
	defer session.Close()

	report := &Report{URL: url, Outputs: map[string]any{}, OK: true}

	// Prep phase: reuse the prep recipe's steps, honoring ContinueOnError.
	for _, s := range prep.Steps {
		res := runTimed(session, s.Name, s.Action)
		report.Steps = append(report.Steps, res)
		if !res.OK && !s.ContinueOnError {
			report.OK = false
			return report, nil
		}
	}

	// Capture the title once the page is prepared.
	_ = session.Run(func(c context.Context) error {
		return chromedp.Title(&report.Title).Do(c)
	})

	// Pscription steps.
	for _, step := range p.Steps {
		action, err := p.buildAction(step, report.Outputs)
		if err != nil {
			report.Steps = append(report.Steps, StepResult{Name: step.Ref, OK: false, Error: err.Error()})
			report.OK = false
			return report, nil
		}
		res := runTimed(session, stepLabel(step), action)
		report.Steps = append(report.Steps, res)
		if !res.OK {
			// A user/extraction step failing makes everything after it
			// unreliable — stop and report failure.
			report.OK = false
			return report, nil
		}
	}

	return report, nil
}

// Recipe converts the pscription into a pipeline.Recipe: the prep phase
// followed by the pscription's own steps. This lets a consumer that needs a
// live session at the end — notably parch, which serializes the prepared
// page — run a custom pscription and then do its own thing, all in one
// browser session. Script outputs are captured into the returned map.
//
// If the pscription specifies no prep, defaultPrep is used (parch passes
// "staticize" so the page is fully self-contained before serialization).
func (p *Pscription) Recipe(defaultPrep string) (pipeline.Recipe, map[string]any, error) {
	prepName := p.Prep
	if prepName == "" {
		prepName = defaultPrep
	}
	prep, ok := pipeline.PrepByName(prepName)
	if !ok {
		return pipeline.Recipe{}, nil, errors.Newf("unknown prep %q", prepName)
	}

	outputs := map[string]any{}
	recipe := prep
	for _, step := range p.Steps {
		action, err := p.buildAction(step, outputs)
		if err != nil {
			return pipeline.Recipe{}, nil, err
		}
		recipe = recipe.Append(pipeline.NamedStep(stepLabel(step), action))
	}
	return recipe, outputs, nil
}

// buildAction turns a Step into a chromedp action, wiring script output into
// the shared outputs map.
func (p *Pscription) buildAction(step Step, outputs map[string]any) (chromedp.ActionFunc, error) {
	switch step.Kind {
	case KindBuiltin:
		fn, ok := pipeline.Lookup(step.Ref)
		if !ok {
			return nil, errors.Newf("unknown step %q", step.Ref)
		}
		return fn().Action, nil

	case KindBuiltinScript:
		sf, ok := js.BuiltinScript(step.Ref)
		if !ok {
			return nil, errors.Newf("unknown built-in script @%s (have %v)", step.Ref, js.BuiltinScriptNames())
		}
		source, err := sf.Source()
		if err != nil {
			return nil, err
		}
		return p.scriptAction(source, step.Out, outputs), nil

	case KindScriptFile:
		source, err := os.ReadFile(step.Ref)
		if err != nil {
			return nil, errors.Wrapf(err, "cannot read script %q", step.Ref)
		}
		return p.scriptAction(string(source), step.Out, outputs), nil

	case KindWait:
		return p.waitAction(step.Ref)
	}
	return nil, errors.Newf("unhandled step kind for %q", step.Ref)
}

// scriptAction evaluates a user/built-in script with vars as its argument and
// stores the return value under out (if named).
func (p *Pscription) scriptAction(source, out string, outputs map[string]any) chromedp.ActionFunc {
	return func(ctx context.Context) error {
		var result any
		if err := js.EvalSource(ctx, source, &result, p.Vars); err != nil {
			return err
		}
		if out != "" {
			outputs[out] = result
		}
		return nil
	}
}

func (p *Pscription) waitAction(spec string) (chromedp.ActionFunc, error) {
	if spec == "user" {
		return actions.WaitForUser(""), nil
	}
	d, err := time.ParseDuration(spec)
	if err != nil {
		return nil, errors.Wrapf(err, "bad wait spec %q (want 'user' or a duration like 2s)", spec)
	}
	return func(ctx context.Context) error {
		return chromedp.Sleep(d).Do(ctx)
	}, nil
}

func runTimed(session *runner.Session, name string, action chromedp.ActionFunc) StepResult {
	start := time.Now()
	err := session.Run(action)
	res := StepResult{Name: name, Ms: time.Since(start).Milliseconds(), OK: err == nil}
	if err != nil {
		res.Error = err.Error()
	}
	return res
}

func stepLabel(step Step) string {
	switch step.Kind {
	case KindBuiltinScript:
		return "@" + step.Ref
	case KindWait:
		return "wait:" + step.Ref
	default:
		return step.Ref
	}
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// JSON renders the report as indented JSON.
func (r *Report) JSON() ([]byte, error) {
	return json.MarshalIndent(r, "", "  ")
}

// String implements a compact description for logs.
func (r *Report) String() string {
	return fmt.Sprintf("%s (%d steps, ok=%v)", r.URL, len(r.Steps), r.OK)
}
