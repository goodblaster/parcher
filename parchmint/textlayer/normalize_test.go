package textlayer

import "testing"

func TestTokenizeNormalization(t *testing.T) {
	tests := []struct {
		name string
		text string
		want []string // normalized token forms, in order
	}{
		{"plain", "Hello World", []string{"hello", "world"}},
		{"punctuation dropped", "Hello, world!", []string{"hello", "world"}},
		{"accents folded (NFKD)", "café naïve Zürich", []string{"cafe", "naive", "zurich"}},
		{"curly quotes to straight", "don’t “quote”", []string{"don't", "quote"}},
		{"soft hyphen bridges word", "hyphen­ation", []string{"hyphenation"}},
		{"zero-width bridges word", "wo​rd", []string{"word"}},
		{"hyphen splits", "state-of-the-art", []string{"state", "of", "the", "art"}},
		{"apostrophe kept inside", "dogs' O'Brien", []string{"dogs", "o'brien"}},
		{"digits kept", "iPhone 15 Pro", []string{"iphone", "15", "pro"}},
		{"cjk one token per rune", "中文", []string{"中", "文"}},
		{"em dash separates", "a—b", []string{"a", "b"}},
		{"ligature expands", "ﬁre", []string{"fire"}}, // ﬁ -> fi
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			toks := Tokenize(tt.text, false)
			got := make([]string, len(toks))
			for i, tk := range toks {
				got[i] = tk.Norm
			}
			if len(got) != len(tt.want) {
				t.Fatalf("token count: got %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("token %d: got %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestTokenRawRangeProvenance(t *testing.T) {
	// "café" folds to "cafe" (4 units → 4 units, but é is one UTF-16 unit),
	// and a substring match on the folded form must map back to the exact
	// original characters.
	text := "iPhone" // folds to "iphone"; find "phone" -> raw offsets [1,6)
	toks := Tokenize(text, false)
	if len(toks) != 1 || toks[0].Norm != "iphone" {
		t.Fatalf("unexpected tokens: %+v", toks)
	}
	// byte range of "phone" inside "iphone" is [1,6)
	start, end := toks[0].RawRange(1, 6)
	if got := UTF16Slice(text, start, end); got != "Phone" {
		t.Errorf("RawRange mapped to %q, want %q", got, "Phone")
	}

	// Ligature: "ﬁre" folds to "fire" (ﬁ one source rune → two folded
	// bytes). Matching "re" (bytes [2,4)) must map back past the ligature.
	lig := "ﬁre"
	lt := Tokenize(lig, false)[0]
	if lt.Norm != "fire" {
		t.Fatalf("ligature norm %q", lt.Norm)
	}
	s, e := lt.RawRange(2, 4)
	if got := UTF16Slice(lig, s, e); got != "re" {
		t.Errorf("ligature RawRange -> %q, want %q", got, "re")
	}
}
