package plugin_system

import (
	"fmt"
	"testing"
)

// failingQuerier fails every read. It exists to prove that a plugin can tell a
// database failure from an empty result — before this, both arrived in Lua as a
// bare nil (or a 0 from count_*), so a plugin branching on "no rows" took the
// empty-data branch during an outage.
type failingQuerier struct {
	mockQuerier
}

var errBoom = fmt.Errorf("database is down")

func (f *failingQuerier) GetNoteData(id uint) (map[string]any, error)  { return nil, errBoom }
func (f *failingQuerier) GetResourceData(id uint) (map[string]any, error) {
	return nil, errBoom
}
func (f *failingQuerier) GetGroupData(id uint) (map[string]any, error) { return nil, errBoom }
func (f *failingQuerier) GetTagData(id uint) (map[string]any, error)   { return nil, errBoom }
func (f *failingQuerier) GetCategoryData(id uint) (map[string]any, error) {
	return nil, errBoom
}
func (f *failingQuerier) QueryNotes(filter map[string]any) ([]map[string]any, error) {
	return nil, errBoom
}
func (f *failingQuerier) QueryResources(filter map[string]any) ([]map[string]any, error) {
	return nil, errBoom
}
func (f *failingQuerier) QueryGroups(filter map[string]any) ([]map[string]any, error) {
	return nil, errBoom
}
func (f *failingQuerier) CountNotes(filter map[string]any) (int64, error) { return 0, errBoom }
func (f *failingQuerier) CountResources(filter map[string]any) (int64, error) {
	return 0, errBoom
}
func (f *failingQuerier) CountGroups(filter map[string]any) (int64, error) { return 0, errBoom }
func (f *failingQuerier) ListTags(filter map[string]any) ([]map[string]any, error) {
	return nil, errBoom
}

// stubWriter satisfies EntityWriter without doing anything. Tests embed it and
// override only the writers they exercise.
type stubWriter struct{}

var errStubWriter = fmt.Errorf("not implemented in stubWriter")

func (stubWriter) CreateGroup(map[string]any) (map[string]any, error) { return nil, errStubWriter }
func (stubWriter) UpdateGroup(uint, map[string]any) (map[string]any, error) {
	return nil, errStubWriter
}
func (stubWriter) PatchGroup(uint, map[string]any) (map[string]any, error) {
	return nil, errStubWriter
}
func (stubWriter) DeleteGroup(uint) error                            { return errStubWriter }
func (stubWriter) CreateNote(map[string]any) (map[string]any, error) { return nil, errStubWriter }
func (stubWriter) UpdateNote(uint, map[string]any) (map[string]any, error) {
	return nil, errStubWriter
}
func (stubWriter) PatchNote(uint, map[string]any) (map[string]any, error) {
	return nil, errStubWriter
}
func (stubWriter) DeleteNote(uint) error                            { return errStubWriter }
func (stubWriter) CreateTag(map[string]any) (map[string]any, error) { return nil, errStubWriter }
func (stubWriter) UpdateTag(uint, map[string]any) (map[string]any, error) {
	return nil, errStubWriter
}
func (stubWriter) PatchTag(uint, map[string]any) (map[string]any, error) { return nil, errStubWriter }
func (stubWriter) DeleteTag(uint) error                                  { return errStubWriter }
func (stubWriter) CreateCategory(map[string]any) (map[string]any, error) {
	return nil, errStubWriter
}
func (stubWriter) UpdateCategory(uint, map[string]any) (map[string]any, error) {
	return nil, errStubWriter
}
func (stubWriter) PatchCategory(uint, map[string]any) (map[string]any, error) {
	return nil, errStubWriter
}
func (stubWriter) DeleteCategory(uint) error { return errStubWriter }
func (stubWriter) CreateResourceCategory(map[string]any) (map[string]any, error) {
	return nil, errStubWriter
}
func (stubWriter) UpdateResourceCategory(uint, map[string]any) (map[string]any, error) {
	return nil, errStubWriter
}
func (stubWriter) PatchResourceCategory(uint, map[string]any) (map[string]any, error) {
	return nil, errStubWriter
}
func (stubWriter) DeleteResourceCategory(uint) error { return errStubWriter }
func (stubWriter) CreateNoteType(map[string]any) (map[string]any, error) {
	return nil, errStubWriter
}
func (stubWriter) UpdateNoteType(uint, map[string]any) (map[string]any, error) {
	return nil, errStubWriter
}
func (stubWriter) PatchNoteType(uint, map[string]any) (map[string]any, error) {
	return nil, errStubWriter
}
func (stubWriter) DeleteNoteType(uint) error { return errStubWriter }
func (stubWriter) CreateGroupRelation(map[string]any) (map[string]any, error) {
	return nil, errStubWriter
}
func (stubWriter) UpdateGroupRelation(map[string]any) (map[string]any, error) {
	return nil, errStubWriter
}
func (stubWriter) PatchGroupRelation(map[string]any) (map[string]any, error) {
	return nil, errStubWriter
}
func (stubWriter) DeleteGroupRelation(uint) error { return errStubWriter }
func (stubWriter) CreateRelationType(map[string]any) (map[string]any, error) {
	return nil, errStubWriter
}
func (stubWriter) UpdateRelationType(map[string]any) (map[string]any, error) {
	return nil, errStubWriter
}
func (stubWriter) PatchRelationType(map[string]any) (map[string]any, error) {
	return nil, errStubWriter
}
func (stubWriter) DeleteRelationType(uint) error                        { return errStubWriter }
func (stubWriter) AddTagsToEntity(string, uint, []uint) error           { return errStubWriter }
func (stubWriter) RemoveTagsFromEntity(string, uint, []uint) error      { return errStubWriter }
func (stubWriter) AddGroupsToEntity(string, uint, []uint) error         { return errStubWriter }
func (stubWriter) RemoveGroupsFromEntity(string, uint, []uint) error    { return errStubWriter }
func (stubWriter) AddResourcesToNote(uint, []uint) error                { return errStubWriter }
func (stubWriter) RemoveResourcesFromNote(uint, []uint) error           { return errStubWriter }
func (stubWriter) UpdateResource(uint, map[string]any) (map[string]any, error) {
	return nil, errStubWriter
}
func (stubWriter) PatchResource(uint, map[string]any) (map[string]any, error) {
	return nil, errStubWriter
}
func (stubWriter) DeleteResource(uint) error { return errStubWriter }

