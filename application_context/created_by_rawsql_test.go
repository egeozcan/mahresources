package application_context

import (
	"testing"

	"mahresources/auth"
	"mahresources/models"
)

// seriesCreatedBy loads the created_by_user_id for a series slug.
func seriesCreatedBy(t *testing.T, ctx *MahresourcesContext, slug string) *uint {
	t.Helper()
	var s models.Series
	if err := ctx.db.Where("slug = ?", slug).First(&s).Error; err != nil {
		t.Fatalf("load series %q: %v", slug, err)
	}
	return s.CreatedByUserId
}

func TestRawStamp_SeriesFindOrCreate_NoAuthRoot(t *testing.T) {
	ctx := newStampTestContext(t, false)
	root := makeAdmin(t, ctx, "root")

	if _, _, err := ctx.GetOrCreateSeriesForResource(ctx.db, "newslug"); err != nil {
		t.Fatalf("GetOrCreateSeriesForResource: %v", err)
	}
	got := seriesCreatedBy(t, ctx, "newslug")
	if got == nil || *got != root.ID {
		t.Fatalf("no-auth implicit series should be stamped root %d, got %v", root.ID, got)
	}
}

func TestRawStamp_SeriesFindOrCreate_AuthOnActingUser(t *testing.T) {
	ctx := newStampTestContext(t, true)
	u, err := ctx.CreateUser(&UserInput{Username: "u", Password: "password1", Role: models.RoleUser})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	scoped := ctx.WithPrincipal(auth.FromUser(u))
	if _, _, err := scoped.GetOrCreateSeriesForResource(scoped.db, "authslug"); err != nil {
		t.Fatalf("GetOrCreateSeriesForResource: %v", err)
	}
	got := seriesCreatedBy(t, ctx, "authslug")
	if got == nil || *got != u.ID {
		t.Fatalf("auth-on implicit series should be stamped acting user %d, got %v", u.ID, got)
	}
}

// mergeRelationCreatedBy sets up winner/loser/far groups, a relation type, and a
// loser→far relation, runs the merge on mergeCtx, and returns the copied
// winner→far relation's created_by_user_id.
func mergeRelationCreatedBy(t *testing.T, base *MahresourcesContext, mergeCtx *MahresourcesContext) *uint {
	t.Helper()
	winner := makeTestGroup(t, base, "winner")
	loser := makeTestGroup(t, base, "loser")
	far := makeTestGroup(t, base, "far")

	rt := &models.GroupRelationType{Name: "rel"}
	if err := base.db.Create(rt).Error; err != nil {
		t.Fatalf("create relation type: %v", err)
	}
	rel := &models.GroupRelation{FromGroupId: &loser.ID, ToGroupId: &far.ID, RelationTypeId: &rt.ID, Name: "r"}
	if err := base.db.Create(rel).Error; err != nil {
		t.Fatalf("create relation: %v", err)
	}

	if err := mergeCtx.MergeGroups(winner.ID, []uint{loser.ID}); err != nil {
		t.Fatalf("MergeGroups: %v", err)
	}

	var copied models.GroupRelation
	if err := base.db.Where("from_group_id = ? AND to_group_id = ?", winner.ID, far.ID).First(&copied).Error; err != nil {
		t.Fatalf("load copied relation: %v", err)
	}
	return copied.CreatedByUserId
}

func TestRawStamp_GroupMergeRelations_NoAuthRoot(t *testing.T) {
	ctx := newStampTestContext(t, false)
	root := makeAdmin(t, ctx, "root")
	got := mergeRelationCreatedBy(t, ctx, ctx)
	if got == nil || *got != root.ID {
		t.Fatalf("no-auth merge relation should be stamped root %d, got %v", root.ID, got)
	}
}

func TestRawStamp_GroupMergeRelations_AuthOnActingUser(t *testing.T) {
	ctx := newStampTestContext(t, true)
	u, err := ctx.CreateUser(&UserInput{Username: "op", Password: "password1", Role: models.RoleEditor})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	got := mergeRelationCreatedBy(t, ctx, ctx.WithPrincipal(auth.FromUser(u)))
	if got == nil || *got != u.ID {
		t.Fatalf("auth-on merge relation should be stamped acting user %d, got %v", u.ID, got)
	}
}
