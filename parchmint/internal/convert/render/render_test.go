package render

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/goodblaster/go-mht/pkg/documents"
)

// mht assembles an MHT document from part texts, each of which must start
// with its headers. Bodies use 8bit so fixtures can be written literally,
// without quoted-printable escaping.
func mht(t *testing.T, parts ...string) *documents.Document {
	t.Helper()

	const boundary = "----MultipartBoundary--test----"
	var builder strings.Builder
	builder.WriteString("From: <Saved by Blink>\r\n")
	builder.WriteString("Snapshot-Content-Location: https://example.com/\r\n")
	builder.WriteString("MIME-Version: 1.0\r\n")
	builder.WriteString("Content-Type: multipart/related;\r\n\ttype=\"text/html\";\r\n\tboundary=\"" + boundary + "\"\r\n")
	builder.WriteString("\r\n\r\n")

	for _, part := range parts {
		builder.WriteString("--" + boundary + "\r\n")
		builder.WriteString(strings.ReplaceAll(part, "\n", "\r\n"))
		builder.WriteString("\r\n")
	}
	builder.WriteString("--" + boundary + "--\r\n")

	doc, err := documents.Parse(strings.NewReader(builder.String()))
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}

	return doc
}

// pixel is a 1x1 PNG, as base64 — the same one the package testdata uses.
const pixel = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg=="

func htmlPart(body string) string {
	return "Content-Type: text/html\n" +
		"Content-Transfer-Encoding: 8bit\n" +
		"Content-Location: https://example.com/\n" +
		"\n" + body
}

func imagePart(location string) string {
	return "Content-Type: image/png\n" +
		"Content-Transfer-Encoding: base64\n" +
		"Content-Location: " + location + "\n" +
		"\n" + pixel
}

func cssPart(location, body string) string {
	return "Content-Type: text/css\n" +
		"Content-Transfer-Encoding: 8bit\n" +
		"Content-Location: " + location + "\n" +
		"\n" + body
}

func render(t *testing.T, doc *documents.Document) string {
	t.Helper()

	out, err := Document(doc, Options{})
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	return string(out)
}

func TestImageBecomesDataURI(t *testing.T) {
	doc := mht(t,
		htmlPart(`<img src="/logo.png">`),
		imagePart("https://example.com/logo.png"),
	)

	out := render(t, doc)
	if !strings.Contains(out, `src="data:image/png;base64,`+pixel+`"`) {
		t.Errorf("image not inlined:\n%s", out)
	}
}

func TestAbsoluteAndRelativeReferencesBothResolve(t *testing.T) {
	doc := mht(t,
		htmlPart(`<img src="logo.png"><img src="https://example.com/logo.png">`),
		imagePart("https://example.com/logo.png"),
	)

	if got := strings.Count(render(t, doc), "data:image/png"); got != 2 {
		t.Errorf("inlined %d of 2 references", got)
	}
}

func TestStylesheetInlinedAsStyleElement(t *testing.T) {
	doc := mht(t,
		htmlPart(`<link rel="stylesheet" media="print" href="/site.css">`),
		cssPart("https://example.com/site.css", `body { background: url(logo.png); }`),
		imagePart("https://example.com/logo.png"),
	)

	out := render(t, doc)
	if !strings.Contains(out, `<style media="print">`) {
		t.Errorf("stylesheet not inlined as <style>:\n%s", out)
	}
	if !strings.Contains(out, `url("data:image/png;base64,`) {
		t.Errorf("url() inside stylesheet not rewritten:\n%s", out)
	}
	if strings.Contains(out, "<link") {
		t.Errorf("<link> element survived:\n%s", out)
	}
}

// A stylesheet is resolved relative to its own location, not the page's.
func TestStylesheetURLsResolveAgainstStylesheet(t *testing.T) {
	doc := mht(t,
		htmlPart(`<link rel="stylesheet" href="/css/site.css">`),
		cssPart("https://example.com/css/site.css", `div { background: url(../img/logo.png); }`),
		imagePart("https://example.com/img/logo.png"),
	)

	if !strings.Contains(render(t, doc), `url("data:image/png;base64,`) {
		t.Error("relative url() in stylesheet did not resolve against the stylesheet")
	}
}

func TestStyleAttributeAndStyleElement(t *testing.T) {
	doc := mht(t,
		htmlPart(`<style>b{background:url(/logo.png)}</style><div style="background:url(/logo.png)">`),
		imagePart("https://example.com/logo.png"),
	)

	if got := strings.Count(render(t, doc), "data:image/png"); got != 2 {
		t.Errorf("inlined %d of 2 CSS references", got)
	}
}

