package ocr

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/goodblaster/parcher/parchmint/internal/ocr/std"
)

// GoogleVision is the engine name for Google Cloud Vision.
const GoogleVision = "google"

const (
	// Documented limits of images:annotate.
	googleMaxImagesPerRequest = 16
	googleMaxRequestBytes     = 10 << 20
	// Raw image bytes allowed in one request: base64 grows them by a third,
	// and the JSON around them needs room.
	googleMaxRawPerRequest = 7 << 20
)

// googleVision reads images with Google Cloud Vision's images:annotate,
// sixteen images to a request.
//
// NOT YET RUN AGAINST THE REAL SERVICE: it was written from the API
// reference without a Google Cloud account. The request and response
// shapes are checked against the documented ones (see the tests); the
// first run with a real key is the first real test.
//
// What it sends: each image's bytes, to vision.googleapis.com.
// Language hints are NOT sent unless PARCH_GOOGLE_VISION_HINTS=1: Google
// documents that no hint usually gives the best result, and that a hint it
// does not support fails the request.
type googleVision struct {
	endpoint string // https://vision.googleapis.com
	apiKey   string
	token    string // OAuth access token, alternative to apiKey
	project  string // quota project, sent with a token
	feature  string
	hints    bool
	workers  int
}

func init() {
	Register(GoogleVision, func() (Recognizer, error) { return newGoogleVisionFromEnv() })
}

func newGoogleVisionFromEnv() (*googleVision, error) {
	g := &googleVision{
		endpoint: "https://vision.googleapis.com",
		apiKey:   os.Getenv("GOOGLE_VISION_API_KEY"),
		token:    os.Getenv("GOOGLE_VISION_ACCESS_TOKEN"),
		project:  os.Getenv("GOOGLE_CLOUD_PROJECT"),
		feature:  "TEXT_DETECTION",
		hints:    os.Getenv("PARCH_GOOGLE_VISION_HINTS") == "1",
		workers:  envInt("PARCH_OCR_CONCURRENCY", 4),
	}
	if g.apiKey == "" && g.token == "" {
		return nil, fmt.Errorf("the google OCR engine sends images to Google Cloud Vision and needs GOOGLE_VISION_API_KEY " +
			"(an API key allowed to use the Cloud Vision API), or GOOGLE_VISION_ACCESS_TOKEN (an OAuth access token, " +
			"e.g. from `gcloud auth print-access-token`) with GOOGLE_CLOUD_PROJECT, in the environment")
	}
	if v := os.Getenv("PARCH_GOOGLE_VISION_ENDPOINT"); v != "" {
		g.endpoint = strings.TrimRight(v, "/")
	}
	// DOCUMENT_TEXT_DETECTION is tuned for dense pages; web images are
	// sparse. Overridable for the page that is a scan.
	if v := os.Getenv("PARCH_GOOGLE_VISION_FEATURE"); v != "" {
		g.feature = v
	}
	return g, nil
}

func (g *googleVision) Name() string { return GoogleVision }

func (g *googleVision) Recognize(ctx context.Context, images []Image, langs []string) ([]Result, error) {
	results := make([]Result, len(images))
	for i, img := range images {
		results[i].ID = img.ID
	}

	// Pack into requests within the documented count and size limits.
	var batches [][]int
	var cur []int
	size := 0
	for i, img := range images {
		if len(img.Data) > googleMaxRawPerRequest {
			results[i].Err = fmt.Errorf("image is %d bytes; one google vision request holds about %d", len(img.Data), googleMaxRawPerRequest)
			continue
		}
		if len(cur) == googleMaxImagesPerRequest || size+len(img.Data) > googleMaxRawPerRequest {
			batches = append(batches, cur)
			cur, size = nil, 0
		}
		cur = append(cur, i)
		size += len(img.Data)
	}
	if len(cur) > 0 {
		batches = append(batches, cur)
	}

	batchErrs := make([]error, len(batches))
	forEach(len(batches), g.workers, func(b int) {
		batchErrs[b] = g.batch(ctx, images, batches[b], langs, results)
	})
	for b, err := range batchErrs {
		if err == nil {
			continue
		}
		if isAuthError(err) {
			return nil, err // every request will fail the same way
		}
		for _, i := range batches[b] {
			results[i].Err = err
		}
	}
	return results, nil
}

