package actions

import (
	"context"
	"sync"
)

// downloadCache deduplicates a batch, including failures, and bounds requests
// per host and globally. Callers may download concurrently but apply results
// in source order so scheduling never changes the CSS cascade.
type downloadCache[T any] struct {
	mu      sync.Mutex
	entries map[string]*downloadResult[T]
	gate    *hostGate
	load    func(context.Context, string) (T, error)
}

type downloadResult[T any] struct {
	done  chan struct{}
	value T
	err   error
}

func newDownloadCache[T any](load func(context.Context, string) (T, error)) *downloadCache[T] {
	return &downloadCache[T]{entries: map[string]*downloadResult[T]{}, gate: newHostGate(perHostLimit(), maxFetchConcurrency), load: load}
}

func (c *downloadCache[T]) get(ctx context.Context, url string) (T, error) {
	c.mu.Lock()
	result, exists := c.entries[url]
	if !exists {
		result = &downloadResult[T]{done: make(chan struct{})}
		c.entries[url] = result
	}
	c.mu.Unlock()
	if exists {
		select {
		case <-result.done:
			return result.value, result.err
		case <-ctx.Done():
			var zero T
			return zero, ctx.Err()
		}
	}
	defer close(result.done)
	release, ok := c.gate.acquire(ctx, url)
	if !ok {
		result.err = ctx.Err()
		return result.value, result.err
	}
	defer release()
	result.value, result.err = c.load(ctx, url)
	return result.value, result.err
}

// parallelEach joins all work before the caller reads its indexed results.
func parallelEach(n int, work func(int)) {
	var group sync.WaitGroup
	for i := 0; i < n; i++ {
		group.Add(1)
		go func() { defer group.Done(); work(i) }()
	}
	group.Wait()
}
