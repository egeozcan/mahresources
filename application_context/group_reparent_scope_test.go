package application_context

import (
	"errors"
	"testing"

	"gorm.io/gorm"
	"mahresources/auth"
	"mahresources/models"
	"mahresources/models/query_models"
)

// A group-limited principal's subtree allow-list is bound once, when the
// request context is built, and a parked request can carry it for as long as
// the server lets it read a body. Re-parenting is a durable change to what that
// principal can see, so it must be decided against the tree as it is when the
// write runs, not against the snapshot: otherwise a request bound before an
// admin moved B out of the user's subtree can put B straight back.

type reparentScopeFixture struct {
	ctx     *MahresourcesContext
	s, a, b *models.Group
	out     *models.Group
	tag     *models.Tag
	p       *auth.Principal
}

func newReparentScopeFixture(t *testing.T) *reparentScopeFixture {
	t.Helper()
	ctx := createTestContextWithPlugins(t, t.TempDir())
	mk := func(name string, owner *uint) *models.Group {
		g := &models.Group{Name: name, OwnerId: owner}
		if err := ctx.db.Create(g).Error; err != nil {
			t.Fatalf("create group %s: %v", name, err)
		}
		return g
	}
	f := &reparentScopeFixture{ctx: ctx}
	f.s = mk("S", nil)
	f.a = mk("A", &f.s.ID)
	f.b = mk("B", &f.s.ID)
	f.out = mk("Out", nil)
	f.tag = &models.Tag{Name: "b-tag"}
	if err := ctx.db.Create(f.tag).Error; err != nil {
		t.Fatalf("create tag: %v", err)
	}
	f.p = accountPrincipal(t, ctx, "u", models.RoleUser, &f.s.ID)
	return f
}

func (f *reparentScopeFixture) move(t *testing.T, ctx *MahresourcesContext, id, owner uint) error {
	t.Helper()
	ed := query_models.GroupEditor{}
	ed.ID = id
	ed.Name = "moved"
	ed.OwnerId = owner
	_, err := ctx.UpdateGroup(&ed)
	return err
}

func (f *reparentScopeFixture) ownerOf(t *testing.T, id uint) *uint {
	t.Helper()
	var g models.Group
	if err := f.ctx.db.First(&g, id).Error; err != nil {
		t.Fatalf("reload group %d: %v", id, err)
	}
	return g.OwnerId
}

func (f *reparentScopeFixture) tagCount(t *testing.T, id uint) int64 {
	t.Helper()
	var n int64
	if err := f.ctx.db.Table("group_tags").Where("group_id = ?", id).Count(&n).Error; err != nil {
		t.Fatalf("count group_tags: %v", err)
	}
	return n
}

func TestUpdateGroup_StaleScopeCannotReparentARevokedGroup(t *testing.T) {
	f := newReparentScopeFixture(t)
	stale := f.ctx.WithPrincipal(f.p) // bound before the revocation

	if err := f.move(t, f.ctx, f.b.ID, f.out.ID); err != nil {
		t.Fatalf("admin move of B out of S: %v", err)
	}
	// Tagged after the admin's move, whose own full update replaces tags.
	if err := f.ctx.db.Model(f.b).Association("Tags").Append(f.tag); err != nil {
		t.Fatalf("tag B: %v", err)
	}

	if err := f.move(t, stale, f.b.ID, f.a.ID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("stale binding re-parented a revoked group: err=%v", err)
	}
	if got := f.ownerOf(t, f.b.ID); got == nil || *got != f.out.ID {
		t.Fatalf("B moved back: owner=%v, want %d", got, f.out.ID)
	}
	if f.ctx.WithPrincipal(f.p).GroupVisible(f.b.ID) {
		t.Fatal("B is visible to the scoped user again")
	}
	// The UPDATE alone matching no row is not a refusal: the tag Replace that
	// follows it is keyed on the id, and would clear B's tags.
	if n := f.tagCount(t, f.b.ID); n != 1 {
		t.Fatalf("B has %d tags after a refused re-parent, want 1", n)
	}
}

