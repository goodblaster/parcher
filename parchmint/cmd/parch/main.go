// parch — page archiver. Captures a URL as a single self-contained file
// that renders faithfully with no network: dialogs dismissed, lazy content
// loaded, videos frozen to stills, every resource embedded.
//
//	parch https://example.com                  → example.com.html
//	parch -o page.html https://example.com
//	parch -f mht https://example.com           → Chrome MHTML instead of HTML
//	parch -f png https://example.com           → full-page screenshot
//	parch --links new-tab https://example.com  → external links open in a new tab
//	parch --width 1280 https://example.com     → render at a 1280px-wide layout
//	parch -M "phrase" https://example.com      → mark matches at capture time
//	parch --cache ~/.parch/cache https://…     → reuse an HTTP cache across runs
//	parch https://example.com > page.html      → content on stdout when piped
//	parch ./saved-page.html                    → a file on disk is captured too
//
// Defaults, a shared cache directory, an output directory, and an output
// filename template can also be set in a TOML config file — see the internal
// config package. Precedence, high to low: CLI flags, ./.parch/config (nearest
// ancestor), ~/.parch/config, built-in defaults.
//
// Logs always go to stderr; captured content goes to the output file, or to
// stdout when -o - is given (or stdout is piped and no output is configured).
package main

import (
	"context"
	"fmt"
	flag "github.com/spf13/pflag"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/goodblaster/logos"
	"github.com/goodblaster/parcher/parchmint/capture"
	"github.com/goodblaster/parcher/parchmint/internal/config"
	"github.com/goodblaster/parcher/parchmint/textlayer"
	"github.com/goodblaster/parcher/pscription/actions"
	"github.com/goodblaster/parcher/pscription/pipeline"
	"github.com/goodblaster/parcher/pscription/pscription"
	"github.com/goodblaster/parcher/pscription/runner"
)

// Built-in defaults, used when neither a flag nor the config file sets a value.
const (
	defFormat  = "html"
	defLinks   = "keep"
	defTimeout = 300
)

// version identifies the build; stamp releases with
// -ldflags "-X main.version=v1.2.3".
var version = "dev"

// subcommands maps each archive-ops command to its entry point — the
// dispatch table for both main and `parch help <cmd>`.
var subcommands = map[string]func([]string){
	"text":   runTextCommand,
	"lines":  runLinesCommand,
	"find":   runFindCommand,
	"ocr":    runOCRCommand,
	"mark":   runMarkCommand,
	"render": runRenderCommand,
}