// mockWriterQuerier is a querier that can also write. Resource 1 stands in for
// a stored row so patch can be shown to preserve what it was not asked to
// change.
type mockWriterQuerier struct {
	mockQuerier
	stubWriter
}

func (m *mockWriterQuerier) UpdateResource(id uint, opts map[string]any) (map[string]any, error) {
	if id != 1 {
		return nil, fmt.Errorf("resource not found")
	}
	// Replace-all: every field comes from opts, absent ones end up empty.
	return map[string]any{
		"id":          float64(id),
		"name":        opts["name"],
		"description": opts["description"],
	}, nil
}

func (m *mockWriterQuerier) PatchResource(id uint, opts map[string]any) (map[string]any, error) {
	if id != 1 {
		return nil, fmt.Errorf("resource not found")
	}
	stored := map[string]any{"id": float64(id), "name": "original.jpg", "description": "original desc"}
	for k, v := range opts {
		stored[k] = v
	}
	return stored, nil
}

// renderWithQuerier boots a one-plugin manager whose "test" slot body is the
// given Lua, and returns what the slot rendered.
func renderWithQuerier(t *testing.T, q EntityQuerier, body string) string {
	t.Helper()
	dir := t.TempDir()
	writePlugin(t, dir, "db-ext", `
plugin = { name = "db-ext", version = "1.0", description = "db api extensions" }
function init()
    mah.inject("test", function(ctx)
`+body+`
    end)
end
`)
	mgr, err := NewPluginManager(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mgr.Close)
	mgr.SetEntityQuerier(q)
	if w, ok := q.(EntityWriter); ok {
		mgr.SetEntityWriter(w)
	}
	if err := mgr.EnablePlugin("db-ext"); err != nil {
		t.Fatalf("EnablePlugin: %v", err)
	}
	return mgr.RenderSlot("test", map[string]any{})
}

// --- Item 06: reads report failure instead of looking empty ---

func TestDbApi_GettersReturnErrorOnFailure(t *testing.T) {
	cases := []struct{ name, call string }{
		{"get_note", "mah.db.get_note(1)"},
		{"get_resource", "mah.db.get_resource(1)"},
		{"get_group", "mah.db.get_group(1)"},
		{"get_tag", "mah.db.get_tag(1)"},
		{"get_category", "mah.db.get_category(1)"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := renderWithQuerier(t, &failingQuerier{}, `
        local v, err = `+c.call+`
        if v ~= nil then return "unexpected value" end
        if err == nil then return "NO ERROR" end
        return "err:" .. err
`)
			if got != "err:database is down" {
				t.Errorf("%s: got %q, want %q", c.name, got, "err:database is down")
			}
		})
	}
}

func TestDbApi_QueriesReturnErrorOnFailure(t *testing.T) {
	cases := []struct{ name, call string }{
		{"query_notes", "mah.db.query_notes({})"},
		{"query_resources", "mah.db.query_resources({})"},
		{"query_groups", "mah.db.query_groups({})"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := renderWithQuerier(t, &failingQuerier{}, `
        local v, err = `+c.call+`
        if v ~= nil then return "unexpected value" end
        if err == nil then return "NO ERROR" end
        return "err:" .. err
`)
			if got != "err:database is down" {
				t.Errorf("%s: got %q, want %q", c.name, got, "err:database is down")
			}
		})
	}
}

