package application_context

import (
	"errors"
	"fmt"
	"testing"

	"mahresources/auth"
	"mahresources/models"
)

type relatedFixture struct {
	root, child, linked, other *models.Group
	owned                      *models.Resource
	ownedNote, linkedNote      *models.Note
	linkedResource             *models.Resource
	cat                        *models.Category
}

func seedRelated(t *testing.T, ctx *MahresourcesContext) relatedFixture {
	t.Helper()
	f := relatedFixture{cat: &models.Category{Name: "Project"}}
	mk := func(v any) {
		t.Helper()
		if err := ctx.db.Create(v).Error; err != nil {
			t.Fatal(err)
		}
	}
	mk(f.cat)
	f.root = &models.Group{Name: "root"}
	mk(f.root)
	f.child = &models.Group{Name: "child", OwnerId: &f.root.ID, CategoryId: &f.cat.ID}
	f.linked = &models.Group{Name: "linked"}
	f.other = &models.Group{Name: "unrelated"}
	mk(f.child)
	mk(f.linked)
	mk(f.other)
	f.owned = &models.Resource{Name: "owned.png", OwnerId: &f.root.ID}
	f.linkedResource = &models.Resource{Name: "linked.png"}
	mk(f.owned)
	mk(f.linkedResource)
	f.ownedNote = &models.Note{Name: "owned note", OwnerId: &f.root.ID}
	f.linkedNote = &models.Note{Name: "linked note"}
	mk(f.ownedNote)
	mk(f.linkedNote)
	if err := ctx.db.Model(f.root).Association("RelatedGroups").Append(f.linked); err != nil {
		t.Fatal(err)
	}
	if err := ctx.db.Model(f.root).Association("RelatedResources").Append(f.linkedResource); err != nil {
		t.Fatal(err)
	}
	if err := ctx.db.Model(f.root).Association("RelatedNotes").Append(f.linkedNote); err != nil {
		t.Fatal(err)
	}
	return f
}

func relatedKeys(entries []TemplateRelatedEntry) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = fmt.Sprintf("%s:%d", e.Type, e.ID)
	}
	return out
}

