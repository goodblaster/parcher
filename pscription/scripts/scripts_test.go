package scripts

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// A script that fails to PARSE is a programming error, but nothing at runtime
// treats it as one: prep steps are mostly stepIgnoreError, so the exception is
// logged once and swallowed, and the page silently gets none of that step's
// behavior. That is how a stray backtick inside hide_overlays.js's CSS
// template literal disabled overlay hiding entirely — and looked like a FIX,
// because the thing it wrongly hid came back.
//
// These tests are the cheap guard: syntax is checked here, where it is loud.

// TestScriptsParse runs every embedded script through node's parser. It skips
// when node is absent rather than failing, since node is not a build
// requirement — but where node exists, this catches the whole class.
func TestScriptsParse(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not installed; skipping JavaScript syntax check")
	}

	for _, name := range scriptFiles(t) {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(".", name)
			out, err := exec.Command(node, "--check", path).CombinedOutput()
			if err != nil {
				t.Errorf("does not parse as JavaScript:\n%s", out)
			}
		})
	}
}

// TestTemplateLiteralsAreBalanced needs no toolchain and catches the coarser
// version: an odd number of backticks means a literal is unterminated.
//
// It would NOT have caught the bug above — prose quoting `om-` adds a PAIR,
// which keeps the count even while still ending the literal early and turning
// the CSS that follows into stray JavaScript. Only a real parser sees that,
// which is why TestScriptsParse is the guard that matters and this one is a
// backstop for environments without node.
func TestTemplateLiteralsAreBalanced(t *testing.T) {
	for _, name := range scriptFiles(t) {
		source, err := Files.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if n := strings.Count(string(source), "`"); n%2 != 0 {
			t.Errorf("%s: %d backticks — a template literal is unterminated "+
				"(prose inside one must not quote `like this`)", name, n)
		}
	}
}

func scriptFiles(t *testing.T) []string {
	t.Helper()

	entries, err := Files.ReadDir(".")
	if err != nil {
		t.Fatalf("read embedded scripts: %v", err)
	}

	var names []string
	for _, dir := range entries {
		if !dir.IsDir() {
			continue
		}
		files, err := Files.ReadDir(dir.Name())
		if err != nil {
			t.Fatalf("read %s: %v", dir.Name(), err)
		}
		for _, f := range files {
			if strings.HasSuffix(f.Name(), ".js") {
				names = append(names, dir.Name()+"/"+f.Name())
			}
		}
	}
	if len(names) == 0 {
		t.Fatal("no embedded scripts found")
	}
	return names
}
