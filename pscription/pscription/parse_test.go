package pscription

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseStep(t *testing.T) {
	cases := []struct {
		token   string
		want    Step
		wantErr bool
	}{
		{"dismiss-popups", Step{Kind: KindBuiltin, Ref: "dismiss-popups"}, false},
		{"  wait-quiet  ", Step{Kind: KindBuiltin, Ref: "wait-quiet"}, false},
		{"wait:user", Step{Kind: KindWait, Ref: "user"}, false},
		{"wait:2s", Step{Kind: KindWait, Ref: "2s"}, false},
		{"@article", Step{Kind: KindBuiltinScript, Ref: "article", Out: "article"}, false},
		{"@article:body", Step{Kind: KindBuiltinScript, Ref: "article", Out: "body"}, false},
		{"prices.js", Step{Kind: KindScriptFile, Ref: "/base/prices.js", Out: "prices"}, false},
		{"sub/get prices.js:out", Step{Kind: KindScriptFile, Ref: "/base/sub/get prices.js", Out: "out"}, false},
		{"/abs/x.js", Step{Kind: KindScriptFile, Ref: "/abs/x.js", Out: "x"}, false},
		// A builtin step cannot capture output.
		{"dismiss-popups:out", Step{}, true},
		{"", Step{}, true},
	}
	for _, c := range cases {
		got, err := ParseStep(c.token, "/base")
		if (err != nil) != c.wantErr {
			t.Errorf("ParseStep(%q) error = %v, wantErr %v", c.token, err, c.wantErr)
			continue
		}
		if err == nil && got != c.want {
			t.Errorf("ParseStep(%q) = %+v, want %+v", c.token, got, c.want)
		}
	}
}

func TestSplitCapture(t *testing.T) {
	cases := []struct{ token, ref, out string }{
		{"a.js:out", "a.js", "out"},
		{"a.js", "a.js", ""},
		{"a.js:", "a.js:", ""},
		// Not an identifier after the colon: the colon belongs to the ref.
		{`C:\scripts\a.js`, `C:\scripts\a.js`, ""},
		{"a.js:has space", "a.js:has space", ""},
	}
	for _, c := range cases {
		if ref, out := splitCapture(c.token); ref != c.ref || out != c.out {
			t.Errorf("splitCapture(%q) = (%q, %q), want (%q, %q)", c.token, ref, out, c.ref, c.out)
		}
	}
}

func TestParseFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "secrets.json"), []byte(`{"user":"u","n":3}`), 0o600); err != nil {
		t.Fatal(err)
	}
	rx := filepath.Join(dir, "login.rx")
	body := "#!/usr/bin/env pscribe\n" +
		"# a comment\n\n" +
		"prep: staticize\n" +
		"headful: yes\n" +
		"profile: prof\n" +
		"vars: secrets.json\n" +
		"var: token=a=b\n" +
		"dismiss-popups\n" +
		"login.js\n" +
		"@links:found\n" +
		"wait:500ms\n"
	if err := os.WriteFile(rx, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}

	p, err := ParseFile(rx)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if p.Prep != "staticize" || !p.Headful {
		t.Errorf("directives: prep=%q headful=%v", p.Prep, p.Headful)
	}
	if want := filepath.Join(dir, "prof"); p.Profile != want {
		t.Errorf("profile = %q, want %q (relative to the .rx file)", p.Profile, want)
	}
	if p.Vars["user"] != "u" || p.Vars["token"] != "a=b" {
		t.Errorf("vars = %v", p.Vars)
	}
	want := []Step{
		{Kind: KindBuiltin, Ref: "dismiss-popups"},
		{Kind: KindScriptFile, Ref: filepath.Join(dir, "login.js"), Out: "login"},
		{Kind: KindBuiltinScript, Ref: "links", Out: "found"},
		{Kind: KindWait, Ref: "500ms"},
	}
	if len(p.Steps) != len(want) {
		t.Fatalf("steps = %+v", p.Steps)
	}
	for i := range want {
		if p.Steps[i] != want[i] {
			t.Errorf("step %d = %+v, want %+v", i, p.Steps[i], want[i])
		}
	}

	if _, err := ParseFile(filepath.Join(dir, "missing.rx")); err == nil {
		t.Error("missing file: want an error")
	}
}

func TestExpandHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory")
	}
	cases := map[string]string{
		"~":         home,
		"~/x/y":     filepath.Join(home, "x/y"),
		"/abs":      "/abs",
		"rel/~/x":   "rel/~/x",
		"~user/dir": "~user/dir", // another user's home is not ours to guess
	}
	for in, want := range cases {
		if got := ExpandHome(in); got != want {
			t.Errorf("ExpandHome(%q) = %q, want %q", in, got, want)
		}
	}
}
