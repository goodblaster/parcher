// Package fetch loads the bytes behind a resource reference, whichever of
// the three ways a snapshot can name one: inline as a data: URI, on disk as
// a file: URL, or over the network as http(s).
package fetch

import (
	"context"
	"encoding/base64"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/goodblaster/errors"
)

// Defaults for a Loader's zero-valued fields.
const (
	DefaultMaxSize   = 32 << 20 // 32 MiB
	DefaultUserAgent = "go-mht"
)

// Resource is the content behind one reference.
type Resource struct {
	ContentType string
	Data        []byte
}

// Loader retrieves resources. The zero value is usable and offline: without
// a Client, http(s) references are refused rather than fetched.
type Loader struct {
	// Client fetches http(s) references. When nil, they are not fetched.
	Client *http.Client

	// MaxSize caps the bytes read from any one resource. Zero means
	// DefaultMaxSize; a negative value means no limit.
	MaxSize int64

	// UserAgent is sent with HTTP requests.
	UserAgent string
}

// ErrOffline is returned for an http(s) reference when the Loader has no
// Client to fetch it with.
var ErrOffline = errors.New("network fetching is disabled")

// Load retrieves the resource named by target, which must be absolute (or a
// data: URI).
func (l *Loader) Load(ctx context.Context, target *url.URL) (*Resource, error) {
	switch strings.ToLower(target.Scheme) {
	case "data":
		return loadData(target)
	case "file":
		return l.loadFile(target)
	case "http", "https":
		return l.loadHTTP(ctx, target)
	default:
		return nil, errors.Newf("unsupported scheme %q", target.Scheme)
	}
}

// loadData decodes a data: URI. url.Parse leaves the whole payload in Opaque,
// which is exactly the "[<mediatype>][;base64],<data>" form.
func loadData(target *url.URL) (*Resource, error) {
	payload := target.Opaque
	if payload == "" {
		// A data: URI whose payload begins with "//" lands in Host/Path.
		payload = target.Host + target.Path
	}

	meta, data, found := strings.Cut(payload, ",")
	if !found {
		return nil, errors.New("malformed data URI: no comma")
	}

	contentType := meta
	isBase64 := false
	if trimmed, ok := strings.CutSuffix(meta, ";base64"); ok {
		contentType = trimmed
		isBase64 = true
	}
	if contentType == "" {
		contentType = "text/plain;charset=US-ASCII"
	}

	if isBase64 {
		// The payload may have been percent-encoded in transit ("+" as
		// %2B), and base64 alphabets vary in padding.
		decoded, err := decodeBase64(data)
		if err != nil {
			return nil, errors.Wrap(err, "failed to decode data URI")
		}
		return &Resource{ContentType: contentType, Data: decoded}, nil
	}

	unescaped, err := url.PathUnescape(data)
	if err != nil {
		return nil, errors.Wrap(err, "failed to unescape data URI")
	}

	return &Resource{ContentType: contentType, Data: []byte(unescaped)}, nil
}

func (l *Loader) loadFile(target *url.URL) (*Resource, error) {
	path := target.Path
	if path == "" {
		return nil, errors.New("file URL has no path")
	}
	// A Windows-style "file:///C:/x" keeps the drive letter in the path.
	if decoded, err := url.PathUnescape(path); err == nil {
		path = decoded
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, errors.Wrap(err, "failed to open file")
	}
	defer file.Close()

	data, err := l.read(file)
	if err != nil {
		return nil, err
	}

	return &Resource{ContentType: contentTypeFor(path, data), Data: data}, nil
}

func (l *Loader) loadHTTP(ctx context.Context, target *url.URL) (*Resource, error) {
	if l.Client == nil {
		return nil, ErrOffline
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to build request")
	}

	userAgent := l.UserAgent
	if userAgent == "" {
		userAgent = DefaultUserAgent
	}
	request.Header.Set("User-Agent", userAgent)
	request.Header.Set("Accept", "*/*")

	response, err := l.Client.Do(request)
	if err != nil {
		return nil, errors.Wrap(err, "request failed")
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, errors.Newf("unexpected status %s", response.Status)
	}

	data, err := l.read(response.Body)
	if err != nil {
		return nil, err
	}

	contentType := response.Header.Get("Content-Type")
	if contentType == "" {
		contentType = contentTypeFor(target.Path, data)
	}

	return &Resource{ContentType: strings.TrimSpace(contentType), Data: data}, nil
}

// read consumes reader up to the Loader's size limit, treating an oversized
// resource as an error rather than silently truncating it.
func (l *Loader) read(reader io.Reader) ([]byte, error) {
	limit := l.MaxSize
	if limit == 0 {
		limit = DefaultMaxSize
	}
	if limit < 0 {
		data, err := io.ReadAll(reader)
		if err != nil {
			return nil, errors.Wrap(err, "failed to read resource")
		}
		return data, nil
	}

	data, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, errors.Wrap(err, "failed to read resource")
	}
	if int64(len(data)) > limit {
		return nil, errors.Newf("resource exceeds the %d byte limit", limit)
	}

	return data, nil
}

// contentTypeFor guesses a media type from a path's extension, falling back
// to sniffing the content.
func contentTypeFor(path string, data []byte) string {
	if contentType := mime.TypeByExtension(filepath.Ext(path)); contentType != "" {
		return contentType
	}
	return http.DetectContentType(data)
}

// decodeBase64 decodes standard or URL-safe base64, padded or not. Real
// documents supply all four combinations.
func decodeBase64(data string) ([]byte, error) {
	// Whitespace is not legal in a data URI but appears in hand-written HTML
	// where a long URI has been wrapped across lines.
	data = strings.Map(func(r rune) rune {
		if r == ' ' || r == '\t' || r == '\r' || r == '\n' {
			return -1
		}
		return r
	}, data)

	// PathUnescape leaves "+" alone while decoding %2B, which is what a
	// percent-encoded base64 payload needs.
	if unescaped, err := url.PathUnescape(data); err == nil {
		data = unescaped
	}

	encoding := base64.StdEncoding
	if strings.ContainsAny(data, "-_") {
		encoding = base64.URLEncoding
	}
	if len(data)%4 != 0 {
		encoding = encoding.WithPadding(base64.NoPadding)
	}

	return encoding.DecodeString(data)
}