func main() {
	// Before anything can log: logos's own default writes DEBUG to stdout,
	// so a subcommand left on it prepended log lines to `render -o -`
	// output. The capture flow re-sets this once -v is parsed.
	logos.SetDefaultLogger(logos.NewLogger(logos.LevelInfo, logos.ConsoleFormatter(), os.Stderr))

	// Subcommands (the capture flow stays the bare `parch <url>` form).
	if len(os.Args) > 1 {
		if run, ok := subcommands[os.Args[1]]; ok {
			run(os.Args[2:])
			return
		}
		switch os.Args[1] {
		case "version", "--version":
			fmt.Println("parch " + version)
			return
		case "help":
			runHelpCommand(os.Args[2:])
			return
		}
	}

	var (
		output       string
		format       string
		links        string
		rxFile       string
		profile      string
		cache        string
		userAgent    string
		headful      bool
		timeout      int
		noText       bool
		noFavicon    bool
		text         bool
		favicon      bool
		verbose      bool
		strict       bool
		gateMode     string
		autoClear    bool
		clearFile    string
		reportPath   string
		approveGates stringsFlag
		highlight    stringsFlag
	)

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options] <url|file>          capture a page as one file\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "       %s --clear <file.json> [options]   clear a recorded gate\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "       %s <command> [options] <args>   work with captured archives\n", os.Args[0])
		fmt.Fprintln(os.Stderr, "\nCommands:")
		fmt.Fprintln(os.Stderr, "  text     plain text back out of an archive (--json: the raw layer; --blocks: NDJSON)")
		fmt.Fprintln(os.Stderr, "  lines    one plain-text line per paragraph block")
		fmt.Fprintln(os.Stderr, "  find     grep across archives' text layers (multi-file, -l/-c/-q/-r, regex/fuzzy)")
		fmt.Fprintln(os.Stderr, "  ocr      OCR an archive's images into its text layer (needed to mark image text)")
		fmt.Fprintln(os.Stderr, "  mark     mark an archive and render it into any format (--hidden reveals hidden text)")
		fmt.Fprintln(os.Stderr, "  render   convert an archive into another format, without marking")
		fmt.Fprintln(os.Stderr, "  version  print the parch version")
		fmt.Fprintln(os.Stderr, "  help     usage for a command (`parch help find`)")
		fmt.Fprintln(os.Stderr, "\nCapture options:")
		flag.PrintDefaults()
	}
	flag.StringVarP(&output, "output", "o", "", "output file; '-' for stdout (default: config filename, else derived from URL)")
	flag.StringVarP(&format, "format", "f", defFormat, "output format: "+formatList()+" (html is a self-contained page)")
	flag.StringVar(&links, "links", defLinks, "link policy: keep (unchanged), new-tab (external links open in a new tab), disable (links kept but unclickable)")
	flag.StringVar(&rxFile, "recipe", "", "run a .rx pscription as the prep before serializing (e.g. to log in first) — same flag as pscribe's")
	flag.StringVar(&rxFile, "pscribe", "", "deprecated spelling of --recipe")
	_ = flag.CommandLine.MarkDeprecated("pscribe", "use --recipe")
	flag.StringVar(&profile, "profile", "", "persistent Chrome user-data dir; log in once and reuse the session")
	flag.StringVar(&cache, "cache", "", "shared Chrome HTTP cache dir; reused across runs to speed up repeat loads")
	flag.StringVar(&userAgent, "user-agent", "", "user agent to announce (default: the browser's own Chrome one)")
	flag.BoolVar(&headful, "show", false, "launch a visible browser (e.g. to log in at a wait:user step)")
	flag.IntVar(&timeout, "timeout", defTimeout, "timeout in seconds")
	size := registerSizeFlags(flag.CommandLine, false)
	flag.BoolVar(&noText, "no-text", false, "do not embed the text layer in HTML/MHT archives (it is what 'parch text/find/mark' read)")
	flag.BoolVar(&noFavicon, "no-favicon", false, "do not embed the page favicon in HTML/MHT archives")
	flag.BoolVar(&text, "text", true, "deprecated: use --no-text")
	flag.BoolVar(&favicon, "favicon", true, "deprecated: use --no-favicon")
	_ = flag.CommandLine.MarkDeprecated("text", "use --no-text")
	_ = flag.CommandLine.MarkDeprecated("favicon", "use --no-favicon")
	flag.VarP(&highlight, "mark", "M", "mark matches of this phrase at capture time, in every format (repeatable; same matching as 'parch find')")
	flag.Var(&highlight, "highlight", "deprecated spelling of --mark")
	_ = flag.CommandLine.MarkDeprecated("highlight", "use --mark")
	look := registerMarkLook(flag.CommandLine, false)
	match := registerMatchFlags(flag.CommandLine)
	flag.BoolVarP(&verbose, "verbose", "v", false, "debug logging")
	flag.BoolVar(&strict, "strict", false, "fail HTML capture if snapshot resource references cannot be embedded")
	flag.StringVar(&gateMode, "gates", "detect", "access gates: detect (save unresolved view and exit 3) or preserve (save it and exit 0)")
	flag.Var(&approveGates, "approve-gate", "approve the exact gate fingerprint from a report (repeatable; asserts all displayed declarations)")
	flag.BoolVar(&autoClear, "auto-clear", false, "automatically approve supported access gates (asserts their declarations); save cleared captures with the regular filename")
	flag.StringVar(&clearFile, "clear", "", "clear gates recorded in this capture metadata JSON file; URL, format and regular output name come from the file")
	flag.StringVar(&reportPath, "report", "", "write a versioned JSON capture report to this file (separate from the archive)")
	flag.Parse()

	if (clearFile == "" && flag.NArg() != 1) || (clearFile != "" && flag.NArg() > 1) {
		flag.Usage()
		os.Exit(2)
	}
	var savedGate *captureMetadata
	var url string
	if clearFile != "" {
		if autoClear || len(approveGates) > 0 || gateMode == "preserve" {
			usageErr(fmt.Errorf("--clear cannot be combined with --auto-clear, --approve-gate, or --gates=preserve"))
		}
		var err error
		savedGate, err = readClearMetadata(clearFile)
		if err != nil {
			usageErr(err)
		}
		url = savedGate.URL
		if flag.NArg() == 1 && targetURL(flag.Arg(0)) != url {
			usageErr(fmt.Errorf("URL does not match --clear metadata file"))
		}
		for _, gate := range savedGate.Access.Pending {
			approveGates = append(approveGates, gate.ID)
		}
	} else {
		url = targetURL(flag.Arg(0))
	}
	if gateMode != "detect" && gateMode != "preserve" {
		usageErr(fmt.Errorf("invalid --gates %q (want detect or preserve)", gateMode))
	}
	gatePolicy := actions.GatePolicy{Preserve: gateMode == "preserve", AutoClear: autoClear, Approvals: approveGates}
	if err := gatePolicy.Validate(); err != nil {
		usageErr(err)
	}
	if reportPath == "-" {
		usageErr(fmt.Errorf("--report requires a file path; stdout is reserved for the archive"))
	}
	if reportPath != "" && output != "" && output != "-" {
		if err := separateReportPath(reportPath, output); err != nil {
			usageErr(err)
		}
	}

	// Logs go to stderr so captured content can be piped from stdout.
	level := logos.LevelInfo
	if verbose {
		level = logos.LevelDebug
	}
	logger := logos.NewLogger(level, logos.ConsoleFormatter(), os.Stderr)
	logos.SetDefaultLogger(logger)

	// Layer in config: CLI flags override the config file, which overrides
	// built-in defaults. Load first, then resolve each option by precedence.
	cfg, loaded, warnings, err := config.Load()
	if err != nil {
		fail(fmt.Errorf("failed to read config: %w", err))
	}
	set := explicitFlags()
	if savedGate != nil && !set["format"] {
		format = savedGate.Format
		set["format"] = true
	}
	for _, path := range loaded {
		logger.With("config", path).Debug("loaded config")
	}
	for _, w := range warnings {
		logger.Warn("config: " + w)
	}

	format = pickStr(set, "format", format, cfg.Defaults.Format, defFormat)
	links = pickStr(set, "links", links, cfg.Defaults.Links, defLinks)
	profile = pickStr(set, "profile", profile, cfg.Defaults.Profile, "")
	cache = pickStr(set, "cache", cache, cfg.CacheDir, "")
	timeout = pickInt(set, "timeout", timeout, cfg.Defaults.Timeout, defTimeout)
	userAgent = pickStr(set, "user-agent", userAgent, cfg.Defaults.UserAgent, "")
	width := pickInt(set, "width", size.width, cfg.Defaults.Width, 0)
	height := pickInt(set, "height", size.height, cfg.Defaults.Height, 0)
	dpr := size.dpr
	if !set["dpr"] && cfg.Defaults.DPR != 0 {
		dpr = cfg.Defaults.DPR
	}
	paper := pickStr(set, "paper", size.paper, cfg.Defaults.Paper, "")
	// --no-X wins; the deprecated --X=false still works; then config.
	favicon = !noFavicon && pickBool(set, "favicon", favicon, cfg.Defaults.Favicon, true)
	text = !noText && pickBool(set, "text", text, cfg.Defaults.Text, true)

	// Everything that can be wrong with the invocation is rejected here,
	// before a browser starts: exit 2, with the flag named.
	if err := (&sizeFlags{width: width, height: height, dpr: dpr}).validate(); err != nil {
		usageErr(err)
	}
	if err := validTimeout(timeout); err != nil {
		usageErr(err)
	}
	if err := validStyle(look.style); err != nil {
		usageErr(err)
	}
	matchOpts := match.options()
	for _, phrase := range highlight {
		if _, err := textlayer.Compile(phrase, matchOpts); err != nil {
			usageErr(err)
		}
	}
	if len(highlight) == 0 && (len(look.colors) > 0 || look.style != "" || match.any()) {
		usageErr(fmt.Errorf("matching and --mark-* options apply to -M/--mark phrases, and none was given"))
	}
	paperWidthIn, paperHeightIn, err := parsePaper(paper)
	if err != nil {
		usageErr(err)
	}

	backend := backendFor(format)
	if backend == nil {
		usageErr(fmt.Errorf("unknown format %q (want %s)", format, formatList()))
	}
	if savedGate != nil && !set["output"] {
		output = savedGate.destination(clearFile, backend.Ext(), flag.CommandLine.Changed("format"))
		set["output"] = true
	}
	if savedGate != nil {
		if output != "-" {
			if err := separateReportPath(clearFile, output); err != nil {
				usageErr(fmt.Errorf("--output would overwrite the --clear metadata file: %w", err))
			}
		}
		if reportPath != "" {
			if err := separateReportPath(clearFile, reportPath); err != nil {
				usageErr(fmt.Errorf("--report would overwrite the --clear metadata file: %w", err))
			}
		}
	}
	if strict {
		html, ok := backend.(capture.HTML)
		if !ok {
			usageErr(fmt.Errorf("--strict requires -f html"))
		}
		html.Strict = true
		backend = html
	}

	// Prep recipe: either the default staticize, or a custom pscription
	// (e.g. one that logs in via a headful wait-for-user step) whose steps
	// run before serialization, in the same browser session.
	var recipe pipeline.Recipe
	if rxFile != "" {
		p, err := pscription.ParseFile(rxFile)
		if err != nil {
			fail(fmt.Errorf("failed to parse pscription: %w", err))
		}
		r, _, err := p.Recipe("staticize")
		if err != nil {
			fail(fmt.Errorf("failed to build recipe from pscription: %w", err))
		}
		recipe = r
	} else {
		recipe = pipeline.Staticize()
	}

	switch links {
	case "keep":
		// default: links exactly as the page had them
	case "new-tab", "disable":
		recipe = recipe.Append(pipeline.RewriteLinksStep(links))
	default:
		usageErr(fmt.Errorf("unknown link policy %q (want keep, new-tab, or disable)", links))
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()

	runCfg := runner.DefaultConfig()
	runCfg.Headful = headful
	runCfg.UserAgent = userAgent
	if width > 0 {
		runCfg.ViewportWidth = int64(width)
	}
	if height > 0 {
		runCfg.ViewportHeight = int64(height)
	}
	if dpr > 0 {
		runCfg.DeviceScaleFactor = dpr
	}
	if profile != "" {
		dir, err := prepareDir(config.ExpandPath(profile))
		if err != nil {
			fail(err)
		}
		runCfg.UserDataDir = dir
	}
	if cache != "" {
		dir, err := prepareDir(config.ExpandPath(cache))
		if err != nil {
			fail(err)
		}
		runCfg.DiskCacheDir = dir
		logger.With("cache", dir).Debug("using shared HTTP cache")
	}

	logger.With("url", url).With("format", format).Info("archiving")
	start := time.Now()

	opts := capture.Options{
		Gates: gatePolicy,
		// Only a document container can carry these; the backend answers for
		// itself, so a new or compiled-out backend needs no edit here.
		TextLayer:     text && capture.CarriesLayer(backend),
		Favicon:       favicon && capture.CarriesLayer(backend),
		Highlight:     highlight,
		Match:         matchOpts,
		MarkColors:    look.colors,
		MarkStyle:     look.style,
		PaperWidthIn:  paperWidthIn,
		PaperHeightIn: paperHeightIn,
	}
	snap, err := capture.CaptureWithOptions(ctx, url, runCfg, recipe, backend, opts)
	if err != nil {
		fail(fmt.Errorf("capture failed: %w", err))
	}

	// Decide the destination now that we have the title (needed by {title}).
	dest, toStdout := resolveDest(set["output"], output, cfg, url, snap.Title, backend.Ext(), start)
	normalDest := dest
	pending := snap.Access != nil && len(snap.Access.Pending) > 0
	gatePath := ""
	if !toStdout {
		if info, err := os.Stat(dest); err == nil && info.IsDir() {
			fail(fmt.Errorf("output %q is a directory", dest))
		}
	}
	if pending {
		// Stdout has no discoverable filename (shell redirection is outside
		// this process). Keep streaming, and also save the required artifact
		// pair under the URL-derived name in the working directory.
		if toStdout {
			normalDest = deriveFilename(url) + backend.Ext()
		}
		normalDest = plainFilename(normalDest)
		dest = gatedFilename(normalDest)
		gatePath = metadataFilename(dest)
		if err := separateReportPath(gatePath, normalDest); err != nil {
			usageErr(err)
		}
		if err := separateReportPath(dest, normalDest); err != nil {
			usageErr(err)
		}
		if err := separateReportPath(gatePath, dest); err != nil {
			usageErr(err)
		}
	}
	if reportPath != "" && (!toStdout || pending) {
		if err := separateReportPath(reportPath, dest); err != nil {
			usageErr(err)
		}
		if gatePath != "" {
			if err := separateReportPath(reportPath, gatePath); err != nil {
				usageErr(err)
			}
		}
	}
	if !toStdout || pending {
		if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
			fail(fmt.Errorf("create output directory: %w", err))
		}
		if err := os.WriteFile(dest, snap.Bytes, 0644); err != nil {
			fail(fmt.Errorf("write output: %w", err))
		}
	}
	if pending {
		if err := writeCaptureJSON(gatePath, newCaptureMetadata(snap, normalDest, backend.Name())); err != nil {
			fail(fmt.Errorf("write metadata file: %w", err))
		}
		logger.With("metadata", gatePath).Info("saved access decisions for --clear")
	}
	if toStdout {
		if _, err := os.Stdout.Write(snap.Bytes); err != nil {
			fail(fmt.Errorf("write stdout: %w", err))
		}
	}
	if reportPath != "" {
		reportOutput := dest
		if toStdout && !pending {
			reportOutput = "-"
		}
		if err := writeCaptureJSON(reportPath, snap.Report(reportOutput)); err != nil {
			fail(fmt.Errorf("write capture report: %w", err))
		}
	}

	out := dest
	if toStdout && !pending {
		out = "(stdout)"
	}
	logger.With("output", out).
		With("title", snap.Title).
		With("bytes", len(snap.Bytes)).
		With("duration", time.Since(start).Round(time.Second).String()).
		Info("archived")
	if snap.Access != nil && len(snap.Access.Pending) > 0 {
		for _, gate := range snap.Access.Pending {
			logger.With("gate", gate.ID).With("kind", gate.Kind).With("reason", gate.Reason).
				Warn("access gate preserved; rerun with --clear and the matching metadata JSON file")
		}
		if snap.Access.Status == "decision_required" {
			os.Exit(3)
		}
	}
}

