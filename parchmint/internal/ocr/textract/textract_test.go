package textract

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

	"github.com/goodblaster/parcher/parchmint/internal/ocr"
	"github.com/goodblaster/parcher/parchmint/internal/ocr/remote"
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

func newTestTextract(url string) *engine {
	return &engine{endpoint: url, region: "us-east-1", workers: 2, now: time.Now,
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
	remote.RetryBase = time.Millisecond
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

	res, err := newTestTextract(srv.URL).Recognize(context.Background(), []ocr.Image{
		{ID: "a", MIME: "image/png", Data: []byte("\x89PNG fake")},
		{ID: "b", MIME: "image/jpeg", Data: []byte("bad one")},
		{ID: "c", MIME: "image/gif", Data: tinyGIF(t)},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]ocr.Result{}
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
	_, err := newTestTextract(srv.URL).Recognize(context.Background(), []ocr.Image{{ID: "a", MIME: "image/png", Data: []byte("x")}}, nil)
	if err == nil || !strings.Contains(err.Error(), "security token") {
		t.Fatalf("want a batch error naming the cause, got %v", err)
	}
}

// Unconfigured, the engine refuses to start and says what to set.
func TestNeedsConfiguration(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "")
	if _, err := ocr.NewRecognizer(Name); err == nil || !strings.Contains(err.Error(), "AWS_ACCESS_KEY_ID") {
		t.Fatalf("want an error naming AWS_ACCESS_KEY_ID, got %v", err)
	}
}
