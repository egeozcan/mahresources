package plugin_system

import (
	"testing"
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
