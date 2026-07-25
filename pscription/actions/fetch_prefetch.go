package actions

import (
	"context"
	"sync"

	"github.com/chromedp/chromedp"
	"github.com/goodblaster/logos"
	js "github.com/goodblaster/pscription/scripts"
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
}

type prefetchEntry struct {
	done    chan struct{}
	dataURL string
	err     error
}

func NewPrefetchCache() *PrefetchCache {
	return &PrefetchCache{entry: map[string]*prefetchEntry{}}
}

// start kicks off a fetch for url unless one is already recorded.
func (c *PrefetchCache) start(ctx context.Context, url string, gate *hostGate) {
	c.mu.Lock()
	if _, ok := c.entry[url]; ok {
		c.mu.Unlock()
		return
	}
	e := &prefetchEntry{done: make(chan struct{})}
	c.entry[url] = e
	c.mu.Unlock()

	go func() {
		defer close(e.done)
		release, ok := gate.acquire(ctx, url)
		if !ok {
			e.err = ctx.Err()
			return
		}
		defer release()
		e.dataURL, e.err = fetchAsDataURL(ctx, fetchClient, url)
	}()
}

// take returns a prefetched result, waiting if the fetch is still in
// flight. ok is false when the URL was never prefetched.
func (c *PrefetchCache) take(ctx context.Context, url string) (string, bool) {
	c.mu.Lock()
	e, ok := c.entry[url]
	c.mu.Unlock()
	if !ok {
		return "", false
	}
	select {
	case <-e.done:
	case <-ctx.Done():
		return "", false
	}
	if e.err != nil {
		return "", false // fall back to a normal fetch
	}
	return e.dataURL, true
}

// PrefetchImages starts fetching every image URL the page references so
// far and returns IMMEDIATELY, leaving the downloads in flight. Pair it
// with FetchAndInjectImagesCached later in the recipe.
func PrefetchImages(cache *PrefetchCache) chromedp.ActionFunc {
	return func(ctx context.Context) error {
		var urls []string
		if err := chromedp.Run(ctx, js.ExtractImageURLs.Action(&urls)); err != nil {
			return nil // speculative: never fail the capture
		}
		gate := newHostGate(perHostLimit(), maxFetchConcurrency)
		for _, u := range urls {
			cache.start(ctx, u, gate)
		}
		logos.With("count", len(urls)).Debug("prefetching images")
		return nil
	}
}

// fetchAllCached is fetchAllAsDataURLs that first collects anything a
// PrefetchCache already has (or has in flight), fetching only the rest.
func fetchAllCached(ctx context.Context, cache *PrefetchCache, urls []string, what string) (map[string]string, int, int, int) {
	out := make(map[string]string, len(urls))
	var missing []string
	seen := map[string]bool{}
	prefetched := 0
	for _, u := range urls {
		if seen[u] {
			continue
		}
		seen[u] = true
		if dataURL, ok := cache.take(ctx, u); ok {
			out[u] = dataURL
			prefetched++
			continue
		}
		missing = append(missing, u)
	}
	fetched := prefetched
	failed := 0
	if len(missing) > 0 {
		late, f, fail := fetchAllAsDataURLs(ctx, missing, what)
		for k, v := range late {
			out[k] = v
		}
		fetched += f
		failed = fail
	}
	return out, fetched, failed, prefetched
}