// A count that fails must not answer 0. That is the destructive case: a plugin
// that reads "0 resources" during an outage and then archives or re-uploads.
func TestDbApi_CountsReturnErrorOnFailure(t *testing.T) {
	cases := []struct{ name, call string }{
		{"count_notes", "mah.db.count_notes({})"},
		{"count_resources", "mah.db.count_resources({})"},
		{"count_groups", "mah.db.count_groups({})"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := renderWithQuerier(t, &failingQuerier{}, `
        local n, err = `+c.call+`
        if n ~= nil then return "got a count: " .. tostring(n) end
        if err == nil then return "NO ERROR" end
        return "err:" .. err
`)
			if got != "err:database is down" {
				t.Errorf("%s: got %q, want %q", c.name, got, "err:database is down")
			}
		})
	}
}

// The single-value call shape every bundled plugin uses must keep working.
func TestDbApi_ReadsStayBackwardCompatible(t *testing.T) {
	got := renderWithQuerier(t, &mockQuerier{}, `
        local note = mah.db.get_note(1)
        local notes = mah.db.query_notes({})
        local n = mah.db.count_notes({})
        return note.name .. "|" .. #notes .. "|" .. n
`)
	if got != "Test Note|2|0" {
		t.Errorf("got %q, want %q", got, "Test Note|2|0")
	}
}

// --- Item 03: taxonomy list and get ---

func TestDbApi_ListTaxonomy(t *testing.T) {
	cases := []struct{ name, call, want string }{
		{"list_tags", "mah.db.list_tags({})", "tag-a,tag-b"},
		{"list_categories", "mah.db.list_categories({})", "Cat A"},
		{"list_note_types", "mah.db.list_note_types({})", "Meeting"},
		{"list_resource_categories", "mah.db.list_resource_categories({})", "Photos"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := renderWithQuerier(t, &mockQuerier{}, `
        local items, err = `+c.call+`
        if err ~= nil then return "err:" .. err end
        if items == nil then return "nil" end
        local names = {}
        for i, it in ipairs(items) do names[i] = it.name end
        return table.concat(names, ",")
`)
			if got != c.want {
				t.Errorf("%s: got %q, want %q", c.name, got, c.want)
			}
		})
	}
}

// The find-or-create-a-tag pattern: look a tag up by name, and only create it
// when the lookup comes back empty rather than failed.
func TestDbApi_ListTagsByName(t *testing.T) {
	got := renderWithQuerier(t, &mockQuerier{}, `
        local items, err = mah.db.list_tags({ name = "tag-a" })
        if err ~= nil then return "err:" .. err end
        return tostring(#items) .. ":" .. items[1].name
`)
	if got != "1:tag-a" {
		t.Errorf("got %q, want %q", got, "1:tag-a")
	}
}

func TestDbApi_ListTagsReportsFailure(t *testing.T) {
	got := renderWithQuerier(t, &failingQuerier{}, `
        local items, err = mah.db.list_tags({})
        if items ~= nil then return "unexpected value" end
        if err == nil then return "NO ERROR" end
        return "err:" .. err
`)
	if got != "err:database is down" {
		t.Errorf("got %q, want %q", got, "err:database is down")
	}
}

func TestDbApi_GetNoteTypeAndResourceCategory(t *testing.T) {
	got := renderWithQuerier(t, &mockQuerier{}, `
        local nt = mah.db.get_note_type(1)
        local rc = mah.db.get_resource_category(1)
        return nt.name .. "|" .. rc.name
`)
	if got != "Meeting|Photos" {
		t.Errorf("got %q, want %q", got, "Meeting|Photos")
	}
}

// --- Item 04: update_resource / patch_resource ---

func TestDbApi_UpdateResource(t *testing.T) {
	got := renderWithQuerier(t, &mockWriterQuerier{}, `
        local r, err = mah.db.update_resource(1, { name = "renamed.jpg", description = "new" })
        if err ~= nil then return "err:" .. err end
        return r.name .. "|" .. r.description
`)
	if got != "renamed.jpg|new" {
		t.Errorf("got %q, want %q", got, "renamed.jpg|new")
	}
}

// patch must leave unmentioned fields alone — the whole reason it ships
// alongside update, which replaces everything including associations.
func TestDbApi_PatchResource(t *testing.T) {
	got := renderWithQuerier(t, &mockWriterQuerier{}, `
        local r, err = mah.db.patch_resource(1, { description = "only this" })
        if err ~= nil then return "err:" .. err end
        return r.name .. "|" .. r.description
`)
	if got != "original.jpg|only this" {
		t.Errorf("got %q, want %q", got, "original.jpg|only this")
	}
}

func TestDbApi_UpdateResourceReportsFailure(t *testing.T) {
	got := renderWithQuerier(t, &mockWriterQuerier{}, `
        local r, err = mah.db.update_resource(999, { name = "x" })
        if r ~= nil then return "unexpected value" end
        if err == nil then return "NO ERROR" end
        return "err:" .. err
`)
	if got != "err:resource not found" {
		t.Errorf("got %q, want %q", got, "err:resource not found")
	}
}
