package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"mime/quotedprintable"
	"os"
	"path/filepath"
	"testing"

	"github.com/goodblaster/parcher/parchmint/internal/ocr"
	"github.com/goodblaster/parcher/parchmint/internal/ocr/std"
	"github.com/goodblaster/parcher/parchmint/textlayer"
)

type sizeCheckingOCR struct {
	t     *testing.T
	width int
}

func (f sizeCheckingOCR) Name() string { return "size-check" }
func (f sizeCheckingOCR) Recognize(_ context.Context, images []ocr.Image, _ []string) ([]ocr.Result, error) {
	f.t.Helper()
	if len(images) != 1 {
		f.t.Fatalf("engine received %d images", len(images))
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(images[0].Data))
	if err != nil || cfg.Width != f.width {
		f.t.Fatalf("OCR received prematurely resized image: %+v, %v", cfg, err)
	}
	return []ocr.Result{{ID: images[0].ID, Lines: []std.Line{{Confidence: 1, Words: []std.Word{{Text: "Readable", Left: .1, Top: .2, Width: .5, Height: .1}}}}}}, nil
}

func TestOCRDownsizeAfterRecognition(t *testing.T) {
	im := image.NewRGBA(image.Rect(0, 0, 800, 400))
	for y := 0; y < 400; y++ {
		for x := 0; x < 800; x++ {
			im.SetRGBA(x, y, color.RGBA{uint8(x*11 + y), uint8(x + y*31), uint8(y), 255})
		}
	}
	var b bytes.Buffer
	if err := jpeg.Encode(&b, im, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatal(err)
	}
	src := "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(b.Bytes())
	hash := textlayer.FNV1a(src)
	for _, format := range []string{"html", "mht"} {
		for _, downsize := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/downsize=%v", format, downsize), func(t *testing.T) {
				engineName := fmt.Sprintf("size-check-%s-%v", format, downsize)
				expectedWidth := 800
				ocr.Register(engineName, func() (ocr.Recognizer, error) { return sizeCheckingOCR{t, expectedWidth}, nil })
				// No sizing attributes: this also verifies compatibility with old archives.
				doc := []byte(`<html lang="en-US"><body><img src="` + src + `" style="width:100px;height:50px"><img src="` + src + `" style="width:200px;height:100px"></body></html>`)
				data := doc
				if format == "mht" {
					var qp bytes.Buffer
					w := quotedprintable.NewWriter(&qp)
					_, _ = w.Write(doc)
					_ = w.Close()
					data = []byte("From: <Saved by Blink>\r\nContent-Type: multipart/related; boundary=\"B\"\r\n\r\n--B\r\nContent-Type: text/html\r\nContent-Transfer-Encoding: quoted-printable\r\nContent-Location: https://example.com/\r\n\r\n" + qp.String() + "\r\n--B\r\nContent-Type: text/css\r\nContent-Location: https://example.com/style.css\r\n\r\nbody { color: black; }\r\n--B--\r\n")
				}
				layer := &textlayer.Layer{Format: textlayer.Format, Viewport: textlayer.Viewport{DPR: 2, Width: 800}, Images: []textlayer.Image{
					{Node: "1/0", Box: textlayer.Box{0, 0, 100, 50}, Natural: [2]int{800, 400}, Hash: hash},
					{Node: "1/1", Box: textlayer.Box{0, 100, 200, 100}, Natural: [2]int{800, 400}, Hash: hash},
				}}
				raw, err := layer.Marshal()
				if err != nil {
					t.Fatal(err)
				}
				data, err = textlayer.EmbedInArchive(data, raw)
				if err != nil {
					t.Fatal(err)
				}
				dir := t.TempDir()
				input := filepath.Join(dir, "source."+format)
				output := filepath.Join(dir, "output."+format)
				if err := os.WriteFile(input, data, 0600); err != nil {
					t.Fatal(err)
				}
				args := []string{"--engine", engineName, "-o", output, input}
				if downsize {
					args = append(args, "--downsize-images")
				}
				runOCRCommand(args)
				unchanged, _ := os.ReadFile(input)
				if !bytes.Equal(unchanged, data) {
					t.Fatal("-o changed the original")
				}
				result, err := os.ReadFile(output)
				if err != nil {
					t.Fatal(err)
				}
				indexed, err := textlayer.FromBytes(result, output)
				if err != nil {
					t.Fatal(err)
				}
				if len(indexed.Blocks) != 2 {
					t.Fatalf("OCR blocks: %+v", indexed.Blocks)
				}
				if indexed.Blocks[1].Words[0].Box != (textlayer.Box{20, 120, 100, 10}) {
					t.Fatal("OCR coordinates changed")
				}
				doc = result
				if format == "mht" {
					doc, err = textlayer.MHTDocument(result)
					if err != nil {
						t.Fatal(err)
					}
					if !bytes.Contains(result, []byte("body { color: black; }")) {
						t.Fatal("lost unrelated MIME part")
					}
				}
				images := imageBytesByHash(doc)
				for _, img := range indexed.Images {
					ai, ok := images[img.Hash]
					if !ok {
						t.Fatal("stale image hash")
					}
					cfg, _, err := image.DecodeConfig(bytes.NewReader(ai.data))
					if err != nil {
						t.Fatal(err)
					}
					want := 800
					if downsize {
						want = 400
					}
					if cfg.Width != want {
						t.Fatalf("pixels=%d, want=%d", cfg.Width, want)
					}
					if img.Natural != [2]int{800, 400} {
						t.Fatal("changed CSS intrinsic dimensions")
					}
				}
				for _, block := range indexed.Blocks {
					if block.Image != indexed.Images[0].Hash {
						t.Fatal("stale OCR image link")
					}
				}
				if downsize {
					if len(result) >= len(data) {
						t.Fatal("archive did not get smaller")
					}
					// Reindexing must find the resized sources, with no stale hashes or
					// duplicated OCR blocks. No browser is involved in either pass.
					expectedWidth = 400
					runOCRCommand([]string{"--engine", engineName, "--downsize-images", output})
					again, err := textlayer.FromFile(output)
					if err != nil {
						t.Fatal(err)
					}
					if len(again.Blocks) != 2 {
						t.Fatal("repeated OCR duplicated or lost blocks")
					}
				}
			})
		}
	}
}
