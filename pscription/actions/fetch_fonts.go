package actions

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	neturl "net/url"
	"strings"

	"github.com/goodblaster/parcher/pscription/act"
	"github.com/goodblaster/parcher/pscription/internal/log"
	js "github.com/goodblaster/parcher/pscription/scripts"
)

// fontFace mirrors one @font-face rule extracted from the page.
type fontFace struct {
	Family       string   `json:"family"`
	Style        string   `json:"style"`
	Weight       string   `json:"weight"`
	Stretch      string   `json:"stretch"`
	UnicodeRange string   `json:"unicodeRange"`
	Display      string   `json:"display"`
	URLs         []string `json:"urls"`
}

// FetchAndInjectFonts embeds the page's web fonts as data: URIs. Chrome's
// MHTML snapshot omits CSS web fonts entirely, so without this an MHT
// archive falls back to system fonts and icon fonts (Font Awesome dropdown
// carets, etc.) render as tofu boxes. It fetches each @font-face source
// server-side — which also sidesteps the CORS restriction that normally
// applies to cross-origin fonts — and injects overriding @font-face rules
// with the same descriptors, so the data: source wins offline.
//
// It can only see faces it can READ: a cross-origin stylesheet throws on
// cssRules and contributes nothing, so InlineCrossOriginStylesheets has to
// run first or whole typefaces go missing without a word.
func FetchAndInjectFonts() act.Func {
	return func(ctx context.Context) error {
		var faces []fontFace
		if err := act.Run(ctx, js.ExtractFontFaces.Action(&faces)); err != nil {
			return fmt.Errorf("failed to extract font faces: %w", err)
		}
		if len(faces) == 0 {
			log.Info("no web fonts to fetch")
			return nil
		}

		var css strings.Builder
		embedded, failed := 0, 0
		userAgent := fetchUserAgent(ctx)

		for _, face := range faces {
			dataURL, format, ok := fetchFirstFont(ctx, fetchClient, face.URLs, userAgent)
			if !ok {
				failed++
				continue
			}
			css.WriteString("@font-face{")
			fmt.Fprintf(&css, "font-family:%s;", face.Family)
			if face.Style != "" {
				fmt.Fprintf(&css, "font-style:%s;", face.Style)
			}
			if face.Weight != "" {
				fmt.Fprintf(&css, "font-weight:%s;", face.Weight)
			}
			if face.Stretch != "" {
				fmt.Fprintf(&css, "font-stretch:%s;", face.Stretch)
			}
			if face.Display != "" {
				fmt.Fprintf(&css, "font-display:%s;", face.Display)
			}
			fmt.Fprintf(&css, "src:url(%s)", dataURL)
			if format != "" {
				fmt.Fprintf(&css, " format('%s')", format)
			}
			css.WriteString(";")
			if face.UnicodeRange != "" {
				fmt.Fprintf(&css, "unicode-range:%s;", face.UnicodeRange)
			}
			css.WriteString("}")
			embedded++
		}

		log.With("embedded", embedded).With("failed", failed).Info("embedded web fonts")
		if embedded == 0 {
			return nil
		}

		// Inject as a single late <style> so these data: sources win the
		// cascade over the original external @font-face rules.
		return act.Run(ctx, js.InjectCss.Action(nil, "gb-embedded-fonts", css.String()))
	}
}

// fetchFirstFont fetches the first URL that succeeds, returning its data URL
// and a CSS format() hint.
func fetchFirstFont(ctx context.Context, client *http.Client, urls []string, userAgent string) (dataURL, format string, ok bool) {
	for _, url := range urls {
		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			continue
		}
		req.Header.Set("User-Agent", userAgent)
		req.Header.Set("Accept", "*/*")
		// Many sites hotlink-protect fonts by Referer; present one matching
		// the font's own origin so the fetch looks same-site.
		if u, perr := neturl.Parse(url); perr == nil && u.Scheme != "" {
			req.Header.Set("Referer", u.Scheme+"://"+u.Host+"/")
		}
		resp, err := client.Do(req)
		if err != nil {
			continue
		}
		data, err := readCapped(resp.Body, maxResourceBytes)
		resp.Body.Close()
		if err != nil || resp.StatusCode != http.StatusOK || len(data) == 0 {
			continue
		}
		mime, format := fontMIMEAndFormat(url, resp.Header.Get("Content-Type"))
		encoded := base64.StdEncoding.EncodeToString(data)
		return fmt.Sprintf("data:%s;base64,%s", mime, encoded), format, true
	}
	return "", "", false
}

// fontMIMEAndFormat returns the data-URI MIME type and the CSS format() hint
// for a font, preferring the URL extension (content types for fonts are
// frequently wrong or generic).
func fontMIMEAndFormat(url, contentType string) (mime, format string) {
	lower := strings.ToLower(url)
	switch {
	case strings.Contains(lower, ".woff2"):
		return "font/woff2", "woff2"
	case strings.Contains(lower, ".woff"):
		return "font/woff", "woff"
	case strings.Contains(lower, ".ttf"):
		return "font/ttf", "truetype"
	case strings.Contains(lower, ".otf"):
		return "font/otf", "opentype"
	case strings.Contains(lower, ".eot"):
		return "application/vnd.ms-fontobject", "embedded-opentype"
	}
	// Fall back to the server's content type, no format hint.
	if contentType != "" {
		if i := strings.IndexByte(contentType, ';'); i >= 0 {
			contentType = contentType[:i]
		}
		return strings.TrimSpace(contentType), ""
	}
	return "font/woff2", "woff2"
}
