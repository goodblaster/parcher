package config

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestRender(t *testing.T) {
	now := time.Date(2026, 7, 18, 15, 4, 5, 0, time.UTC)
	long := strings.Repeat("word ", 100)
	cases := []struct {
		name, tmpl, url, title, want string
	}{
		{"host and ext", "{host}.{ext}", "https://www.example.com/", "", "example.com.html"},
		{"path flattened", "{host}-{path}.{ext}", "https://example.com/a/b?q=1", "", "example.com-a-b.html"},
		{"date tokens", "{date}_{time}", "https://example.com/", "", "2026-07-18_15-04-05"},
		{"custom layout", "{date:2006-01}", "https://example.com/", "", "2026-07"},
		{"unknown token kept", "{nope}.{ext}", "https://example.com/", "", "{nope}.html"},
		{"ascii title", "{title}", "https://example.com/", "Hello, World! (2026)", "Hello-World-2026"},
		{"empty title", "{title}", "https://example.com/", "", "untitled"},
		{"punctuation-only title", "{title}", "https://example.com/", "?!/\\", "untitled"},
		{"cjk title survives", "{title}", "https://example.com/", "首页 - 维基", "首页-维基"},
		{"cyrillic title survives", "{title}", "https://example.com/", "Привет мир", "Привет-мир"},
		{"leading dot stripped", "{title}", "https://example.com/", ".hidden file", "hidden-file"},
		{"shell-hostile characters", "{title}", "https://example.com/", `a\b*c|d"e<f>g`, "a-b-c-d-e-f-g"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Render(c.tmpl, Vars{URL: c.url, Title: c.title, Ext: "html", Now: now})
			if got != c.want {
				t.Fatalf("Render(%q) = %q, want %q", c.tmpl, got, c.want)
			}
		})
	}

	t.Run("long title capped on a rune boundary", func(t *testing.T) {
		got := Render("{title}", Vars{URL: "https://example.com/", Title: long + strings.Repeat("首", 200), Now: now})
		if len(got) > maxComponent {
			t.Fatalf("title component is %d bytes, cap is %d", len(got), maxComponent)
		}
		if !utf8.ValidString(got) {
			t.Fatalf("truncation split a rune: %q", got)
		}
	})
}

func TestTruncate(t *testing.T) {
	if got := Truncate("abc", 10); got != "abc" {
		t.Fatalf("short string changed: %q", got)
	}
	// "é" is two bytes; cutting at 3 must not leave half of the second one.
	if got := Truncate("ééé", 3); got != "é" {
		t.Fatalf("Truncate split a rune: %q", got)
	}
}