func TestUpdateGroup_StaleScopeCannotReparentUnderARevokedOwner(t *testing.T) {
	f := newReparentScopeFixture(t)
	stale := f.ctx.WithPrincipal(f.p)

	if err := f.move(t, f.ctx, f.a.ID, f.out.ID); err != nil {
		t.Fatalf("admin move of A out of S: %v", err)
	}

	err := f.move(t, stale, f.b.ID, f.a.ID)
	if err == nil || err.Error() != "owner group not found" {
		t.Fatalf("stale binding re-parented B under a revoked owner: err=%v", err)
	}
	if got := f.ownerOf(t, f.b.ID); got == nil || *got != f.s.ID {
		t.Fatalf("B's owner = %v, want %d", got, f.s.ID)
	}
}

func TestUpdateGroup_StaleScopeAfterOwnerClear(t *testing.T) {
	f := newReparentScopeFixture(t)
	stale := f.ctx.WithPrincipal(f.p)

	// The admin revokes by making B a root group.
	if err := f.move(t, f.ctx, f.b.ID, 0); err != nil {
		t.Fatalf("admin owner clear on B: %v", err)
	}
	if got := f.ownerOf(t, f.b.ID); got != nil {
		t.Fatalf("control: B still has owner %d", *got)
	}

	if err := f.move(t, stale, f.b.ID, f.a.ID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("stale binding re-parented a group made root: err=%v", err)
	}
	if got := f.ownerOf(t, f.b.ID); got != nil {
		t.Fatalf("B moved back under %d", *got)
	}
}

// Controls, so a blanket refusal cannot pass the tests above.
func TestUpdateGroup_ReparentControls(t *testing.T) {
	t.Run("fresh scoped binding re-parents inside its subtree", func(t *testing.T) {
		f := newReparentScopeFixture(t)
		if err := f.move(t, f.ctx.WithPrincipal(f.p), f.b.ID, f.a.ID); err != nil {
			t.Fatalf("re-parent: %v", err)
		}
		if got := f.ownerOf(t, f.b.ID); got == nil || *got != f.a.ID {
			t.Fatalf("B's owner = %v, want %d", got, f.a.ID)
		}
	})
	t.Run("stale binding still re-parents a group that stayed inside", func(t *testing.T) {
		f := newReparentScopeFixture(t)
		stale := f.ctx.WithPrincipal(f.p)
		c := &models.Group{Name: "C", OwnerId: &f.s.ID}
		if err := f.ctx.db.Create(c).Error; err != nil {
			t.Fatalf("create C: %v", err)
		}
		// Something unrelated changes in the tree; B and A stay inside.
		if err := f.move(t, f.ctx, c.ID, f.out.ID); err != nil {
			t.Fatalf("admin move of C: %v", err)
		}
		if err := f.move(t, stale, f.b.ID, f.a.ID); err != nil {
			t.Fatalf("re-parent: %v", err)
		}
		if got := f.ownerOf(t, f.b.ID); got == nil || *got != f.a.ID {
			t.Fatalf("B's owner = %v, want %d", got, f.a.ID)
		}
	})
	t.Run("admin and editor re-parent across trees", func(t *testing.T) {
		f := newReparentScopeFixture(t)
		if err := f.move(t, f.ctx, f.b.ID, f.out.ID); err != nil {
			t.Fatalf("unscoped re-parent: %v", err)
		}
		editor := f.ctx.WithPrincipal(&auth.Principal{UserID: 2, Username: "e", Role: models.RoleEditor})
		if err := f.move(t, editor, f.a.ID, f.out.ID); err != nil {
			t.Fatalf("editor re-parent: %v", err)
		}
		if got := f.ownerOf(t, f.a.ID); got == nil || *got != f.out.ID {
			t.Fatalf("A's owner = %v, want %d", got, f.out.ID)
		}
	})
	t.Run("scoped user cannot move a group outside its subtree", func(t *testing.T) {
		f := newReparentScopeFixture(t)
		if err := f.move(t, f.ctx.WithPrincipal(f.p), f.b.ID, f.out.ID); err == nil {
			t.Fatal("scoped user moved B under a group outside its subtree")
		}
		if got := f.ownerOf(t, f.b.ID); got == nil || *got != f.s.ID {
			t.Fatalf("B's owner = %v, want %d", got, f.s.ID)
		}
	})
}

