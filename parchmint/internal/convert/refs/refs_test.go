package refs

import (
	"reflect"
	"testing"
)

func TestParseSrcset(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []Candidate
	}{
		{
			name: "descriptors",
			in:   "small.png 1x, large.png 2x",
			want: []Candidate{{"small.png", "1x"}, {"large.png", "2x"}},
		},
		{
			name: "widths",
			in:   "a.png 320w,\n b.png 640w",
			want: []Candidate{{"a.png", "320w"}, {"b.png", "640w"}},
		},
		{
			name: "no descriptor",
			in:   "only.png",
			want: []Candidate{{"only.png", ""}},
		},
		{
			// A URL runs to the next whitespace, so an embedded comma stays
			// part of it. This is what lets data: URIs survive parsing.
			name: "comma inside a url",
			in:   "a.png,b.png 2x",
			want: []Candidate{{"a.png,b.png", "2x"}},
		},
		{
			// Trailing commas are separators, and leave no descriptor.
			name: "trailing comma separates candidates",
			in:   "a.png, b.png 2x",
			want: []Candidate{{"a.png", ""}, {"b.png", "2x"}},
		},
		{
			name: "data uri containing commas",
			in:   "data:image/png;base64,AAAA 1x, b.png 2x",
			want: []Candidate{{"data:image/png;base64,AAAA", "1x"}, {"b.png", "2x"}},
		},
		{
			name: "empty",
			in:   "   ",
			want: nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := ParseSrcset(test.in)
			if !reflect.DeepEqual(got, test.want) {
				t.Errorf("ParseSrcset(%q) = %v, want %v", test.in, got, test.want)
			}
		})
	}
}

func TestFormatSrcsetRoundTrip(t *testing.T) {
	in := "a.png 1x, b.png 2x"
	if got := FormatSrcset(ParseSrcset(in)); got != in {
		t.Errorf("round trip = %q, want %q", got, in)
	}
}

func TestFetchable(t *testing.T) {
	tests := map[string]bool{
		"logo.png":                 true,
		"/logo.png":                true,
		"https://example.com/x":    true,
		"data:image/png;base64,AA": true,
		"cid:part@example":         true,
		"//example.com/x.png":      true,
		"#anchor":                  false,
		"":                         false,
		"   ":                      false,
		"javascript:void(0)":       false,
		"JavaScript:void(0)":       false,
		"about:blank":              false,
		"mailto:a@example.com":     false,
		"blob:https://example/x":   false,
	}

	for ref, want := range tests {
		if got := Fetchable(ref); got != want {
			t.Errorf("Fetchable(%q) = %v, want %v", ref, got, want)
		}
	}
}

func TestAttrs(t *testing.T) {
	if got := Attrs("IMG"); !reflect.DeepEqual(got, []string{"src", "srcset"}) {
		t.Errorf("Attrs(img) = %v", got)
	}
	// Hyperlinks are not subresources.
	if got := Attrs("a"); got != nil {
		t.Errorf("Attrs(a) = %v, want nil", got)
	}
	if got := Attrs("form"); got != nil {
		t.Errorf("Attrs(form) = %v, want nil", got)
	}
}

func TestLinkRelations(t *testing.T) {
	resource := []string{"stylesheet", "STYLESHEET", "icon", "preload", "shortcut icon", ""}
	for _, rel := range resource {
		if !IsResourceLink(rel) {
			t.Errorf("IsResourceLink(%q) = false, want true", rel)
		}
	}

	for _, rel := range []string{"canonical", "alternate", "dns-prefetch", "next"} {
		if IsResourceLink(rel) {
			t.Errorf("IsResourceLink(%q) = true, want false", rel)
		}
	}

	if !IsStylesheet("alternate stylesheet") {
		t.Error("IsStylesheet(alternate stylesheet) = false")
	}
	if IsStylesheet("icon") {
		t.Error("IsStylesheet(icon) = true")
	}
}
