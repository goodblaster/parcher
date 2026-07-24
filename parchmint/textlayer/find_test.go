package textlayer

import "testing"

// matchStrings runs a query against one block's text and returns the
// matched substrings.
func matchStrings(t *testing.T, query, text string) []string {
	t.Helper()
	q, err := ParseQuery(query)
	if err != nil {
		t.Fatalf("ParseQuery(%q): %v", query, err)
	}
	b := &Block{Text: text}
	var out []string
	for _, h := range q.FindBlock(b) {
		out = append(out, h.Text())
	}
	return out
}

func TestFindBlock(t *testing.T) {
	tests := []struct {
		name  string
		query string
		text  string
		want  []string
	}{
		{"substring inside word", "phone", "Buy the new iPhone today", []string{"Phone"}},
		{"whole word", "apple", "an Apple a day", []string{"Apple"}},
		{"two consecutive words", "brown fox", "the quick brown fox ran", []string{"brown fox"}},
		{"case and accent insensitive", "cafe", "at the CAFÉ", []string{"CAFÉ"}},
		{"punctuation between", "hello world", "Hello, world!", []string{"Hello, world"}},
		{"star bridges words", "apple*card", "an Apple Gift Card here", []string{"Apple Gift Card"}},
		{"star spaced bridges words", "apple * card", "an Apple Gift Card here", []string{"Apple Gift Card"}},
		{"star within word", "trans*ion", "the transaction posted", []string{"transaction"}},
		{"br newline not a barrier", "world intro", "hello world\nintro run", []string{"world\nintro"}},
		{"multiple hits", "the", "the cat and the dog", []string{"the", "the"}},
		{"substring-precise edges", "brown fox jumps", "the brown foxes jumped", nil}, // "jumps" is not a substring of "jumped"
		{"cjk phrase", "中文", "这是中文测试", []string{"中文"}},
		{"no match", "zebra", "the quick brown fox", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchStrings(t, tt.query, tt.text)
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("hit %d: got %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestFindBlockSubstringEdges(t *testing.T) {
	// "phone" must highlight only the matched characters, not the whole word.
	q, _ := ParseQuery("phone")
	b := &Block{Text: "an iPhone"}
	hits := q.FindBlock(b)
	if len(hits) != 1 {
		t.Fatalf("got %d hits", len(hits))
	}
	if hits[0].Text() != "Phone" {
		t.Errorf("hit text %q, want %q", hits[0].Text(), "Phone")
	}
}

func TestMaxGap(t *testing.T) {
	// `*` bridges up to maxGap words; beyond that, no match.
	within := "a x x x x b"         // 4 words between a and b (<= maxGap 5)
	beyond := "a x x x x x x x x b" // 8 words between (> maxGap)
	if got := matchStrings(t, "a*b", within); len(got) != 1 {
		t.Errorf("within gap: got %v, want one hit", got)
	}
	if got := matchStrings(t, "a*b", beyond); got != nil {
		t.Errorf("beyond gap: got %v, want none", got)
	}
}

func TestMergeHitRanges(t *testing.T) {
	b := &Block{ID: 7}
	hits := []Hit{
		{Block: b, Start: 0, End: 5},
		{Block: b, Start: 3, End: 8},   // overlaps previous -> merge to [0,8]
		{Block: b, Start: 8, End: 10},  // adjacent -> merge to [0,10]
		{Block: b, Start: 20, End: 25}, // separate
	}
	got := MergeHitRanges(hits)
	want := [][2]int{{0, 10}, {20, 25}}
	ranges := got[7]
	if len(ranges) != len(want) {
		t.Fatalf("got %v, want %v", ranges, want)
	}
	for i := range want {
		if ranges[i] != want[i] {
			t.Errorf("range %d: got %v, want %v", i, ranges[i], want[i])
		}
	}
}

func TestHitGeometry(t *testing.T) {
	// A block with two words on one line; a two-word hit's box is their union.
	b := &Block{
		Text: "brown fox",
		Words: []Word{
			{Start: 0, End: 5, Box: Box{10, 20, 40, 12}},
			{Start: 6, End: 9, Box: Box{55, 20, 25, 12}},
		},
	}
	q, _ := ParseQuery("brown fox")
	h := q.FindBlock(b)[0]
	if len(h.Words()) != 2 {
		t.Fatalf("expected 2 words, got %d", len(h.Words()))
	}
	// union: x 10..80, y 20..32
	if got := h.Box(); got != (Box{10, 20, 70, 12}) {
		t.Errorf("box %v, want {10 20 70 12}", got)
	}
}

func TestHitContext(t *testing.T) {
	b := &Block{Text: "the quick brown fox jumps over the lazy dog"}
	q, _ := ParseQuery("brown fox")
	h := q.FindBlock(b)[0]
	ctx := h.Context(6, "«", "»")
	want := "…quick «brown fox» jumps…" // ellipses because there is more text each side
	if ctx != want {
		t.Errorf("context %q, want %q", ctx, want)
	}
}

func TestCrossBlockNoMatch(t *testing.T) {
	// A phrase never bridges two blocks (separate paragraphs).
	layer := &Layer{Blocks: []Block{
		{ID: 0, Text: "hello world"},
		{ID: 1, Text: "intro run"},
	}}
	q, _ := ParseQuery("world intro")
	if hits := q.Find(layer); hits != nil {
		t.Errorf("phrase should not bridge blocks; got %d hits", len(hits))
	}
}

func TestParseQueryEmpty(t *testing.T) {
	if _, err := ParseQuery("   !!!  "); err == nil {
		t.Error("expected error for query with no searchable tokens")
	}
}
