// Package pipeline composes named browser-automation steps into recipes.
//
// A Step is a chromedp action with a name; a Recipe is an ordered list of
// steps executed with per-step logging and timing. Recipes are Go-composed
// for now; the step registry exists so recipes can later be declared as
// data (e.g. per-site YAML overrides) without changing this package's model.
package pipeline

import (
	"context"
	"time"

	"github.com/goodblaster/parcher/pscription/act"
	"github.com/goodblaster/parcher/pscription/internal/log"
)

// Step is a single named unit of browser work.
type Step struct {
	// Name identifies the step in logs and (later) in data-defined recipes.
	Name string

	// Action performs the work.
	Action act.Func

	// ContinueOnError logs the step's error and moves on instead of
	// aborting the recipe.
	ContinueOnError bool
}

// PreNavFunc builds a pre-navigation action for the target URL. These run
// before the page exists — cookie seeding, evaluate-on-new-document
// scripts, request blocking.
type PreNavFunc func(url string) act.Action

// Recipe is an ordered list of steps, optionally with pre-navigation setup.
type Recipe struct {
	Name        string
	PreNavigate []PreNavFunc
	Steps       []Step
}

// PreNavigateActions materializes the pre-navigation actions for url.
func (r Recipe) PreNavigateActions(url string) []act.Action {
	out := make([]act.Action, 0, len(r.PreNavigate))
	for _, fn := range r.PreNavigate {
		out = append(out, fn(url))
	}
	return out
}

// Append returns a copy of the recipe with extra steps added.
func (r Recipe) Append(steps ...Step) Recipe {
	out := Recipe{
		Name:        r.Name,
		PreNavigate: r.PreNavigate,
		Steps:       make([]Step, 0, len(r.Steps)+len(steps)),
	}
	out.Steps = append(out.Steps, r.Steps...)
	out.Steps = append(out.Steps, steps...)
	return out
}

// Action returns the whole recipe as a single chromedp action, so a recipe
// can run anywhere an action can (runner.Run, another recipe, ...).
func (r Recipe) Action() act.Func {
	return func(ctx context.Context) error {
		for _, step := range r.Steps {
			start := time.Now()
			err := step.Action.Do(ctx)
			logger := log.With("recipe", r.Name).With("step", step.Name).With("duration", time.Since(start).Round(time.Millisecond).String())
			if err != nil {
				if !step.ContinueOnError {
					logger.WithError(err).Error("step failed")
					return err
				}
				logger.WithError(err).Warn("step failed; continuing")
				continue
			}
			logger.Debug("step done")
		}
		return nil
	}
}

// NamedStep wraps an action as a named step (ContinueOnError false).
func NamedStep(name string, action act.Func) Step {
	return Step{Name: name, Action: action}
}

// Sleep is a named fixed pause. Prefer condition-based waits (network idle,
// images loaded); use Sleep only where no observable condition exists.
func Sleep(name string, d time.Duration) Step {
	return Step{
		Name: name,
		Action: func(ctx context.Context) error {
			return act.Sleep(d).Do(ctx)
		},
	}
}

// SleepIf is Sleep that only pays when cond says the wait can matter —
// for pauses that exist to let some EARLIER action's effects play out. If
// that action did nothing, there is nothing to wait for.
func SleepIf(name string, d time.Duration, cond func() bool) Step {
	return Step{
		Name: name,
		Action: func(ctx context.Context) error {
			if !cond() {
				return nil
			}
			return act.Sleep(d).Do(ctx)
		},
	}
}
