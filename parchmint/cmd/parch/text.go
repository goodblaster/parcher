package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/goodblaster/parchmint/textlayer"
)

// runTextCommand implements `parch text <archive.html>`: read the embedded
// parchmint-text layer back out of an archive. Default output is the
// page's plain text (blocks separated by blank lines) — clean feed for
// pipes and LLMs; -json dumps the whole layer.
func runTextCommand(args []string) {
	fs := flag.NewFlagSet("text", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "print the raw text layer as indented JSON")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s text [-json] <archive.html>\n\nOptions:\n", os.Args[0])
		fs.PrintDefaults()
	}
	_ = fs.Parse(reorderFlags(args, map[string]bool{"json": true}))
	if fs.NArg() != 1 {
		fs.Usage()
		os.Exit(2)
	}

	layer, err := textlayer.FromFile(fs.Arg(0))
	if err != nil {
		fmt.Fprintln(os.Stderr, "parch: "+err.Error())
		os.Exit(1)
	}

	if *asJSON {
		out, err := json.MarshalIndent(layer, "", "  ")
		if err != nil {
			fmt.Fprintln(os.Stderr, "parch: "+err.Error())
			os.Exit(1)
		}
		fmt.Println(string(out))
		return
	}

	for i, b := range layer.Blocks {
		if i > 0 {
			fmt.Println()
		}
		fmt.Println(b.Text)
	}
}
