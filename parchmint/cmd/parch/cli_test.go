package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/goodblaster/parcher/parchmint/internal/config"
	"github.com/goodblaster/parcher/parchmint/textlayer"
)

func TestResolveDest(t *testing.T) {
	now := time.Date(2026, 7, 18, 0, 0, 0, 0, time.UTC)
	const url = "https://www.example.com/a/b?q=1"
	cases := []struct {
		name      string
		explicitO bool
		output    string
		cfg       config.Config
		wantDest  string
		wantOut   bool
	}{
		{"dash is stdout", true, "-", config.Config{}, "", true},
		{"explicit path verbatim", true, "out/x.html", config.Config{OutputDir: "/ignored"}, "out/x.html", false},
		{"output_dir with derived name", false, "", config.Config{OutputDir: "/arch"}, "/arch/example.com_a_b_q_1.html", false},
		{"filename template", false, "", config.Config{Filename: "{host}-{date}.{ext}"}, "example.com-2026-07-18.html", false},
		{"both", false, "", config.Config{OutputDir: "/arch", Filename: "{title}.{ext}"}, "/arch/My-Page.html", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dest, toStdout := resolveDest(c.explicitO, c.output, c.cfg, url, "My Page", ".html", now)
			if dest != c.wantDest || toStdout != c.wantOut {
				t.Fatalf("got (%q, %v), want (%q, %v)", dest, toStdout, c.wantDest, c.wantOut)
			}
		})
	}
}

func TestDeriveFilename(t *testing.T) {
	cases := map[string]string{
		"https://www.example.com/":            "example.com",
		"https://example.com/a/b?x=1&y=2#top": "example.com_a_b_x_1_y_2_top",
		`https://example.com/a\b*c|d"e<f>g`:   "example.com_a_b_c_d_e_f_g",
		"https://":                            "page",
	}
	for in, want := range cases {
		if got := deriveFilename(in); got != want {
			t.Errorf("deriveFilename(%q) = %q, want %q", in, got, want)
		}
	}
	// A long query string must not push the name past what a filesystem takes.
	long := deriveFilename("https://example.com/?" + strings.Repeat("k=v&", 200))
	if len(long) > 200 {
		t.Errorf("derived name is %d bytes", len(long))
	}
}

func TestTargetURL(t *testing.T) {
	file := filepath.Join(t.TempDir(), "saved page#1.html")
	if err := os.WriteFile(file, []byte("<p>x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := targetURL("example.com/x"); got != "https://example.com/x" {
		t.Errorf("bare host: %q", got)
	}
	if got := targetURL("http://example.com"); got != "http://example.com" {
		t.Errorf("explicit scheme changed: %q", got)
	}
	got := targetURL(file)
	if !strings.HasPrefix(got, "file:///") || !strings.Contains(got, "saved%20page%231.html") {
		t.Errorf("existing file should become an escaped file: URL, got %q", got)
	}
}

func TestSelectBlocks(t *testing.T) {
	layer := &textlayer.Layer{
		Blocks: []textlayer.Block{{ID: 0, Text: "shown"}},
		Hidden: []textlayer.Block{
			{ID: 1, Text: "faq", Why: textlayer.HiddenCollapsed},
			{ID: 2, Text: "tab", Why: textlayer.HiddenDisplay},
			{ID: 3, Text: "popup chrome", Why: textlayer.HiddenPrep},
		},
	}
	ids := func(reasons string) string {
		blocks, err := selectBlocks(layer, hiddenReasons(reasons))
		if err != nil {
			t.Fatalf("selectBlocks(%q): %v", reasons, err)
		}
		var b strings.Builder
		for _, blk := range blocks {
			b.WriteByte(byte('0' + blk.ID))
		}
		return b.String()
	}
	for reasons, want := range map[string]string{
		"":              "0",
		"all":           "012", // never prep
		"collapsed":     "01",
		"display, prep": "023",
		"all,prep":      "0123",
	} {
		if got := ids(reasons); got != want {
			t.Errorf("--hidden=%q selected %s, want %s", reasons, got, want)
		}
	}
	if _, err := selectBlocks(layer, []string{"nonsense"}); err == nil {
		t.Error("unknown reason: want an error")
	}
	if len(layer.Blocks) != 1 {
		t.Error("selectBlocks must not grow the layer's own slice")
	}
}
