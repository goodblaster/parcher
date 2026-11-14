// Package remote is the plumbing the service-backed OCR engines share:
// retrying HTTP, format conversion, and how a service's own failure is
// reported.
package remote

import (
	"bytes"
	"context"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"io"
	"net/http"
	"time"

	_ "golang.org/x/image/webp"
)

// Shared plumbing for the engines that send images to a service.
//
// THE REMOTE ENGINES SEND THE ARCHIVE'S IMAGES TO A THIRD PARTY. They are
// never a default: Default() only ever returns a local engine, and a remote
// one runs only when named with --engine and configured through its own
// environment variables.

// client is the HTTP client the remote engines share. The per-request
// timeout bounds one image (or one batch); `parch ocr --timeout` bounds the
// whole run through the context.
var client = &http.Client{Timeout: 2 * time.Minute}

// ToPNG re-encodes any image this build can decode (PNG, JPEG, GIF, WebP) as
// PNG, for services that accept fewer formats than a web page uses.
func ToPNG(data []byte) ([]byte, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("cannot decode image to convert it: %w", err)
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// retryAttempts is how many times a throttled or failed-on-their-side
// request is sent in all.
const retryAttempts = 5

// Do sends the request built by build, retrying while retryable
// says the response is the service asking us to slow down or failing on its
// own side. It returns the final status and body. Backoff doubles from
// RetryBase and stops early when ctx is done.
func Do(ctx context.Context, build func() (*http.Request, error), retryable func(status int, body []byte) bool) (int, []byte, error) {
	delay := RetryBase
	for attempt := 1; ; attempt++ {
		req, err := build()
		if err != nil {
			return 0, nil, err
		}
		resp, err := client.Do(req.WithContext(ctx))
		var status int
		var body []byte
		if err == nil {
			status = resp.StatusCode
			body, err = io.ReadAll(io.LimitReader(resp.Body, 64<<20))
			resp.Body.Close()
		}
		if ctx.Err() != nil {
			return 0, nil, ctx.Err()
		}
		again := err != nil || retryable(status, body)
		if !again || attempt == retryAttempts {
			return status, body, err
		}
		select {
		case <-ctx.Done():
			return 0, nil, ctx.Err()
		case <-time.After(delay):
		}
		delay *= 2
	}
}

// RetryBase is the first backoff delay; a variable so tests need not wait.
var RetryBase = 500 * time.Millisecond

// Clamp01 keeps a normalized coordinate inside the image.
func Clamp01(v float64) float64 { return max(0, min(1, v)) }

// ServiceError is a failure the service itself reported.
type ServiceError struct {
	Service, Code, Message string
	Status                 int
}

func (e *ServiceError) Error() string {
	return fmt.Sprintf("%s: %s (HTTP %d): %s", e.Service, e.Code, e.Status, e.Message)
}

// IsAuthError reports whether err is the service rejecting our credentials
// or permissions — a failure every image in a batch shares, so it is worth
// reporting once for the batch instead of once per image.
func IsAuthError(err error) bool {
	se, ok := err.(*ServiceError)
	if !ok {
		return false
	}
	switch se.Code {
	case "AccessDeniedException", "UnrecognizedClientException", "InvalidSignatureException",
		"ExpiredTokenException", "MissingAuthenticationTokenException", "IncompleteSignatureException",
		"UNAUTHENTICATED", "PERMISSION_DENIED":
		return true
	}
	return se.Status == http.StatusUnauthorized || se.Status == http.StatusForbidden
}
