package actions

import (
	"context"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"regexp"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
	"github.com/goodblaster/logos"
	js "github.com/goodblaster/pscription/scripts"
)

// relativeURLRef matches a CSS url() reference and captures the opening quote
// and the target, so a relative target can be resolved without disturbing the
// quoting around it. The closing quote is not tied to the opening one with a
// backreference — Go's RE2 has none — so the capture is used to rebuild the
// pair instead.
var relativeURLRef = regexp.MustCompile(`url\(\s*(["']?)([^"')]+)["']?\s*\)`)

// InlineCrossOriginStylesheets fetches stylesheets the page cannot read
// through the CSSOM and inlines their text beside the <link> that loaded
// them.
//
// A cross-origin stylesheet without CORS headers loads and renders normally,
// but `sheet.cssRules` throws, so every step that inspects the CSSOM is blind
// to it. That cost real fidelity: on test-site-2 the Google Fonts
// sheets were unreadable, so the @font-face rules for Orbitron — the site's
// heading typeface — were invisible to font extraction and the archive fell
// back to a system font. parchmint's former SingleFile backend never hit
// this, because it re-fetched such sheets through its own page-context fetch
// bridge; every backend built on Chrome's snapshot needs the same thing done
// for it, which is why the gap only surfaced once that backend was retired.
//
// Fetching happens server-side, which sidesteps CORS entirely. Relative
// url() references are resolved against the sheet's own address first,
// because the inlined copy sits in the DOCUMENT and would otherwise resolve
// against the page.
//
// Failures are logged and skipped: the <link> is left in place, so the page
// renders exactly as it did before — just as unreadable as it was.
func InlineCrossOriginStylesheets() chromedp.ActionFunc {
	return func(ctx context.Context) error {
		var result struct {
			Hrefs []string `json:"hrefs"`
		}
		if err := chromedp.Run(ctx, js.ListCrossOriginSheets.Action(&result)); err != nil {
			return fmt.Errorf("failed to list cross-origin stylesheets: %w", err)
		}
		if len(result.Hrefs) == 0 {
			return nil
		}

		client := &http.Client{Timeout: 15 * time.Second}
		inlined, failed := 0, 0

		for _, href := range result.Hrefs {
			css, err := fetchStylesheet(ctx, client, href)
			if err != nil {
				logos.With("href", href).With("error", err).Debug("could not fetch cross-origin stylesheet")
				failed++
				continue
			}

			var res struct {
				Inlined bool   `json:"inlined"`
				Reason  string `json:"reason"`
			}
			if err := chromedp.Run(ctx, js.InlineSheetText.Action(&res, href, absolutizeRefs(css, href))); err != nil {
				logos.With("href", href).With("error", err).Debug("could not inline stylesheet")
				failed++
				continue
			}
			if res.Inlined {
				inlined++
			}
		}

		logos.With("inlined", inlined).With("failed", failed).With("found", len(result.Hrefs)).
			Debug("inlined cross-origin stylesheets")
		return nil
	}
}

// fetchStylesheet retrieves a stylesheet's text. Accept-Encoding is left
// alone on purpose — setting it disables Go's transparent gzip handling and
// the body comes back compressed.
func fetchStylesheet(ctx context.Context, client *http.Client, href string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, href, nil)
	if err != nil {
		return "", err
	}
	// Google Fonts and friends serve different CSS per user agent (woff2 vs
	// woff vs ttf), so ask as the browser that is rendering the page.
	req.Header.Set("Accept", "text/css,*/*;q=0.1")
	if ua, err := browserUserAgent(ctx); err == nil && ua != "" {
		req.Header.Set("User-Agent", ua)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("http %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// browserUserAgent reads the UA of the page being captured, so a fetched
// stylesheet is the same one the browser was served.
func browserUserAgent(ctx context.Context) (string, error) {
	var ua string
	err := chromedp.Run(ctx, chromedp.Evaluate(`navigator.userAgent`, &ua))
	return ua, err
}

// absolutizeRefs resolves relative url() targets against the stylesheet's own
// address. Data URIs and already-absolute references are left alone.
func absolutizeRefs(css, sheetURL string) string {
	base, err := neturl.Parse(sheetURL)
	if err != nil {
		return css
	}
	return relativeURLRef.ReplaceAllStringFunc(css, func(match string) string {
		groups := relativeURLRef.FindStringSubmatch(match)
		if len(groups) != 3 {
			return match
		}
		quote, ref := groups[1], strings.TrimSpace(groups[2])
		if ref == "" || strings.HasPrefix(ref, "data:") || strings.HasPrefix(ref, "#") {
			return match
		}
		resolved, err := base.Parse(ref)
		if err != nil {
			return match
		}
		return "url(" + quote + resolved.String() + quote + ")"
	})
}
