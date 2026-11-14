package pack

import (
	"bytes"
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/goodblaster/go-mht/pkg/documents"
	"github.com/goodblaster/go-mht/pkg/sections"
)

// pixelPNG is a 1x1 PNG.
var pixelPNG, _ = base64.StdEncoding.DecodeString(
	"iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg==")

// write creates a file under dir, making parent directories as needed.
func write(t *testing.T, dir, name string, data []byte) string {
	t.Helper()

	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	return path
}

// section finds the packed section whose Content-Location ends with suffix.
func section(t *testing.T, doc *documents.Document, suffix string) *sections.Section {
	t.Helper()

	for i := range doc.Sections {
		if strings.HasSuffix(doc.Sections[i].Headers.Get("Content-Location"), suffix) {
			return &doc.Sections[i]
		}
	}
	t.Fatalf("no section ending in %q; have %v", suffix, locations(doc))

	return nil
}

func locations(doc *documents.Document) []string {
	var found []string
	for i := range doc.Sections {
		found = append(found, doc.Sections[i].Headers.Get("Content-Location")+
			doc.Sections[i].Headers.Get("Content-ID"))
	}
	return found
}

func TestPackLocalFiles(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "img/logo.png", pixelPNG)
	write(t, dir, "css/site.css", []byte(`body { background: url(../img/logo.png) }`))
	page := write(t, dir, "page.html", []byte(
		`<html><head><title>Hello</title><link rel="stylesheet" href="css/site.css">`+
			`</head><body><img src="img/logo.png"></body></html>`))

	doc, err := File(t.Context(), page, Options{})
	if err != nil {
		t.Fatalf("pack: %v", err)
	}

	if len(doc.Sections) != 3 {
		t.Fatalf("got %d sections, want 3: %v", len(doc.Sections), locations(doc))
	}

	if got := doc.Headers.Get("Subject"); got != "Hello" {
		t.Errorf("Subject = %q, want %q", got, "Hello")
	}

	root := doc.Sections[0]
	if got := root.Headers.Get("Content-Type"); got != "text/html" {
		t.Errorf("root Content-Type = %q", got)
	}
	if !strings.HasSuffix(root.Headers.Get("Content-Location"), "/page.html") {
		t.Errorf("root Content-Location = %q", root.Headers.Get("Content-Location"))
	}

	image := section(t, doc, "img/logo.png")
	if got := image.Headers.Get("Content-Transfer-Encoding"); got != "base64" {
		t.Errorf("image encoding = %q, want base64", got)
	}
	data, err := image.ImageBytes()
	if err != nil {
		t.Fatalf("image bytes: %v", err)
	}
	if !bytes.Equal(data, pixelPNG) {
		t.Error("image bytes do not match the source file")
	}

	stylesheet := section(t, doc, "css/site.css")
	if got := stylesheet.Headers.Get("Content-Transfer-Encoding"); got != "quoted-printable" {
		t.Errorf("stylesheet encoding = %q, want quoted-printable", got)
	}
	// The reference stays relative: the stylesheet part carries its own
	// location, so it still resolves.
	if !strings.Contains(stylesheet.Body, "../img/logo.png") {
		t.Errorf("stylesheet reference rewritten unexpectedly: %q", stylesheet.Body)
	}
}

func TestPackedDocumentReparses(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "logo.png", pixelPNG)
	page := write(t, dir, "page.html", []byte(`<img src="logo.png">`))

	doc, err := File(t.Context(), page, Options{})
	if err != nil {
		t.Fatalf("pack: %v", err)
	}

	var buf bytes.Buffer
	if err := doc.Write(&buf); err != nil {
		t.Fatalf("write: %v", err)
	}

	reparsed, err := documents.Parse(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("reparse: %v", err)
	}
	if len(reparsed.Sections) != len(doc.Sections) {
		t.Errorf("reparsed %d sections, want %d", len(reparsed.Sections), len(doc.Sections))
	}

	image := section(t, reparsed, "logo.png")
	data, err := image.ImageBytes()
	if err != nil {
		t.Fatalf("image bytes: %v", err)
	}
	if !bytes.Equal(data, pixelPNG) {
		t.Error("image did not survive the write/parse round trip")
	}
}