// Refuse lexical aliases, symlinks and hard links that would overwrite the
// archive with its report after a successful capture.
func separateReportPath(report, output string) error {
	r, err := filepath.Abs(report)
	if err != nil {
		return err
	}
	o, err := filepath.Abs(output)
	if err != nil {
		return err
	}
	if r == o {
		return fmt.Errorf("--report and --output must name different files")
	}
	ri, re := os.Stat(r)
	oi, oe := os.Stat(o)
	if re == nil && oe == nil && os.SameFile(ri, oi) {
		return fmt.Errorf("--report and --output refer to the same file")
	}
	// Resolve existing parent symlinks even when the leaf does not yet exist.
	rd, re := filepath.EvalSymlinks(filepath.Dir(r))
	od, oe := filepath.EvalSymlinks(filepath.Dir(o))
	if re == nil && oe == nil && filepath.Join(rd, filepath.Base(r)) == filepath.Join(od, filepath.Base(o)) {
		return fmt.Errorf("--report and --output refer to the same path")
	}
	return nil
}

// runHelpCommand implements `parch help [cmd]`: the subcommand's own
// usage (via its -help path), or the top-level usage without one.
func runHelpCommand(args []string) {
	if len(args) == 0 {
		flag.Usage()
		return
	}
	if run, ok := subcommands[args[0]]; ok {
		run([]string{"--help"})
		return
	}
	usageErr(fmt.Errorf("unknown command %q (try `parch help`)", args[0]))
}

