package main

import (
	"fmt"
	flag "github.com/spf13/pflag"
	"strings"

	"github.com/goodblaster/parcher/parchmint/textlayer"
)

// matchFlags holds the matching-mode flags shared by the find, mark, and
// pdf subcommands, so every consumer of the textlayer matcher exposes the
// same modes with the same names.
type matchFlags struct {
	regex      bool
	ignoreCase bool
	fuzzy      int
	wholeWord  bool
	caseSens   bool
	fixed      bool
	inTypes    string
	source     string
}

// registerMatchFlags adds the shared matching flags to fs.
func registerMatchFlags(fs *flag.FlagSet) *matchFlags {
	m := &matchFlags{}
	fs.BoolVarP(&m.regex, "regex", "e", false, "treat the query as a regular expression (RE2) over the raw block text")
	fs.BoolVarP(&m.ignoreCase, "ignore-case", "i", false, "case-insensitive regex (only with -e)")
	fs.IntVarP(&m.fuzzy, "fuzzy", "z", 0, "accept words within this edit distance (1-3) of each query word — for OCR'd text")
	fs.BoolVarP(&m.wholeWord, "word", "w", false, "match whole words only (opt out of the loose substring default)")
	fs.BoolVarP(&m.caseSens, "case-sensitive", "s", false, "case-sensitive (accents and punctuation still fold)")
	fs.BoolVarP(&m.fixed, "fixed", "F", false, "literal substring of the raw text: no punctuation folding, no * wildcard")
	fs.StringVar(&m.inTypes, "in", "", "only match in blocks of these types, comma-separated (p,h1..h6,li,td,th,caption,pre,blockquote,img,other)")
	fs.StringVar(&m.source, "source", "", "only match text from this source: ocr (inside images, needs 'parch ocr') or dom (page text)")
	return m
}

// options assembles the MatchOptions after fs.Parse has run.
func (m *matchFlags) options() textlayer.MatchOptions {
	opts := textlayer.MatchOptions{
		Regex:      m.regex,
		IgnoreCase: m.ignoreCase,
		Fuzzy:      m.fuzzy,
		WholeWord:  m.wholeWord,
		CaseSens:   m.caseSens,
		Fixed:      m.fixed,
		Source:     m.source,
	}
	if m.inTypes != "" {
		opts.InTypes = strings.Split(m.inTypes, ",")
	}
	return opts
}

// validStyle vets the shared -style value for mark/pdf.
func validStyle(s string) error {
	switch s {
	case "", "bg", "underline", "box", "bold":
		return nil
	}
	return fmt.Errorf("unknown style %q (want bg, underline, box, or bold)", s)
}
