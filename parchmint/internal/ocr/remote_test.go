package ocr

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/gif"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

// The signature the AWS CLI (2.27) produced for this exact request, with
// AWS's published example credentials — recorded by pointing the CLI at a
// local listener. Our signer must produce the same one: a wrong signature is
// the one mistake no fake server can catch.
func TestSignV4MatchesAWSCLI(t *testing.T) {
	body := []byte(`{"Document": {"Bytes": "aGVsbG8="}}`)
	req, _ := http.NewRequest(http.MethodPost, "http://127.0.0.1:18765/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", "Textract.DetectDocumentText")
	cred := awsCredentials{AccessKeyID: "AKIDEXAMPLE", SecretAccessKey: "wJalrXUtnFEMI/K7MDENG+bPxRfiCYEXAMPLEKEY"}
	signV4(req, body, cred, "us-east-1", "textract", time.Date(2026, 11, 14, 0, 0, 6, 0, time.UTC))

	want := "AWS4-HMAC-SHA256 Credential=AKIDEXAMPLE/20261114/us-east-1/textract/aws4_request, " +
		"SignedHeaders=content-type;host;x-amz-date;x-amz-target, " +
		"Signature=c29b18ff1c2155eaaef50ecbf953af69f661ecdc3dc688e5c97f8701e0ad2c9a"
	if got := req.Header.Get("Authorization"); got != want {
		t.Fatalf("Authorization\n got %s\nwant %s", got, want)
	}
}

// A response in the documented DetectDocumentText shape.
const textractResponse = `{"DocumentMetadata":{"Pages":1},"Blocks":[
 {"BlockType":"PAGE","Id":"p","Geometry":{"BoundingBox":{"Left":0,"Top":0,"Width":1,"Height":1}},"Relationships":[{"Type":"CHILD","Ids":["l1"]}]},
 {"BlockType":"LINE","Id":"l1","Text":"SERIES 12","Confidence":98.5,
  "Geometry":{"BoundingBox":{"Left":0.10,"Top":0.20,"Width":0.50,"Height":0.10}},
  "Relationships":[{"Type":"CHILD","Ids":["w1","w2"]}]},
 {"BlockType":"WORD","Id":"w1","Text":"SERIES","Confidence":99,"Geometry":{"BoundingBox":{"Left":0.10,"Top":0.20,"Width":0.30,"Height":0.10}}},
 {"BlockType":"WORD","Id":"w2","Text":"12","Confidence":98,"Geometry":{"BoundingBox":{"Left":0.45,"Top":0.20,"Width":0.15,"Height":0.10}}}
]}`

func newTestTextract(url string) *textract {
	return &textract{endpoint: url, region: "us-east-1", workers: 2, now: time.Now,
		cred: awsCredentials{AccessKeyID: "AKIDEXAMPLE", SecretAccessKey: "secret"}}
}