func TestSrcsetRewritten(t *testing.T) {
	doc := mht(t,
		htmlPart(`<img srcset="/logo.png 1x, /logo.png 2x" src="/logo.png">`),
		imagePart("https://example.com/logo.png"),
	)

	out := render(t, doc)
	if got := strings.Count(out, "data:image/png"); got != 3 {
		t.Errorf("inlined %d of 3 references:\n%s", got, out)
	}
	if !strings.Contains(out, ` 2x`) {
		t.Errorf("srcset descriptor lost:\n%s", out)
	}
}

func TestContentIDReference(t *testing.T) {
	doc := mht(t,
		htmlPart(`<iframe src="cid:frame-2@mhtml.blink"></iframe>`),
		"Content-Type: text/html\n"+
			"Content-Transfer-Encoding: 8bit\n"+
			"Content-ID: <frame-2@mhtml.blink>\n"+
			"Content-Location: https://example.com/frame\n"+
			"\n<p>inner</p>",
	)

	out := render(t, doc)
	prefix := "data:text/html;base64,"
	index := strings.Index(out, prefix)
	if index < 0 {
		t.Fatalf("frame not inlined:\n%s", out)
	}

	encoded := out[index+len(prefix):]
	encoded, _, _ = strings.Cut(encoded, `"`)
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("decode frame: %v", err)
	}
	if !strings.Contains(string(decoded), "<p>inner</p>") {
		t.Errorf("frame content wrong: %q", decoded)
	}
}

func TestMissingResourceLeftAlone(t *testing.T) {
	doc := mht(t, htmlPart(`<img src="https://elsewhere.example/x.png">`))

	var warnings []string
	out, err := Document(doc, Options{OnWarn: func(message string) {
		warnings = append(warnings, message)
	}})
	if err != nil {
		t.Fatalf("render: %v", err)
	}

	if !strings.Contains(string(out), `src="https://elsewhere.example/x.png"`) {
		t.Errorf("missing resource was not left intact:\n%s", out)
	}
	if len(warnings) != 1 {
		t.Errorf("got %d warnings, want 1: %v", len(warnings), warnings)
	}
}

// Anchors and form targets are documents, not subresources.
func TestHyperlinksUntouched(t *testing.T) {
	doc := mht(t,
		htmlPart(`<a href="/logo.png">link</a>`),
		imagePart("https://example.com/logo.png"),
	)

	if strings.Contains(render(t, doc), "data:") {
		t.Error("anchor href was inlined")
	}
}

func TestSelfReferencingStylesheetTerminates(t *testing.T) {
	doc := mht(t,
		htmlPart(`<link rel="stylesheet" href="/a.css">`),
		cssPart("https://example.com/a.css", `@import "b.css";`),
		cssPart("https://example.com/b.css", `@import "a.css";`),
	)

	// The cycle must be broken rather than recursed into; the render itself
	// still succeeds.
	out := render(t, doc)
	if !strings.Contains(out, "<style>") {
		t.Errorf("stylesheet not inlined:\n%s", out)
	}
}

func TestOmitScripts(t *testing.T) {
	doc := mht(t,
		htmlPart(`<script src="/app.js"></script>`),
		"Content-Type: text/javascript\nContent-Transfer-Encoding: 8bit\n"+
			"Content-Location: https://example.com/app.js\n\nalert(1)",
	)

	out, err := Document(doc, Options{OmitScripts: true})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if strings.Contains(string(out), "alert") {
		t.Errorf("script body survived OmitScripts:\n%s", out)
	}
}

func TestBaseElementChangesResolution(t *testing.T) {
	doc := mht(t,
		htmlPart(`<head><base href="https://example.com/assets/"></head><img src="logo.png">`),
		imagePart("https://example.com/assets/logo.png"),
	)

	if !strings.Contains(render(t, doc), "data:image/png") {
		t.Error("<base href> was not honoured")
	}
}

func TestTestdataSample(t *testing.T) {
	out, err := File("../testdata/sample.mht", Options{})
	if err != nil {
		t.Fatalf("render sample: %v", err)
	}
	if !strings.Contains(string(out), "data:image/png;base64,") {
		t.Errorf("sample image not inlined:\n%s", out)
	}
	if !strings.Contains(string(out), "1 + 1 = 2") {
		t.Errorf("quoted-printable body not decoded:\n%s", out)
	}
}
