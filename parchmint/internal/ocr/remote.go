package ocr

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
	"sync"
	"time"

	_ "golang.org/x/image/webp"
)

// Shared plumbing for the engines that send images to a service.
//
// THE REMOTE ENGINES SEND THE ARCHIVE'S IMAGES TO A THIRD PARTY. They are
// never a default: Default() only ever returns a local engine, and a remote
// one runs only when named with --engine and configured through its own
// environment variables.

// remoteClient is the HTTP client the remote engines share. The per-request
// timeout bounds one image (or one batch); `parch ocr --timeout` bounds the
// whole run through the context.
var remoteClient = &http.Client{Timeout: 2 * time.Minute}

// toPNG re-encodes any image this build can decode (PNG, JPEG, GIF, WebP) as
// PNG, for services that accept fewer formats than a web page uses.
func toPNG(data []byte) ([]byte, error) {
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

// forEach runs fn(i) for i in [0,n) on up to `workers` goroutines and waits
// for all of them. fn records its own outcome; a cancelled context is fn's
// to notice.
func forEach(n, workers int, fn func(i int)) {
	if workers < 1 {
		workers = 1
	}
	var wg sync.WaitGroup
	next := make(chan int)
	for w := 0; w < min(workers, n); w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range next {
				fn(i)
			}
		}()
	}
	for i := 0; i < n; i++ {
		next <- i
	}
	close(next)
	wg.Wait()
}

// retryAttempts is how many times a throttled or failed-on-their-side
// request is sent in all.
const retryAttempts = 5

// doWithRetry sends the request built by build, retrying while retryable
// says the response is the service asking us to slow down or failing on its
// own side. It returns the final status and body. Backoff doubles from
// retryBase and stops early when ctx is done.
func doWithRetry(ctx context.Context, build func() (*http.Request, error), retryable func(status int, body []byte) bool) (int, []byte, error) {
	delay := retryBase
	for attempt := 1; ; attempt++ {
		req, err := build()
		if err != nil {
			return 0, nil, err
		}
		resp, err := remoteClient.Do(req.WithContext(ctx))
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

// retryBase is the first backoff delay; a variable so tests need not wait.
var retryBase = 500 * time.Millisecond

// clamp01 keeps a normalized coordinate inside the image.
func clamp01(v float64) float64 { return max(0, min(1, v)) }
