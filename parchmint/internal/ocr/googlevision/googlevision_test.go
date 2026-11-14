package googlevision

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/goodblaster/parcher/parchmint/internal/ocr"
)

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

// One image in the documented fullTextAnnotation shape: "SLOW HORSES" on
// one line and "tv" on the next. The first word's left edge is x=0, which
// Google OMITS from the JSON.
const googleAnnotation = `{"pages":[{"width":200,"height":100,"blocks":[{"paragraphs":[{"words":[
 {"confidence":0.99,"boundingBox":{"vertices":[{"y":10},{"x":80,"y":10},{"x":80,"y":30},{"y":30}]},
  "symbols":[{"text":"S"},{"text":"L"},{"text":"O"},{"text":"W","property":{"detectedBreak":{"type":"SPACE"}}}]},
 {"confidence":0.97,"boundingBox":{"vertices":[{"x":90,"y":10},{"x":190,"y":10},{"x":190,"y":30},{"x":90,"y":30}]},
  "symbols":[{"text":"HORSES","property":{"detectedBreak":{"type":"EOL_SURE_SPACE"}}}]},
 {"confidence":0.90,"boundingBox":{"vertices":[{"x":20,"y":50},{"x":60,"y":50},{"x":60,"y":70},{"x":20,"y":70}]},
  "symbols":[{"text":"t"},{"text":"v","property":{"detectedBreak":{"type":"LINE_BREAK"}}}]}
]}]}]}]}`

func TestGoogleVision(t *testing.T) {
	var requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path != "/v1/images:annotate" || r.Header.Get("X-Goog-Api-Key") != "k" || r.URL.RawQuery != "" {
			t.Errorf("bad request: %s %v", r.URL, r.Header)
		}
		var body struct {
			Requests []struct {
				Image    struct{ Content string }
				Features []struct{ Type string }
				Context  any `json:"imageContext"`
			}
		}
		raw, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatal(err)
		}
		if len(body.Requests) > googleMaxImagesPerRequest {
			t.Errorf("%d images in one request, limit %d", len(body.Requests), googleMaxImagesPerRequest)
		}
		var out strings.Builder
		out.WriteString(`{"responses":[`)
		for i, rq := range body.Requests {
			if i > 0 {
				out.WriteString(",")
			}
			if rq.Features[0].Type != "TEXT_DETECTION" || rq.Context != nil {
				t.Errorf("request %d: features %v context %v", i, rq.Features, rq.Context)
			}
			data, _ := base64.StdEncoding.DecodeString(rq.Image.Content)
			switch string(data) {
			case "text":
				out.WriteString(`{"fullTextAnnotation":` + googleAnnotation + `}`)
			case "bad":
				out.WriteString(`{"error":{"code":3,"message":"Bad image data."}}`)
			default:
				out.WriteString(`{}`) // no text found
			}
		}
		out.WriteString(`]}`)
		io.WriteString(w, out.String())
	}))
	defer srv.Close()

	g := &engine{endpoint: srv.URL, apiKey: "k", feature: "TEXT_DETECTION", workers: 2}
	images := []ocr.Image{{ID: "text", Data: []byte("text")}, {ID: "bad", Data: []byte("bad")}}
	for i := 0; i < 18; i++ { // 20 images: must split into two requests
		images = append(images, ocr.Image{ID: "blank" + string(rune('a'+i)), Data: []byte("blank")})
	}
	res, err := g.Recognize(context.Background(), images, []string{"en-US"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != len(images) || requests.Load() != 2 {
		t.Fatalf("%d results for %d images in %d requests (want 2)", len(res), len(images), requests.Load())
	}
	text := res[0]
	if text.ID != "text" || text.Err != nil || len(text.Lines) != 2 {
		t.Fatalf("text image: %+v", text)
	}
	if text.Lines[0].LineText != "SLOW HORSES" || text.Lines[1].LineText != "tv" {
		t.Errorf("lines: %q / %q", text.Lines[0].LineText, text.Lines[1].LineText)
	}
	// Pixels over the page size; the omitted x is 0.
	if w := text.Lines[0].Words[0]; !near(w.Left, 0) || !near(w.Top, 0.10) || !near(w.Width, 0.40) || !near(w.Height, 0.20) {
		t.Errorf("first word box: %+v", w)
	}
	if !near(text.Lines[0].Confidence, 0.98) {
		t.Errorf("line confidence %v", text.Lines[0].Confidence)
	}
	if res[1].Err == nil || !strings.Contains(res[1].Err.Error(), "Bad image data") {
		t.Errorf("per-image error lost: %v", res[1].Err)
	}
	if res[2].Err != nil || len(res[2].Lines) != 0 {
		t.Errorf("an image with no text is not an error: %+v", res[2])
	}
}

func TestGoogleVisionAuthFailureIsBatchError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		io.WriteString(w, `{"error":{"code":403,"message":"API key not valid.","status":"PERMISSION_DENIED"}}`)
	}))
	defer srv.Close()
	g := &engine{endpoint: srv.URL, apiKey: "k", feature: "TEXT_DETECTION", workers: 1}
	if _, err := g.Recognize(context.Background(), []ocr.Image{{ID: "a", Data: []byte("x")}}, nil); err == nil || !strings.Contains(err.Error(), "API key not valid") {
		t.Fatalf("want a batch error naming the cause, got %v", err)
	}
}

// Unconfigured, the engine refuses to start and says what to set.
func TestNeedsConfiguration(t *testing.T) {
	t.Setenv("GOOGLE_VISION_API_KEY", "")
	t.Setenv("GOOGLE_VISION_ACCESS_TOKEN", "")
	if _, err := ocr.NewRecognizer(Name); err == nil || !strings.Contains(err.Error(), "GOOGLE_VISION_API_KEY") {
		t.Fatalf("want an error naming GOOGLE_VISION_API_KEY, got %v", err)
	}
}
