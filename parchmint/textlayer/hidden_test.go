package textlayer

import (
	"encoding/json"
	"testing"
)

func hiddenLayer() *Layer {
	return &Layer{
		Format: Format,
		Blocks: []Block{{ID: 0, Type: "p", Text: "Returns are easy."}},
		Hidden: []Block{
			{ID: 1, Type: "p", Text: "Returns accepted within thirty days.", Why: HiddenCollapsed},
			{ID: 2, Type: "p", Text: "Returns tab two.", Why: HiddenDisplay},
			{ID: 3, Type: "p", Text: "Returns newsletter popup.", Why: HiddenPrep},
		},
	}
}

func hitIDs(t *testing.T, opts MatchOptions) []int {
	t.Helper()
	m, err := Compile("returns", opts)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	var ids []int
	for _, h := range FindAll(m, hiddenLayer()) {
		ids = append(ids, h.Block.ID)
	}
	return ids
}

func TestHiddenMatching(t *testing.T) {
	cases := []struct {
		name string
		opts MatchOptions
		want []int
	}{
		{"default is visible only", MatchOptions{}, []int{0}},
		{"all excludes prep", MatchOptions{Hidden: []string{HiddenAll}}, []int{0, 1, 2}},
		{"one reason", MatchOptions{Hidden: []string{HiddenCollapsed}}, []int{0, 1}},
		{"prep only when named", MatchOptions{Hidden: []string{HiddenPrep}}, []int{0, 3}},
		{"hidden only", MatchOptions{HiddenOnly: true}, []int{1, 2}},
		{"hidden only, one reason", MatchOptions{Hidden: []string{HiddenDisplay}, HiddenOnly: true}, []int{2}},
		{"composes with a mode", MatchOptions{Regex: true, IgnoreCase: true, Hidden: []string{HiddenAll}}, []int{0, 1, 2}},
		{"composes with a type scope", MatchOptions{InTypes: []string{"h1"}, Hidden: []string{HiddenAll}}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := hitIDs(t, c.opts)
			if len(got) != len(c.want) {
				t.Fatalf("hit blocks = %v, want %v", got, c.want)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Fatalf("hit blocks = %v, want %v", got, c.want)
				}
			}
		})
	}

	if _, err := Compile("x", MatchOptions{Hidden: []string{"sideways"}}); err == nil {
		t.Error("unknown hidden reason: want an error")
	}
}

// The hidden array is a separate top-level field precisely so that a reader
// built before it existed keeps seeing visible text only.
func TestHiddenInvisibleToOldReaders(t *testing.T) {
	raw, err := hiddenLayer().Marshal()
	if err != nil {
		t.Fatal(err)
	}
	var old struct {
		Blocks []struct {
			Text string `json:"text"`
		} `json:"blocks"`
	}
	if err := json.Unmarshal(raw, &old); err != nil {
		t.Fatal(err)
	}
	if len(old.Blocks) != 1 || old.Blocks[0].Text != "Returns are easy." {
		t.Fatalf("an old reader saw %+v", old.Blocks)
	}

	// And it round-trips through both document containers.
	html := EmbedInHTML([]byte("<html><body></body></html>"), raw)
	back, err := FromBytes(html, "t.html")
	if err != nil {
		t.Fatal(err)
	}
	if len(back.Hidden) != 3 || back.Hidden[0].Why != HiddenCollapsed {
		t.Fatalf("hidden blocks did not survive the HTML round trip: %+v", back.Hidden)
	}

	// A layer with nothing hidden omits the field entirely.
	plain, _ := (&Layer{Format: Format, Blocks: []Block{}}).Marshal()
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(plain, &fields)
	if _, present := fields["hidden"]; present {
		t.Error(`"hidden" should be omitted when empty`)
	}
}