// MergeGroups reads its winner and losers through the scoped handle, so a
// stale binding could merge a revoked loser into an in-scope winner and pull
// its children and content back into the subtree.
func TestMergeGroups_StaleScopeCannotMergeARevokedLoser(t *testing.T) {
	f := newReparentScopeFixture(t)
	child := &models.Group{Name: "B-child", OwnerId: &f.b.ID}
	if err := f.ctx.db.Create(child).Error; err != nil {
		t.Fatalf("create child: %v", err)
	}
	res := &models.Resource{Name: "B-res", OwnerId: &f.b.ID}
	if err := f.ctx.db.Create(res).Error; err != nil {
		t.Fatalf("create resource: %v", err)
	}
	stale := f.ctx.WithPrincipal(f.p)

	if err := f.move(t, f.ctx, f.b.ID, f.out.ID); err != nil {
		t.Fatalf("admin move of B out of S: %v", err)
	}

	if err := stale.MergeGroups(f.a.ID, []uint{f.b.ID}); err == nil {
		t.Fatal("stale binding merged a revoked loser")
	}
	var count int64
	f.ctx.db.Model(&models.Group{}).Where("id = ?", f.b.ID).Count(&count)
	if count != 1 {
		t.Fatal("the revoked loser was deleted")
	}
	if got := f.ownerOf(t, child.ID); got == nil || *got != f.b.ID {
		t.Fatalf("B's child owner = %v, want %d", got, f.b.ID)
	}
	var r models.Resource
	if err := f.ctx.db.First(&r, res.ID).Error; err != nil {
		t.Fatalf("reload resource: %v", err)
	}
	if r.OwnerId == nil || *r.OwnerId != f.b.ID {
		t.Fatalf("B's resource owner = %v, want %d", r.OwnerId, f.b.ID)
	}
}

// EditResource re-owns through the scoped handle; a stale binding could
// re-own a resource whose group was moved out of the subtree.
func TestEditResource_StaleScopeCannotReownAResourceOfARevokedGroup(t *testing.T) {
	f := newReparentScopeFixture(t)
	res := &models.Resource{Name: "B-res", OwnerId: &f.b.ID}
	if err := f.ctx.db.Create(res).Error; err != nil {
		t.Fatalf("create resource: %v", err)
	}
	stale := f.ctx.WithPrincipal(f.p)

	if err := f.move(t, f.ctx, f.b.ID, f.out.ID); err != nil {
		t.Fatalf("admin move of B out of S: %v", err)
	}

	edit := &query_models.ResourceEditor{}
	edit.ID = res.ID
	edit.Name = "B-res"
	edit.OwnerId = f.a.ID
	edit.PatchFields = map[string]bool{"owner_id": true}
	if _, err := stale.EditResource(edit); err == nil {
		t.Fatal("stale binding re-owned a resource of a revoked group")
	}
	var r models.Resource
	if err := f.ctx.db.First(&r, res.ID).Error; err != nil {
		t.Fatalf("reload resource: %v", err)
	}
	if r.OwnerId == nil || *r.OwnerId != f.b.ID {
		t.Fatalf("resource owner = %v, want %d", r.OwnerId, f.b.ID)
	}

	// Control: a fresh binding re-owns a resource still inside the subtree.
	inside := &models.Resource{Name: "A-res", OwnerId: &f.a.ID}
	if err := f.ctx.db.Create(inside).Error; err != nil {
		t.Fatalf("create inside resource: %v", err)
	}
	ok := &query_models.ResourceEditor{}
	ok.ID = inside.ID
	ok.Name = "A-res"
	ok.OwnerId = f.s.ID
	ok.PatchFields = map[string]bool{"owner_id": true}
	if _, err := stale.EditResource(ok); err != nil {
		t.Fatalf("re-own inside the subtree: %v", err)
	}
}

