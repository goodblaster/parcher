package actions

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/goodblaster/logos"
	js "github.com/goodblaster/webrun/scripts"
)

// FetchAndInjectBackgrounds extracts background image URLs, fetches them server-side, and injects as data URIs
func FetchAndInjectBackgrounds() chromedp.ActionFunc {
	return func(ctx context.Context) error {
		// Step 1: Extract all background image URLs from the page
		var urls []string
		err := chromedp.Run(ctx, js.ExtractBackgroundURLs.Action(&urls))
		if err != nil {
			return fmt.Errorf("failed to extract background URLs: %w", err)
		}

		if len(urls) == 0 {
			logos.Info("no background images to fetch")
			return nil
		}

		logos.With("count", len(urls)).Info("fetching background images")

		// Step 2: Fetch all images and convert to data URLs
		urlToDataURL := make(map[string]string)
		client := &http.Client{
			Timeout: 30 * time.Second,
		}

		fetched := 0
		failed := 0

		for _, url := range urls {
			dataURL, err := fetchAsDataURL(client, url)
			if err != nil {
				logos.With("url", url).With("error", err).Debug("failed to fetch background image")
				failed++
				continue
			}
			urlToDataURL[url] = dataURL
			fetched++
		}

		logos.With("fetched", fetched).With("failed", failed).Info("fetched background images")

		if len(urlToDataURL) == 0 {
			return nil
		}

		// Step 3: Inject the fetched images back into the page
		var result map[string]interface{}
		err = chromedp.Run(ctx, js.InjectBackgroundImages.Action(&result, urlToDataURL))
		if err != nil {
			return fmt.Errorf("failed to inject background images: %w", err)
		}

		injected := int(result["injected"].(float64))
		processed := int(result["processed"].(float64))
		logos.With("injected", injected).With("processed", processed).Info("injected background images")

		return nil
	}
}

// fetchAsDataURL fetches a URL and converts it to a data URL
func fetchAsDataURL(client *http.Client, url string) (string, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}

	// Set headers to mimic a browser
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")
	req.Header.Set("Accept", "image/webp,image/apng,image/*,*/*;q=0.8")
	// Note: do NOT set Accept-Encoding manually — that disables Go's transparent
	// gzip decompression and gzip-served bodies (commonly SVGs) would be
	// embedded as compressed bytes.

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	// Read the image data
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	// Get content type
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		// Guess from URL or default to image/jpeg
		if strings.HasSuffix(strings.ToLower(url), ".png") {
			contentType = "image/png"
		} else if strings.HasSuffix(strings.ToLower(url), ".gif") {
			contentType = "image/gif"
		} else if strings.HasSuffix(strings.ToLower(url), ".webp") {
			contentType = "image/webp"
		} else if strings.HasSuffix(strings.ToLower(url), ".svg") {
			contentType = "image/svg+xml"
		} else {
			contentType = "image/jpeg"
		}
	}

	// Convert to data URL
	encoded := base64.StdEncoding.EncodeToString(data)
	dataURL := fmt.Sprintf("data:%s;base64,%s", contentType, encoded)

	return dataURL, nil
}