// A data: URI is already self-contained. Moving it into a part and pointing
// back at it with cid: deduplicates repeats, but Chrome does not resolve
// those references and every inline image renders as a broken placeholder —
// so the URI stays where it is.
func TestDataURIStaysInline(t *testing.T) {
	encoded := base64.StdEncoding.EncodeToString(pixelPNG)
	uri := "data:image/png;base64," + encoded

	doc, err := HTML(t.Context(), []byte(`<img src="`+uri+`">`), Options{})
	if err != nil {
		t.Fatalf("pack: %v", err)
	}

	if len(doc.Sections) != 1 {
		t.Fatalf("got %d sections, want 1 (the document alone)", len(doc.Sections))
	}
	if !strings.Contains(doc.Sections[0].Body, uri) {
		t.Errorf("data URI did not survive intact: %q", doc.Sections[0].Body)
	}
	if strings.Contains(doc.Sections[0].Body, "cid:") {
		t.Error("data URI was turned into a cid: reference")
	}
}

// The same applies inside CSS, where a data: URI is just as self-contained.
func TestDataURIInCSSStaysInline(t *testing.T) {
	uri := "data:image/png;base64," + base64.StdEncoding.EncodeToString(pixelPNG)

	doc, err := HTML(t.Context(), []byte(`<style>b{background:url(`+uri+`)}</style>`), Options{})
	if err != nil {
		t.Fatalf("pack: %v", err)
	}
	if len(doc.Sections) != 1 {
		t.Errorf("got %d sections, want 1", len(doc.Sections))
	}
	if !strings.Contains(doc.Sections[0].Body, uri) {
		t.Errorf("data URI in CSS did not survive: %q", doc.Sections[0].Body)
	}
}

func TestRepeatedFileReferenceFetchedOnce(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "logo.png", pixelPNG)
	page := write(t, dir, "page.html", []byte(`<img src="logo.png"><img src="./logo.png">`))

	doc, err := File(t.Context(), page, Options{})
	if err != nil {
		t.Fatalf("pack: %v", err)
	}
	if len(doc.Sections) != 2 {
		t.Errorf("got %d sections, want 2: %v", len(doc.Sections), locations(doc))
	}
}

func TestOfflineLeavesRemoteReferences(t *testing.T) {
	source := []byte(`<img src="https://cdn.example.com/logo.png">`)

	var warnings []string
	doc, err := HTML(t.Context(), source, Options{OnWarn: func(message string) {
		warnings = append(warnings, message)
	}})
	if err != nil {
		t.Fatalf("pack: %v", err)
	}

	if len(doc.Sections) != 1 {
		t.Errorf("got %d sections, want 1", len(doc.Sections))
	}
	if !strings.Contains(doc.Sections[0].Body, "https://cdn.example.com/logo.png") {
		t.Error("remote reference was not left in place")
	}
	if len(warnings) != 1 {
		t.Errorf("got %d warnings, want 1: %v", len(warnings), warnings)
	}
}

