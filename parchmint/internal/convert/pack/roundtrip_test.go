package pack_test

import (
	"bytes"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goodblaster/go-mht/pkg/documents"
	"github.com/goodblaster/parcher/parchmint/internal/convert/pack"
	"github.com/goodblaster/parcher/parchmint/internal/convert/render"
)

var pixel, _ = base64.StdEncoding.DecodeString(
	"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg==")

// site writes a small page with a stylesheet, an image, and a script, and
// returns the path to the page.
func site(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	files := map[string][]byte{
		"img/logo.png": pixel,
		"css/site.css": []byte("body { background: url(../img/logo.png); color: red }"),
		"js/app.js":    []byte("console.log('hi')"),
		"index.html": []byte(`<!doctype html><html><head><title>Round Trip</title>` +
			`<link rel="stylesheet" href="css/site.css"></head>` +
			`<body><h1>heading</h1><img src="img/logo.png" alt="logo">` +
			`<script src="js/app.js"></script></body></html>`),
	}

	for name, data := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	return filepath.Join(dir, "index.html")
}

// A page packed into an MHT and rendered back out must carry every resource
// it started with, inlined.
func TestPackThenRender(t *testing.T) {
	doc, err := pack.File(t.Context(), site(t), pack.Options{})
	if err != nil {
		t.Fatalf("pack: %v", err)
	}

	out, err := render.Document(doc, render.Options{})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	page := string(out)

	if !strings.Contains(page, "<h1>heading</h1>") {
		t.Error("page text lost")
	}
	// The stylesheet is inlined, and the image it references with it.
	if !strings.Contains(page, "color: red") {
		t.Errorf("stylesheet not inlined:\n%s", page)
	}
	if !strings.Contains(page, `url("data:image/png;base64,`) {
		t.Error("image referenced by the stylesheet not inlined")
	}
	// The <img> and the script too.
	if !strings.Contains(page, `src="data:image/png;base64,`) {
		t.Error("img src not inlined")
	}
	if !strings.Contains(page, `src="data:text/javascript`) {
		t.Errorf("script not inlined:\n%s", page)
	}
	// A data: URI is a URL, so it must not contain spaces even when the
	// media type carries parameters.
	for _, uri := range strings.Split(page, `src="data:`)[1:] {
		uri, _, _ = strings.Cut(uri, `"`)
		if strings.ContainsAny(uri, " \t") {
			t.Errorf("data URI contains whitespace: %q", uri)
		}
	}
	// Nothing should still point at the filesystem.
	if strings.Contains(page, "file://") {
		t.Errorf("a file: reference survived:\n%s", page)
	}
}

// Going the other way: an MHT rendered to HTML and packed again must
// describe the same resources.
func TestRenderThenPack(t *testing.T) {
	original, err := documents.ParseFile("../testdata/sample.mht")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	page, err := render.Document(original, render.Options{})
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	repacked, err := pack.HTML(t.Context(), page, pack.Options{
		Base: original.Headers.Get("Snapshot-Content-Location"),
	})
	if err != nil {
		t.Fatalf("pack: %v", err)
	}

	// Rendering inlined the image as a data: URI and packing leaves those
	// alone, so the archive comes back as a single self-contained document
	// rather than a document plus an image part.
	if len(repacked.Sections) != 1 {
		t.Fatalf("repacked %d sections, want 1", len(repacked.Sections))
	}

	// The image bytes still survive the round trip, now carried inline.
	want, err := original.ImageBytes("https://example.com/pixel.png")
	if err != nil {
		t.Fatalf("original image: %v", err)
	}
	encoded := base64.StdEncoding.EncodeToString(want)
	if !strings.Contains(repacked.Sections[0].Body, encoded) {
		t.Error("image bytes did not survive the round trip")
	}
}

// Rendering is stable: packing a rendered page and rendering it again gives
// the same document back.
func TestRenderIsStable(t *testing.T) {
	doc, err := pack.File(t.Context(), site(t), pack.Options{})
	if err != nil {
		t.Fatalf("pack: %v", err)
	}

	first, err := render.Document(doc, render.Options{})
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	repacked, err := pack.HTML(t.Context(), first, pack.Options{Base: "https://example.com/index.html"})
	if err != nil {
		t.Fatalf("repack: %v", err)
	}

	second, err := render.Document(repacked, render.Options{})
	if err != nil {
		t.Fatalf("re-render: %v", err)
	}

	if !bytes.Equal(first, second) {
		t.Errorf("render is not stable across a repack\nfirst:  %d bytes\nsecond: %d bytes", len(first), len(second))
	}
}