// batch sends one images:annotate request and files each response under
// its image.
func (g *googleVision) batch(ctx context.Context, images []Image, idx []int, langs []string, results []Result) error {
	type request struct {
		Image struct {
			Content []byte `json:"content"` // base64 by encoding/json
		} `json:"image"`
		Features []map[string]string `json:"features"`
		Context  *struct {
			LanguageHints []string `json:"languageHints"`
		} `json:"imageContext,omitempty"`
	}
	var payload struct {
		Requests []request `json:"requests"`
	}
	for _, i := range idx {
		var r request
		r.Image.Content = images[i].Data
		r.Features = []map[string]string{{"type": g.feature}}
		if g.hints && len(langs) > 0 {
			r.Context = &struct {
				LanguageHints []string `json:"languageHints"`
			}{langs}
		}
		payload.Requests = append(payload.Requests, r)
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if len(body) > googleMaxRequestBytes {
		return fmt.Errorf("google vision request is %d bytes, over the %d limit", len(body), googleMaxRequestBytes)
	}

	status, respBody, err := doWithRetry(ctx, func() (*http.Request, error) {
		req, err := http.NewRequest(http.MethodPost, g.endpoint+"/v1/images:annotate", bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json; charset=utf-8")
		if g.apiKey != "" {
			// The header, not ?key=: a key in the URL ends up in logs.
			req.Header.Set("X-Goog-Api-Key", g.apiKey)
		} else {
			req.Header.Set("Authorization", "Bearer "+g.token)
			if g.project != "" {
				req.Header.Set("X-Goog-User-Project", g.project)
			}
		}
		return req, nil
	}, func(status int, _ []byte) bool {
		return status == http.StatusTooManyRequests || status >= 500
	})
	if err != nil {
		return err
	}

	type status_ struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	}
	var resp struct {
		Responses []struct {
			FullTextAnnotation json.RawMessage `json:"fullTextAnnotation"`
			Error              *status_        `json:"error"`
		} `json:"responses"`
		Error *status_ `json:"error"`
	}
	if jsonErr := json.Unmarshal(respBody, &resp); jsonErr != nil && status == http.StatusOK {
		return fmt.Errorf("google vision: unreadable response: %w", jsonErr)
	}
	if status != http.StatusOK || resp.Error != nil {
		se := &serviceError{Service: "google vision", Code: "UnknownError", Status: status}
		if resp.Error != nil {
			se.Message = resp.Error.Message
			if resp.Error.Status != "" {
				se.Code = resp.Error.Status
			}
		}
		return se
	}
	if len(resp.Responses) != len(idx) {
		return fmt.Errorf("google vision returned %d responses for %d images", len(resp.Responses), len(idx))
	}
	// Responses come back in request order.
	for n, r := range resp.Responses {
		i := idx[n]
		// An entry may carry an error AND annotations; the annotations are
		// documented as still correct, so keep them when there are any.
		if len(r.FullTextAnnotation) > 0 {
			results[i].Lines, results[i].Err = parseGoogleText(r.FullTextAnnotation)
			continue
		}
		if r.Error != nil {
			results[i].Err = &serviceError{Service: "google vision", Code: fmt.Sprintf("code %d", r.Error.Code), Status: status, Message: r.Error.Message}
		}
		// Neither: an image with no text in it.
	}
	return nil
}

// parseGoogleText maps a fullTextAnnotation onto the std contract.
//
// Google reports word boxes as four pixel vertices (a coordinate of 0 is
// OMITTED from the JSON, so a missing x or y means 0) alongside the page's
// pixel size; ours are fractions of the image. A word has no text of its
// own: it is its symbols joined. Lines are not a level of the hierarchy —
// they end where a symbol carries a line-ending detectedBreak, and at the
// end of each paragraph.
func parseGoogleText(raw json.RawMessage) ([]std.Line, error) {
	var ann struct {
		Pages []struct {
			Width, Height float64
			Blocks        []struct {
				Paragraphs []struct {
					Words []struct {
						Confidence  float64
						BoundingBox struct {
							Vertices []struct{ X, Y float64 }
						}
						Symbols []struct {
							Text     string
							Property struct {
								DetectedBreak struct{ Type string }
							}
						}
					}
				}
			}
		}
	}
	if err := json.Unmarshal(raw, &ann); err != nil {
		return nil, fmt.Errorf("google vision: unreadable text annotation: %w", err)
	}

	var lines []std.Line
	for _, page := range ann.Pages {
		if page.Width <= 0 || page.Height <= 0 {
			continue
		}
		for _, block := range page.Blocks {
			for _, para := range block.Paragraphs {
				var cur std.Line
				var confSum float64
				flush := func() {
					if len(cur.Words) > 0 {
						cur.Confidence = confSum / float64(len(cur.Words))
						setBounds(&cur)
						lines = append(lines, cur)
					}
					cur, confSum = std.Line{}, 0
				}
				for _, w := range para.Words {
					var text strings.Builder
					lineEnds := false
					for _, s := range w.Symbols {
						text.WriteString(s.Text)
						switch s.Property.DetectedBreak.Type {
						case "EOL_SURE_SPACE", "LINE_BREAK", "HYPHEN":
							lineEnds = true
						}
					}
					if strings.TrimSpace(text.String()) != "" && len(w.BoundingBox.Vertices) > 0 {
						v := w.BoundingBox.Vertices
						x1, y1, x2, y2 := v[0].X, v[0].Y, v[0].X, v[0].Y
						for _, p := range v[1:] {
							x1, y1, x2, y2 = min(x1, p.X), min(y1, p.Y), max(x2, p.X), max(y2, p.Y)
						}
						cur.Words = append(cur.Words, std.Word{
							Text:   text.String(),
							Left:   clamp01(x1 / page.Width),
							Top:    clamp01(y1 / page.Height),
							Width:  clamp01((x2 - x1) / page.Width),
							Height: clamp01((y2 - y1) / page.Height),
						})
						confSum += w.Confidence
					}
					if lineEnds {
						flush()
					}
				}
				flush()
			}
		}
	}
	return lines, nil
}

// setBounds fills a line's text and bounding rect from its words.
func setBounds(line *std.Line) {
	var texts []string
	x1, y1, x2, y2 := 1.0, 1.0, 0.0, 0.0
	for _, w := range line.Words {
		texts = append(texts, w.Text)
		x1, y1 = min(x1, w.Left), min(y1, w.Top)
		x2, y2 = max(x2, w.Left+w.Width), max(y2, w.Top+w.Height)
	}
	line.LineText = strings.Join(texts, " ")
	line.Rect = std.Rect{Left: x1, Top: y1, Width: x2 - x1, Height: y2 - y1}
}
