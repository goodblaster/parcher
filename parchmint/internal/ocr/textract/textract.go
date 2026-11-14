// Package textract is OCR through AWS Textract. It sends images to AWS.
package textract

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/goodblaster/parcher/parchmint/internal/ocr"
	"github.com/goodblaster/parcher/parchmint/internal/ocr/remote"
	"github.com/goodblaster/parcher/parchmint/internal/ocr/std"
)

// Name is the engine's name on the command line.
const Name = "textract"

// textractMaxBytes is the documented length limit of Document.Bytes
// (10485760). One AWS page states 5 MB for a document passed as bytes; the
// API's own constraint and the limits page say 10 MB, so that is what is
// enforced here and the service has the last word.
const textractMaxBytes = 10 << 20

// engine reads images with AWS Textract's synchronous DetectDocumentText,
// one request per image, a few at a time.
//
// NOT YET RUN AGAINST THE REAL SERVICE: it was written from the API
// reference without an AWS account. The request signing is checked against
// what the AWS CLI produces for the same request, and the response mapping
// against the documented response shape (see the tests), but the first run
// with real credentials is the first real test.
//
// What it sends: each image's bytes, to textract.<region>.amazonaws.com.
// What it cannot do: take a language hint (the API has none; the
// synchronous operations are documented as English-centric), or read WebP
// or GIF, which are converted to PNG first.
type engine struct {
	endpoint string // https://textract.<region>.amazonaws.com
	region   string
	cred     awsCredentials
	workers  int
	now      func() time.Time
}

func init() {
	ocr.Register(Name, func() (ocr.Recognizer, error) { return newFromEnv() })
}

// newFromEnv configures the engine from the standard AWS
// environment variables.
func newFromEnv() (*engine, error) {
	cred := awsCredentials{
		AccessKeyID:     os.Getenv("AWS_ACCESS_KEY_ID"),
		SecretAccessKey: os.Getenv("AWS_SECRET_ACCESS_KEY"),
		SessionToken:    os.Getenv("AWS_SESSION_TOKEN"),
	}
	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = os.Getenv("AWS_DEFAULT_REGION")
	}
	if cred.AccessKeyID == "" || cred.SecretAccessKey == "" || region == "" {
		return nil, fmt.Errorf("the textract OCR engine sends images to AWS and needs AWS_ACCESS_KEY_ID, " +
			"AWS_SECRET_ACCESS_KEY and AWS_REGION in the environment (AWS_SESSION_TOKEN too for temporary credentials); " +
			"the identity needs the textract:DetectDocumentText permission")
	}
	endpoint := os.Getenv("PARCH_TEXTRACT_ENDPOINT")
	if endpoint == "" {
		endpoint = "https://textract." + region + ".amazonaws.com"
	}
	return &engine{
		endpoint: strings.TrimRight(endpoint, "/"),
		region:   region,
		cred:     cred,
		workers:  ocr.Concurrency(4),
		now:      time.Now,
	}, nil
}

func (t *engine) Name() string { return Name }

func (t *engine) Recognize(ctx context.Context, images []ocr.Image, _ []string) ([]ocr.Result, error) {
	results := make([]ocr.Result, len(images))
	ocr.ForEach(len(images), t.workers, func(i int) {
		lines, err := t.one(ctx, images[i])
		results[i] = ocr.Result{ID: images[i].ID, Lines: lines, Err: err}
	})
	// A failure every image shares is a configuration problem, not thirty
	// image problems: report it once, as the batch error.
	if len(results) > 0 {
		first := results[0].Err
		if remote.IsAuthError(first) {
			return nil, first
		}
	}
	return results, nil
}

