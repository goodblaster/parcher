package main

import (
	"fmt"
	"os"
	"strings"

	flag "github.com/spf13/pflag"

	"github.com/goodblaster/parcher/parchmint/textlayer"
)

// The flag vocabulary, in one place. Every command draws its options from
// these groups, so a name means the same thing wherever it appears and a
// letter is never reused:
//
//	-o --output       where the result goes ('-' = stdout)
//	-f --format       output format
//	-v --verbose      debug logging                    (capture)
//	-M --mark         phrase to mark at capture time   (capture)
//	-e -i -z -w -s -F matching modes                   (capture, find, mark)
//	-l -c -q -m -r -H -0   grep-shaped output control  (find)
//	-h --help         usage, on every command
//
//	--width --height --dpr --paper   the three size axes (capture, mark, render)
//	--timeout                        seconds             (capture, mark, render)
//	--mark-color --mark-style        how marks look      (capture, mark)
//	--hidden[=reasons]               include hidden text (find, text, lines, mark)
//
// Negatable switches are spelled --no-X (--no-text, --no-favicon,
// --no-browser, --no-filename), never --X=false.
//
// Exit status, every command: 0 success, 1 the operation failed (or, for
// find and mark, nothing matched), 2 the invocation was wrong.

// usageErr reports a bad invocation and exits 2.
func usageErr(err error) {
	fmt.Fprintln(os.Stderr, "parch: "+err.Error())
	os.Exit(2)
}

// fail reports a failed operation and exits 1.
func fail(err error) {
	fmt.Fprintln(os.Stderr, "parch: "+err.Error())
	os.Exit(1)
}

// sizeFlags are the three independent size axes (see parchmint/CLAUDE.md).
type sizeFlags struct {
	width, height int
	dpr           float64
	paper         string
}

// registerSizeFlags adds --width/--height/--dpr/--paper. rerender words the
// help for commands that re-lay-out an existing archive rather than load a
// live page.
func registerSizeFlags(fs *flag.FlagSet, rerender bool) *sizeFlags {
	s := &sizeFlags{}
	if rerender {
		fs.IntVar(&s.width, "width", 0, "re-render at this layout width in px, reflowing the archive (0 = the width it was captured at)")
		fs.IntVar(&s.height, "height", 0, "re-render at this viewport height in px (0 = the height it was captured at)")
		fs.Float64Var(&s.dpr, "dpr", 0, "device pixel ratio: output pixels per CSS pixel, e.g. 0.4 to shrink the output without reflowing (0 = 1)")
	} else {
		fs.IntVar(&s.width, "width", 0, "viewport/layout width in px — picks the responsive breakpoint (0 = default 1600)")
		fs.IntVar(&s.height, "height", 0, "viewport height in px (0 = default 900); pages laid out in an unrealistically short viewport mis-render")
		fs.Float64Var(&s.dpr, "dpr", 0, "device pixel ratio: output pixels per CSS pixel, e.g. 2 for retina rasters (0 = 1)")
	}
	fs.StringVar(&s.paper, "paper", "", "pdf sheet size, independent of --width: auto (default, sheet follows the layout), letter, legal, tabloid, a3, a4, a5, inches (10.5), or WxH (8.5x11)")
	return s
}

// validate rejects sizes that used to be silently ignored (a negative width
// fell back to the default without a word).
func (s *sizeFlags) validate() error {
	switch {
	case s.width < 0:
		return fmt.Errorf("--width must not be negative, got %d", s.width)
	case s.height < 0:
		return fmt.Errorf("--height must not be negative, got %d", s.height)
	case s.dpr < 0:
		return fmt.Errorf("--dpr must not be negative, got %g", s.dpr)
	}
	return nil
}

// validTimeout rejects a zero or negative --timeout, which used to build a
// context that had already expired: every run died instantly with "context
// deadline exceeded" and nothing naming the flag.
func validTimeout(seconds int) error {
	if seconds <= 0 {
		return fmt.Errorf("--timeout must be a positive number of seconds, got %d", seconds)
	}
	return nil
}

// markLook holds how marks are drawn: shared by capture-time -M and `mark`.
type markLook struct {
	colors stringsFlag
	style  string
}

// registerMarkLook adds --mark-color/--mark-style. legacy also accepts the
// spellings `mark` used to have (--color/-c and --style), deprecated: -c is
// find's --count, and one letter gets one meaning.
func registerMarkLook(fs *flag.FlagSet, legacy bool) *markLook {
	m := &markLook{}
	fs.Var(&m.colors, "mark-color", "highlight color, repeatable: the Nth --mark-color colors the Nth phrase; unpaired phrases keep the default yellow")
	fs.StringVar(&m.style, "mark-style", "", "highlight style: bg (default), underline, box, or bold")
	if legacy {
		fs.VarP(&m.colors, "color", "c", "deprecated spelling of --mark-color")
		_ = fs.MarkDeprecated("color", "use --mark-color")
		fs.StringVar(&m.style, "style", "", "deprecated spelling of --mark-style")
		_ = fs.MarkDeprecated("style", "use --mark-style")
	}
	return m
}

// registerHidden adds --hidden[=reasons]. Bare --hidden means every reason;
// --hidden=collapsed,display narrows it. (The value must be attached with
// '=': a flag with an optional value cannot take it from the next word.)
func registerHidden(fs *flag.FlagSet, usage string) *string {
	v := fs.String("hidden", "", usage+" Optionally only some reasons: --hidden="+strings.Join(textlayer.HiddenReasons, ","))
	fs.Lookup("hidden").NoOptDefVal = textlayer.HiddenAll
	return v
}

// hiddenReasons parses a --hidden value into MatchOptions.Hidden.
func hiddenReasons(v string) []string {
	if v == "" {
		return nil
	}
	var out []string
	for _, r := range strings.Split(v, ",") {
		if r = strings.TrimSpace(r); r != "" {
			out = append(out, r)
		}
	}
	return out
}

// selectBlocks returns the layer's blocks for the text-shaped commands:
// visible ones, plus the hidden ones whose reason is selected. Text hidden
// by parch's own prep is included only when named explicitly.
func selectBlocks(layer *textlayer.Layer, reasons []string) ([]textlayer.Block, error) {
	if err := (textlayer.MatchOptions{Hidden: reasons}).Validate(); err != nil {
		return nil, err
	}
	out := layer.Blocks
	if len(reasons) == 0 {
		return out, nil
	}
	want := map[string]bool{}
	for _, r := range reasons {
		if r == textlayer.HiddenAll {
			for _, known := range textlayer.HiddenReasons {
				if known != textlayer.HiddenPrep {
					want[known] = true
				}
			}
			continue
		}
		want[r] = true
	}
	out = append([]textlayer.Block(nil), out...)
	for _, b := range layer.Hidden {
		if want[b.Why] {
			out = append(out, b)
		}
	}
	return out, nil
}
