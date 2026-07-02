package application_context

import (
	"fmt"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/spf13/afero"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"mahresources/auth"
	"mahresources/constants"
	"mahresources/models"
	"mahresources/server/interfaces"
)

// newSuggestTestContext builds an in-memory context with the tables the
// suggested-tags ranking touches (groups, resources, tags + the resource_tags
// join, and resource_similarities).
func newSuggestTestContext(t *testing.T) *MahresourcesContext {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=private", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if sqlDB, err := db.DB(); err == nil {
		sqlDB.SetMaxOpenConns(1)
	}
	if err := db.AutoMigrate(
		&models.Category{}, &models.ResourceCategory{},
		&models.Group{}, &models.Resource{}, &models.Note{},
		&models.Tag{}, &models.ResourceSimilarity{},
		// GetResource preloads clause.Associations, so every first-level
		// association table the Resource model declares must exist.
		&models.Series{}, &models.Preview{}, &models.ResourceVersion{}, &models.ImageHash{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	sqlDB, _ := db.DB()
	roDB := sqlx.NewDb(sqlDB, "sqlite3")
	return NewMahresourcesContext(afero.NewMemMapFs(), db, roDB, &MahresourcesConfig{DbType: constants.DbTypeSqlite})
}

func suggestMakeGroup(t *testing.T, ctx *MahresourcesContext, name string, parent *uint) *models.Group {
	t.Helper()
	g := &models.Group{Name: name, OwnerId: parent}
	if err := ctx.db.Create(g).Error; err != nil {
		t.Fatalf("create group %s: %v", name, err)
	}
	return g
}

func suggestMakeTag(t *testing.T, ctx *MahresourcesContext, name string) *models.Tag {
	t.Helper()
	tag := &models.Tag{Name: name}
	if err := ctx.db.Create(tag).Error; err != nil {
		t.Fatalf("create tag %s: %v", name, err)
	}
	return tag
}

// suggestMakeResource creates a resource owned by ownerID (nil = ownerless) and
// attaches the supplied tags.
func suggestMakeResource(t *testing.T, ctx *MahresourcesContext, name string, ownerID *uint, tags ...*models.Tag) *models.Resource {
	t.Helper()
	r := &models.Resource{Name: name, OwnerId: ownerID, Meta: []byte("{}"), OwnMeta: []byte("{}")}
	if err := ctx.db.Create(r).Error; err != nil {
		t.Fatalf("create resource %s: %v", name, err)
	}
	if len(tags) > 0 {
		if err := ctx.db.Model(r).Association("Tags").Append(tags); err != nil {
			t.Fatalf("attach tags to %s: %v", name, err)
		}
	}
	return r
}

func suggestLinkSimilar(t *testing.T, ctx *MahresourcesContext, a, b uint, dist uint8) {
	t.Helper()
	id1, id2 := a, b
	if id1 > id2 {
		id1, id2 = id2, id1
	}
	if err := ctx.db.Create(&models.ResourceSimilarity{ResourceID1: id1, ResourceID2: id2, HammingDistance: dist}).Error; err != nil {
		t.Fatalf("link similar %d<->%d: %v", a, b, err)
	}
}

func suggestNames(s []interfaces.SuggestedTag) []string {
	names := make([]string, len(s))
	for i, x := range s {
		names[i] = x.Name
	}
	return names
}

func suggestHas(s []interfaces.SuggestedTag, name string) bool {
	for _, x := range s {
		if x.Name == name {
			return true
		}
	}
	return false
}

// TestSuggestedTags_GroupRankingOnly: with no similarity rows the suggestions
// are the owner group's tags ordered by usage count descending.
func TestSuggestedTags_GroupRankingOnly(t *testing.T) {
	ctx := newSuggestTestContext(t)
	owner := suggestMakeGroup(t, ctx, "owner", nil)
	tagA := suggestMakeTag(t, ctx, "alpha")
	tagB := suggestMakeTag(t, ctx, "beta")
	tagC := suggestMakeTag(t, ctx, "gamma")

	// alpha used by 3 resources, beta by 2, gamma by 1.
	suggestMakeResource(t, ctx, "r1", &owner.ID, tagA, tagB, tagC)
	suggestMakeResource(t, ctx, "r2", &owner.ID, tagA, tagB)
	suggestMakeResource(t, ctx, "r3", &owner.ID, tagA)

	target := suggestMakeResource(t, ctx, "target", &owner.ID)

	got, err := ctx.GetSuggestedTags(target.ID, 8)
	if err != nil {
		t.Fatalf("GetSuggestedTags: %v", err)
	}
	names := suggestNames(got)
	want := []string{"alpha", "beta", "gamma"}
	if len(names) != len(want) {
		t.Fatalf("expected %v, got %v", want, names)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("ranking mismatch: expected %v, got %v", want, names)
		}
	}
	for _, s := range got {
		if len(s.Sources) == 0 || s.Sources[0] != "group" {
			t.Fatalf("expected group source, got %+v", s)
		}
	}
}

