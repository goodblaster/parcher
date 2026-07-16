// rx runs a pscription — an ordered list of preparation steps and scripts —
// against a URL in a headless (or visible) browser, and prints a JSON report
// of what the scripts returned. Like a doctor's ℞ (Latin recipe, "take
// thou"): a recipe someone else executes precisely.
//
//	# steps given inline (order = execution order)
//	rx https://news.site @article                       # built-in article extractor
//	rx https://shop.com dismiss.js prices.js:prices      # your own scripts, capture "prices"
//	rx https://site.com @links -only links | xargs parch # crawler-without-a-crawler
//
//	# steps given as a .rx pscription file
//	rx -f login-and-scrape.rx https://portal.site
//	./login-and-scrape.rx https://portal.site            # if chmod +x with a shebang
//
// stdout carries the JSON report (or, with -only, one output raw). Logs go to
// stderr. Exit status is non-zero if any step failed.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/goodblaster/logos"
	"github.com/goodblaster/pscription/internal/common"
	"github.com/goodblaster/pscription/internal/log"
	"github.com/goodblaster/pscription/pscription"
)

func main() {
	var (
		file    string
		prep    string
		only    string
		profile string
		headful bool
		timeout int
		verbose bool
	)

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options] <url> [step ...]\n\n", os.Args[0])
		fmt.Fprintln(os.Stderr, "Steps (order = execution order):")
		fmt.Fprintln(os.Stderr, "  <name>          a built-in step (e.g. dismiss-popups, wait-quiet)")
		fmt.Fprintln(os.Stderr, "  @name[:out]     a built-in script (@article, @links)")
		fmt.Fprintln(os.Stderr, "  path.js[:out]   your own page-context script")
		fmt.Fprintln(os.Stderr, "  wait:user       pause for manual action (needs -show)")
		fmt.Fprintln(os.Stderr, "  wait:2s         pause for a duration")
		fmt.Fprintln(os.Stderr, "\nOptions:")
		flag.PrintDefaults()
	}
	flag.StringVar(&file, "f", "", "run a .rx pscription file instead of inline steps")
	flag.StringVar(&prep, "prep", "", "prep phase: none, minimal (default), staticize")
	flag.StringVar(&only, "only", "", "print just this output value (raw), not the full report")
	flag.StringVar(&profile, "profile", "", "persistent Chrome user-data dir; log in once (with -show) and reuse the session on later runs")
	flag.BoolVar(&headful, "show", false, "launch a visible browser (needed for wait:user)")
	flag.IntVar(&timeout, "timeout", 120, "timeout in seconds")
	flag.BoolVar(&verbose, "v", false, "debug logging")

	// Steps are positional and trail the URL, but flags should still work
	// anywhere (e.g. `rx URL @links -only links | xargs parch`). Reorder so
	// flags precede positionals before the stdlib parser, which otherwise
	// stops at the first positional.
	flag.CommandLine.Parse(reorderFlags(os.Args[1:], map[string]bool{"show": true, "v": true}))

	level := logos.LevelInfo
	if verbose {
		level = logos.LevelDebug
	}
	log.SetDefault(log.LogosAdapter(logos.NewLogger(level, logos.ConsoleFormatter(), os.Stderr)))
	if err := common.Init(); err != nil {
		die(err)
	}

	args := flag.Args()

	// Shebang form: `./thing.rx https://site` execs `rx ./thing.rx
	// https://site`, so a .rx file as the first positional means "run this
	// pscription". Equivalent to -f.
	if file == "" && len(args) > 0 && strings.HasSuffix(args[0], ".rx") {
		file = args[0]
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

	// Build the pscription: from a file, or from inline step tokens.
	var p *pscription.Pscription
	var err error
	if file != "" {
		p, err = pscription.ParseFile(file)
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
		Timeout: time.Duration(timeout) * time.Second,
		Profile: profile,
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
// named output (raw string as-is, other values as JSON) with -only.
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
	fmt.Fprintln(os.Stderr, "rx: "+err.Error())
	os.Exit(1)
}

// prepareProfileDir expands ~ and ensures the persistent profile directory
// exists.
func prepareProfileDir(dir string) (string, error) {
	if strings.HasPrefix(dir, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, dir[2:])
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("cannot create profile dir %q: %w", dir, err)
	}
	return dir, nil
}

// reorderFlags moves flag tokens ahead of positional tokens so trailing
// flags parse. boolFlags names the flags that take no value (so the token
// after them isn't mistaken for their value).
func reorderFlags(args []string, boolFlags map[string]bool) []string {
	var flags, pos []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "-" || !strings.HasPrefix(a, "-") {
			pos = append(pos, a)
			continue
		}
		flags = append(flags, a)
		name := strings.TrimLeft(a, "-")
		if strings.Contains(a, "=") || boolFlags[name] {
			continue
		}
		// Consumes the following token as its value.
		if i+1 < len(args) {
			i++
			flags = append(flags, args[i])
		}
	}
	return append(flags, pos...)
}