func runRelatedReaders(t *testing.T, ctx *MahresourcesContext) {
	f := seedRelated(t, ctx)

	t.Run("up is the owner", func(t *testing.T) {
		entries, more, err := ctx.ListTemplateRunRelated("group", f.child.ID, "up", "", 0, 50)
		if err != nil || more {
			t.Fatalf("err=%v more=%v", err, more)
		}
		if got := relatedKeys(entries); len(got) != 1 || got[0] != fmt.Sprintf("group:%d", f.root.ID) {
			t.Fatalf("up = %v", got)
		}
		entries, _, _ = ctx.ListTemplateRunRelated("resource", f.owned.ID, "up", "", 0, 50)
		if got := relatedKeys(entries); len(got) != 1 || got[0] != fmt.Sprintf("group:%d", f.root.ID) {
			t.Fatalf("resource up = %v", got)
		}
	})

	t.Run("down is what the group owns, groups then resources then notes", func(t *testing.T) {
		entries, _, err := ctx.ListTemplateRunRelated("group", f.root.ID, "down", "", 0, 50)
		if err != nil {
			t.Fatal(err)
		}
		want := []string{fmt.Sprintf("group:%d", f.child.ID), fmt.Sprintf("resource:%d", f.owned.ID), fmt.Sprintf("note:%d", f.ownedNote.ID)}
		if got := relatedKeys(entries); fmt.Sprint(got) != fmt.Sprint(want) {
			t.Fatalf("down = %v, want %v", got, want)
		}
		if entries[0].CarrierID != f.cat.ID || entries[0].CarrierName != "Project" {
			t.Errorf("carrier of the child = %d %q", entries[0].CarrierID, entries[0].CarrierName)
		}
		// The kind filter narrows to one type.
		entries, _, _ = ctx.ListTemplateRunRelated("group", f.root.ID, "down", "note", 0, 50)
		if got := relatedKeys(entries); len(got) != 1 || got[0] != fmt.Sprintf("note:%d", f.ownedNote.ID) {
			t.Fatalf("down notes = %v", got)
		}
		// A resource owns nothing.
		entries, more, err := ctx.ListTemplateRunRelated("resource", f.owned.ID, "down", "", 0, 50)
		if err != nil || len(entries) != 0 || more {
			t.Fatalf("resource down = %v more=%v err=%v", entries, more, err)
		}
	})

	t.Run("across is links, not ownership", func(t *testing.T) {
		entries, _, err := ctx.ListTemplateRunRelated("group", f.root.ID, "across", "", 0, 50)
		if err != nil {
			t.Fatal(err)
		}
		want := []string{fmt.Sprintf("group:%d", f.linked.ID), fmt.Sprintf("resource:%d", f.linkedResource.ID), fmt.Sprintf("note:%d", f.linkedNote.ID)}
		if got := relatedKeys(entries); fmt.Sprint(got) != fmt.Sprint(want) {
			t.Fatalf("across = %v, want %v", got, want)
		}
		// From the other side: the linked resource reaches the group.
		entries, _, _ = ctx.ListTemplateRunRelated("resource", f.linkedResource.ID, "across", "group", 0, 50)
		if got := relatedKeys(entries); len(got) != 1 || got[0] != fmt.Sprintf("group:%d", f.root.ID) {
			t.Fatalf("resource across = %v", got)
		}
	})

	t.Run("paging crosses sources and reports more from the extra row", func(t *testing.T) {
		entries, more, err := ctx.ListTemplateRunRelated("group", f.root.ID, "down", "", 0, 2)
		if err != nil || len(entries) != 2 || !more {
			t.Fatalf("page 1: %d more=%v err=%v", len(entries), more, err)
		}
		entries, more, err = ctx.ListTemplateRunRelated("group", f.root.ID, "down", "", 2, 2)
		if err != nil || len(entries) != 1 || more {
			t.Fatalf("page 2: %v more=%v err=%v", relatedKeys(entries), more, err)
		}
		if got := relatedKeys(entries); got[0] != fmt.Sprintf("note:%d", f.ownedNote.ID) {
			t.Fatalf("page 2 = %v", got)
		}
	})

	t.Run("a missing entity is the unavailable error", func(t *testing.T) {
		_, _, err := ctx.ListTemplateRunRelated("group", 999999, "up", "", 0, 50)
		if !errors.Is(err, ErrTemplateRunEntityUnavailable) {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("an entity outside the subtree is the same error", func(t *testing.T) {
		scoped := ctx.WithPrincipal(&auth.Principal{UserID: 1, Role: models.RoleEditor, ScopeGroupID: &f.child.ID})
		_, _, err := scoped.ListTemplateRunRelated("group", f.root.ID, "down", "", 0, 50)
		if !errors.Is(err, ErrTemplateRunEntityUnavailable) {
			t.Fatalf("out-of-scope err = %v", err)
		}
	})

	t.Run("members are the carrier's entities, newest first", func(t *testing.T) {
		entries, more, err := ctx.ListTemplateRunMembers(models.TemplateCarrierCategory, f.cat.ID, 0, 50)
		if err != nil || more {
			t.Fatalf("err=%v more=%v", err, more)
		}
		if got := relatedKeys(entries); len(got) != 1 || got[0] != fmt.Sprintf("group:%d", f.child.ID) {
			t.Fatalf("members = %v", got)
		}
		if _, _, err := ctx.ListTemplateRunMembers(models.TemplateCarrierCategory, 999999, 0, 50); !errors.Is(err, ErrTemplateRunEntityUnavailable) {
			t.Fatalf("missing carrier err = %v", err)
		}
	})
}

func TestTemplateRunRelatedReaders(t *testing.T) {
	runRelatedReaders(t, createTestContext(t))
}
