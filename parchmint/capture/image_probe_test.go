package capture

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/goodblaster/parcher/parchmint/textlayer"
	"github.com/goodblaster/parcher/pscription/act"
	"github.com/goodblaster/parcher/pscription/pipeline"
	"github.com/goodblaster/parcher/pscription/runner"
)

func imageFixtureURI(t *testing.T) string {
	t.Helper()
	im := image.NewRGBA(image.Rect(0, 0, 1200, 800))
	for y := 0; y < 800; y++ {
		for x := 0; x < 1200; x++ {
			im.SetRGBA(x, y, color.RGBA{uint8(x*31 + y*11), uint8(x*7 + y*23), uint8(x + y), 255})
		}
	}
	var b bytes.Buffer
	if err := jpeg.Encode(&b, im, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatal(err)
	}
	return "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(b.Bytes())
}

// Verify saved bytes AND reflow after reopening: Chrome strips srcset, and
// reducing intrinsic dimensions must not change a responsive page's layout.
func TestProbeImageDensity(t *testing.T) {
	needBrowser(t)
	src := imageFixtureURI(t)
	page := []byte(fmt.Sprintf(`<!doctype html><html><head><style>img:not([src]){display:none}</style></head><body>
<img id="fixed" src="%[1]s" style="width:120px;height:80px">
<img id="responsive" src="%[1]s" style="width:25vw;height:auto">
<img id="large" src="%[1]s" style="width:300px;height:200px">
<img id="contain" src="%[1]s" style="width:160px;height:160px;object-fit:contain">
<div style='width:220px;height:100px;background-image:url("%[1]s");background-size:cover'></div>
<div><template shadowrootmode="open"><img src="%[1]s" style="width:100px;height:auto"></template></div>
</body></html>`, src))
	for _, format := range []string{"html", "mht"} {
		for _, preserve := range []bool{false, true} {
			for _, dpr := range []float64{1, 2} {
				t.Run(fmt.Sprintf("%s/preserve=%v/dpr=%g", format, preserve, dpr), func(t *testing.T) {
					ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
					defer cancel()
					cfg := runner.DefaultConfig()
					cfg.ViewportWidth = 800
					cfg.DeviceScaleFactor = dpr
					snap, err := CaptureWithOptions(ctx, archiveFixture(t, page), cfg, pipeline.Recipe{}, Lookup(format), Options{TextLayer: true, PreserveImageSizes: preserve})
					if err != nil {
						t.Fatal(err)
					}
					layer, err := textlayer.FromBytes(snap.Bytes, "page."+format)
					if err != nil {
						t.Fatal(err)
					}
					if len(layer.Images) != 5 {
						t.Fatalf("images=%d", len(layer.Images))
					}
					doc := snap.Bytes
					if format == "mht" {
						doc, err = textlayer.MHTDocument(doc)
						if err != nil {
							t.Fatal(err)
						}
					}
					wantW, wantH := int(300*dpr), int(200*dpr)
					if preserve {
						wantW, wantH = 1200, 800
					}
					uris := regexp.MustCompile(`data:image/jpeg;base64,[A-Za-z0-9+/=]+`).FindAllString(string(doc), -1)
					if len(uris) < 5 {
						t.Fatalf("lost image data: %d uses", len(uris))
					}
					for _, uri := range uris {
						b, _ := base64.StdEncoding.DecodeString(strings.SplitN(uri, ",", 2)[1])
						c, _, err := image.DecodeConfig(bytes.NewReader(b))
						if err != nil || c.Width != wantW || c.Height != wantH {
							t.Fatalf("bitmap=%+v error=%v want=%dx%d", c, err, wantW, wantH)
						}
					}
					for _, im := range layer.Images {
						if im.Natural != [2]int{1200, 800} {
							t.Errorf("intrinsic dimensions changed: %+v", im)
						}
						matched := false
						for _, uri := range uris {
							if textlayer.FNV1a(uri) == im.Hash {
								matched = true
							}
						}
						if !matched {
							t.Error("layer hash does not match archived bytes")
						}
					}
					path := filepath.Join(t.TempDir(), "reopen."+format)
					if err := os.WriteFile(path, snap.Bytes, 0600); err != nil {
						t.Fatal(err)
					}
					cfg.ViewportWidth = 400
					session, err := runner.Start(ctx, (&url.URL{Scheme: "file", Path: path}).String(), cfg)
					if err != nil {
						t.Fatal(err)
					}
					defer session.Close()
					var measurements []struct {
						ID           string
						W, H, NW, NH float64
					}
					err = session.Run(act.Evaluate(`Array.from(document.images,im=>{const r=im.getBoundingClientRect();return {ID:im.id,W:r.width,H:r.height,NW:im.naturalWidth,NH:im.naturalHeight}})`, &measurements))
					if err != nil {
						t.Fatal(err)
					}
					if len(measurements) != 4 {
						t.Fatalf("reopened %d images", len(measurements))
					}
					for _, m := range measurements {
						if m.NW != 1200 || m.NH != 800 {
							t.Errorf("reopened intrinsic dimensions: %+v", m)
						}
						if m.ID == "responsive" && (m.W != 100 || m.H < 66 || m.H > 67) {
							t.Errorf("responsive layout distorted: %+v", m)
						}
						if m.ID == "fixed" && (m.W != 120 || m.H != 80) {
							t.Errorf("fixed layout changed: %+v", m)
						}
					}
					t.Logf("source bitmap %d bytes; archive %d bytes", len(src), len(snap.Bytes))
					if format == "html" && !preserve && dpr == 1 {
						// Re-snapshot an archive whose only nonempty source is in
						// srcset. Its density and source must survive conversion.
						converted, err := RenderArchive(ctx, cfg, (&url.URL{Scheme: "file", Path: path}).String(), nil, layer, RenderOptions{Backend: MHT{}})
						if err != nil {
							t.Fatal(err)
						}
						convertedDoc, err := textlayer.MHTDocument(converted.Bytes)
						if err != nil {
							t.Fatal(err)
						}
						if !bytes.Contains(convertedDoc, []byte(`srcset="`)) || !bytes.Contains(convertedDoc, []byte(uris[0])) {
							t.Fatal("re-snapshot lost resized source")
						}
						layer.Blocks = []textlayer.Block{{ID: 0, Type: "img", Source: "ocr", Image: layer.Images[0].Hash, Text: "Readable", Box: layer.Images[0].Box, Words: []textlayer.Word{{Start: 0, End: 8, Box: layer.Images[0].Box}}}}
						marked, err := RenderArchive(ctx, cfg, (&url.URL{Scheme: "file", Path: path}).String(), []string{"Readable"}, layer, RenderOptions{})
						if err != nil {
							t.Fatal(err)
						}
						if marked.ImagesMarked != 5 {
							t.Fatalf("resized images could not be marked: %d", marked.ImagesMarked)
						}
						if bytes.Contains(marked.Bytes, []byte(`data-parch-image-density=`)) {
							t.Fatal("marked images retained stale density")
						}
					}
				})
			}
		}
	}
}
