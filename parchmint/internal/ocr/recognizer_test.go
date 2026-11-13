package ocr

import (
	"context"
	"testing"

	"github.com/goodblaster/parcher/parchmint/internal/ocr/std"
)

// fake stands in for a remote service: it sees the whole batch at once.
type fake struct{ got []Image }

func (f *fake) Name() string { return "fake" }

func (f *fake) Recognize(_ context.Context, images []Image, _ []string) ([]Result, error) {
	f.got = images
	var out []Result
	for _, img := range images {
		out = append(out, Result{ID: img.ID, Lines: []std.Line{{LineText: string(img.Data)}}})
	}
	return out, nil
}

func TestRegisterAndSelect(t *testing.T) {
	f := &fake{}
	Register("fake", func() (Recognizer, error) { return f, nil })

	r, err := NewRecognizer("fake")
	if err != nil {
		t.Fatal(err)
	}
	res, err := r.Recognize(context.Background(), []Image{{ID: "a", Data: []byte("one")}, {ID: "b", Data: []byte("two")}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.got) != 2 || len(res) != 2 || res[1].ID != "b" || res[1].Lines[0].LineText != "two" {
		t.Fatalf("batch not passed through whole: got %d images, results %+v", len(f.got), res)
	}

	if _, err := NewRecognizer("no-such-engine"); err == nil {
		t.Fatal("unknown engine must be an error")
	}
	// The built-in engines are registered through the same door.
	for _, name := range []string{AppleVision, Tesseract} {
		if _, err := NewRecognizer(name); err != nil {
			t.Errorf("built-in %q not registered: %v", name, err)
		}
	}
}

func TestPerImageStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	res, err := PerImage(New(WindowsOCR)).Recognize(ctx, []Image{{ID: "a"}}, nil)
	if err != nil || len(res) != 1 || res[0].Err == nil {
		t.Fatalf("cancelled batch should report each image failed, got %+v, %v", res, err)
	}
}
