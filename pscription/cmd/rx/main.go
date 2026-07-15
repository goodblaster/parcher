// rx executes a pscription — an ordered list of preparation steps and
// scripts — against a URL in a headless browser.
// Interim behavior: capture-harness CLI; the .rx pscription file format and
// JSON report output are the next feature: it loads a URL, runs a
// preparation recipe, and captures the page with a chosen backend.
//
//	rx https://example.com                    → example.com.html (SingleFile)
//	rx -backend mht https://example.com       → example.com.mht
//	rx -o page.html https://example.com
//	rx https://example.com > page.html        (content to stdout when piped)
//
// Logs always go to stderr; captured content goes to the output file, or to
// stdout when -o - is given or stdout is not a terminal.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/goodblaster/logos"
	"github.com/goodblaster/pscription/capture"
	"github.com/goodblaster/pscription/internal/common"
	"github.com/goodblaster/pscription/internal/log"
	"github.com/goodblaster/pscription/pipeline"
)

func main() {
	var (
		output  string
		backend string
		links   string
		timeout int
		verbose bool
	)

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options] <url>\n\nOptions:\n", os.Args[0])
		flag.PrintDefaults()
	}
	flag.StringVar(&output, "o", "", "output file; '-' for stdout (default: derived from URL)")
	flag.StringVar(&backend, "backend", "singlefile", "capture backend: singlefile, mht, screenshot")
	flag.StringVar(&links, "links", "keep", "link policy: keep (unchanged), new-tab (external links open in a new tab), disable (links kept but unclickable)")
	flag.IntVar(&timeout, "timeout", 300, "timeout in seconds")
	flag.BoolVar(&verbose, "v", false, "debug logging")
	flag.Parse()

	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}
	url := flag.Arg(0)
	if !strings.Contains(url, "://") {
		url = "https://" + url
	}

	// Logs go to stderr so captured content can be piped from stdout.
	level := logos.LevelInfo
	if verbose {
		level = logos.LevelDebug
	}
	log.SetDefault(log.LogosAdapter(logos.NewLogger(level, logos.ConsoleFormatter(), os.Stderr)))

	if err := common.Init(); err != nil {
		log.WithError(err).Fatal("initialization failed")
	}

	var b capture.Backend
	switch backend {
	case "singlefile":
		b = capture.SingleFile{}
	case "mht":
		b = capture.MHT{}
	case "screenshot":
		b = capture.Screenshot{}
	default:
		fmt.Fprintf(os.Stderr, "unknown backend %q (want singlefile, mht, or screenshot)\n", backend)
		os.Exit(2)
	}

	toStdout := output == "-" || (output == "" && !stdoutIsTerminal())
	if output == "" && !toStdout {
		output = deriveFilename(url) + b.Ext()
	}

	recipe := pipeline.Staticize()
	switch links {
	case "keep":
		// default: links exactly as the page had them
	case "new-tab", "disable":
		recipe = recipe.Append(pipeline.RewriteLinksStep(links))
	default:
		fmt.Fprintf(os.Stderr, "unknown link policy %q (want keep, new-tab, or disable)\n", links)
		os.Exit(2)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()

	log.With("url", url).With("backend", b.Name()).With("links", links).Info("capturing")
	start := time.Now()

	snap, err := capture.Capture(ctx, url, recipe, b)
	if err != nil {
		log.WithError(err).Fatal("capture failed")
	}

	if toStdout {
		if _, err := os.Stdout.Write(snap.Bytes); err != nil {
			log.WithError(err).Fatal("failed to write to stdout")
		}
	} else if err := os.WriteFile(output, snap.Bytes, 0644); err != nil {
		log.WithError(err).Fatal("failed to write output")
	}

	log.With("output", displayName(toStdout, output)).
		With("title", snap.Title).
		With("bytes", len(snap.Bytes)).
		With("duration", time.Since(start).Round(time.Second).String()).
		Info("capture complete")
}

func stdoutIsTerminal() bool {
	info, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func displayName(toStdout bool, output string) string {
	if toStdout {
		return "(stdout)"
	}
	return output
}

// deriveFilename turns a URL into a safe base filename, e.g.
// "https://test-site-8/wiki/首页" → "test-site-8_wiki_首页".
func deriveFilename(url string) string {
	name := url
	if i := strings.Index(name, "://"); i != -1 {
		name = name[i+3:]
	}
	name = strings.TrimPrefix(name, "www.")
	name = strings.Map(func(r rune) rune {
		switch r {
		case '/', '?', '&', '=', ':', '%', '#':
			return '_'
		}
		return r
	}, name)
	return strings.Trim(name, "_")
}
