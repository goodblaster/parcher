package ocr

import (
	"context"
	"errors"
	"sync/atomic"
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

	if !Registered("fake") || Registered("no-such-engine") {
		t.Fatal("Registered disagrees with the registry")
	}
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
}

// echo is a local one-image engine that records how many run at once.
type echo struct{ running, peak atomic.Int32 }

func (e *echo) Name() string { return "echo" }

func (e *echo) ParseBytes(data []byte, _ []string) ([]std.Line, error) {
	n := e.running.Add(1)
	defer e.running.Add(-1)
	for {
		p := e.peak.Load()
		if n <= p || e.peak.CompareAndSwap(p, n) {
			break
		}
	}
	if string(data) == "bad" {
		return nil, errors.New("unreadable")
	}
	return []std.Line{{LineText: string(data)}}, nil
}

// Parallel reading must not reorder results, lose one, or let one failure
// touch another image — and must never exceed the worker count.
func TestPerImageKeepsOrder(t *testing.T) {
	var images []Image
	for i := 0; i < 50; i++ {
		data := string(rune('a' + i%26))
		if i == 7 {
			data = "bad"
		}
		images = append(images, Image{ID: string(rune('A'+i%26)) + string(rune('0'+i/26)), Data: []byte(data)})
	}
	e := &echo{}
	res, err := PerImage(e, 4).Recognize(context.Background(), images, nil)
	if err != nil || len(res) != len(images) {
		t.Fatalf("%d results for %d images, err %v", len(res), len(images), err)
	}
	for i, r := range res {
		if r.ID != images[i].ID {
			t.Fatalf("result %d is for %s, want %s", i, r.ID, images[i].ID)
		}
		if i == 7 {
			if r.Err == nil {
				t.Error("the bad image should carry its error")
			}
			continue
		}
		if r.Err != nil || r.Lines[0].LineText != string(images[i].Data) {
			t.Fatalf("result %d: %+v", i, r)
		}
	}
	if p := e.peak.Load(); p > 4 {
		t.Errorf("%d images read at once with 4 workers", p)
	}
}

func TestPerImageStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	res, err := PerImage(&echo{}, 2).Recognize(ctx, []Image{{ID: "a"}}, nil)
	if err != nil || len(res) != 1 || res[0].Err == nil {
		t.Fatalf("cancelled batch should report each image failed, got %+v, %v", res, err)
	}
}
