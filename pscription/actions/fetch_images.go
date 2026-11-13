package actions

import (
	"context"
	"fmt"

	"github.com/goodblaster/parcher/pscription/act"
	"github.com/goodblaster/parcher/pscription/internal/log"
	js "github.com/goodblaster/parcher/pscription/scripts"
)

// FetchAndInjectImages extracts image URLs, fetches them server-side, and injects as data URIs
func FetchAndInjectImages() act.Func {
	return FetchAndInjectImagesCached(nil)
}

// FetchAndInjectImagesCached is FetchAndInjectImages that collects
// anything an earlier PrefetchImages already downloaded (cache may be
// nil, meaning fetch everything here).
func FetchAndInjectImagesCached(cache *PrefetchCache) act.Func {
	return func(ctx context.Context) error {
		// Step 1: Extract all image URLs from the page
		var urls []string
		err := act.Run(ctx, js.ExtractImageURLs.Action(&urls))
		if err != nil {
			return fmt.Errorf("failed to extract image URLs: %w", err)
		}

		if len(urls) == 0 {
			log.Info("no images to fetch")
			return nil
		}

		log.With("count", len(urls)).Info("fetching images")

		// Step 2: Fetch all images (parallel, deduplicated) as data URLs,
		// collecting whatever the prefetch already has in hand.
		var urlToDataURL map[string]string
		var fetched, failed, prefetched int
		if cache != nil {
			urlToDataURL, fetched, failed, prefetched = fetchAllCached(ctx, cache, urls, "image")
		} else {
			urlToDataURL, fetched, failed = fetchAllAsDataURLs(ctx, urls, "image")
		}

		log.With("fetched", fetched).With("failed", failed).
			With("prefetched", prefetched).Info("fetched images")

		if len(urlToDataURL) == 0 {
			return nil
		}

		// Step 3: Inject the fetched images back into the page
		var result injectStats
		err = act.Run(ctx, js.InjectImages.Action(&result, urlToDataURL))
		if err != nil {
			return fmt.Errorf("failed to inject images: %w", err)
		}
		log.With("injected", result.Injected).With("processed", result.Processed).Info("injected images")

		return nil
	}
}
