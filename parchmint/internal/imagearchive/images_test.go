package imagearchive

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"mime/multipart"
	"net/textproto"
	"strings"
	"testing"

	"github.com/goodblaster/parcher/parchmint/textlayer"
	"golang.org/x/net/html"
)

func fixtureSource(t *testing.T) string {
	im := image.NewRGBA(image.Rect(0, 0, 800, 400))
	for y := 0; y < 400; y++ {
		for x := 0; x < 800; x++ {
			im.SetRGBA(x, y, color.RGBA{uint8(x*31 + y), uint8(y*17 + x), uint8(x), 255})
		}
	}
	var b bytes.Buffer
	if err := jpeg.Encode(&b, im, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatal(err)
	}
	return "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(b.Bytes())
}

func TestDownsizeSharedSourceAcrossMHTFrames(t *testing.T) {
	src := fixtureSource(t)
	var b bytes.Buffer
	b.WriteString("Content-Type: multipart/related; boundary=\"B\"\r\n\r\n")
	w := multipart.NewWriter(&b)
	if err := w.SetBoundary("B"); err != nil {
		t.Fatal(err)
	}
	for i, size := range []string{"100,50,800", "300,150,800"} {
		part, err := w.CreatePart(textproto.MIMEHeader{"Content-Type": {"text/html"}, "Content-Location": {fmt.Sprintf("https://example.com/%d", i)}})
		if err != nil {
			t.Fatal(err)
		}
		_, _ = fmt.Fprintf(part, `<img src="%s" data-parch-image-size="%s">`, src, size)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	layer := &textlayer.Layer{Images: []textlayer.Image{{Hash: textlayer.FNV1a(src), Natural: [2]int{800, 400}, Box: textlayer.Box{0, 0, 100, 50}}, {Hash: textlayer.FNV1a(src), Natural: [2]int{800, 400}, Frame: "frame", Box: textlayer.Box{0, 0, 300, 150}}}}
	out, count, err := Downsize(b.Bytes(), layer)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("changed %d images", count)
	}
	documents := 0
	_, err = MapDocuments(out, func(doc []byte) []byte {
		documents++
		editTags(doc, func(token *html.Token) bool {
			if token.Data != "img" {
				return false
			}
			selected := imageSource(*token)
			raw, _ := base64.StdEncoding.DecodeString(strings.SplitN(selected, ",", 2)[1])
			cfg, _, err := image.DecodeConfig(bytes.NewReader(raw))
			if err != nil || cfg.Width != 300 || cfg.Height != 150 {
				t.Errorf("largest use not preserved: %+v %v", cfg, err)
			}
			for _, im := range layer.Images {
				if im.Hash != textlayer.FNV1a(selected) {
					t.Error("frame hash became inconsistent")
				}
			}
			return false
		})
		return doc
	})
	if err != nil || documents != 2 {
		t.Fatalf("documents=%d error=%v", documents, err)
	}
}

func TestDownsizeMeasurementsAndUnmeasuredUses(t *testing.T) {
	src := fixtureSource(t)
	layer := &textlayer.Layer{Viewport: textlayer.Viewport{DPR: 2}, Images: []textlayer.Image{{Hash: textlayer.FNV1a(src), Natural: [2]int{800, 400}, Box: textlayer.Box{0, 0, 400, 400}}}}
	// New captured measurements take precedence over the box/DPR fallback.
	// The object-fit:contain image only paints 100x50 device pixels here.
	doc := []byte(`<img src="` + src + `" data-parch-image-size="100,50,800"><div data-parch-background-size="100,50,0" style='mask-image:url("` + src + `");background-image:url("` + src + `") !important'></div>`)
	out, count, err := Downsize(doc, layer)
	if err != nil || count != 2 {
		t.Fatalf("changed=%d error=%v", count, err)
	}
	if !bytes.Contains(out, []byte(`mask-image:url(&#34;`+src+`&#34;)`)) {
		t.Fatal("changed unrelated mask source")
	}
	var selected string
	editTags(out, func(t *html.Token) bool {
		if t.Data == "img" {
			selected = imageSource(*t)
		}
		return false
	})
	raw, _ := base64.StdEncoding.DecodeString(strings.SplitN(selected, ",", 2)[1])
	cfg, _, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil || cfg.Width != 100 {
		t.Fatalf("ignored measured target: %+v %v", cfg, err)
	}
	// Explicitly unmeasurable/hidden uses prevent shrinking a shared bitmap.
	doc = append(doc, []byte(`<img hidden src="`+src+`" data-parch-image-size="0,0,800">`)...)
	out, count, err = Downsize(doc, layer)
	if err != nil || count != 0 || !bytes.Equal(doc, out) {
		t.Fatal("changed a source needed at an unknown size")
	}
}
