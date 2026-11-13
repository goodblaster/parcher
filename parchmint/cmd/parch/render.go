package main

import (
	"context"
	"fmt"
	flag "github.com/spf13/pflag"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/goodblaster/parcher/parchmint/capture"
	"github.com/goodblaster/parcher/parchmint/textlayer"
	"github.com/goodblaster/parcher/pscription/runner"
)

// `parch mark` and `parch render` are the same operation with and without
// phrases: load an archive, optionally mark it, render it into the
// requested format. Two verbs rather than one flag because the semantics
// differ in a way that matters — mark REQUIRES phrases and reports "no
// matches" as exit 1, while render takes none and can never fail that way.
func runMarkCommand(args []string)   { runMarkOrRender(args, true) }
func runRenderCommand(args []string) { runMarkOrRender(args, false) }

// resolveFormat maps -f to a rendering backend plus the output extension.
// A nil backend means the document path: serialize the (marked) DOM as an
// HTML file rather than rendering it.
func resolveFormat(format string, marking bool) (capture.Backend, string, error) {
	prefix := ""
	if marking {
		prefix = ".marked"
	}
	switch format {
	case "":
		if !marking {
			return nil, "", fmt.Errorf("render needs -f (%s)", formatList())
		}
		return nil, prefix + ".html", nil // container fixed up later for MHT
	case "html":
		// Deliberately NOT capture.HTML: rendering an archive serializes the
		// loaded (and possibly marked) DOM directly. Going through a backend
		// here would re-snapshot a document that is already self-contained.
		return nil, prefix + ".html", nil
	}
	b := backendFor(format)
	if b == nil {
		return nil, "", fmt.Errorf("unknown format %q (want %s)", format, formatList())
	}
	return b, prefix + b.Ext(), nil
}