func tinyGIF(t *testing.T) []byte {
	var buf bytes.Buffer
	if err := gif.Encode(&buf, image.NewGray(image.Rect(0, 0, 4, 4)), nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestTextract(t *testing.T) {
	retryBase = time.Millisecond
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if r.Header.Get("X-Amz-Target") != "Textract.DetectDocumentText" ||
			r.Header.Get("Content-Type") != "application/x-amz-json-1.1" ||
			!strings.Contains(r.Header.Get("Authorization"), "/us-east-1/textract/aws4_request") {
			t.Errorf("bad request headers: %v", r.Header)
		}
		var body struct{ Document struct{ Bytes string } }
		raw, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Errorf("body: %v", err)
		}
		data, _ := base64.StdEncoding.DecodeString(body.Document.Bytes)
		switch {
		case n == 1:
			// The first call is throttled: it must be retried, not failed.
			w.WriteHeader(400)
			io.WriteString(w, `{"__type":"ProvisionedThroughputExceededException","message":"slow down"}`)
		case bytes.HasPrefix(data, []byte("bad")):
			w.WriteHeader(400)
			io.WriteString(w, `{"__type":"com.amazonaws.textract#UnsupportedDocumentException","message":"nope"}`)
		case bytes.HasPrefix(data, []byte("GIF8")):
			t.Error("a GIF was sent as-is; textract takes PNG and JPEG only")
		default:
			io.WriteString(w, textractResponse)
		}
	}))
	defer srv.Close()

	res, err := newTestTextract(srv.URL).Recognize(context.Background(), []Image{
		{ID: "a", MIME: "image/png", Data: []byte("\x89PNG fake")},
		{ID: "b", MIME: "image/jpeg", Data: []byte("bad one")},
		{ID: "c", MIME: "image/gif", Data: tinyGIF(t)},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]Result{}
	for _, r := range res {
		by[r.ID] = r
	}
	a := by["a"]
	if a.Err != nil || len(a.Lines) != 1 || a.Lines[0].LineText != "SERIES 12" || len(a.Lines[0].Words) != 2 {
		t.Fatalf("image a: %+v", a)
	}
	if w := a.Lines[0].Words[1]; w.Text != "12" || !near(w.Left, 0.45) || !near(w.Width, 0.15) || !near(w.Top, 0.20) {
		t.Errorf("word box not carried over as fractions: %+v", w)
	}
	if !near(a.Lines[0].Confidence, 0.985) {
		t.Errorf("confidence %v, want 0..1 scale", a.Lines[0].Confidence)
	}
	// One bad image is that image's failure only.
	if by["b"].Err == nil || !strings.Contains(by["b"].Err.Error(), "UnsupportedDocumentException") {
		t.Errorf("image b should carry the service's error, got %v", by["b"].Err)
	}
	if by["c"].Err != nil || len(by["c"].Lines) != 1 {
		t.Errorf("GIF should be converted and read: %+v", by["c"])
	}
}

// Bad credentials fail the batch once, not every image separately.
func TestTextractAuthFailureIsBatchError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		io.WriteString(w, `{"__type":"UnrecognizedClientException","message":"The security token included in the request is invalid."}`)
	}))
	defer srv.Close()
	_, err := newTestTextract(srv.URL).Recognize(context.Background(), []Image{{ID: "a", MIME: "image/png", Data: []byte("x")}}, nil)
	if err == nil || !strings.Contains(err.Error(), "security token") {
		t.Fatalf("want a batch error naming the cause, got %v", err)
	}
}

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

	g := &googleVision{endpoint: srv.URL, apiKey: "k", feature: "TEXT_DETECTION", workers: 2}
	images := []Image{{ID: "text", Data: []byte("text")}, {ID: "bad", Data: []byte("bad")}}
	for i := 0; i < 18; i++ { // 20 images: must split into two requests
		images = append(images, Image{ID: "blank" + string(rune('a'+i)), Data: []byte("blank")})
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
	g := &googleVision{endpoint: srv.URL, apiKey: "k", feature: "TEXT_DETECTION", workers: 1}
	if _, err := g.Recognize(context.Background(), []Image{{ID: "a", Data: []byte("x")}}, nil); err == nil || !strings.Contains(err.Error(), "API key not valid") {
		t.Fatalf("want a batch error naming the cause, got %v", err)
	}
}

// Unconfigured, a remote engine refuses to start and says what to set.
func TestRemoteEnginesNeedConfiguration(t *testing.T) {
	for _, v := range []string{"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "GOOGLE_VISION_API_KEY", "GOOGLE_VISION_ACCESS_TOKEN"} {
		t.Setenv(v, "")
	}
	for name, want := range map[string]string{Textract: "AWS_ACCESS_KEY_ID", GoogleVision: "GOOGLE_VISION_API_KEY"} {
		if _, err := NewRecognizer(name); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: want an error naming %s, got %v", name, want, err)
		}
	}
	if d := Default(); d == Textract || d == GoogleVision {
		t.Fatalf("Default() must never pick an engine that sends images away, got %q", d)
	}
}