// CreateOrUpdateNote replaces the owner on every update; a stale binding could
// re-own a note whose group was moved out of the subtree.
func TestCreateOrUpdateNote_StaleScopeCannotReownANoteOfARevokedGroup(t *testing.T) {
	f := newReparentScopeFixture(t)
	note := &models.Note{Name: "B-note", OwnerId: &f.b.ID}
	if err := f.ctx.db.Create(note).Error; err != nil {
		t.Fatalf("create note: %v", err)
	}
	stale := f.ctx.WithPrincipal(f.p)

	if err := f.move(t, f.ctx, f.b.ID, f.out.ID); err != nil {
		t.Fatalf("admin move of B out of S: %v", err)
	}

	if _, err := stale.CreateOrUpdateNote(&query_models.NoteEditor{
		ID:          note.ID,
		NoteCreator: query_models.NoteCreator{Name: "B-note", OwnerId: f.a.ID},
	}); err == nil {
		t.Fatal("stale binding re-owned a note of a revoked group")
	}
	var n models.Note
	if err := f.ctx.db.First(&n, note.ID).Error; err != nil {
		t.Fatalf("reload note: %v", err)
	}
	if n.OwnerId == nil || *n.OwnerId != f.b.ID {
		t.Fatalf("note owner = %v, want %d", n.OwnerId, f.b.ID)
	}

	// Control: a stale binding still updates a note that stayed inside.
	inside := &models.Note{Name: "A-note", OwnerId: &f.a.ID}
	if err := f.ctx.db.Create(inside).Error; err != nil {
		t.Fatalf("create inside note: %v", err)
	}
	if _, err := stale.CreateOrUpdateNote(&query_models.NoteEditor{
		ID:          inside.ID,
		NoteCreator: query_models.NoteCreator{Name: "A-note", OwnerId: f.s.ID},
	}); err != nil {
		t.Fatalf("update inside the subtree: %v", err)
	}
}

// accountChangeWrites are the containment writes rescopeTx guards, each
// pulling B (or B's content) under A. s, a and b are S, A under S and B under
// S, as in newReparentScopeFixture.
func accountChangeWrites(t *testing.T, ctx *MahresourcesContext, a, b *models.Group) map[string]func(stale *MahresourcesContext) error {
	t.Helper()
	res := &models.Resource{Name: "B-res", OwnerId: &b.ID}
	if err := ctx.db.Create(res).Error; err != nil {
		t.Fatalf("create resource: %v", err)
	}
	note := &models.Note{Name: "B-note", OwnerId: &b.ID}
	if err := ctx.db.Create(note).Error; err != nil {
		t.Fatalf("create note: %v", err)
	}
	return map[string]func(stale *MahresourcesContext) error{
		"UpdateGroup": func(stale *MahresourcesContext) error {
			ed := query_models.GroupEditor{}
			ed.ID, ed.Name, ed.OwnerId = b.ID, b.Name, a.ID
			_, err := stale.UpdateGroup(&ed)
			return err
		},
		"MassEditGroups": func(stale *MahresourcesContext) error {
			_, err := stale.MassEditGroups(&query_models.MassEditQuery{ID: []uint{b.ID}, OwnerOp: "set", OwnerId: a.ID})
			return err
		},
		"MergeGroups": func(stale *MahresourcesContext) error {
			return stale.MergeGroups(a.ID, []uint{b.ID})
		},
		"EditResource": func(stale *MahresourcesContext) error {
			ed := &query_models.ResourceEditor{}
			ed.ID, ed.Name, ed.OwnerId = res.ID, res.Name, a.ID
			ed.PatchFields = map[string]bool{"owner_id": true}
			_, err := stale.EditResource(ed)
			return err
		},
		"CreateOrUpdateNote": func(stale *MahresourcesContext) error {
			_, err := stale.CreateOrUpdateNote(&query_models.NoteEditor{
				ID: note.ID, NoteCreator: query_models.NoteCreator{Name: note.Name, OwnerId: a.ID},
			})
			return err
		},
	}
}

