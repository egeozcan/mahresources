package plugin_system

import (
	"fmt"
	"testing"
	"time"

	lua "github.com/yuin/gopher-lua"
)

// recordingWriter remembers the ids it was asked to delete, so a truncated id
// can be shown to reach the writer as the wrong row.
type recordingWriter struct {
	mockQuerier
	stubWriter
	deleted []uint
}

func (r *recordingWriter) DeleteResource(id uint) error {
	r.deleted = append(r.deleted, id)
	return nil
}

func (r *recordingWriter) UpdateResource(id uint, opts map[string]any) (map[string]any, error) {
	return map[string]any{"id": float64(id)}, nil
}

// Lua has a single number type, so an id can arrive fractional — from a
// division, or a JSON round-trip. uint() truncates it silently, and
// delete_resource(1.9) deleting resource 1 is not a rounding error, it is the
// wrong row.
func TestDbApi_RejectsFractionalEntityID(t *testing.T) {
	w := &recordingWriter{}
	got := renderWithQuerier(t, w, `
        local ok, err = pcall(function() return mah.db.delete_resource(1.9) end)
        if ok then return "ACCEPTED" end
        return "rejected"
`)
	if got != "rejected" {
		t.Errorf("got %q, want a rejection", got)
	}
	if len(w.deleted) != 0 {
		t.Errorf("nothing should have been deleted, got %v", w.deleted)
	}
}

func TestDbApi_RejectsNonPositiveEntityID(t *testing.T) {
	for _, id := range []string{"0", "-1"} {
		w := &recordingWriter{}
		got := renderWithQuerier(t, w, `
        local ok = pcall(function() return mah.db.delete_resource(`+id+`) end)
        if ok then return "ACCEPTED" end
        return "rejected"
`)
		if got != "rejected" {
			t.Errorf("id %s: got %q, want a rejection", id, got)
		}
		if len(w.deleted) != 0 {
			t.Errorf("id %s: nothing should have been deleted, got %v", id, w.deleted)
		}
	}
}

// A whole-number id still works, including one that arrives as a float.
func TestDbApi_AcceptsWholeEntityID(t *testing.T) {
	w := &recordingWriter{}
	got := renderWithQuerier(t, w, `
        local ok, err = mah.db.delete_resource(4 / 2)
        if not ok then return "err:" .. tostring(err) end
        return "deleted"
`)
	if got != "deleted" {
		t.Errorf("got %q, want %q", got, "deleted")
	}
	if len(w.deleted) != 1 || w.deleted[0] != 2 {
		t.Errorf("expected resource 2 deleted, got %v", w.deleted)
	}
}

// The getters guard their id the same way.
func TestDbApi_GettersRejectFractionalID(t *testing.T) {
	got := renderWithQuerier(t, &mockQuerier{}, `
        local ok = pcall(function() return mah.db.get_note(2.5) end)
        if ok then return "ACCEPTED" end
        return "rejected"
`)
	if got != "rejected" {
		t.Errorf("got %q, want a rejection", got)
	}
}

// An id embedded in an options table is as load-bearing as a positional one,
// and every way of not honouring a fractional value is silently wrong:
// truncating picks the wrong group, treating it as absent clears the owner or
// widens a filter. Raise instead.
func TestDbApi_RejectsFractionalEmbeddedID(t *testing.T) {
	cases := []struct{ name, call string }{
		{"create owner_id", `mah.db.create_note({ name = "x", owner_id = 2.9 })`},
		{"update owner_id", `mah.db.update_resource(1, { owner_id = 2.9 })`},
		{"patch owner_id", `mah.db.patch_resource(1, { owner_id = 2.9 })`},
		{"tag list", `mah.db.patch_resource(1, { tags = {2.9} })`},
		{"query filter", `mah.db.query_notes({ owner_id = 2.9 })`},
		{"count filter", `mah.db.count_notes({ owner_id = 2.9 })`},
		{"add_tags list", `mah.db.add_tags("resource", 1, {2.9})`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := renderWithQuerier(t, &mockWriterQuerier{}, `
        local ok = pcall(function() return `+c.call+` end)
        if ok then return "ACCEPTED" end
        return "rejected"
`)
			if got != "rejected" {
				t.Errorf("%s: got %q, want a rejection", c.call, got)
			}
		})
	}
}

