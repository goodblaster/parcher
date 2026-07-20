package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/goodblaster/parchmint/capture"
	"github.com/goodblaster/parchmint/textlayer"
	"github.com/goodblaster/pscription/runner"
)

// runMarkCommand implements `parch mark <phrase> <archive.html>`:
// highlight-on-read. Produces a marked COPY of an existing archive — DOM
// matches wrapped in <mark>, OCR matches (parch index) baked into their
// images — closing the loop: capture → index → store anywhere → find →
// view with highlights, even when the text lives inside an image.
func runMarkCommand(args []string) {
	fs := flag.NewFlagSet("mark", flag.ExitOnError)
	output := fs.String("o", "", "output file (default <archive>.marked.html; '-' for stdout)")
	grayscale := fs.Bool("grayscale", false, "mute images containing hits so the highlight pops")
	color := fs.String("color", "rgba(255, 220, 0, 0.5)", "highlight fill for image hits")
	timeout := fs.Int("timeout", 60, "timeout in seconds")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s mark [-grayscale] [-o out.html] <phrase> [phrase ...] <archive.html>\n\n", os.Args[0])
		fmt.Fprintln(os.Stderr, "Every argument before the archive is a phrase; all are highlighted")
		fmt.Fprintln(os.Stderr, "(overlaps merge). Same matching as `parch find`. Run `parch index`")
		fmt.Fprintln(os.Stderr, "first if you want matches inside images.")
		fmt.Fprintln(os.Stderr, "\nOptions:")
		fs.PrintDefaults()
	}
	_ = fs.Parse(args)
	if fs.NArg() < 2 {
		fs.Usage()
		os.Exit(2)
	}
	phrases := fs.Args()[:fs.NArg()-1]
	path := fs.Arg(fs.NArg() - 1)

	die := func(err error) {
		fmt.Fprintln(os.Stderr, "parch: "+err.Error())
		os.Exit(2)
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		die(err)
	}
	layer, err := textlayer.FromFile(abs)
	if err != nil {
		die(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(*timeout)*time.Second)
	defer cancel()

	res, err := capture.MarkArchive(ctx, runner.DefaultConfig(), "file://"+abs, phrases, layer, capture.MarkOptions{
		Grayscale: *grayscale,
		Color:     *color,
		Stroke:    "rgba(200, 160, 0, 0.9)",
	})
	if err != nil {
		die(err)
	}

	total := res.DOMMatches + res.OCRMatches
	if total == 0 {
		fmt.Fprintln(os.Stderr, "0 matches; nothing written")
		os.Exit(1)
	}

	dest := *output
	switch dest {
	case "-":
		if _, err := os.Stdout.Write(res.HTML); err != nil {
			die(err)
		}
		dest = "(stdout)"
	case "":
		dest = strings.TrimSuffix(path, filepath.Ext(path)) + ".marked.html"
		fallthrough
	default:
		if err := os.WriteFile(dest, res.HTML, 0o644); err != nil {
			die(err)
		}
	}

	fmt.Fprintf(os.Stderr, "%s: %d text match(es), %d image match(es), %d image(s) marked\n",
		dest, res.DOMMatches, res.OCRMatches, res.ImagesMarked)
}