func (t *engine) one(ctx context.Context, img ocr.Image) ([]std.Line, error) {
	data := img.Data
	if img.MIME != "image/png" && img.MIME != "image/jpeg" {
		converted, err := remote.ToPNG(data)
		if err != nil {
			return nil, err
		}
		data = converted
	}
	if len(data) > textractMaxBytes {
		return nil, fmt.Errorf("image is %d bytes; textract accepts at most %d", len(data), textractMaxBytes)
	}

	// encoding/json writes []byte as base64, which is what Bytes wants.
	body, err := json.Marshal(map[string]any{"Document": map[string]any{"Bytes": data}})
	if err != nil {
		return nil, err
	}
	status, respBody, err := remote.Do(ctx, func() (*http.Request, error) {
		req, err := http.NewRequest(http.MethodPost, t.endpoint+"/", bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/x-amz-json-1.1")
		req.Header.Set("X-Amz-Target", "Textract.DetectDocumentText")
		signV4(req, body, t.cred, t.region, "textract", t.now())
		return req, nil
	}, func(status int, body []byte) bool {
		if status >= 500 {
			return true // InternalServerError, and ThrottlingException is a 500 here
		}
		code := awsErrorCode(body)
		return code == "ProvisionedThroughputExceededException" || code == "ThrottlingException" ||
			code == "LimitExceededException"
	})
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		var e struct {
			Message  string `json:"message"`
			Message2 string `json:"Message"`
		}
		_ = json.Unmarshal(respBody, &e)
		return nil, &remote.ServiceError{Service: "textract", Code: awsErrorCode(respBody), Status: status, Message: e.Message + e.Message2}
	}
	return parseTextract(respBody)
}

// awsErrorCode pulls the exception name out of an AWS JSON error body,
// whose __type is "Name" or "com.amazonaws.service#Name".
func awsErrorCode(body []byte) string {
	var e struct {
		Type string `json:"__type"`
	}
	if json.Unmarshal(body, &e) != nil || e.Type == "" {
		return "UnknownError"
	}
	if i := strings.LastIndexByte(e.Type, '#'); i >= 0 {
		return e.Type[i+1:]
	}
	return e.Type
}

// parseTextract maps a DetectDocumentText response onto the std contract.
// Textract already reports boxes as fractions of the page, top-left origin;
// its confidence is 0..100 and ours 0..1. A LINE block lists its WORD
// blocks as CHILD relationships.
func parseTextract(body []byte) ([]std.Line, error) {
	type block struct {
		BlockType  string
		ID         string `json:"Id"`
		Text       string
		Confidence float64
		Geometry   struct {
			BoundingBox struct{ Left, Top, Width, Height float64 }
		}
		Relationships []struct {
			Type string
			IDs  []string `json:"Ids"`
		}
	}
	var resp struct{ Blocks []block }
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("textract: unreadable response: %w", err)
	}
	byID := make(map[string]block, len(resp.Blocks))
	for _, b := range resp.Blocks {
		byID[b.ID] = b
	}
	word := func(b block) std.Word {
		bb := b.Geometry.BoundingBox
		return std.Word{Text: b.Text, Left: remote.Clamp01(bb.Left), Top: remote.Clamp01(bb.Top), Width: remote.Clamp01(bb.Width), Height: remote.Clamp01(bb.Height)}
	}

	var lines []std.Line
	for _, b := range resp.Blocks {
		if b.BlockType != "LINE" {
			continue
		}
		bb := b.Geometry.BoundingBox
		line := std.Line{
			LineText:   b.Text,
			Confidence: b.Confidence / 100,
			Rect:       std.Rect{Left: remote.Clamp01(bb.Left), Top: remote.Clamp01(bb.Top), Width: remote.Clamp01(bb.Width), Height: remote.Clamp01(bb.Height)},
		}
		for _, rel := range b.Relationships {
			if rel.Type != "CHILD" {
				continue
			}
			for _, id := range rel.IDs {
				if w, ok := byID[id]; ok && w.BlockType == "WORD" && strings.TrimSpace(w.Text) != "" {
					line.Words = append(line.Words, word(w))
				}
			}
		}
		if len(line.Words) == 0 && strings.TrimSpace(b.Text) != "" {
			// A line with no word children still has text and a box.
			line.Words = []std.Word{word(b)}
		}
		if len(line.Words) > 0 {
			lines = append(lines, line)
		}
	}
	return lines, nil
}
