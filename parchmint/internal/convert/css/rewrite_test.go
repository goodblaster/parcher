package css

import (
	"reflect"
	"strings"
	"testing"
)

func upper(url string) string { return strings.ToUpper(url) }

func TestRewrite(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "unquoted url",
			in:   `a { background: url(logo.png) }`,
			want: `a { background: url("LOGO.PNG") }`,
		},
		{
			name: "quoted url",
			in:   `a { background: url( "logo.png" ) }`,
			want: `a { background: url("LOGO.PNG") }`,
		},
		{
			name: "single quoted url",
			in:   `a { background: url('logo.png') }`,
			want: `a { background: url("LOGO.PNG") }`,
		},
		{
			name: "uppercase URL function",
			in:   `a { background: URL(logo.png) }`,
			want: `a { background: url("LOGO.PNG") }`,
		},
		{
			name: "import string",
			in:   `@import "other.css";`,
			want: `@import "OTHER.CSS";`,
		},
		{
			name: "import url",
			in:   `@import url(other.css) screen;`,
			want: `@import url("OTHER.CSS") screen;`,
		},
		{
			name: "content string is not a reference",
			in:   `a::after { content: "logo.png" }`,
			want: `a::after { content: "logo.png" }`,
		},
		{
			name: "url inside a comment is left alone",
			in:   `/* url(logo.png) */ a { color: red }`,
			want: `/* url(logo.png) */ a { color: red }`,
		},
		{
			name: "identifier ending in url is not a function",
			in:   `a { my-url(logo.png): 1 }`,
			want: `a { my-url(logo.png): 1 }`,
		},
		{
			name: "data uri with parens and commas",
			in:   `a { background: url("data:image/svg+xml,<svg x=(1)/>") }`,
			want: `a { background: url("DATA:IMAGE/SVG+XML,<SVG X=(1)/>") }`,
		},
		{
			name: "unterminated url is passed through",
			in:   `a { background: url(logo.png`,
			want: `a { background: url(logo.png`,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := Rewrite(test.in, upper); got != test.want {
				t.Errorf("Rewrite() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestRewriteEscapesReplacement(t *testing.T) {
	got := Rewrite(`a { background: url(x) }`, func(string) string {
		return `he said "hi"\`
	})
	want := `a { background: url("he said \"hi\"\\") }`
	if got != want {
		t.Errorf("Rewrite() = %q, want %q", got, want)
	}
}

func TestURLs(t *testing.T) {
	css := `@import "a.css";
	        /* url(skipped.png) */
	        div { background: url(b.png), url('c.png'); content: "d.png" }`

	want := []string{"a.css", "b.png", "c.png"}
	if got := URLs(css); !reflect.DeepEqual(got, want) {
		t.Errorf("URLs() = %v, want %v", got, want)
	}
}

// Rewriting with an identity function must not disturb the stylesheet apart
// from normalising the quoting of the references themselves.
func TestRewriteIdentityIsStable(t *testing.T) {
	css := `@media screen { a { background: url("x.png") } }`
	once := Rewrite(css, func(url string) string { return url })
	twice := Rewrite(once, func(url string) string { return url })
	if once != twice {
		t.Errorf("not stable:\n%q\n%q", once, twice)
	}
}
