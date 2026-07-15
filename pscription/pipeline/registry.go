package pipeline

import (
	"sort"
	"sync"
)

// StepFunc constructs a step. Registered constructors are the bridge to
// data-defined recipes: once recipes are loaded from files, a recipe entry's
// name is resolved through this registry.
type StepFunc func() Step

var (
	regMu    sync.RWMutex
	registry = map[string]StepFunc{}
)

// Register makes a step constructor available by name. Later registrations
// replace earlier ones, so applications can override built-in steps.
func Register(name string, fn StepFunc) {
	regMu.Lock()
	defer regMu.Unlock()
	registry[name] = fn
}

// Lookup returns the constructor registered under name.
func Lookup(name string) (StepFunc, bool) {
	regMu.RLock()
	defer regMu.RUnlock()
	fn, ok := registry[name]
	return fn, ok
}

// Names lists all registered step names, sorted.
func Names() []string {
	regMu.RLock()
	defer regMu.RUnlock()
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
