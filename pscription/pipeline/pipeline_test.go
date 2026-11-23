package pipeline

import (
	"context"
	"errors"
	"testing"
)

func TestGuardStopsBeforeDestructiveStep(t *testing.T) {
	state := 0
	recipe := Recipe{Steps: []Step{
		{Name: "gate-arrives", Action: func(context.Context) error { state = 1; return nil }},
		{Name: "must-not-run", ContinueOnError: true, Action: func(context.Context) error { state = 2; return nil }},
	}}
	err := recipe.ActionWithGuard(func(context.Context) (bool, error) { return state == 0, nil })(context.Background())
	if err != nil || state != 1 {
		t.Fatalf("state=%d err=%v", state, err)
	}
	failure := errors.New("detector failed")
	err = recipe.ActionWithGuard(func(context.Context) (bool, error) { return false, failure })(context.Background())
	if !errors.Is(err, failure) {
		t.Fatalf("guard error swallowed: %v", err)
	}
}

func TestGuardRunsWithoutPrep(t *testing.T) {
	checked := false
	err := (Recipe{}).ActionWithGuard(func(context.Context) (bool, error) { checked = true; return false, nil })(context.Background())
	if err != nil || !checked {
		t.Fatal("empty/custom recipe bypassed guard")
	}
}