// backendFor resolves -f to a backend. Backends register themselves in
// capture, so this stays correct however many of them the build contains.
func backendFor(format string) capture.Backend {
	return capture.Lookup(format)
}

// formatList renders the registered formats for help text and errors. Built
// from the registry rather than written out, so it can never advertise a
// format this binary does not have.
func formatList() string {
	return strings.Join(capture.Formats(), ", ")
}

// resolveDest picks the output path and whether to write to stdout, applying:
//   - "-o -"                → stdout
//   - explicit "-o <path>"  → that path, verbatim
//   - config output_dir or filename → a file built from them
//   - otherwise             → stdout when piped, else the URL-derived name in cwd
func resolveDest(explicitO bool, output string, cfg config.Config, url, title, ext string, now time.Time) (dest string, toStdout bool) {
	if output == "-" {
		return "", true
	}
	if explicitO {
		return output, false
	}

	name := cfg.Filename
	if name != "" {
		name = config.Render(name, config.Vars{URL: url, Title: title, Ext: ext, Now: now})
	}

	outputDir := config.ExpandPath(cfg.OutputDir)

	// With no output configured, keep the historical behavior: content to
	// stdout when piped, otherwise a URL-derived filename in the cwd.
	if name == "" && outputDir == "" {
		if !stdoutIsTerminal() {
			return "", true
		}
		return deriveFilename(url) + ext, false
	}

	if name == "" {
		name = deriveFilename(url) + ext
	}
	return filepath.Join(outputDir, name), false
}

