package capture

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/goodblaster/parcher/parchmint/textlayer"
	"github.com/goodblaster/parcher/pscription/pipeline"
	"github.com/goodblaster/parcher/pscription/runner"
)

func archiveFixture(t *testing.T, source []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "input.html")
	if err := os.WriteFile(path, source, 0600); err != nil {
		t.Fatal(err)
	}
	return (&url.URL{Scheme: "file", Path: filepath.ToSlash(path)}).String()
}

func TestProbeRenderShadowRoots(t *testing.T) {
	needBrowser(t)
	source := []byte(`<!DOCTYPE html><html><body><p>Light marker</p>
<div><template shadowrootmode="open"><p>Open marker</p>
<div><template shadowrootmode="closed"><p>Nested marker</p></template></div>
</template></div><div><template shadowrootmode="closed"><p>Closed marker</p></template></div>
</body></html>`)
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	for _, marking := range []bool{false, true} {
		var phrases []string
		if marking {
			phrases = []string{"Open"}
		}
		res, err := RenderArchive(ctx, runner.DefaultConfig(), archiveFixture(t, source), phrases, &textlayer.Layer{}, RenderOptions{})
		if err != nil {
			t.Fatal(err)
		}
		for _, marker := range []string{"Light marker", "Nested marker", "Closed marker", `shadowrootmode="open"`, `shadowrootmode="closed"`} {
			if !strings.Contains(string(res.Bytes), marker) {
				t.Errorf("marking=%v: lost %q", marking, marker)
			}
		}
		if marking && (res.DOMMatches != 1 || !strings.Contains(string(res.Bytes), `<mark data-parchmint="mark"`)) {
			t.Error("shadow match was not preserved")
		}
		// Reload to prove the serialized templates attach and render again.
		reopened, err := RenderArchive(ctx, runner.DefaultConfig(), archiveFixture(t, res.Bytes), nil, &textlayer.Layer{}, RenderOptions{ExtractLayer: true})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(reopened.TextLayer), "Open marker") {
			t.Error("shadow text missing after reopening")
		}
	}
}

func TestProbeRenderPDFLayer(t *testing.T) {
	needBrowser(t)
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	fileURL := archiveFixture(t, []byte(`<!DOCTYPE html><html><body><p>Searchable marker</p></body></html>`))
	for _, marking := range []bool{false, true} {
		var phrases []string
		if marking {
			phrases = []string{"Searchable"}
		}
		res, err := RenderArchive(ctx, runner.DefaultConfig(), fileURL, phrases, &textlayer.Layer{}, RenderOptions{Backend: PDF{}, ExtractLayer: true, SourceURL: "https://example.com/"})
		if err != nil {
			t.Fatal(err)
		}
		layer, err := textlayer.FromBytes(res.Bytes, "output.pdf")
		if err != nil {
			t.Fatal(err)
		}
		if layer.URL != "https://example.com/" || len(layer.Blocks) != 1 || layer.Blocks[0].Text != "Searchable marker" {
			t.Fatalf("wrong PDF layer: %+v", layer)
		}
		// Existing indexed archives retain their original layer on export.
		res, err = RenderArchive(ctx, runner.DefaultConfig(), fileURL, nil, layer, RenderOptions{Backend: PDF{}})
		if err != nil {
			t.Fatal(err)
		}
		got, err := textlayer.FromBytes(res.Bytes, "indexed.pdf")
		if err != nil {
			t.Fatal(err)
		}
		wantJSON, _ := layer.Marshal()
		gotJSON, _ := got.Marshal()
		if string(wantJSON) != string(gotJSON) {
			t.Error("existing PDF layer changed")
		}
	}
}

func TestProbeStrictHTML(t *testing.T) {
	needBrowser(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<!DOCTYPE html><p>Partial archive</p><img src="/missing.png">`))
	}))
	defer srv.Close()
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	for _, strict := range []bool{false, true} {
		_, err := CaptureWithOptions(ctx, srv.URL, runner.DefaultConfig(), pipeline.Recipe{}, HTML{Strict: strict}, Options{})
		if strict {
			if err == nil || !strings.Contains(err.Error(), "strict capture refused") {
				t.Fatalf("strict capture: %v", err)
			}
		} else if err != nil {
			t.Fatal(err)
		}
	}
	// A complete document still succeeds in strict mode.
	_, err := CaptureWithOptions(ctx, archiveFixture(t, []byte(`<!DOCTYPE html><p>Complete archive</p>`)), runner.DefaultConfig(), pipeline.Recipe{}, HTML{Strict: true}, Options{})
	if err != nil {
		t.Fatal(err)
	}
}