// TestSuggestedTags_ExcludesAlreadyApplied: tags already on the target are not
// suggested.
func TestSuggestedTags_ExcludesAlreadyApplied(t *testing.T) {
	ctx := newSuggestTestContext(t)
	owner := suggestMakeGroup(t, ctx, "owner", nil)
	tagA := suggestMakeTag(t, ctx, "alpha")
	tagB := suggestMakeTag(t, ctx, "beta")

	suggestMakeResource(t, ctx, "r1", &owner.ID, tagA, tagB)
	suggestMakeResource(t, ctx, "r2", &owner.ID, tagA)

	// target already has alpha.
	target := suggestMakeResource(t, ctx, "target", &owner.ID, tagA)

	got, err := ctx.GetSuggestedTags(target.ID, 8)
	if err != nil {
		t.Fatalf("GetSuggestedTags: %v", err)
	}
	if suggestHas(got, "alpha") {
		t.Fatalf("already-applied tag alpha must be excluded, got %v", suggestNames(got))
	}
	if !suggestHas(got, "beta") {
		t.Fatalf("expected beta suggestion, got %v", suggestNames(got))
	}
}

// TestSuggestedTags_CapHonored: limit truncates the result to the top N.
func TestSuggestedTags_CapHonored(t *testing.T) {
	ctx := newSuggestTestContext(t)
	owner := suggestMakeGroup(t, ctx, "owner", nil)
	tags := []*models.Tag{}
	for i := 0; i < 6; i++ {
		tags = append(tags, suggestMakeTag(t, ctx, fmt.Sprintf("t%d", i)))
	}
	// Give descending usage counts so ordering is deterministic.
	for i, tg := range tags {
		for j := 0; j <= (6 - i); j++ {
			suggestMakeResource(t, ctx, fmt.Sprintf("r%d-%d", i, j), &owner.ID, tg)
		}
	}
	target := suggestMakeResource(t, ctx, "target", &owner.ID)

	got, err := ctx.GetSuggestedTags(target.ID, 2)
	if err != nil {
		t.Fatalf("GetSuggestedTags: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("cap not honored: expected 2, got %d (%v)", len(got), suggestNames(got))
	}
	if got[0].Name != "t0" || got[1].Name != "t1" {
		t.Fatalf("expected top-2 t0,t1, got %v", suggestNames(got))
	}
}

// TestSuggestedTags_SimilarSource: with an ownerless target, suggestions are
// drawn from perceptual-hash-similar resources, ranked by tag frequency.
func TestSuggestedTags_SimilarSource(t *testing.T) {
	ctx := newSuggestTestContext(t)
	tagX := suggestMakeTag(t, ctx, "xray")
	tagY := suggestMakeTag(t, ctx, "yankee")

	target := suggestMakeResource(t, ctx, "target", nil) // ownerless → group source skipped
	sim1 := suggestMakeResource(t, ctx, "sim1", nil, tagX, tagY)
	sim2 := suggestMakeResource(t, ctx, "sim2", nil, tagX)
	suggestLinkSimilar(t, ctx, target.ID, sim1.ID, 2)
	suggestLinkSimilar(t, ctx, target.ID, sim2.ID, 4)

	got, err := ctx.GetSuggestedTags(target.ID, 8)
	if err != nil {
		t.Fatalf("GetSuggestedTags: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 suggestions from similar resources, got %v", suggestNames(got))
	}
	// xray appears on 2 similar resources, yankee on 1 → xray ranks first.
	if got[0].Name != "xray" {
		t.Fatalf("expected xray first (freq 2), got %v", suggestNames(got))
	}
	if got[0].Sources[0] != "similar" {
		t.Fatalf("expected similar source, got %+v", got[0])
	}
}

// TestSuggestedTags_EmptyDegradesGracefully: a target with no owner, no
// similarity rows, and no hash returns an empty slice and no error.
func TestSuggestedTags_EmptyDegradesGracefully(t *testing.T) {
	ctx := newSuggestTestContext(t)
	target := suggestMakeResource(t, ctx, "target", nil)

	got, err := ctx.GetSuggestedTags(target.ID, 8)
	if err != nil {
		t.Fatalf("GetSuggestedTags should not error on empty: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty suggestions, got %v", suggestNames(got))
	}
}

// TestSimilarResources_LimitBoundsFetch: the limited variant caps how many
// similar resources are actually loaded (not just scored in memory), returning
// the N nearest by Hamming distance, while the unlimited public method returns
// every match. This is the contract GetSuggestedTags relies on to keep a
// resource in a large near-duplicate cluster from loading (and tag-preloading)
// every similar row.
func TestSimilarResources_LimitBoundsFetch(t *testing.T) {
	ctx := newSuggestTestContext(t)
	target := suggestMakeResource(t, ctx, "target", nil)

	// Read-time filtering (image similarity v2) only surfaces pairs within the
	// default threshold (10), so every seeded distance must be within 0..10.
	// Distances cycle 0..10 (11 buckets of 5), giving suggestedTagsMaxSimilar+5
	// resources. ORDER BY dist ASC + LIMIT cap therefore drops exactly the five
	// distance-10 rows, which keeps the exclusion deterministic at the distance
	// level despite ties.
	const buckets = 11 // distances 0..10, all within the read threshold
	total := suggestedTagsMaxSimilar + 5
	sims := make([]*models.Resource, total)
	dist := make([]uint8, total)
	for i := 0; i < total; i++ {
		dist[i] = uint8(i % buckets)
		sims[i] = suggestMakeResource(t, ctx, fmt.Sprintf("sim%d", i), nil)
		suggestLinkSimilar(t, ctx, target.ID, sims[i].ID, dist[i])
	}

	// Unlimited: every similar resource is returned (all within threshold).
	all, err := ctx.GetSimilarResources(target.ID)
	if err != nil {
		t.Fatalf("GetSimilarResources: %v", err)
	}
	if len(all) != total {
		t.Fatalf("unlimited should return all %d similar resources, got %d", total, len(all))
	}

	// Limited: exactly the cap, dropping the highest-distance rows. With
	// total-cap == 5 and exactly 5 rows at distance 10, the dropped set is the
	// distance-10 group; every distance 0..9 row is retained.
	limited, err := ctx.getSimilarResourcesLimited(target.ID, suggestedTagsMaxSimilar)
	if err != nil {
		t.Fatalf("getSimilarResourcesLimited: %v", err)
	}
	if len(limited) != suggestedTagsMaxSimilar {
		t.Fatalf("limited fetch should return exactly %d, got %d", suggestedTagsMaxSimilar, len(limited))
	}
	got := make(map[uint]struct{}, len(limited))
	for _, r := range limited {
		got[r.ID] = struct{}{}
		// Every returned resource carries its perceptual distance.
		if r.SimilarityDistance == nil {
			t.Errorf("similar resource %d missing SimilarityDistance", r.ID)
		}
	}
	for i := 0; i < total; i++ {
		_, present := got[sims[i].ID]
		if dist[i] < 10 && !present {
			t.Fatalf("resource sim%d (distance %d) should be within the limited fetch", i, dist[i])
		}
		if dist[i] == 10 && present {
			t.Fatalf("resource sim%d (distance 10) should be excluded by the cap", i)
		}
	}
}

// TestSuggestedTags_ScopedPrincipalConfined: a group-limited principal is 404'd
// on an out-of-subtree resource and never receives tags sourced from
// out-of-subtree similar resources.
func TestSuggestedTags_ScopedPrincipalConfined(t *testing.T) {
	ctx := newSuggestTestContext(t)
	root := suggestMakeGroup(t, ctx, "root", nil)
	child := suggestMakeGroup(t, ctx, "child", &root.ID)
	outside := suggestMakeGroup(t, ctx, "outside", nil)

	tagIn := suggestMakeTag(t, ctx, "insider")
	tagOut := suggestMakeTag(t, ctx, "outsider")

	target := suggestMakeResource(t, ctx, "target", &child.ID)
	simIn := suggestMakeResource(t, ctx, "simIn", &child.ID, tagIn)
	simOut := suggestMakeResource(t, ctx, "simOut", &outside.ID, tagOut)
	outsideRes := suggestMakeResource(t, ctx, "outsideRes", &outside.ID, tagOut)

	suggestLinkSimilar(t, ctx, target.ID, simIn.ID, 2)
	suggestLinkSimilar(t, ctx, target.ID, simOut.ID, 3)

	scoped := ctx.WithPrincipal(&auth.Principal{Role: models.RoleUser, ScopeGroupID: &root.ID})

	// (i) out-of-subtree resource → error.
	if _, err := scoped.GetSuggestedTags(outsideRes.ID, 8); err == nil {
		t.Fatalf("scoped principal should be denied an out-of-subtree resource")
	}

	// (ii) in-subtree target → only in-subtree-sourced tags.
	got, err := scoped.GetSuggestedTags(target.ID, 8)
	if err != nil {
		t.Fatalf("scoped GetSuggestedTags(in-subtree): %v", err)
	}
	if !suggestHas(got, "insider") {
		t.Fatalf("expected insider tag from in-subtree similar resource, got %v", suggestNames(got))
	}
	if suggestHas(got, "outsider") {
		t.Fatalf("scoped principal must not receive out-of-subtree-sourced tag, got %v", suggestNames(got))
	}
}
