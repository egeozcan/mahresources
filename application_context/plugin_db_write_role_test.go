package application_context

import (
	"context"
	"encoding/base64"
	"errors"
	"testing"

	"mahresources/models"
	"mahresources/plugin_system"
)

// bindPluginActor returns the querier/writer pair a plugin call receives for an
// actor, the way the render seam binds it.
func bindPluginActor(ctx *MahresourcesContext, actorID uint) (plugin_system.EntityQuerier, plugin_system.EntityWriter, *pluginDBAdapter) {
	a := &pluginDBAdapter{ctx: ctx}
	q, w := a.BindInvocation(&plugin_system.Invocation{ActorUserID: actorID})
	return q, w, a
}

// TestPluginDBWritersRefuseAReadOnlyPrincipal: mah.db writes from a read
// surface run as the viewing guest, and the URL-path rule that refuses guests
// every mutating endpoint is not in that path.
func TestPluginDBWritersRefuseAReadOnlyPrincipal(t *testing.T) {
	ctx := newRoleCapabilityTestContext(t)

	g := &models.Group{Name: "readable"}
	if err := ctx.db.Create(g).Error; err != nil {
		t.Fatal(err)
	}
	child := &models.Group{Name: "child", OwnerId: &g.ID}
	tag := &models.Tag{Name: "global-tag"}
	note := &models.Note{Name: "n1", OwnerId: &g.ID}
	res := &models.Resource{Name: "r1", OwnerId: &g.ID, Hash: "h"}
	for _, row := range []any{child, tag, note, res} {
		if err := ctx.db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}

	guest, err := ctx.CreateUser(&UserInput{Username: "ro", Password: "password1", Role: models.RoleGuest, ScopeGroupId: &g.ID})
	if err != nil {
		t.Fatal(err)
	}
	disabled, err := ctx.CreateUser(&UserInput{Username: "off", Password: "password1", Role: models.RoleUser, ScopeGroupId: &g.ID})
	if err != nil {
		t.Fatal(err)
	}
	if err := ctx.db.Model(&models.User{}).Where("id = ?", disabled.ID).Update("disabled", true).Error; err != nil {
		t.Fatal(err)
	}

	before := map[string]int64{
		"groups":    countRows(t, ctx, &models.Group{}, "1=1"),
		"notes":     countRows(t, ctx, &models.Note{}, "1=1"),
		"tags":      countRows(t, ctx, &models.Tag{}, "1=1"),
		"resources": countRows(t, ctx, &models.Resource{}, "1=1"),
	}

	for name, actor := range map[string]uint{"guest": guest.ID, "deleted": 4242, "disabled": disabled.ID} {
		t.Run(name, func(t *testing.T) {
			q, w, a := bindPluginActor(ctx, actor)
			owner := map[string]any{"owner_id": float64(g.ID)}
			ran := false
			calls := map[string]func() error{
				"CreateGroup":    func() error { _, e := w.CreateGroup(map[string]any{"name": "x", "owner_id": float64(g.ID)}); return e },
				"UpdateGroup":    func() error { _, e := w.UpdateGroup(child.ID, map[string]any{"name": "y"}); return e },
				"PatchGroup":     func() error { _, e := w.PatchGroup(child.ID, map[string]any{"name": "y"}); return e },
				"DeleteGroup":    func() error { return w.DeleteGroup(child.ID) },
				"CreateNote":     func() error { _, e := w.CreateNote(map[string]any{"name": "x", "owner_id": float64(g.ID)}); return e },
				"UpdateNote":     func() error { _, e := w.UpdateNote(note.ID, map[string]any{"name": "y"}); return e },
				"PatchNote":      func() error { _, e := w.PatchNote(note.ID, map[string]any{"name": "y"}); return e },
				"DeleteNote":     func() error { return w.DeleteNote(note.ID) },
				"CreateTag":      func() error { _, e := w.CreateTag(map[string]any{"name": "guest-tag"}); return e },
				"UpdateTag":      func() error { _, e := w.UpdateTag(tag.ID, map[string]any{"name": "y"}); return e },
				"PatchTag":       func() error { _, e := w.PatchTag(tag.ID, map[string]any{"name": "y"}); return e },
				"DeleteTag":      func() error { return w.DeleteTag(tag.ID) },
				"UpdateResource": func() error { _, e := w.UpdateResource(res.ID, map[string]any{"name": "y"}); return e },
				"PatchResource":  func() error { _, e := w.PatchResource(res.ID, map[string]any{"name": "y"}); return e },
				"DeleteResource": func() error { return w.DeleteResource(res.ID) },
				"AddTags":        func() error { return w.AddTagsToEntity("note", note.ID, []uint{tag.ID}) },
				"RemoveTags":     func() error { return w.RemoveTagsFromEntity("note", note.ID, []uint{tag.ID}) },
				"AddTagsEmpty":   func() error { return w.AddTagsToEntity("note", note.ID, nil) },
				"AddGroups":      func() error { return w.AddGroupsToEntity("resource", res.ID, []uint{g.ID}) },
				"RemoveGroups":   func() error { return w.RemoveGroupsFromEntity("resource", res.ID, []uint{g.ID}) },
				"AddRes":         func() error { return w.AddResourcesToNote(note.ID, []uint{res.ID}) },
				"RemoveRes":      func() error { return w.RemoveResourcesFromNote(note.ID, []uint{res.ID}) },
				"CreateFromData": func() error {
					_, e := q.CreateResourceFromData(base64.StdEncoding.EncodeToString([]byte("x")), owner)
					return e
				},
				"CreateFromURL": func() error {
					_, e := q.CreateResourceFromURL(context.Background(), "https://example.invalid/x", owner)
					return e
				},
				"AddVersionFromURL": func() error {
					_, e := q.AddResourceVersionFromURL(context.Background(), res.ID, "https://example.invalid/x", "c")
					return e
				},
				"Transaction": func() error {
					return a.RunInTransaction(&plugin_system.Invocation{ActorUserID: actor}, func(*plugin_system.Invocation) error {
						ran = true
						return nil
					})
				},
			}
			for cname, call := range calls {
				if err := call(); !errors.Is(err, ErrRoleCapability) {
					t.Errorf("%s: want ErrRoleCapability, got %v", cname, err)
				}
			}
			if ran {
				t.Error("transaction body ran for a read-only principal")
			}
		})
	}

	for k, model := range map[string]any{"groups": &models.Group{}, "notes": &models.Note{}, "tags": &models.Tag{}, "resources": &models.Resource{}} {
		if got := countRows(t, ctx, model, "1=1"); got != before[k] {
			t.Errorf("%s row count changed: %d -> %d", k, before[k], got)
		}
	}
	var got models.Tag
	if err := ctx.db.First(&got, tag.ID).Error; err != nil || got.Name != "global-tag" {
		t.Errorf("tag changed or vanished: %v %q", err, got.Name)
	}
}

