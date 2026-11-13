package capture

import (
	"context"
	"encoding/base64"
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

// The probe suite captures a local page built to exercise the SILENT losses:
// content an archive drops or a text layer mis-files without any error. It
// drives a real Chrome, so it is opt-in:
//
//	make probe        (or PARCH_BROWSER_TESTS=1 go test ./parchmint/capture/)
//
// Every assertion here corresponds to a bug that shipped unnoticed because
// nothing looked: the layer indexing invisible text as visible, `hidden`
// elements missing from the file, an inline <style>'s relative url() left
// pointing at nothing.

const probePixel = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNkYPhfDwAChwGA60e6kgAAAABJRU5ErkJggg=="

func probeServer(t *testing.T) *httptest.Server {
	t.Helper()
	page, err := os.ReadFile("testdata/probe.html")
	if err != nil {
		t.Fatal(err)
	}
	png, _ := base64.StdEncoding.DecodeString(probePixel)
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(page)
	})
	// Hotlink protection: the browser sends a Referer, the server-side
	// fetcher does not. The image reaches the archive only through Chrome's
	// snapshot, as a part an inline <style> refers to by a relative url().
	mux.HandleFunc("/bg.png", func(w http.ResponseWriter, r *http.Request) {
		if r.Referer() == "" {
			http.Error(w, "hotlinking refused", http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(png)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func needBrowser(t *testing.T) {
	t.Helper()
	if os.Getenv("PARCH_BROWSER_TESTS") == "" {
		t.Skip("browser-driven; run with `make probe` (PARCH_BROWSER_TESTS=1)")
	}
}

// whereIs returns "visible", "hidden:<why>", or "absent" for the block
// holding marker.
func whereIs(layer *textlayer.Layer, marker string) string {
	for _, b := range layer.Blocks {
		if strings.Contains(b.Text, marker) {
			return "visible"
		}
	}
	for _, b := range layer.Hidden {
		if strings.Contains(b.Text, marker) {
			return "hidden:" + b.Why
		}
	}
	return "absent"
}

func TestProbeCapture(t *testing.T) {
	needBrowser(t)
	srv := probeServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	for _, format := range []string{"html", "mht"} {
		t.Run(format, func(t *testing.T) {
			snap, err := CaptureWithOptions(ctx, srv.URL+"/", runner.DefaultConfig(), pipeline.Staticize(),
				Lookup(format), Options{TextLayer: true})
			if err != nil {
				t.Fatalf("capture: %v", err)
			}
			layer, err := textlayer.FromBytes(snap.Bytes, "probe."+format)
			if err != nil {
				t.Fatalf("read layer: %v", err)
			}

			want := map[string]string{
				"VISIBLEHEAD":    "visible",
				"VISIBLEPARA":    "visible",
				"VISBACK":        "visible", // visibility:visible inside a hidden parent shows
				"DETAILSSUMMARY": "visible",
				"DETAILSOPEN":    "visible",
				"SHOWNHIDDEN":    "visible", // `hidden` overridden by CSS
				"TABACTIVE":      "visible",
				"TEXTAREATEXT":   "visible",
				"SELECTOPT":      "visible",
				"INPUTVALUE":     "visible",
				"SVGTEXT":        "visible",
				"INLINENONE":     "hidden:display",
				"DISPLAYNONE":    "hidden:display",
				"HIDDENATTR":     "hidden:display",
				"TABINACTIVE":    "hidden:display",
				"VISHIDDEN":      "hidden:visibility",
				"OPACITYZERO":    "hidden:opacity",
				"DETAILSCLOSED":  "hidden:collapsed",
				"UNTILFOUND":     "hidden:collapsed",
				"CONTENTVIS":     "hidden:collapsed",
				"CLIPPATH":       "hidden:clipped",
				"SRONLY":         "hidden:clipped",
				"ACCORDION":      "hidden:clipped",
				"OFFSCREEN":      "hidden:offscreen",
				"SLIDEONE":       "visible",
				"SLIDETWO":       "hidden:clipped", // a carousel slide out of view
				"FONTZERO":       "hidden:zero-size",
				"PREPHIDDEN":     "hidden:prep",
				"SELECTOTHER":    "absent", // an unselected option is not shown
				"PASSWORDVALUE":  "absent", // never recorded
			}
			for marker, where := range want {
				if got := whereIs(layer, marker); got != where {
					t.Errorf("%s: layer has it %s, want %s", marker, got, where)
				}
			}

			// A hidden inline run must not split the visible paragraph
			// around it.
			for _, b := range layer.Blocks {
				if strings.Contains(b.Text, "VISIBLEPARA") && !strings.Contains(b.Text, "a hidden inline run") {
					t.Errorf("hidden inline content split its paragraph: %q", b.Text)
				}
			}

			// Ids are one sequence: visible 0..n-1, hidden after.
			for i, b := range layer.Blocks {
				if b.ID != i {
					t.Fatalf("visible block %d has id %d", i, b.ID)
				}
			}
			for i, b := range layer.Hidden {
				if b.ID != len(layer.Blocks)+i {
					t.Fatalf("hidden block %d has id %d, want %d", i, b.ID, len(layer.Blocks)+i)
				}
			}

			doc := snap.Bytes
			if format == "mht" {
				if doc, err = textlayer.MHTDocument(snap.Bytes); err != nil {
					t.Fatal(err)
				}
			}
			body := string(doc)
			// The serializer drops `hidden` elements unless prep hides the
			// attribute from it.
			for _, marker := range []string{"HIDDENATTR", "UNTILFOUND", "SHOWNHIDDEN"} {
				if !strings.Contains(body, marker) {
					t.Errorf("%s is missing from the archive document", marker)
				}
			}
			if format == "html" {
				if strings.Contains(body, "data-parchmint-hidden") {
					t.Error("html archive kept the data-parchmint-hidden stand-in")
				}
				if !strings.Contains(body, `hidden="until-found"`) {
					t.Error("html archive lost the original hidden attribute value")
				}
				// The relative url() of an inline <style> must be inlined,
				// not left pointing at the network.
				if strings.Contains(body, "bg.png") {
					t.Error("inline-style background was left as a dangling reference")
				}
			}
		})
	}
}

func TestProbeMarkHidden(t *testing.T) {
	needBrowser(t)
	srv := probeServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	snap, err := CaptureWithOptions(ctx, srv.URL+"/", runner.DefaultConfig(), pipeline.Staticize(),
		Lookup("html"), Options{TextLayer: true, Highlight: []string{"zebra"}})
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	// A capture never reveals: nothing visible says "zebra", so no marks.
	if strings.Contains(string(snap.Bytes), "data-parchmint-revealed") {
		t.Fatal("capture-time marking revealed hidden text")
	}

	// Unmarked source archive for the mark runs.
	snap, err = CaptureWithOptions(ctx, srv.URL+"/", runner.DefaultConfig(), pipeline.Staticize(),
		Lookup("html"), Options{TextLayer: true})
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	path := filepath.Join(t.TempDir(), "probe.html")
	if err := os.WriteFile(path, snap.Bytes, 0o600); err != nil {
		t.Fatal(err)
	}
	layer, err := textlayer.FromBytes(snap.Bytes, path)
	if err != nil {
		t.Fatal(err)
	}
	fileURL := (&url.URL{Scheme: "file", Path: filepath.ToSlash(path)}).String()

	// Without --hidden: nothing marked, the hidden matches counted.
	res, err := RenderArchive(ctx, runner.DefaultConfig(), fileURL, []string{"zebra"}, layer, RenderOptions{})
	if err != nil {
		t.Fatalf("mark: %v", err)
	}
	if res.DOMMatches != 0 || res.HiddenMatches != 0 || res.HiddenUnmarked < 10 {
		t.Fatalf("default mark: visible=%d hidden=%d unmarked=%d", res.DOMMatches, res.HiddenMatches, res.HiddenUnmarked)
	}

	// With it: revealed, labelled, marked — except what parch itself hid.
	res, err = RenderArchive(ctx, runner.DefaultConfig(), fileURL, []string{"zebra"}, layer,
		RenderOptions{Match: textlayer.MatchOptions{Hidden: []string{textlayer.HiddenAll}}})
	if err != nil {
		t.Fatalf("mark --hidden: %v", err)
	}
	if res.HiddenMatches < 10 || res.HiddenUnmarked != 0 {
		t.Fatalf("mark --hidden: hidden=%d unmarked=%d", res.HiddenMatches, res.HiddenUnmarked)
	}
	out := string(res.Bytes)
	if !strings.Contains(out, "data-parchmint-revealed") || !strings.Contains(out, "hidden on the original page") {
		t.Error("revealed content is not labelled")
	}
	if n := strings.Count(out, `<mark data-parchmint="mark"`); n != res.HiddenMatches {
		t.Errorf("%d <mark>s for %d hidden matches", n, res.HiddenMatches)
	}
	if !strings.Contains(out, "PREPHIDDEN zebra.") || strings.Contains(out, "PREPHIDDEN <mark") {
		t.Error("text hidden by prep must stay unmarked and unrevealed")
	}

	// A carousel is scrolled to its match, never un-clipped: lifting the
	// overflow of a sized box spills every slide across the page.
	if i := strings.Index(out, `id="carousel"`); i < 0 {
		t.Error("carousel missing from the marked copy")
	} else if tag := out[i : i+strings.Index(out[i:], ">")]; strings.Contains(tag, "overflow: visible") {
		t.Errorf("reveal un-clipped a sized container: %s", tag)
	}

	// A single reason narrows it.
	res, err = RenderArchive(ctx, runner.DefaultConfig(), fileURL, []string{"zebra"}, layer,
		RenderOptions{Match: textlayer.MatchOptions{Hidden: []string{textlayer.HiddenCollapsed}}})
	if err != nil {
		t.Fatalf("mark --hidden=collapsed: %v", err)
	}
	if res.HiddenMatches != 3 {
		t.Errorf("--hidden=collapsed marked %d, want 3 (details, until-found, content-visibility)", res.HiddenMatches)
	}
}