// explicitFlags reports which flags were actually given on the command line
// (as opposed to left at their default), so config can fill only the rest.
// pflag gives every flag one canonical name, so a shorthand and its long
// form are already the same entry.
func explicitFlags() map[string]bool {
	set := map[string]bool{}
	flag.Visit(func(f *flag.Flag) { set[f.Name] = true })
	return set
}

func pickStr(set map[string]bool, name, flagVal, cfgVal, def string) string {
	if set[name] {
		return flagVal
	}
	if cfgVal != "" {
		return cfgVal
	}
	return def
}

func pickInt(set map[string]bool, name string, flagVal, cfgVal, def int) int {
	if set[name] {
		return flagVal
	}
	if cfgVal != 0 {
		return cfgVal
	}
	return def
}

func pickBool(set map[string]bool, name string, flagVal bool, cfgVal *bool, def bool) bool {
	if set[name] {
		return flagVal
	}
	if cfgVal != nil {
		return *cfgVal
	}
	return def
}

func stdoutIsTerminal() bool {
	info, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

// stringsFlag collects a repeatable string flag.
type stringsFlag []string

func (s *stringsFlag) String() string { return strings.Join(*s, ", ") }
func (s *stringsFlag) Set(v string) error {
	*s = append(*s, v)
	return nil
}

// Type names the value in help output ("-M, --mark phrase"); pflag requires
// it, and it replaces the backquote trick Go's flag package needed.
func (s *stringsFlag) Type() string { return "phrase" }

// prepareDir expands ~ (handled by the caller) and ensures the directory exists.
func prepareDir(dir string) (string, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("cannot create dir %q: %w", dir, err)
	}
	return dir, nil
}

