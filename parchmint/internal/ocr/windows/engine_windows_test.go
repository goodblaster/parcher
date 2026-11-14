//go:build windows

package windows

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/goodblaster/parcher/parchmint/internal/ocr"
)

// The real thing: Windows' own OCR reading an image whose text is known.
// testdata/parchment.png says "PARCHMENT 2026" in black on white.
func TestRecognizeFixture(t *testing.T) {
	data, err := os.ReadFile("testdata/parchment.png")
	if err != nil {
		t.Fatal(err)
	}
	r, err := ocr.NewRecognizer(Name)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	res, err := r.Recognize(ctx, []ocr.Image{
		{ID: "png", MIME: "image/png", Data: data},
		{ID: "junk", MIME: "image/png", Data: []byte("not an image")},
	}, []string{"en-US"})
	if err != nil {
		if strings.Contains(err.Error(), "no OCR language") || strings.Contains(err.Error(), "not available") {
			t.Skipf("SKIPPED, NOT VERIFIED — this machine cannot run Windows OCR: %v", err)
		}
		t.Fatal(err)
	}
	if len(res) != 2 || res[0].ID != "png" || res[1].ID != "junk" {
		t.Fatalf("results out of order or missing: %+v", res)
	}
	if res[1].Err == nil {
		t.Error("undecodable bytes should be that image's error")
	}
	if res[0].Err != nil {
		t.Fatalf("fixture: %v", res[0].Err)
	}
	var text []string
	for _, line := range res[0].Lines {
		text = append(text, line.LineText)
		for _, w := range line.Words {
			if w.Left < 0 || w.Top < 0 || w.Left+w.Width > 1.001 || w.Top+w.Height > 1.001 || w.Width <= 0 || w.Height <= 0 {
				t.Errorf("word box is not a fraction of the image: %+v", w)
			}
		}
	}
	got := strings.ToUpper(strings.Join(text, " "))
	t.Logf("Windows OCR read: %q", got)
	if !strings.Contains(got, "PARCHMENT") {
		t.Fatalf("did not read the fixture's text; got %q", got)
	}
}
