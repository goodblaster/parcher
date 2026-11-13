// pscribe runs a pscription — an ordered list of preparation steps and scripts —
// against a URL in a headless (or visible) browser, and prints a JSON report
// of what the scripts returned. Like a doctor's ℞ (Latin recipe, "take
// thou"): a recipe someone else executes precisely.
//
//	# steps given inline (order = execution order)
//	pscribe https://news.site @article                       # built-in article extractor
//	pscribe https://shop.com dismiss.js prices.js:prices      # your own scripts, capture "prices"
//	pscribe https://site.com @links --only links | xargs parch # crawler-without-a-crawler
//
//	# steps given as a .rx pscription file
//	pscribe --recipe login-and-scrape.rx https://portal.site
//	./login-and-scrape.rx https://portal.site            # if chmod +x with a shebang
//
// stdout carries the JSON report (or, with --only, one output raw). Logs go to
// stderr. Exit status is non-zero if any step failed.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	flag "github.com/spf13/pflag"

	"github.com/goodblaster/logos"
	"github.com/goodblaster/parcher/pscription/internal/log"
	"github.com/goodblaster/parcher/pscription/pscription"
)

// version identifies the build; stamp releases with
// -ldflags "-X main.version=v1.2.3".
var version = "dev"

func main() {
	// Checked before flags: every bare word here is otherwise taken for a
	// URL or a step, so `pscribe version` would try to open https://version.
	// -v is already --verbose, so version has no shorthand.
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "version", "-version", "--version":
			fmt.Println("pscribe " + version)
			return
		}
	}

	var (
		recipe    string
		prep      string
		only      string
		profile   string
		headful   bool
		timeout   int
		width     int
		height    int
		dpr       float64
		userAgent string
		verbose   bool
	)

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options] <url> [step ...]\n\n", os.Args[0])
		fmt.Fprintln(os.Stderr, "Steps (order = execution order):")
		fmt.Fprintln(os.Stderr, "  <name>          a built-in step (e.g. dismiss-popups, wait-quiet)")
		fmt.Fprintln(os.Stderr, "  @name[:out]     a built-in script (@article, @links)")
		fmt.Fprintln(os.Stderr, "  path.js[:out]   your own page-context script")
		fmt.Fprintln(os.Stderr, "  wait:user       pause for manual action (needs --show)")
		fmt.Fprintln(os.Stderr, "  wait:2s         pause for a duration")
		fmt.Fprintln(os.Stderr, "\nOther:")
		fmt.Fprintf(os.Stderr, "  %s version     print the version and exit\n", filepath.Base(os.Args[0]))
		fmt.Fprintln(os.Stderr, "\nOptions:")
		flag.PrintDefaults()
	}
	// Flag names and meanings match parch's wherever both have the option
	// (--recipe, --profile, --show, --timeout, --width, --height, --dpr,
	// --user-agent, -v), so nothing has to be relearned between the two.
	flag.StringVar(&recipe, "recipe", "", "run this .rx pscription file instead of inline steps")
	flag.StringVarP(&recipe, "recipe-file", "r", "", "deprecated spelling of --recipe")
	_ = flag.CommandLine.MarkDeprecated("recipe-file", "use --recipe")
	flag.StringVar(&prep, "prep", "", "prep phase: none, minimal (default), staticize")
	flag.StringVar(&only, "only", "", "print just this output value (raw), not the full report")
	flag.StringVar(&profile, "profile", "", "persistent Chrome user-data dir; log in once (with --show) and reuse the session on later runs")
	flag.BoolVar(&headful, "show", false, "launch a visible browser (needed for wait:user)")
	flag.IntVar(&timeout, "timeout", 120, "timeout in seconds")
	flag.IntVar(&width, "width", 0, "viewport/layout width in px (0 = default 1600)")
	flag.IntVar(&height, "height", 0, "viewport height in px (0 = default 900)")
	flag.Float64Var(&dpr, "dpr", 0, "device pixel ratio (0 = 1)")
	flag.StringVar(&userAgent, "user-agent", "", "user agent to announce (default: the browser's own Chrome one)")
	flag.BoolVarP(&verbose, "verbose", "v", false, "debug logging")

	// Steps are positional and trail the URL; pflag parses flags anywhere,
	// so `pscribe URL @links --only links | xargs parch` works with no
	// reordering (Go's stdlib flag package stopped at the first positional,
	// which is why this used to need a helper).
	_ = flag.CommandLine.Parse(os.Args[1:])

	level := logos.LevelInfo
	if verbose {
		level = logos.LevelDebug
	}
	// Configure logos itself, not just this module's seam: the seam's default
	// delegates to the logos package logger, so one call covers both. (An
	// Init helper used to run after this and reset the logos logger to DEBUG,
	// which is how debug lines leaked out without -v.)
	logos.SetDefaultLogger(logos.NewLogger(level, logos.ConsoleFormatter(), os.Stderr))

	switch {
	case timeout <= 0:
		usageErr(fmt.Errorf("--timeout must be a positive number of seconds, got %d", timeout))
	case width < 0 || height < 0:
		usageErr(fmt.Errorf("--width and --height must not be negative"))
	case dpr < 0:
		usageErr(fmt.Errorf("--dpr must not be negative"))
	}

	args := flag.Args()

	// Shebang form: `./thing.rx https://site` execs `pscribe ./thing.rx
	// https://site`, so a .rx file as the first positional means "run this
	// pscription". Equivalent to --recipe.
	if recipe == "" && len(args) > 0 && strings.HasSuffix(args[0], ".rx") {
		recipe = args[0]
		args = args[1:]
	}

	if len(args) < 1 {
		flag.Usage()
		os.Exit(2)
	}
	url := args[0]
	if !strings.Contains(url, "://") {
		url = "https://" + url
	}

	// Build the pscription: from a recipe file, or from inline step tokens.
	var p *pscription.Pscription
	var err error
	if recipe != "" {
		p, err = pscription.ParseFile(recipe)
		if err != nil {
			die(err)
		}
	} else {
		cwd, _ := os.Getwd()
		steps, err := pscription.ParseTokens(args[1:], cwd)
		if err != nil {
			die(err)
		}
		p = &pscription.Pscription{Steps: steps}
	}

	// CLI flags override file directives when set.
	if prep != "" {
		p.Prep = prep
	}
	if headful {
		p.Headful = true
	}
	if profile != "" {
		dir, err := prepareProfileDir(profile)
		if err != nil {
			die(err)
		}
		profile = dir
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()

	log.With("url", url).With("steps", len(p.Steps)).With("prep", firstNonEmpty(p.Prep, "minimal")).Info("running pscription")

	report, err := p.Run(ctx, url, pscription.Options{
		Timeout:        time.Duration(timeout) * time.Second,
		Profile:        profile,
		ViewportWidth:  int64(width),
		ViewportHeight: int64(height),
		DPR:            dpr,
		UserAgent:      userAgent,
	})
	if err != nil {
		die(err)
	}

	if err := emit(report, only); err != nil {
		die(err)
	}

	if !report.OK {
		os.Exit(1)
	}
}

