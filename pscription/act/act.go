// Package act is the action vocabulary of this module: a step is a
// func(ctx) error, and a handful of helpers run the browser operations the
// steps are built from.
//
// It exists so that the browser driver is an implementation detail of ONE
// package. chromedp's action type used to be this module's action type —
// every step, backend and script wrapper was a chromedp.ActionFunc — so when
// chromedp v0.17+ replaced its API wholesale (generic Action[T], protocol
// commands as values), the change reached 44 files. Everything outside this
// package and runner now depends only on act.Func and on cdproto commands
// sent through chromedp.Call.
//
// The context handed to a Func is the browser session's: chromedp.Call and
// chromedp.Run work on it directly.
package act

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

// Action is anything that can run against the session.
type Action interface {
	Do(ctx context.Context) error
}

// Func is the concrete action type: a step, a backend, a script wrapper.
type Func func(ctx context.Context) error

// Do runs the action.
func (f Func) Do(ctx context.Context) error { return f(ctx) }

// Run starts the browser and its tab if they are not up yet, then runs the
// actions in order, stopping at the first error.
func Run(ctx context.Context, actions ...Action) error {
	if err := chromedp.Do(ctx); err != nil {
		return err
	}
	for _, a := range actions {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := a.Do(ctx); err != nil {
			return err
		}
	}
	return nil
}

// Sleep pauses, ending early with the context.
func Sleep(d time.Duration) Func {
	return func(ctx context.Context) error {
		t := time.NewTimer(d)
		defer t.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			return nil
		}
	}
}

// Evaluate runs a JavaScript expression in the page and decodes its value,
// returned by value as JSON, into res. res may be nil to discard the value;
// undefined and null leave res untouched. A thrown exception is an error.
func Evaluate(expression string, res any) Func {
	return func(ctx context.Context) error { return eval(ctx, expression, res, false) }
}

// EvaluateAsync is Evaluate for an expression that yields a promise: the
// promise is awaited and its resolved value decoded.
func EvaluateAsync(expression string, res any) Func {
	return func(ctx context.Context) error { return eval(ctx, expression, res, true) }
}

func eval(ctx context.Context, expression string, res any, await bool) error {
	params := runtime.EvaluateParams{Expression: expression, ReturnByValue: new(true)}
	if await {
		params.AwaitPromise = new(true)
	}
	out, err := chromedp.Call(ctx, runtime.Evaluate, params)
	if err != nil {
		return err
	}
	if out.ExceptionDetails != nil {
		return exceptionError(out.ExceptionDetails)
	}
	if res == nil || out.Result == nil || len(out.Result.Value) == 0 {
		return nil
	}
	if string(out.Result.Value) == "null" {
		return nil
	}
	return json.Unmarshal(out.Result.Value, res)
}

func exceptionError(d *runtime.ExceptionDetails) error {
	msg := d.Text
	if d.Exception != nil && d.Exception.Description != "" {
		msg = d.Exception.Description
	}
	if msg == "" {
		msg = "unknown exception"
	}
	return fmt.Errorf("script threw exception: %s (line %d, column %d)", msg, d.LineNumber, d.ColumnNumber)
}

// Title reads the document title.
func Title(title *string) Func {
	return Evaluate(`document.title`, title)
}
