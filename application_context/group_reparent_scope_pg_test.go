//go:build postgres && json1 && fts5

package application_context

import (
	"errors"
	"testing"
	"time"

	"gorm.io/gorm"

	"mahresources/auth"
	"mahresources/models"
	"mahresources/models/query_models"
)

// Postgres parity for group_reparent_scope_test.go: rescopeTx's subtree CTE is
// a Raw().Scan on the transaction, after pg_advisory_xact_lock, and must see the
// tree as it is then.

func newReparentScopePGFixture(t *testing.T) (ctx *MahresourcesContext, s, a, b, out *models.Group, p *auth.Principal) {
	t.Helper()
	ctx = newMassEditPGContext(t)
	mk := func(name string, owner *uint) *models.Group {
		g := &models.Group{Name: name, OwnerId: owner}
		if err := ctx.db.Create(g).Error; err != nil {
			t.Fatalf("create group %s: %v", name, err)
		}
		return g
	}
	s = mk("S", nil)
	a = mk("A", &s.ID)
	b = mk("B", &s.ID)
	out = mk("Out", nil)
	p = accountPrincipal(t, ctx, "u", models.RoleUser, &s.ID)
	return ctx, s, a, b, out, p
}

// accountStillBound's re-read of the account runs on the transaction after
// pg_advisory_xact_lock, and must see an admin's committed account change.
func TestStaleAccountCannotWriteContainment_Postgres(t *testing.T) {
	runStaleAccountCannotWriteContainment(t, func(t *testing.T) (*MahresourcesContext, *models.Group, *models.Group, *models.Group, *auth.Principal) {
		ctx, s, a, b, _, p := newReparentScopePGFixture(t)
		return ctx, s, a, b, p
	})
}

func pgGroupOwner(t *testing.T, ctx *MahresourcesContext, id uint) *uint {
	t.Helper()
	var g models.Group
	if err := ctx.db.First(&g, id).Error; err != nil {
		t.Fatalf("reload group %d: %v", id, err)
	}
	return g.OwnerId
}

func TestUpdateGroup_StaleScopeCannotReparentARevokedGroup_Postgres(t *testing.T) {
	ctx, _, a, b, out, p := newReparentScopePGFixture(t)
	stale := ctx.WithPrincipal(p)

	adminMove := query_models.GroupEditor{}
	adminMove.ID, adminMove.Name, adminMove.OwnerId = b.ID, "B", out.ID
	if _, err := ctx.UpdateGroup(&adminMove); err != nil {
		t.Fatalf("admin move: %v", err)
	}

	back := query_models.GroupEditor{}
	back.ID, back.Name, back.OwnerId = b.ID, "B", a.ID
	if _, err := stale.UpdateGroup(&back); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("stale binding re-parented a revoked group: err=%v", err)
	}
	if got := pgGroupOwner(t, ctx, b.ID); got == nil || *got != out.ID {
		t.Fatalf("B's owner = %v, want %d", got, out.ID)
	}
}

func TestMassEditStaleScopeCannotReparentARevokedGroup_Postgres(t *testing.T) {
	ctx, _, a, b, out, p := newReparentScopePGFixture(t)
	stale := ctx.WithPrincipal(p)
	if err := ctx.db.Model(&models.Group{}).Where("id = ?", b.ID).Update("owner_id", out.ID).Error; err != nil {
		t.Fatalf("admin move: %v", err)
	}

	if _, err := stale.MassEditGroups(&query_models.MassEditQuery{ID: []uint{b.ID}, OwnerOp: "set", OwnerId: a.ID}); err == nil {
		t.Fatal("stale binding re-parented a revoked group through mass edit")
	}
	if got := pgGroupOwner(t, ctx, b.ID); got == nil || *got != out.ID {
		t.Fatalf("B's owner = %v, want %d", got, out.ID)
	}
}

// The scoped re-parent queues on the tree lock behind an admin transaction that
// is moving B out. When the admin commits, the scoped write must judge B by the
// tree it now sees, not by the allow-list it was bound with.
func TestUpdateGroup_ScopedReparentQueuedBehindAdminMove_Postgres(t *testing.T) {
	ctx, _, a, b, out, p := newReparentScopePGFixture(t)
	stale := ctx.WithPrincipal(p)

	locked := make(chan struct{})
	release := make(chan struct{})
	adminDone := make(chan error, 1)
	go func() {
		adminDone <- ctx.db.Transaction(func(tx *gorm.DB) error {
			if err := ctx.lockGroupTreeMutation(tx); err != nil {
				return err
			}
			if err := tx.Model(&models.Group{}).Where("id = ?", b.ID).Update("owner_id", out.ID).Error; err != nil {
				return err
			}
			close(locked)
			<-release
			return nil
		})
	}()
	select {
	case <-locked:
	case <-time.After(10 * time.Second):
		t.Fatal("admin transaction never took the tree lock")
	}

	scopedDone := make(chan error, 1)
	go func() {
		back := query_models.GroupEditor{}
		back.ID, back.Name, back.OwnerId = b.ID, "B", a.ID
		_, err := stale.UpdateGroup(&back)
		scopedDone <- err
	}()
	select {
	case err := <-scopedDone:
		close(release)
		<-adminDone
		t.Fatalf("scoped re-parent did not wait for the tree lock: err=%v", err)
	case <-time.After(200 * time.Millisecond):
	}
	close(release)
	if err := <-adminDone; err != nil {
		t.Fatalf("admin move: %v", err)
	}

	select {
	case err := <-scopedDone:
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			t.Fatalf("queued scoped re-parent: err=%v, want ErrRecordNotFound", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("queued scoped re-parent never finished")
	}
	if got := pgGroupOwner(t, ctx, b.ID); got == nil || *got != out.ID {
		t.Fatalf("B's owner = %v, want %d", got, out.ID)
	}
}