func TestNetworkFetch(t *testing.T) {
	var requests int

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		switch r.URL.Path {
		case "/site.css":
			w.Header().Set("Content-Type", "text/css")
			w.Write([]byte(`div { background: url(nested/logo.png) }`))
		case "/nested/logo.png", "/logo.png":
			w.Header().Set("Content-Type", "image/png")
			w.Write(pixelPNG)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	source := []byte(`<link rel="stylesheet" href="/site.css"><img src="/logo.png">`)

	doc, err := HTML(t.Context(), source, Options{
		Base:    server.URL + "/page.html",
		Network: true,
		Client:  server.Client(),
	})
	if err != nil {
		t.Fatalf("pack: %v", err)
	}

	// Root, stylesheet, page image, and the image the stylesheet imports.
	if len(doc.Sections) != 4 {
		t.Fatalf("got %d sections, want 4: %v", len(doc.Sections), locations(doc))
	}
	if requests != 3 {
		t.Errorf("made %d requests, want 3", requests)
	}

	nested := section(t, doc, "/nested/logo.png")
	if got := nested.Headers.Get("Content-Type"); got != "image/png" {
		t.Errorf("nested image Content-Type = %q", got)
	}
}

func TestFailedFetchIsWarnedNotFatal(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "gone", http.StatusNotFound)
	}))
	defer server.Close()

	var warnings []string
	doc, err := HTML(t.Context(), []byte(`<img src="/missing.png">`), Options{
		Base:    server.URL + "/page.html",
		Network: true,
		Client:  server.Client(),
		OnWarn:  func(message string) { warnings = append(warnings, message) },
	})
	if err != nil {
		t.Fatalf("pack: %v", err)
	}

	if len(doc.Sections) != 1 {
		t.Errorf("got %d sections, want 1", len(doc.Sections))
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "404") {
		t.Errorf("warnings = %v", warnings)
	}
}

func TestMaxSizeRejectsLargeResource(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "big.png", bytes.Repeat([]byte{0}, 4096))
	page := write(t, dir, "page.html", []byte(`<img src="big.png">`))

	var warnings []string
	doc, err := File(t.Context(), page, Options{
		MaxSize: 1024,
		OnWarn:  func(message string) { warnings = append(warnings, message) },
	})
	if err != nil {
		t.Fatalf("pack: %v", err)
	}

	if len(doc.Sections) != 1 {
		t.Errorf("oversized resource was included")
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "limit") {
		t.Errorf("warnings = %v", warnings)
	}
}

func TestHyperlinksAndScriptsHandled(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "app.js", []byte("alert(1)"))
	write(t, dir, "other.html", []byte("<p>other</p>"))
	page := write(t, dir, "page.html", []byte(
		`<a href="other.html">link</a><script src="app.js"></script>`))

	doc, err := File(t.Context(), page, Options{})
	if err != nil {
		t.Fatalf("pack: %v", err)
	}

	// The script is packed; the anchor target is not.
	if len(doc.Sections) != 2 {
		t.Fatalf("got %d sections, want 2: %v", len(doc.Sections), locations(doc))
	}
	// A text body ends with one line terminator. The line break before the
	// next boundary is the writer's, not part of the body.
	script := section(t, doc, "app.js")
	if script.Body != "alert(1)\r\n" {
		t.Errorf("script body = %q", script.Body)
	}
}

func TestFramesAreFollowed(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "logo.png", pixelPNG)
	write(t, dir, "inner.html", []byte(`<img src="logo.png">`))
	page := write(t, dir, "page.html", []byte(`<iframe src="inner.html"></iframe>`))

	doc, err := File(t.Context(), page, Options{})
	if err != nil {
		t.Fatalf("pack: %v", err)
	}

	if len(doc.Sections) != 3 {
		t.Fatalf("got %d sections, want 3: %v", len(doc.Sections), locations(doc))
	}
	frame := section(t, doc, "inner.html")
	if got := frame.Headers.Get("Content-ID"); got != "<frame-0002@mhtml.goodblaster>" {
		t.Errorf("frame Content-ID = %q", got)
	}
}

func TestCyclicFramesTerminate(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "a.html", []byte(`<iframe src="b.html"></iframe>`))
	write(t, dir, "b.html", []byte(`<iframe src="a.html"></iframe>`))

	doc, err := File(t.Context(), filepath.Join(dir, "a.html"), Options{})
	if err != nil {
		t.Fatalf("pack: %v", err)
	}
	// a.html is the root and b.html the only other part: the cycle back to
	// a.html is already seen.
	if len(doc.Sections) != 2 {
		t.Errorf("got %d sections, want 2: %v", len(doc.Sections), locations(doc))
	}
}

func TestNonASCIISubjectEncoded(t *testing.T) {
	doc, err := HTML(t.Context(), []byte("<title>Café ☕</title>"), Options{})
	if err != nil {
		t.Fatalf("pack: %v", err)
	}

	subject := doc.Headers.Get("Subject")
	if !strings.HasPrefix(subject, "=?utf-8?q?") {
		t.Errorf("Subject = %q, want RFC 2047 encoding", subject)
	}
}