// TestPluginDBWritersStillAllowWriters guards the other direction: roles that
// may write, and the nil principal, keep a working mah.db.
func TestPluginDBWritersStillAllowWriters(t *testing.T) {
	ctx := newRoleCapabilityTestContext(t)
	g := &models.Group{Name: "readable"}
	if err := ctx.db.Create(g).Error; err != nil {
		t.Fatal(err)
	}
	mk := func(name string, role models.Role, scoped bool) uint {
		in := &UserInput{Username: name, Password: "password1", Role: role}
		if scoped {
			in.ScopeGroupId = &g.ID
		}
		u, err := ctx.CreateUser(in)
		if err != nil {
			t.Fatal(err)
		}
		return u.ID
	}
	actors := map[string]uint{
		"scoped-user": mk("su", models.RoleUser, true),
		"user":        mk("u", models.RoleUser, false),
		"editor":      mk("e", models.RoleEditor, false),
		"admin":       mk("a", models.RoleAdmin, false),
		"unbound":     0,
	}
	for name, id := range actors {
		t.Run(name, func(t *testing.T) {
			_, w, a := bindPluginActor(ctx, id)
			if _, err := w.CreateTag(map[string]any{"name": "t-" + name}); err != nil {
				t.Errorf("CreateTag: %v", err)
			}
			if name != "unbound" && name != "user" && name != "editor" && name != "admin" {
				if _, err := w.CreateNote(map[string]any{"name": "n-" + name, "owner_id": float64(g.ID)}); err != nil {
					t.Errorf("CreateNote: %v", err)
				}
			}
			ran := false
			if err := a.RunInTransaction(&plugin_system.Invocation{ActorUserID: id}, func(*plugin_system.Invocation) error { ran = true; return nil }); err != nil || !ran {
				t.Errorf("transaction: ran=%v err=%v", ran, err)
			}
		})
	}
	// Behaviour above capWrite is unchanged: editor and user still refused.
	for _, name := range []string{"editor", "scoped-user"} {
		_, w, _ := bindPluginActor(ctx, actors[name])
		if _, err := w.CreateCategory(map[string]any{"name": "c-" + name}); !errors.Is(err, ErrRoleCapability) {
			t.Errorf("%s CreateCategory: want ErrRoleCapability, got %v", name, err)
		}
	}
}