// emit writes the report to stdout: the whole thing as JSON, or just one
// named output (raw string as-is, other values as JSON) with --only.
func emit(report *pscription.Report, only string) error {
	if only != "" {
		val, ok := report.Outputs[only]
		if !ok {
			return fmt.Errorf("no output named %q (have %v)", only, keys(report.Outputs))
		}
		if s, isString := val.(string); isString {
			_, err := fmt.Fprintln(os.Stdout, s)
			return err
		}
		b, err := json.MarshalIndent(val, "", "  ")
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(os.Stdout, string(b))
		return err
	}

	b, err := report.JSON()
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(os.Stdout, string(b))
	return err
}

func keys(m map[string]any) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// die prints a clean error to stderr and exits 1 — never a Go panic/stack
// trace (logos.Fatal panics, which is wrong for user-facing errors).
func die(err error) {
	fmt.Fprintln(os.Stderr, "pscribe: "+err.Error())
	os.Exit(1)
}

// usageErr reports a bad invocation and exits 2 — the same split parch uses:
// 2 for usage, 1 for a run that failed.
func usageErr(err error) {
	fmt.Fprintln(os.Stderr, "pscribe: "+err.Error())
	os.Exit(2)
}

// prepareProfileDir expands ~ and ensures the persistent profile directory
// exists.
func prepareProfileDir(dir string) (string, error) {
	dir = pscription.ExpandHome(dir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("cannot create profile dir %q: %w", dir, err)
	}
	return dir, nil
}