func TestExplicitOptionsWin(t *testing.T) {
	doc, err := HTML(context.Background(), []byte("<title>page</title>"), Options{
		Base:    "https://example.com/x.html",
		Subject: "override",
		From:    "<Saved by Blink>",
	})
	if err != nil {
		t.Fatalf("pack: %v", err)
	}

	if got := doc.Headers.Get("Subject"); got != "override" {
		t.Errorf("Subject = %q", got)
	}
	if got := doc.Headers.Get("From"); got != "<Saved by Blink>" {
		t.Errorf("From = %q", got)
	}
	if got := doc.Headers.Get("Snapshot-Content-Location"); got != "https://example.com/x.html" {
		t.Errorf("Snapshot-Content-Location = %q", got)
	}
}

// A saved page can reference a resource that escapes the directory it was
// saved into. Against the file on disk that lands where nothing was ever
// written; only the page's own address can supply it.
func TestFallbackBaseRetrievesEscapingReference(t *testing.T) {
	var requested []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requested = append(requested, r.URL.Path)
		if r.URL.Path != "/assets/logo.png" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Write(pixelPNG)
	}))
	defer server.Close()

	// The page sits in a sub-directory and points up out of it.
	dir := t.TempDir()
	page := write(t, dir, "saved/page.html", []byte(`<img src="../assets/logo.png">`))

	options := func(network bool) Options {
		return Options{
			Base:         "file://" + page,
			FallbackBase: server.URL + "/saved/page.html",
			Network:      network,
			OnWarn:       func(string) {},
		}
	}

	// Offline there is no second place to look, so the reference is left.
	doc, err := File(t.Context(), page, options(false))
	if err != nil {
		t.Fatalf("pack offline: %v", err)
	}
	if len(doc.Sections) != 1 {
		t.Errorf("offline: got %d sections, want 1", len(doc.Sections))
	}
	if len(requested) != 0 {
		t.Errorf("offline: made requests %v, want none", requested)
	}

	// With the network, the fallback supplies it — and the part is named by
	// the address it actually came from, which is what the reference will
	// resolve to once the archive is re-keyed.
	doc, err = File(t.Context(), page, options(true))
	if err != nil {
		t.Fatalf("pack online: %v", err)
	}
	if len(doc.Sections) != 2 {
		t.Fatalf("online: got %d sections, want 2: %v", len(doc.Sections), locations(doc))
	}

	fetched := section(t, doc, "/assets/logo.png")
	if got := fetched.Headers.Get("Content-Location"); got != server.URL+"/assets/logo.png" {
		t.Errorf("part named %q, want the address it came from", got)
	}
	data, err := fetched.ImageBytes()
	if err != nil {
		t.Fatalf("image bytes: %v", err)
	}
	if !bytes.Equal(data, pixelPNG) {
		t.Error("fetched image bytes wrong")
	}
}

// A resource present on disk is read from there; the fallback is a last
// resort, not a preference.
func TestFallbackNotUsedWhenLocalCopyExists(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Write([]byte("from the network"))
	}))
	defer server.Close()

	dir := t.TempDir()
	write(t, dir, "logo.png", pixelPNG)
	page := write(t, dir, "page.html", []byte(`<img src="logo.png">`))

	doc, err := File(t.Context(), page, Options{
		FallbackBase: server.URL + "/page.html",
		Network:      true,
		OnWarn:       func(string) {},
	})
	if err != nil {
		t.Fatalf("pack: %v", err)
	}

	if requests != 0 {
		t.Errorf("made %d requests, want 0 — the file was on disk", requests)
	}
	data, err := section(t, doc, "logo.png").ImageBytes()
	if err != nil {
		t.Fatalf("image bytes: %v", err)
	}
	if !bytes.Equal(data, pixelPNG) {
		t.Error("local copy was not the one packed")
	}
}
