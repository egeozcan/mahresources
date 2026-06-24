package models

import "testing"

// T11: HasTextBlock drives whether the note Description is rendered directly.
func TestNote_HasTextBlock(t *testing.T) {
	cases := []struct {
		name   string
		blocks []*NoteBlock
		want   bool
	}{
		{"no blocks", nil, false},
		{"only non-text blocks", []*NoteBlock{{Type: "divider"}, {Type: "gallery"}}, false},
		{"has a text block", []*NoteBlock{{Type: "gallery"}, {Type: "text"}}, true},
		{"nil entry ignored", []*NoteBlock{nil, {Type: "heading"}}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			n := Note{Blocks: c.blocks}
			if got := n.HasTextBlock(); got != c.want {
				t.Fatalf("HasTextBlock() = %v, want %v", got, c.want)
			}
		})
	}
}