// A whole-number embedded id still goes through.
func TestDbApi_AcceptsWholeEmbeddedID(t *testing.T) {
	got := renderWithQuerier(t, &mockWriterQuerier{}, `
        local r, err = mah.db.update_resource(1, { name = "ok", owner_id = 6 / 2 })
        if err ~= nil then return "err:" .. err end
        return "updated"
`)
	if got != "updated" {
		t.Errorf("got %q, want %q", got, "updated")
	}
}

// A Lua table can point at itself. Following it recursively never terminates
// and overflows the Go stack, which kills the process rather than the plugin.
func TestLuaConversion_SurvivesACyclicTable(t *testing.T) {
	done := make(chan map[string]any, 1)
	go func() {
		L := lua.NewState()
		defer L.Close()
		tbl := L.NewTable()
		tbl.RawSetString("name", lua.LString("cyclic"))
		tbl.RawSetString("self", tbl)
		done <- luaTableToGoMap(tbl)
	}()

	select {
	case result := <-done:
		if result["name"] != "cyclic" {
			t.Errorf("the shallow fields should still convert, got %v", result)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("converting a cyclic table did not terminate")
	}
}

// A non-numeric value where an id belongs is not "absent": the adapter reads it
// as 0, which clears an owner or empties an association list.
func TestDbApi_RejectsNonNumericEmbeddedID(t *testing.T) {
	cases := []struct{ name, call string }{
		{"scalar id", `mah.db.update_resource(1, { name = "x", owner_id = "bad" })`},
		{"list member", `mah.db.patch_resource(1, { tags = {"bad"} })`},
		{"relationship list", `mah.db.add_tags("resource", 1, {"bad"})`},
		{"create from data", `mah.db.create_resource_from_data("eA==", { owner_id = 2.9 })`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := renderWithQuerier(t, &mockWriterQuerier{}, `
        local ok = pcall(function() return `+c.call+` end)
        if ok then return "ACCEPTED" end
        return "rejected"
`)
			if got != "rejected" {
				t.Errorf("%s: got %q, want a rejection", c.call, got)
			}
		})
	}
}

