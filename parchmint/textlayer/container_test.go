package textlayer

import (
	"bytes"
	"testing"
)

func sampleLayerJSON() []byte {
	return []byte(`{"format":"parchmint-text/1","url":"https://ex.com","blocks":[{"id":0,"type":"p","text":"hello </script> world","words":[]}]}`)
}

func TestSniffers(t *testing.T) {
	html := []byte("<!DOCTYPE html><html><body>hi</body></html>")
	mht := []byte("From: <Saved by Blink>\r\nContent-Type: multipart/related; boundary=\"B\"\r\n\r\n")
	pdf := []byte("%PDF-1.7\n...")

	if IsMHT(html) || IsPDF(html) {
		t.Error("html misclassified")
	}
	if !IsMHT(mht) {
		t.Error("mht not detected")
	}
	if IsPDF(mht) {
		t.Error("mht classified as pdf")
	}
	if !IsPDF(pdf) {
		t.Error("pdf not detected")
	}
	if IsMHT(pdf) {
		t.Error("pdf classified as mht")
	}
}

func TestEmbedRoundTripHTML(t *testing.T) {
	html := []byte("<html><body><p>content</p></body></html>")
	layer := sampleLayerJSON()

	embedded := EmbedInHTML(html, layer)
	if bytes.Contains(embedded, []byte("</script></script>")) {
		t.Error("payload should not contain a raw closing script tag")
	}
	// The literal `</script>` inside the JSON must have been escaped.
	if bytes.Count(embedded, []byte("</script>")) != 1 {
		t.Errorf("expected exactly one real </script>, got %d", bytes.Count(embedded, []byte("</script>")))
	}

	got, err := FromBytes(embedded, "test.html")
	if err != nil {
		t.Fatalf("FromBytes: %v", err)
	}
	if got.Format != "parchmint-text/1" || len(got.Blocks) != 1 {
		t.Fatalf("parsed layer wrong: %+v", got)
	}
	// The escaped </script> inside block text must round-trip intact.
	if got.Blocks[0].Text != "hello </script> world" {
		t.Errorf("block text %q, want %q", got.Blocks[0].Text, "hello </script> world")
	}
}

func TestEmbedIdempotentHTML(t *testing.T) {
	html := []byte("<html><body></body></html>")
	once := EmbedInHTML(html, sampleLayerJSON())
	twice := EmbedInHTML(once, sampleLayerJSON())
	if bytes.Count(twice, []byte(`type="`+ScriptType+`"`)) != 1 {
		t.Error("re-embedding should replace, not duplicate, the layer element")
	}
}

func TestEmbedRoundTripMHT(t *testing.T) {
	mht := []byte("From: <Saved by Blink>\r\n" +
		"Content-Type: multipart/related; type=\"text/html\"; boundary=\"----B----\"\r\n\r\n" +
		"------B----\r\nContent-Type: text/html\r\nContent-Transfer-Encoding: quoted-printable\r\n\r\n" +
		"<html><body>hi</body></html>\r\n" +
		"------B------\r\n")

	embedded, err := EmbedInMHT(mht, sampleLayerJSON())
	if err != nil {
		t.Fatalf("EmbedInMHT: %v", err)
	}
	got, err := FromBytes(embedded, "test.mht")
	if err != nil {
		t.Fatalf("FromBytes(mht): %v", err)
	}
	if len(got.Blocks) != 1 || got.Blocks[0].Text != "hello </script> world" {
		t.Fatalf("mht layer wrong: %+v", got)
	}

	// Idempotent: re-embedding replaces the part.
	twice, err := EmbedInMHT(embedded, sampleLayerJSON())
	if err != nil {
		t.Fatalf("re-embed: %v", err)
	}
	if bytes.Count(twice, []byte("Content-Type: "+MHTPartType)) != 1 {
		t.Error("re-embedding MHT should replace, not duplicate, the layer part")
	}
	// The primary document is still present after embedding.
	doc, err := MHTDocument(twice)
	if err != nil || !bytes.Contains(doc, []byte("<body>hi</body>")) {
		t.Errorf("MHTDocument lost the document part: %q err=%v", doc, err)
	}
}

func TestFromBytesNoLayer(t *testing.T) {
	if _, err := FromBytes([]byte("<html><body>nothing</body></html>"), "x.html"); err == nil {
		t.Error("expected error when no layer is embedded")
	}
}

func TestFNV1aMatchesJS(t *testing.T) {
	// Locks the hash to the value the JS (fnv1a in text_layer.js /
	// overlay_ocr_text.js) produces, so image pairing stays in sync.
	// FNV-1a/32 of "hello": 0x4f9f2cab.
	if got := FNV1a("hello"); got != "4f9f2cab" {
		t.Errorf("FNV1a(hello) = %s, want 4f9f2cab", got)
	}
}
