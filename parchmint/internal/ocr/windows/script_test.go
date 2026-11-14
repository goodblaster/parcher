package windows

import (
	"math"
	"strings"
	"testing"
)

func near(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

// Output in the shape the script writes, including the quirks: a UTF-8 BOM,
// and a one-element array collapsed to a bare object.
const sampleOutput = "\xef\xbb\xbf" +
	`{"id":"a","width":200,"height":100,"lines":[{"words":[{"text":"SLOW","x":0,"y":10,"w":80,"h":20},{"text":"HORSES","x":90,"y":10,"w":100,"h":20}]},{"words":{"text":"tv","x":20,"y":50,"w":40,"h":20}}]}` + "\n" +
	`{"id":"b","error":"The component cannot be found."}` + "\n" +
	`{"id":"c","width":50,"height":50,"lines":[]}` + "\n"

func TestParseOutput(t *testing.T) {
	res, err := parseOutput([]byte(sampleOutput))
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 3 {
		t.Fatalf("%d results, want 3", len(res))
	}
	a := res[0]
	if a.ID != "a" || a.Err != nil || len(a.Lines) != 2 || a.Lines[0].LineText != "SLOW HORSES" || a.Lines[1].LineText != "tv" {
		t.Fatalf("image a: %+v", a)
	}
	// Pixels become fractions of the image.
	if w := a.Lines[0].Words[1]; !near(w.Left, 0.45) || !near(w.Top, 0.10) || !near(w.Width, 0.50) || !near(w.Height, 0.20) {
		t.Errorf("word box: %+v", w)
	}
	if res[1].Err == nil || !strings.Contains(res[1].Err.Error(), "cannot be found") {
		t.Errorf("per-image error lost: %+v", res[1])
	}
	if res[2].Err != nil || len(res[2].Lines) != 0 {
		t.Errorf("an image with no text is not an error: %+v", res[2])
	}
}

// No OCR language on the machine fails the batch with what to do about it.
func TestParseOutputFatal(t *testing.T) {
	_, err := parseOutput([]byte(`{"fatal":"Windows has no OCR language installed."}` + "\n"))
	if err == nil || !strings.Contains(err.Error(), "no OCR language") {
		t.Fatalf("want the fatal message as the error, got %v", err)
	}
}

// The script is assembled around Go's raw-string backticks, which are also
// PowerShell's escape character; a slip there breaks it only on Windows.
func TestScriptEscapes(t *testing.T) {
	for _, want := range []string{"\"`n\"", "'IAsyncOperation`1'", "\"`t\"", "$env:PARCH_OCR_LIST", "$env:PARCH_OCR_OUT", "$env:PARCH_OCR_LANG"} {
		if !strings.Contains(script, want) {
			t.Errorf("script is missing %s", want)
		}
	}
}
