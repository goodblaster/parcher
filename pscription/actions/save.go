package actions

import (
	"context"
	"encoding/base64"
	"io"
	"os"
	"strings"
	"time"

	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
	"github.com/goodblaster/errors"
	"github.com/goodblaster/parcher/pscription/act"
	"github.com/goodblaster/parcher/pscription/internal/log"
)

func SaveScreenshotToDisk(filename string) act.Func {
	return func(ctx context.Context) error {
		var err error
		var screenshot []byte

		// Let everything settle a last little bit.
		_ = act.Sleep(time.Second / 10).Do(ctx)

		// Take the screenshot
		screenshot, err = chromedp.Run(ctx, chromedp.FullScreenshot(100))
		if err != nil {
			return errors.Wrap(err, "failed to take screenshot")
		}

		// Save the screenshot
		err = os.WriteFile(filename, screenshot, 0644)
		if err != nil {
			return errors.Wrap(err, "failed to save screenshot")
		}

		log.With("filename", filename).Debug("screenshot saved")
		return nil
	}
}

func SaveScreenshotToBase64(img *string) act.Func {
	return func(ctx context.Context) error {
		var err error
		var screenshot []byte

		// Let everything settle a last little bit.
		_ = act.Sleep(time.Second / 10).Do(ctx)

		// Take the screenshot
		screenshot, err = chromedp.Run(ctx, chromedp.FullScreenshot(100))
		if err != nil {
			return errors.Wrap(err, "failed to take screenshot")
		}

		// Convert the screenshot to base64
		*img = base64.StdEncoding.EncodeToString(screenshot)
		return nil
	}
}

func SaveMhtToString(out *string) act.Func {
	return func(ctx context.Context) error {
		// Capture MIME HTML content.
		snap, err := chromedp.Call(ctx, page.CaptureSnapshot, page.CaptureSnapshotParams{})
		if err != nil {
			return errors.Wrap(err, "failed to capture MHT snapshot")
		}
		*out = snap.Data

		return nil
	}
}

func SaveMhtToDisk(filename string, contentLocation string) act.Func {
	return func(ctx context.Context) error {
		snap, err := chromedp.Call(ctx, page.CaptureSnapshot, page.CaptureSnapshotParams{})
		if err != nil {
			return errors.Wrap(err, "failed to capture MHT snapshot")
		}
		content := snap.Data

		content = FixContentLocation(content, contentLocation)

		out, err := os.Create(filename)
		if err != nil {
			return errors.Wrap(err, "failed to create file")
		}
		defer out.Close()

		_, err = out.Write([]byte(content))
		if err != nil {
			return errors.Wrap(err, "failed to write to file")
		}

		return nil
	}
}

func SaveMht(out io.Writer) act.Func {
	return func(ctx context.Context) error {
		snap, err := chromedp.Call(ctx, page.CaptureSnapshot, page.CaptureSnapshotParams{})
		if err != nil {
			return errors.Wrap(err, "failed to capture MHT snapshot")
		}
		content := snap.Data

		_, err = out.Write([]byte(content))
		if err != nil {
			return errors.Wrap(err, "failed to write to file")
		}

		return nil
	}
}

// FixContentLocation - When an MHT is opened and re-exported using ChromeDP,
// the Content-Location header is changed to the file path of the MHT file.
// This change creates rendering problems.
func FixContentLocation(content string, location string) string {
	// Find the first line that begins with "Content-Location:" and the location string.
	lines := strings.Split(content, "\r\n")
	for i, line := range lines {
		if strings.HasPrefix(line, "Content-Location:") {
			lines[i] = "Content-Location: " + location
			break
		}
	}
	return strings.Join(lines, "\r\n")
}