func runMarkOrRender(args []string, marking bool) {
	verb := "render"
	if marking {
		verb = "mark"
	}
	fs := flag.NewFlagSet(verb, flag.ExitOnError)

	output := fs.StringP("output", "o", "", "output file ('-' for stdout)")
	format := fs.StringP("format", "f", "", "output format: "+formatList())
	timeout := fs.Int("timeout", 120, "timeout in seconds")
	size := registerSizeFlags(fs, true)

	// Conversion between the two document containers can also be done by
	// rewriting the archive's bytes, with no browser at all.
	var noBrowser, fetchRemote *bool
	if !marking {
		noBrowser = fs.Bool("no-browser", false, "convert html<->mht by rewriting the archive instead of re-rendering it: no Chrome, no reflow (html and mht only)")
		fetchRemote = fs.Bool("fetch", false, "with --no-browser, download the http(s) resources the page still points at; without it they stay live references")
	}

	// Marking-only flags: presentation, matching, and hidden text.
	var grayscale *bool
	var look *markLook
	var match *matchFlags
	var hidden *string
	if marking {
		grayscale = fs.Bool("grayscale", false, "mute images containing hits so the highlight pops")
		look = registerMarkLook(fs, true)
		match = registerMatchFlags(fs)
		hidden = registerHidden(fs, "also mark matches in text the page hides (collapsed sections, inactive tabs, screen-reader text), REVEALING it in the copy with an outline and a label.")
	}

	fs.Usage = func() {
		if marking {
			fmt.Fprintf(os.Stderr, "Usage: %s mark [-f fmt] [--width N] [-o out] <phrase> [phrase ...] <archive.html|.mht>\n\n", os.Args[0])
			fmt.Fprintln(os.Stderr, "Marks an archive and renders it into any format. Every argument before")
			fmt.Fprintln(os.Stderr, "the archive is a phrase; all are highlighted (overlaps merge; with")
			fmt.Fprintln(os.Stderr, "--mark-color, the last phrase wins). Same matching as `parch find`,")
			fmt.Fprintln(os.Stderr, "including its mode flags (-e regex, …).")
			fmt.Fprintln(os.Stderr, "\nOnly text the page SHOWS is marked by default. A page also carries text")
			fmt.Fprintln(os.Stderr, "it hides — a collapsed FAQ answer, the tabs not selected — and the run")
			fmt.Fprintln(os.Stderr, "reports how many more matches sit there. --hidden marks those too and")
			fmt.Fprintln(os.Stderr, "reveals them in the copy (never in the archive), each outlined and")
			fmt.Fprintln(os.Stderr, "labelled \"hidden on the original page\". Revealing reflows the page.")
			fmt.Fprintln(os.Stderr, "\nText inside IMAGES is only marked if you ran `parch ocr` on the archive")
			fmt.Fprintln(os.Stderr, "first; page text is marked either way.")
			fmt.Fprintln(os.Stderr, "\nDefault output keeps the archive's own format. -f pdf/png/jpeg/webp")
			fmt.Fprintln(os.Stderr, "renders the marked page instead, so image and text marks both come out")
			fmt.Fprintln(os.Stderr, "in the result.")
			fmt.Fprintln(os.Stderr, "\nExample: parch mark --mark-color yellow --mark-color cyan revenue profit report.html")
			fmt.Fprintln(os.Stderr, "         parch mark -f png --dpr 0.4 phone test-site-1.html")
			fmt.Fprintln(os.Stderr, "         parch mark --hidden \"return policy\" faq.html")
			fmt.Fprintln(os.Stderr, "         parch mark --hidden=collapsed,display warranty product.html")
		} else {
			fmt.Fprintf(os.Stderr, "Usage: %s render -f fmt [--width N] [-o out] <archive.html|.mht>\n\n", os.Args[0])
			fmt.Fprintln(os.Stderr, "Converts an archive into another format, with no marking — the same")
			fmt.Fprintln(os.Stderr, "rendering `parch mark` does, minus the phrases. No `parch ocr` needed.")
			fmt.Fprintln(os.Stderr, "\nA PDF still gets everything that makes it searchable: native selectable")
			fmt.Fprintln(os.Stderr, "text, invisible overlay text for any image words `parch ocr` found, and")
			fmt.Fprintln(os.Stderr, "the text layer attached.")
			fmt.Fprintln(os.Stderr, "\n-f html and -f mht convert between the two archive containers; the")
			fmt.Fprintln(os.Stderr, "embedded text layer travels with the document either way. --no-browser")
			fmt.Fprintln(os.Stderr, "does that conversion by rewriting the bytes: no Chrome, and the page")
			fmt.Fprintln(os.Stderr, "keeps the exact layout it was captured at rather than being re-rendered.")
			fmt.Fprintln(os.Stderr, "\nIt packs what is on disk. A page saved by a browser may still point at")
			fmt.Fprintln(os.Stderr, "the network for resources it never wrote out; --fetch downloads those.")
			fmt.Fprintln(os.Stderr, "\nExample: parch render -f pdf --paper a4 report.html")
			fmt.Fprintln(os.Stderr, "         parch render -f png --dpr 0.5 test-site-1.html")
			fmt.Fprintln(os.Stderr, "         parch render -f mht --no-browser test-site-1.html")
			fmt.Fprintln(os.Stderr, "         parch render -f mht --no-browser --fetch saved-page.html")
		}
		fmt.Fprintln(os.Stderr, "\n'-' as the archive reads it from stdin (output then defaults to stdout).")
		fmt.Fprintln(os.Stderr, "\n--width RE-LAYS OUT the page at that width (a different responsive")
		fmt.Fprintln(os.Stderr, "breakpoint, which can change how much matches); to keep the captured")
		fmt.Fprintln(os.Stderr, "layout and just resize the output, use --dpr.")
		fmt.Fprintln(os.Stderr, "\nOptions:")
		fs.PrintDefaults()
	}

	_ = fs.Parse(args)

	// Two ways to stop: `die` for an invocation that is wrong (exit 2), and
	// `fail` for an operation that did not work (exit 1) — a missing archive
	// is the second, and used to be reported as the first.
	die := usageErr

	if marking {
		if err := validStyle(look.style); err != nil {
			die(err)
		}
	}
	if err := size.validate(); err != nil {
		die(err)
	}
	if err := validTimeout(*timeout); err != nil {
		die(err)
	}
	width, height, dpr, paper := &size.width, &size.height, &size.dpr, &size.paper
	backend, ext, err := resolveFormat(*format, marking)
	if err != nil {
		die(err)
	}
	paperWidthIn, paperHeightIn, err := parsePaper(*paper)
	if err != nil {
		die(err)
	}

	// mark needs at least one phrase plus the archive; render takes the
	// archive alone.
	minArgs := 1
	if marking {
		minArgs = 2
	}
	if fs.NArg() < minArgs {
		fs.Usage()
		os.Exit(2)
	}
	var phrases []string
	if marking {
		phrases = fs.Args()[:fs.NArg()-1]
	}
	path := fs.Arg(fs.NArg() - 1)

	matchOpts := textlayer.MatchOptions{}
	if marking {
		matchOpts = match.options()
		matchOpts.Hidden = hiddenReasons(*hidden)
		// Fail fast on a bad query (a typo'd -e regex, a contradictory flag
		// combo) BEFORE paying for a browser session.
		for _, p := range phrases {
			if _, err := textlayer.Compile(p, matchOpts); err != nil {
				die(err)
			}
		}
	}

	srcData, srcName, err := readArchive(path)
	if err != nil {
		fail(err)
	}
	// A missing layer is not a reason to refuse the work. Plenty of HTML is
	// not a parch archive — a page saved by the browser, or one captured
	// with --no-text — and it converts perfectly well. Index it during
	// the render instead, so what comes out IS a proper archive.
	layer, err := textlayer.FromBytes(srcData, srcName)
	reindex := err != nil
	if reindex {
		layer = &textlayer.Layer{}
	}

	// A freshly indexed layer still wants the page's real address. Browsers
	// record it in the file when they save a page; failing that, the layer
	// (if any) knows it.
	sourceURL := layer.URL
	if sourceURL == "" {
		sourceURL = savedFromURL(srcData)
	}

	if textlayer.IsPDF(srcData) {
		die(fmt.Errorf("%s: a .pdf cannot be re-rendered — its images are XObjects, not data URIs; %s the source .html/.mht instead", srcName, verb))
	}
	isMHT := textlayer.IsMHT(srcData)
	if isMHT && *format == "mht" {
		die(fmt.Errorf("-f mht: the source is already an .mht; drop -f to keep its own container, which preserves every part (and so every cid: reference) instead of re-snapshotting it"))
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(*timeout)*time.Second)
	defer cancel()

	var res *capture.RenderResult

	// The browser path already loads the page with network access, so --fetch
	// there describes something that always happens. Refuse it rather than
	// let it read as a no-op.
	if fetchRemote != nil && *fetchRemote && !*noBrowser {
		die(fmt.Errorf("--fetch applies to --no-browser; without it the page is loaded in a browser, which fetches what it references anyway"))
	}

	if noBrowser != nil && *noBrowser {
		// Re-rendering flags describe a browser laying the page out again;
		// this path never does, so honour them by refusing rather than
		// ignoring them.
		for _, given := range []string{"width", "height", "dpr", "paper"} {
			if fs.Changed(given) {
				die(fmt.Errorf("--%s re-renders the page, which --no-browser does not do; drop one of them", given))
			}
		}
		if reindex {
			// Indexing means reading the rendered page, which is exactly
			// what this path skips.
			fmt.Fprintf(os.Stderr, "parch: %s has no text layer, and --no-browser cannot index one; converting without it (drop --no-browser to index)\n", srcName)
		}
		res, err = convertOffline(ctx, offlineRequest{
			source:    srcData,
			path:      path,
			sourceURL: sourceURL,
			format:    *format,
			isMHT:     isMHT,
			network:   fetchRemote != nil && *fetchRemote,
			warn:      convertWarner(false),
		})
		if err != nil {
			fail(err)
		}
	} else {
		// The browser loads the archive over file://; a stdin archive is
		// spilled to a temp file first (removed after the session ends).
		var abs, tmp string
		if path == stdinName {
			if tmp, err = spillArchive(srcData); err != nil {
				fail(err)
			}
			abs = tmp
		} else if abs, err = filepath.Abs(path); err != nil {
			fail(err)
		}

		if reindex {
			fmt.Fprintf(os.Stderr, "parch: %s has no text layer; indexing it during the conversion\n", srcName)
		}

		opts := capture.RenderOptions{
			Match:          matchOpts,
			ViewportWidth:  *width,
			ViewportHeight: *height,
			DPR:            *dpr,
			Backend:        backend,
			PaperWidthIn:   paperWidthIn,
			PaperHeightIn:  paperHeightIn,
			ExtractLayer:   reindex,
			SourceURL:      sourceURL,
		}
		if marking {
			opts.Grayscale = *grayscale
			opts.Color = "rgba(255, 220, 0, 0.5)" // default image fill; -c overrides per phrase
			opts.Stroke = "rgba(200, 160, 0, 0.9)"
			opts.Colors = look.colors
			opts.Style = look.style
		}

		res, err = capture.RenderArchive(ctx, runner.DefaultConfig(), fileURL(abs), phrases, layer, opts)
		if tmp != "" {
			os.Remove(tmp) // session is over; the spilled stdin copy is done
		}
		if err != nil {
			fail(err)
		}
	}

	// What the page hides is never marked unasked — a highlight inside a
	// collapsed section is one nobody can see — but it is never passed over
	// in silence either.
	hiddenNote := ""
	if marking && res.HiddenUnmarked > 0 {
		hiddenNote = fmt.Sprintf("%d more match(es) in hidden text — rerun with --hidden to reveal and mark them", res.HiddenUnmarked)
	}
	if marking && res.DOMMatches+res.HiddenMatches+res.OCRMatches == 0 {
		fmt.Fprintln(os.Stderr, "0 matches; nothing written")
		if hiddenNote != "" {
			fmt.Fprintln(os.Stderr, hiddenNote)
		}
		os.Exit(1)
	}

	// A rendered format is finished bytes. Otherwise the output is a
	// document copy, and for an MHT source we put it back into the
	// container so every other part — and so every cid: reference — stays
	// intact.
	out := res.Bytes
	if backend == nil && isMHT && *format == "" {
		if out, err = textlayer.ReplaceMHTDocument(srcData, res.Bytes); err != nil {
			fail(err)
		}
		ext = ".marked.mht"
		if !marking {
			ext = ".mht"
		}
	}

	// Converting between containers moves the text layer with it: an .mht
	// keeps it in a MIME part, an .html in a <script> element. Carry it
	// across, or `parch text`/`find`/`mark` stop working on the result —
	// an .mht rendered to .html used to lose it silently.
	//
	// Outputs that already carry a layer are left alone: an HTML source's
	// script element survives serialization, and the MHT container path
	// above keeps every part. So are the formats with no layer of their
	// own — PDF gets its attachment inside RenderArchive, rasters have none.
	// An MHT built from a page on disk names its parts by file: URL, which
	// Chrome's MHTML loader refuses to resolve — the archive would open as
	// unstyled markup with every resource present but unbound. Re-key it onto
	// the address the page came from. Both conversion paths need this: the
	// browser one because Chrome names the parts, the offline one because it
	// resolves them from the filesystem.
	if _, toMHT := backend.(capture.MHT); toMHT {
		fileBase, declaredBase := archiveBases(path, sourceURL)
		out = rekeyFileBase(out, fileBase, declaredBase)
	}

	// The layer to write is the one just indexed, when the source had none,
	// otherwise the source's own. A reindex that failed leaves nothing to
	// write — better an archive with no layer than one with an empty layer.
	layerJSON := res.TextLayer
	if layerJSON == nil && !reindex {
		if marshaled, err := layer.Marshal(); err != nil {
			fmt.Fprintln(os.Stderr, "parch: could not carry the text layer across: "+err.Error())
		} else {
			layerJSON = marshaled
		}
	}

	if layerJSON != nil && !textlayer.HasLayer(out) {
		switch backend.(type) {
		case nil: // the HTML document path
			out = textlayer.EmbedInHTML(out, layerJSON)
		case capture.MHT:
			embedded, err := textlayer.EmbedInMHT(out, layerJSON)
			if err != nil {
				fmt.Fprintln(os.Stderr, "parch: could not carry the text layer into the .mht: "+err.Error())
			} else {
				out = embedded
			}
		}
	}

	dest := *output
	if dest == "" && path == stdinName {
		dest = "-" // stdin in, stdout out: no filename to derive from
	}
	switch dest {
	case "-":
		if _, err := os.Stdout.Write(out); err != nil {
			fail(err)
		}
		dest = "(stdout)"
	case "":
		// `render -f html page.html` derives page.html — the source itself,
		// which was silently replaced. Step aside the way mark's .marked does.
		stem := strings.TrimSuffix(path, filepath.Ext(path))
		if dest = stem + ext; dest == path {
			dest = stem + ".rendered" + ext
		}
		fallthrough
	default:
		if err := os.WriteFile(dest, out, 0o644); err != nil {
			fail(err)
		}
	}

	report := fmt.Sprintf("%s: %d bytes", dest, len(out))
	if marking {
		report = fmt.Sprintf("%s: %d text match(es), %d image match(es), %d image(s) marked",
			dest, res.DOMMatches, res.OCRMatches, res.ImagesMarked)
		if res.HiddenMatches > 0 {
			report += fmt.Sprintf("; %d match(es) in hidden text revealed and marked", res.HiddenMatches)
		}
	}
	if res.OverlayWords > 0 {
		report += fmt.Sprintf("; %d image(s) overlaid (%d words)", res.ImagesOverlaid, res.OverlayWords)
	}
	fmt.Fprintln(os.Stderr, report)
	if hiddenNote != "" {
		fmt.Fprintln(os.Stderr, hiddenNote)
	}
}