// targetURL turns the capture argument into a URL: one that already has a
// scheme is used as given, a path to an existing file becomes its file: URL
// (so a page saved on disk can be captured like a live one), and anything
// else is taken for a host and gets https://.
func targetURL(arg string) string {
	if strings.Contains(arg, "://") {
		return arg
	}
	if info, err := os.Stat(arg); err == nil && !info.IsDir() {
		if abs, err := filepath.Abs(arg); err == nil {
			return fileURL(abs)
		}
	}
	return "https://" + arg
}

// deriveFilename turns a URL into a safe base filename: scheme and "www."
// dropped, every character a filesystem or shell objects to replaced, and
// the result capped so a long query string cannot push the name past the
// filesystem's limit — which used to fail the write after the whole capture
// had been paid for.
func deriveFilename(url string) string {
	name := url
	if i := strings.Index(name, "://"); i != -1 {
		name = name[i+3:]
	}
	name = strings.TrimPrefix(name, "www.")
	name = strings.Map(func(r rune) rune {
		switch r {
		case '/', '?', '&', '=', ':', '%', '#', '\\', '*', '|', '"', '<', '>':
			return '_'
		}
		if r < 0x20 {
			return '_'
		}
		return r
	}, name)
	name = strings.Trim(config.Truncate(strings.Trim(name, "_."), 200), "_.")
	if name == "" {
		name = "page"
	}
	return name
}
