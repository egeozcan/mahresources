package plugin_system

import "testing"

func uintPtr(v uint) *uint { return &v }

// category_ids was parsed at registration, stored, and shipped to the browser,
// and then read by nothing: only note-type ids were ever enforced, while the
// docs said the field restricted availability.
func TestBlockTypeFilter_Allows(t *testing.T) {
	cases := []struct {
		name            string
		filter          BlockTypeFilter
		noteTypeID      *uint
		ownerCategoryID *uint
		want            bool
	}{
		{
			name: "empty filter admits everything",
			want: true,
		},
		{
			name:            "empty filter admits an untyped, unowned note",
			noteTypeID:      nil,
			ownerCategoryID: nil,
			want:            true,
		},
		{
			name:       "note type matches",
			filter:     BlockTypeFilter{NoteTypeIDs: []uint{2, 3}},
			noteTypeID: uintPtr(3),
			want:       true,
		},
		{
			name:       "note type does not match",
			filter:     BlockTypeFilter{NoteTypeIDs: []uint{2, 3}},
			noteTypeID: uintPtr(9),
			want:       false,
		},
		{
			name:       "an untyped note cannot satisfy a note-type filter",
			filter:     BlockTypeFilter{NoteTypeIDs: []uint{2}},
			noteTypeID: nil,
			want:       false,
		},
		{
			name:            "category matches",
			filter:          BlockTypeFilter{CategoryIDs: []uint{7}},
			ownerCategoryID: uintPtr(7),
			want:            true,
		},
		{
			name:            "category does not match",
			filter:          BlockTypeFilter{CategoryIDs: []uint{7}},
			ownerCategoryID: uintPtr(8),
			want:            false,
		},
		{
			name:            "an unowned note cannot satisfy a category filter",
			filter:          BlockTypeFilter{CategoryIDs: []uint{7}},
			ownerCategoryID: nil,
			want:            false,
		},
		{
			name:            "both filters are AND-joined: both match",
			filter:          BlockTypeFilter{NoteTypeIDs: []uint{2}, CategoryIDs: []uint{7}},
			noteTypeID:      uintPtr(2),
			ownerCategoryID: uintPtr(7),
			want:            true,
		},
		{
			name:            "both filters are AND-joined: category fails",
			filter:          BlockTypeFilter{NoteTypeIDs: []uint{2}, CategoryIDs: []uint{7}},
			noteTypeID:      uintPtr(2),
			ownerCategoryID: uintPtr(8),
			want:            false,
		},
		{
			name:            "both filters are AND-joined: note type fails",
			filter:          BlockTypeFilter{NoteTypeIDs: []uint{2}, CategoryIDs: []uint{7}},
			noteTypeID:      uintPtr(5),
			ownerCategoryID: uintPtr(7),
			want:            false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.filter.Allows(c.noteTypeID, c.ownerCategoryID); got != c.want {
				t.Errorf("Allows(%v, %v) = %v, want %v", c.noteTypeID, c.ownerCategoryID, got, c.want)
			}
		})
	}
}

// The registration side of the same field: category_ids must survive parsing,
// which it always did — it was the reading that was missing.
func TestBlockTypeFilter_CategoryIDsParsed(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "filtered", `
plugin = { name = "filtered", version = "1.0", description = "block filters" }
function init()
    mah.block_type({
        type = "recipe",
        label = "Recipe",
        filters = { category_ids = {7, 9}, note_type_ids = {2} },
        render_view = function(ctx) return "<p>view</p>" end,
        render_edit = function(ctx) return "<p>edit</p>" end,
    })
end
`)
	mgr, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close()
	if err := mgr.EnablePlugin("filtered"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}

	bt := mgr.GetPluginBlockType("plugin:filtered:recipe")
	if bt == nil {
		t.Fatal("block type not registered")
	}
	if len(bt.Filters.CategoryIDs) != 2 || bt.Filters.CategoryIDs[0] != 7 {
		t.Errorf("category_ids = %v, want [7 9]", bt.Filters.CategoryIDs)
	}
	if !bt.Filters.Allows(uintPtr(2), uintPtr(9)) {
		t.Error("a note of type 2 in category 9 should be allowed")
	}
	if bt.Filters.Allows(uintPtr(2), uintPtr(1)) {
		t.Error("a note in category 1 should not be allowed")
	}
}

// A filter states where a block type may be used, so a fractional id quietly
// naming category 2 offers it somewhere the author did not choose.
func TestBlockTypeFilter_RejectsNonWholeFilterIDs(t *testing.T) {
	for _, bad := range []string{`{2.9}`, `{"nope"}`, `{0}`, `{-1}`} {
		dir := t.TempDir()
		writePlugin(t, dir, "badfilter", `
plugin = { name = "badfilter", version = "1.0", description = "bad filters" }
function init()
    mah.block_type({
        type = "thing",
        label = "Thing",
        filters = { category_ids = `+bad+` },
        render_view = function(ctx) return "v" end,
        render_edit = function(ctx) return "e" end,
    })
end
`)
		mgr, err := NewPluginManager(dir)
		if err != nil {
			t.Fatal(err)
		}
		if err := mgr.EnablePlugin("badfilter"); err == nil {
			t.Errorf("category_ids = %s should be rejected", bad)
		}
		mgr.Close()
	}
}