// A cycle is not the only way to make the walk explode: two self-references
// double the work at every level, so a depth cap alone still exhausts memory.
// Cycle detection stops it at the first repeat.
func TestLuaConversion_SurvivesAMultiCycleTable(t *testing.T) {
	done := make(chan map[string]any, 1)
	go func() {
		L := lua.NewState()
		defer L.Close()
		tbl := L.NewTable()
		tbl.RawSetString("name", lua.LString("multi"))
		tbl.RawSetString("a", tbl)
		tbl.RawSetString("b", tbl)
		done <- luaTableToGoMap(tbl)
	}()

	select {
	case result := <-done:
		if result["name"] != "multi" {
			t.Errorf("the shallow fields should still convert, got %v", result)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("converting a multi-cycle table did not terminate")
	}
}

// Deep but acyclic data must survive: a nested meta object is legitimate, and
// dropping its leaf silently corrupts what the plugin sent.
func TestLuaConversion_KeepsDeepAcyclicData(t *testing.T) {
	L := lua.NewState()
	defer L.Close()

	leaf := L.NewTable()
	leaf.RawSetString("leaf", lua.LString("ok"))
	current := leaf
	for i := 0; i < 40; i++ {
		wrapper := L.NewTable()
		wrapper.RawSetString("child", current)
		current = wrapper
	}

	result := luaTableToGoMap(current)
	for i := 0; i < 40; i++ {
		next, ok := result["child"].(map[string]any)
		if !ok {
			t.Fatalf("level %d: expected a nested map, got %T", i, result["child"])
		}
		result = next
	}
	if result["leaf"] != "ok" {
		t.Errorf("the leaf of 40 levels of acyclic nesting should survive, got %v", result)
	}
}

// A value of the wrong shape where an id list belongs is not "no ids": the
// adapter reads it as an empty list and clears the association.
func TestDbApi_RejectsMalformedIDList(t *testing.T) {
	got := renderWithQuerier(t, &mockWriterQuerier{}, `
        local ok = pcall(function() return mah.db.update_resource(1, { name = "x", tags = "bad" }) end)
        if ok then return "ACCEPTED" end
        return "rejected"
`)
	if got != "rejected" {
		t.Errorf("got %q, want a rejection", got)
	}
}

// scope_entity_id names an entity like any other id, and truncating it scopes
// the query to a different group's subtree.
func TestDbApi_RejectsFractionalScopeEntityID(t *testing.T) {
	got := renderWithQuerier(t, &mockWriterQuerier{}, `
        local ok = pcall(function()
            return mah.db.mrql_query("type=resource", { scope = "entity", entity_type = "group", scope_entity_id = 2.9 })
        end)
        if ok then return "ACCEPTED" end
        return "rejected"
`)
	if got != "rejected" {
		t.Errorf("got %q, want a rejection", got)
	}
}

// The budget counts values emitted, not tables visited: a table shared by both
// branches at each level is only a few dozen distinct tables, but re-expanding
// a fat leaf under every path emits tens of thousands of copies of it.
func TestLuaConversion_BoundsASharedDAG(t *testing.T) {
	done := make(chan int, 1)
	go func() {
		L := lua.NewState()
		defer L.Close()
		leaf := L.NewTable()
		for i := 0; i < 2000; i++ {
			leaf.RawSetString(fmt.Sprintf("k%d", i), lua.LNumber(i))
		}
		current := leaf
		for i := 0; i < 17; i++ {
			wrapper := L.NewTable()
			wrapper.RawSetString("a", current)
			wrapper.RawSetString("b", current)
			current = wrapper
		}
		done <- countValues(luaTableToGoMap(current))
	}()

	select {
	case emitted := <-done:
		if emitted > maxLuaConversionNodes+1000 {
			t.Errorf("emitted %d values, want the budget to hold it near %d", emitted, maxLuaConversionNodes)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("converting a shared DAG did not terminate in time")
	}
}

func countValues(v any) int {
	switch val := v.(type) {
	case map[string]any:
		total := 0
		for _, item := range val {
			total += 1 + countValues(item)
		}
		return total
	case []any:
		total := 0
		for _, item := range val {
			total += 1 + countValues(item)
		}
		return total
	default:
		return 0
	}
}

// A table keyed by anything but consecutive integers is not an id list: the Go
// conversion drops what it cannot use, and the writer reads the result as "no
// ids" and clears the association.
func TestDbApi_RejectsNonArrayIDList(t *testing.T) {
	for _, bad := range []string{`{[true] = 7}`, `{named = 7}`, `{[2] = 7}`} {
		got := renderWithQuerier(t, &mockWriterQuerier{}, `
        local ok = pcall(function() return mah.db.patch_resource(1, { tags = `+bad+` }) end)
        if ok then return "ACCEPTED" end
        return "rejected"
`)
		if got != "rejected" {
			t.Errorf("tags = %s: got %q, want a rejection", bad, got)
		}
	}
}

// 2^53 itself cannot be trusted: 2^53+1 is not representable and arrives as
// 2^53, so accepting it would let one specific unrepresentable id through as a
// different row.
func TestDbApi_RejectsIDsAtTheRepresentableBoundary(t *testing.T) {
	w := &recordingWriter{}
	got := renderWithQuerier(t, w, `
        local ok = pcall(function() return mah.db.delete_resource(9007199254740992) end)
        if ok then return "ACCEPTED" end
        return "rejected"
`)
	if got != "rejected" {
		t.Errorf("2^53 should be rejected, got %q", got)
	}
	if len(w.deleted) != 0 {
		t.Errorf("nothing should have been deleted, got %v", w.deleted)
	}

	// One below the boundary is exact, and allowed.
	w2 := &recordingWriter{}
	got = renderWithQuerier(t, w2, `
        local ok, err = mah.db.delete_resource(9007199254740991)
        if not ok then return "err:" .. tostring(err) end
        return "deleted"
`)
	if got != "deleted" {
		t.Errorf("2^53-1 should be accepted, got %q", got)
	}
}
