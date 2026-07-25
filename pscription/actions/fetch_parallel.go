package actions

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/goodblaster/logos"
)

// fetchConcurrency bounds parallel server-side resource fetches. Fetching
// was serial before, and a page with dozens of CDN images paid ~1s each
// in sequence (54s of a 70s test-site-1 capture was this loop). Eight
// workers keeps per-host load polite while collapsing the wall time to
// network-latency depth.
const fetchConcurrency = 8

// fetchAllAsDataURLs fetches urls in parallel — deduplicated, bounded by
// fetchConcurrency, aborted by ctx — and returns url → data URL for the
// successes plus fetched/failed counts. Per-URL failures are logged and
// counted, never fatal: a missing image beats a failed capture. what
// names the resource kind in logs.
func fetchAllAsDataURLs(ctx context.Context, urls []string, what string) (map[string]string, int, int) {
	client := &http.Client{Timeout: 30 * time.Second}
	out := make(map[string]string, len(urls))
	seen := make(map[string]bool, len(urls))
	var (
		mu              sync.Mutex
		wg              sync.WaitGroup
		fetched, failed int
	)
	sem := make(chan struct{}, fetchConcurrency)

	for _, url := range urls {
		if seen[url] {
			continue
		}
		seen[url] = true
		wg.Add(1)
		go func(url string) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				mu.Lock()
				failed++
				mu.Unlock()
				return
			}
			defer func() { <-sem }()

			dataURL, err := fetchAsDataURL(ctx, client, url)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				logos.With("url", url).With("error", err).Debug("failed to fetch " + what)
				failed++
				return
			}
			out[url] = dataURL
			fetched++
		}(url)
	}
	wg.Wait()
	return out, fetched, failed
}