// assertBUntouched checks that B, its resource and its note still sit where
// accountChangeWrites found them.
func assertBUntouched(t *testing.T, ctx *MahresourcesContext, s, b *models.Group) {
	t.Helper()
	var g models.Group
	if err := ctx.db.First(&g, b.ID).Error; err != nil {
		t.Fatalf("reload B: %v", err)
	}
	if g.OwnerId == nil || *g.OwnerId != s.ID {
		t.Fatalf("B's owner = %v, want %d", g.OwnerId, s.ID)
	}
	var r models.Resource
	if err := ctx.db.Where("name = ?", "B-res").First(&r).Error; err != nil || r.OwnerId == nil || *r.OwnerId != b.ID {
		t.Fatalf("B's resource owner = %v (err %v), want %d", r.OwnerId, err, b.ID)
	}
	var n models.Note
	if err := ctx.db.Where("name = ?", "B-note").First(&n).Error; err != nil || n.OwnerId == nil || *n.OwnerId != b.ID {
		t.Fatalf("B's note owner = %v (err %v), want %d", n.OwnerId, err, b.ID)
	}
}

// runStaleAccountCannotWriteContainment: the principal was bound while its
// account was scoped to S. An admin then narrows the account to A (removing B
// from its subtree) or disables it. A containment write through the stale
// binding must be refused rather than judged against subtree(S).
func runStaleAccountCannotWriteContainment(t *testing.T, newFixture func(t *testing.T) (ctx *MahresourcesContext, s, a, b *models.Group, p *auth.Principal)) {
	changes := map[string]func(a *models.Group) *UserUpdate{
		"scope narrowed": func(a *models.Group) *UserUpdate {
			return &UserUpdate{ScopeGroupID: UserField[*uint]{Set: true, Value: &a.ID}}
		},
		"disabled": func(*models.Group) *UserUpdate {
			return &UserUpdate{Disabled: UserField[bool]{Set: true, Value: true}}
		},
		"demoted to guest": func(*models.Group) *UserUpdate {
			return &UserUpdate{Role: UserField[models.Role]{Set: true, Value: models.RoleGuest}}
		},
	}
	for change, update := range changes {
		for _, write := range []string{"UpdateGroup", "MassEditGroups", "MergeGroups", "EditResource", "CreateOrUpdateNote"} {
			t.Run(change+"/"+write, func(t *testing.T) {
				ctx, s, a, b, p := newFixture(t)
				writes := accountChangeWrites(t, ctx, a, b)
				stale := ctx.WithPrincipal(p)
				if _, err := ctx.UpdateUser(p.UserID, update(a)); err != nil {
					t.Fatalf("admin account change: %v", err)
				}
				if err := writes[write](stale); !errors.Is(err, gorm.ErrRecordNotFound) {
					t.Fatalf("stale binding wrote after the account changed: err=%v", err)
				}
				assertBUntouched(t, ctx, s, b)
			})
		}
	}

	// Control: the same writes through a binding whose account is unchanged
	// still land.
	for _, write := range []string{"UpdateGroup", "MassEditGroups", "MergeGroups", "EditResource", "CreateOrUpdateNote"} {
		t.Run("unchanged/"+write, func(t *testing.T) {
			ctx, _, a, b, p := newFixture(t)
			writes := accountChangeWrites(t, ctx, a, b)
			if err := writes[write](ctx.WithPrincipal(p)); err != nil {
				t.Fatalf("write inside an unchanged subtree: %v", err)
			}
		})
	}
}

func TestStaleAccountCannotWriteContainment(t *testing.T) {
	runStaleAccountCannotWriteContainment(t, func(t *testing.T) (*MahresourcesContext, *models.Group, *models.Group, *models.Group, *auth.Principal) {
		f := newReparentScopeFixture(t)
		return f.ctx, f.s, f.a, f.b, f.p
	})
}
