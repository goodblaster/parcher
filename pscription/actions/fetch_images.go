package actions

import (
	"context"
	"fmt"

	"github.com/chromedp/chromedp"
	"github.com/goodblaster/logos"
	js "github.com/goodblaster/pscription/scripts"
)

// FetchAndInjectImages extracts image URLs, fetches them server-side, and injects as data URIs
func FetchAndInjectImages() chromedp.ActionFunc {
	return func(ctx context.Context) error {
		// Step 1: Extract all image URLs from the page
		var urls []string
		err := chromedp.Run(ctx, js.ExtractImageURLs.Action(&urls))
		if err != nil {
			return fmt.Errorf("failed to extract image URLs: %w", err)
		}

		if len(urls) == 0 {
			logos.Info("no images to fetch")
			return nil
		}

		logos.With("count", len(urls)).Info("fetching images")

		// Step 2: Fetch all images (parallel, deduplicated) as data URLs
		urlToDataURL, fetched, failed := fetchAllAsDataURLs(ctx, urls, "image")

		logos.With("fetched", fetched).With("failed", failed).Info("fetched images")

		if len(urlToDataURL) == 0 {
			return nil
		}

		// Step 3: Inject the fetched images back into the page
		var result map[string]interface{}
		err = chromedp.Run(ctx, js.InjectImages.Action(&result, urlToDataURL))
		if err != nil {
			return fmt.Errorf("failed to inject images: %w", err)
		}

		injected := int(result["injected"].(float64))
		processed := int(result["processed"].(float64))
		logos.With("injected", injected).With("processed", processed).Info("injected images")

		return nil
	}
}
