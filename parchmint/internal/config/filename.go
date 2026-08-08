package config

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Filename template tokens (used in the "filename" config key):
//
//	{host}      request host, without a leading "www."   test-site-1
//	{path}      request path, slashes flattened          support_contact
//	{title}     page <title>, sanitized                  Apple
//	{ext}       output extension, without the dot         html
//	{date}      local date                               2026-07-18
//	{time}      local time                               15-04-05
//	{datetime}  local date and time                      2026-07-18_15-04-05
//	{unix}      Unix timestamp (seconds)                 1752854400
//	{date:L}    custom Go time layout L                  {date:2006-01} -> 2026-07
//
// Any other {token} is left untouched so typos are visible in the output name.
type Vars struct {
	URL   string    // the captured URL
	Title string    // page title (may be empty)
	Ext   string    // extension without the leading dot, e.g. "html"
	Now   time.Time // capture time
}

var tokenRe = regexp.MustCompile(`\{([a-zA-Z]+)(?::([^}]+))?\}`)

// Render expands a filename template. Values derived from the URL or title are
// sanitized to safe filename components; literal characters in the template
// (separators like "-" or ".") are preserved.
func Render(tmpl string, v Vars) string {
	now := v.Now
	if now.IsZero() {
		now = time.Now()
	}

	host, path := hostAndPath(v.URL)
	title := sanitize(v.Title)
	if title == "" {
		title = "untitled"
	}

	out := tokenRe.ReplaceAllStringFunc(tmpl, func(match string) string {
		m := tokenRe.FindStringSubmatch(match)
		name, arg := m[1], m[2]
		switch name {
		case "host":
			return host
		case "path":
			return path
		case "title":
			return title
		case "ext":
			return strings.TrimPrefix(v.Ext, ".")
		case "date":
			if arg != "" {
				return now.Format(arg)
			}
			return now.Format("2006-01-02")
		case "time":
			return now.Format("15-04-05")
		case "datetime":
			return now.Format("2006-01-02_15-04-05")
		case "unix":
			return strconv.FormatInt(now.Unix(), 10)
		default:
			return match // unknown token: leave as-is
		}
	})
	return out
}

// hostAndPath extracts a sanitized host (sans www.) and flattened path from a
// URL. A path of "/" or "" yields an empty path component.
func hostAndPath(raw string) (host, path string) {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return sanitize(raw), ""
	}
	host = strings.TrimPrefix(u.Hostname(), "www.")
	path = sanitize(strings.Trim(u.Path, "/"))
	return sanitize(host), path
}

// sanitize turns an arbitrary string into a safe filename component: anything
// that isn't a letter, digit, dot, dash, or underscore becomes a dash, with
// runs of dashes collapsed and leading/trailing dashes trimmed.
func sanitize(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '.', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	collapsed := multiDash.ReplaceAllString(b.String(), "-")
	return strings.Trim(collapsed, "-")
}

var multiDash = regexp.MustCompile(`-+`)
