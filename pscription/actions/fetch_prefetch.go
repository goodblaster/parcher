package actions

import (
	"context"
	"sync"

	"github.com/goodblaster/parcher/pscription/act"
	"github.com/goodblaster/parcher/pscription/internal/log"
	js "github.com/goodblaster/parcher/pscription/scripts"
)

// PrefetchCache memoizes server-side resource fetches so they can START
// early — as soon as the URLs are known — and be COLLECTED late, when the
// pipeline is ready to inject them. Everything between those two points
// (scrolling, layout settling, freezing iframes) is time the network
// would otherwise sit idle.
//
// It is a cache, never a source of truth: whatever the page turns out to
// reference at injection time is what gets fetched, so a URL that
// appears late is still fetched normally and one that disappears is
// simply unused.
type PrefetchCache struct {
	mu    sync.Mutex
	entry map[string]*prefetchEntry
	gate  *hostGate
}

type prefetchEntry struct {
	ctx     context.Context
	done    chan struct{}
	dataURL string
	err     error
}

func NewPrefetchCache() *PrefetchCache {
	return &PrefetchCache{entry: map[string]*prefetchEntry{}, gate: newHostGate(perHostLimit(), maxFetchConcurrency)}
}

// start kicks off a fetch unless this session already recorded it, returning
// whether a new download was queued. A recipe may be reused after a capture
// retry: an ended browser context must not poison the new session's cache.
func (c *PrefetchCache) start(ctx context.Context, url, userAgent string) bool {
	c.mu.Lock()
	if e, ok := c.entry[url]; ok && (e.ctx.Err() == nil || ctx.Err() != nil) {
		c.mu.Unlock()
		return false
	}
	e := &prefetchEntry{ctx: ctx, done: make(chan struct{})}
	c.entry[url] = e
	c.mu.Unlock()

	go func() {
		defer close(e.done)
		release, ok := c.gate.acquire(ctx, url)
		if !ok {
			e.err = ctx.Err()
			return
		}
		defer release()
		e.dataURL, e.err = fetchAsDataURL(ctx, fetchClient, url, userAgent)
	}()
	return true
}

// take waits for a queued result. Failures are cached too: a refused or
// timed-out request must not be downloaded again during the same capture.
func (c *PrefetchCache) take(ctx context.Context, url string) (string, error) {
	c.mu.Lock()
	e := c.entry[url]
	c.mu.Unlock()
	select {
	case <-e.done:
		return e.dataURL, e.err
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

// PrefetchImages starts fetching every image URL the page references so
// far and returns IMMEDIATELY, leaving the downloads in flight. Pair it
// with FetchAndInjectImagesCached later in the recipe.
func PrefetchImages(cache *PrefetchCache) act.Func {
	return func(ctx context.Context) error {
		var urls []string
		if err := act.Run(ctx, js.ExtractImageURLs.Action(&urls)); err != nil {
			return nil // speculative: never fail the capture
		}
		userAgent := fetchUserAgent(ctx)
		for _, u := range urls {
			cache.start(ctx, u, userAgent)
		}
		log.With("count", len(urls)).Debug("prefetching images")
		return nil
	}
}

// PrefetchBackgrounds overlaps background downloads with layout and media
// preparation. Images and backgrounds share a cache and concurrency limits.
func PrefetchBackgrounds(cache *PrefetchCache) act.Func {
	return func(ctx context.Context) error {
		var urls []string
		if err := js.ExtractBackgroundURLs.Action(&urls).Do(ctx); err != nil {
			return err
		}
		userAgent := fetchUserAgent(ctx)
		for _, u := range urls {
			cache.start(ctx, u, userAgent)
		}
		return nil
	}
}

// Enqueue every missing URL BEFORE collecting any result. A slow prefetched
// image must never prevent a newly discovered image from starting.
func fetchAllCached(ctx context.Context, cache *PrefetchCache, urls []string, what string) (map[string]string, int, int, int) {
	return collectCached(ctx, cache, urls, what, fetchUserAgent(ctx))
}

func collectCached(ctx context.Context, cache *PrefetchCache, urls []string, what, userAgent string) (map[string]string, int, int, int) {
	out := make(map[string]string, len(urls))
	existing := map[string]bool{}
	for _, u := range urls {
		if _, seen := existing[u]; seen {
			continue
		}
		existing[u] = !cache.start(ctx, u, userAgent)
	}
	prefetched, failed := 0, 0
	for u, wasPrefetched := range existing {
		data, err := cache.take(ctx, u)
		if err != nil {
			log.With("url", u).With("reason", err.Error()).Debug("failed to fetch " + what)
			failed++
			continue
		}
		out[u] = data
		if wasPrefetched {
			prefetched++
		}
	}
	return out, len(out), failed, prefetched
}
