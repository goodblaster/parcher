package actions

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/goodblaster/parcher/pscription/act"
	"github.com/goodblaster/parcher/pscription/agents"
	"github.com/goodblaster/parcher/pscription/internal/log"
	js "github.com/goodblaster/parcher/pscription/scripts"
)

// FetchAndInjectBackgrounds extracts background image URLs, fetches them server-side, and injects as data URIs
func FetchAndInjectBackgrounds() act.Func {
	return func(ctx context.Context) error {
		// Step 1: Extract all background image URLs from the page
		var urls []string
		err := act.Run(ctx, js.ExtractBackgroundURLs.Action(&urls))
		if err != nil {
			return fmt.Errorf("failed to extract background URLs: %w", err)
		}

		if len(urls) == 0 {
			log.Info("no background images to fetch")
			return nil
		}

		log.With("count", len(urls)).Info("fetching background images")

		// Step 2: Fetch all images (parallel, deduplicated) as data URLs
		urlToDataURL, fetched, failed := fetchAllAsDataURLs(ctx, urls, "background image")

		log.With("fetched", fetched).With("failed", failed).Info("fetched background images")

		if len(urlToDataURL) == 0 {
			return nil
		}

		// Step 3: Inject the fetched images back into the page
		var result injectStats
		err = act.Run(ctx, js.InjectBackgroundImages.Action(&result, urlToDataURL))
		if err != nil {
			return fmt.Errorf("failed to inject background images: %w", err)
		}
		log.With("injected", result.Injected).With("processed", result.Processed).Info("injected background images")

		return nil
	}
}

// injectStats is what the inject_*.js scripts return. Decoded into a struct
// rather than type-asserted out of a map: an unexpected result shape used to
// panic the whole capture on `result["injected"].(float64)`.
type injectStats struct {
	Injected  int `json:"injected"`
	Processed int `json:"processed"`
}

// maxResourceBytes bounds one server-side image or font fetch. A resource
// becomes a base64 data: URI held in memory and shipped into the page over
// CDP, so an endless or mislabelled response (a video behind an <img>, a
// stream) must not be read to the end.
const maxResourceBytes = 64 << 20

// readCapped reads r to EOF, failing rather than truncating past limit — a
// truncated image embeds as a corrupt one.
func readCapped(r io.Reader, limit int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("larger than the %d MB resource cap", limit>>20)
	}
	return data, nil
}

// fetchUserAgent is what the server-side fetchers announce: the session
// browser's own user agent, so a resource request looks like it came from
// the browser that loaded the page (a CDN that serves by user agent then
// answers both the same way). Falls back to a static Chrome string when the
// page cannot be asked. Call it ONCE per batch, never per request — it is a
// round trip to the browser.
func fetchUserAgent(ctx context.Context) string {
	if ua, err := browserUserAgent(ctx); err == nil && ua != "" {
		return ua
	}
	return agents.Current
}

// fetchAsDataURL fetches a URL and converts it to a data URL. The request
// carries ctx so a hung server dies with the capture deadline, not after
// the full client timeout.
func fetchAsDataURL(ctx context.Context, client *http.Client, url, userAgent string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}

	// Set headers to mimic a browser
	req.Header.Set("User-Agent", userAgent)
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

	data, err := readCapped(resp.Body, maxResourceBytes)
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
