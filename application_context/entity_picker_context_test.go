package application_context

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"mahresources/auth"
	"mahresources/models"
	"mahresources/models/query_models"
)

// Private per-connection DB with one connection: do not pollute the older
// createTestContext helper's shared database with pagination fixtures.
func newPickerTestContext(t *testing.T) *MahresourcesContext {
	t.Helper()
	ctx := newScopingTestContext(t)
	require.NoError(t, ctx.db.AutoMigrate(&models.Query{}, &models.Tag{}, &models.NoteType{}, &models.Series{}, &models.GroupRelationType{}))
	db, err := ctx.db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return ctx
}

func TestEntityPickerPaginationIsBoundedAndStable(t *testing.T) {
	ctx := newPickerTestContext(t)
	var ids []uint
	for i := 0; i < 51; i++ {
		group := models.Group{Name: "Identical name"}
		require.NoError(t, ctx.db.Create(&group).Error)
		ids = append(ids, group.ID)
	}
	first, err := ctx.BrowseEntities(&query_models.EntityPickerQuery{Entity: "group", Filter: "name=Identical&MaxResults=999999&SortBy=random()"})
	require.NoError(t, err)
	require.Len(t, first.Items, 50)
	require.Equal(t, 1, first.Page)
	require.True(t, first.HasNext)
	for i, item := range first.Items {
		require.Equal(t, ids[i], item.ID)
	}
	second, err := ctx.BrowseEntities(&query_models.EntityPickerQuery{Entity: "group", Filter: "Name=Identical", Page: 2})
	require.NoError(t, err)
	require.Len(t, second.Items, 1)
	require.Equal(t, ids[50], second.Items[0].ID)
	require.False(t, second.HasNext)
}

func TestEntityPickerConstraintsIntersectRatherThanWiden(t *testing.T) {
	ctx := newPickerTestContext(t)
	a, b := models.Category{Name: "A"}, models.Category{Name: "B"}
	require.NoError(t, ctx.db.Create(&a).Error)
	require.NoError(t, ctx.db.Create(&b).Error)
	left, right := models.Group{Name: "Same", CategoryId: &a.ID}, models.Group{Name: "Same", CategoryId: &b.ID}
	require.NoError(t, ctx.db.Create(&left).Error)
	require.NoError(t, ctx.db.Create(&right).Error)
	page, err := ctx.BrowseEntities(&query_models.EntityPickerQuery{Entity: "group",
		Filter: fmt.Sprintf("Categories=%d&Categories=%d", a.ID, b.ID), Constraints: fmt.Sprintf("Categories=%d", a.ID)})
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	require.Equal(t, left.ID, page.Items[0].ID)
	page, err = ctx.BrowseEntities(&query_models.EntityPickerQuery{Entity: "group",
		Filter: fmt.Sprintf("Categories=%d", b.ID), Constraints: fmt.Sprintf("Categories=%d", a.ID)})
	require.NoError(t, err)
	require.Empty(t, page.Items)
}

func TestEntityPickerResolveAndHydrationKeepPrincipalScope(t *testing.T) {
	ctx := newPickerTestContext(t)
	root, outside := models.Group{Name: "Inside"}, models.Group{Name: "Outside"}
	require.NoError(t, ctx.db.Create(&root).Error)
	require.NoError(t, ctx.db.Create(&outside).Error)
	note := models.Note{Name: "Visible", OwnerId: &root.ID}
	hidden := models.Note{Name: "Hidden", OwnerId: &outside.ID}
	require.NoError(t, ctx.db.Create(&note).Error)
	require.NoError(t, ctx.db.Create(&hidden).Error)
	scoped := ctx.WithPrincipal(&auth.Principal{UserID: 1, Role: models.RoleUser, ScopeGroupID: &root.ID})
	page, err := scoped.BrowseEntities(&query_models.EntityPickerQuery{Entity: "note"})
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	require.Equal(t, note.ID, page.Items[0].ID)
	raw := page.Items[0].Raw.(models.Note)
	require.NotNil(t, raw.Owner)
	require.Equal(t, root.ID, raw.Owner.ID)
	rows, err := scoped.ResolvePickerEntities(&query_models.EntityPickerResolveQuery{Entity: "note", IDs: []uint{hidden.ID, note.ID}})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, note.ID, rows[0].ID)
	require.NoError(t, ctx.db.Model(&note).Update("owner_id", outside.ID).Error)
	rows, err = scoped.ResolvePickerEntities(&query_models.EntityPickerResolveQuery{Entity: "note", IDs: []uint{note.ID}})
	require.NoError(t, err)
	require.Empty(t, rows)
}

func TestEntityPickerResourceMetadataAndNoteAssociationFilters(t *testing.T) {
	ctx := newPickerTestContext(t)
	note := models.Note{Name: "Related note"}
	require.NoError(t, ctx.db.Create(&note).Error)
	large := models.Resource{Name: "Large", Meta: []byte(`{"size":2000}`)}
	small := models.Resource{Name: "Small", Meta: []byte(`{"size":20}`)}
	require.NoError(t, ctx.db.Create(&large).Error)
	require.NoError(t, ctx.db.Create(&small).Error)
	require.NoError(t, ctx.db.Model(&note).Association("Resources").Append(&large, &small))
	page, err := ctx.BrowseEntities(&query_models.EntityPickerQuery{Entity: "resource",
		Filter: "MetaQuery=size:GT:1000", Constraints: fmt.Sprintf("Notes=%d", note.ID)})
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	require.Equal(t, large.ID, page.Items[0].ID)
}

func TestEntityPickerEveryCatalogTypeCanBrowseAndResolve(t *testing.T) {
	ctx := newPickerTestContext(t)
	for _, test := range []struct {
		entity string
		row    any
	}{
		{"category", &models.Category{Name: "Picker catalog"}},
		{"noteType", &models.NoteType{Name: "Picker catalog"}},
		{"resourceCategory", &models.ResourceCategory{Name: "Picker catalog"}},
		{"tag", &models.Tag{Name: "Picker catalog"}},
		{"query", &models.Query{Name: "Picker catalog"}},
		{"series", &models.Series{Name: "Picker catalog", Slug: "picker-catalog"}},
		{"relationType", &models.GroupRelationType{Name: "Picker catalog"}},
		{"resource", &models.Resource{Name: "Picker catalog"}},
		{"note", &models.Note{Name: "Picker catalog"}},
		{"group", &models.Group{Name: "Picker catalog"}},
	} {
		t.Run(test.entity, func(t *testing.T) {
			require.NoError(t, ctx.db.Create(test.row).Error)
			page, err := ctx.BrowseEntities(&query_models.EntityPickerQuery{Entity: test.entity, Filter: "Name=Picker catalog"})
			require.NoError(t, err)
			require.Len(t, page.Items, 1)
			require.Equal(t, "Picker catalog", page.Items[0].Name)
			resolved, err := ctx.ResolvePickerEntities(&query_models.EntityPickerResolveQuery{Entity: test.entity, IDs: []uint{page.Items[0].ID}})
			require.NoError(t, err)
			require.Len(t, resolved, 1)
			require.Equal(t, page.Items[0].ID, resolved[0].ID)
		})
	}
}

func TestEntityPickerPaginationDoesNotCountInvisibleRows(t *testing.T) {
	ctx := newPickerTestContext(t)
	root, outside := models.Group{Name: "Inside"}, models.Group{Name: "Outside"}
	require.NoError(t, ctx.db.Create(&root).Error)
	require.NoError(t, ctx.db.Create(&outside).Error)
	for i := 0; i < 51; i++ {
		require.NoError(t, ctx.db.Create(&models.Note{Name: "Hidden", OwnerId: &outside.ID}).Error)
	}
	visible := models.Note{Name: "Visible after hidden rows", OwnerId: &root.ID}
	require.NoError(t, ctx.db.Create(&visible).Error)
	scoped := ctx.WithPrincipal(&auth.Principal{UserID: 1, Role: models.RoleGuest, ScopeGroupID: &root.ID})
	page, err := scoped.BrowseEntities(&query_models.EntityPickerQuery{Entity: "note"})
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	require.Equal(t, visible.ID, page.Items[0].ID)
	require.False(t, page.HasNext, "invisible rows must not contribute to continuation")
}

func TestEntityPickerUsesTheCallersTransaction(t *testing.T) {
	ctx := newPickerTestContext(t)
	rollback := errors.New("roll back fixture")
	err := ctx.WithTransaction(func(tx *MahresourcesContext) error {
		row := models.Group{Name: "Uncommitted picker entity"}
		require.NoError(t, tx.db.Create(&row).Error)
		page, err := tx.BrowseEntities(&query_models.EntityPickerQuery{Entity: "group", Filter: "Name=Uncommitted"})
		require.NoError(t, err)
		require.Len(t, page.Items, 1)
		require.Equal(t, row.ID, page.Items[0].ID)
		resolved, err := tx.ResolvePickerEntities(&query_models.EntityPickerResolveQuery{Entity: "group", IDs: []uint{row.ID}})
		require.NoError(t, err)
		require.Len(t, resolved, 1)
		return rollback
	})
	require.ErrorIs(t, err, rollback)
	page, err := ctx.BrowseEntities(&query_models.EntityPickerQuery{Entity: "group", Filter: "Name=Uncommitted"})
	require.NoError(t, err)
	require.Empty(t, page.Items)
}

func TestEntityPickerResolvePreservesOrderConstraintsAndFullNames(t *testing.T) {
	ctx := newPickerTestContext(t)
	prefix := strings.Repeat("Long identical prefix ", 20)
	first, second := models.Group{Name: prefix + "first"}, models.Group{Name: prefix + "second"}
	require.NoError(t, ctx.db.Create(&first).Error)
	require.NoError(t, ctx.db.Create(&second).Error)
	rows, err := ctx.ResolvePickerEntities(&query_models.EntityPickerResolveQuery{Entity: "group", IDs: []uint{second.ID, first.ID}})
	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.Equal(t, second.Name, rows[0].Name)
	require.Equal(t, first.Name, rows[1].Name)
	rows, err = ctx.ResolvePickerEntities(&query_models.EntityPickerResolveQuery{Entity: "group", IDs: []uint{second.ID, first.ID}, Constraints: "Name=first"})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, first.ID, rows[0].ID)
	require.NoError(t, ctx.db.Delete(&first).Error)
	rows, err = ctx.ResolvePickerEntities(&query_models.EntityPickerResolveQuery{Entity: "group", IDs: []uint{first.ID}})
	require.NoError(t, err)
	require.Empty(t, rows)
}

func TestEntityPickerRejectsInvalidInputs(t *testing.T) {
	ctx := newPickerTestContext(t)
	for _, q := range []*query_models.EntityPickerQuery{nil, {Entity: "user"}, {Entity: "group", Page: -1},
		{Entity: "group", Page: math.MaxInt}, {Entity: "resource", Filter: "OwnerId=oops"}, {Entity: "group", Constraints: "%zz"}} {
		_, err := ctx.BrowseEntities(q)
		require.ErrorIs(t, err, query_models.ErrEntityPickerInput, "query: %#v", q)
	}
	for _, q := range []*query_models.EntityPickerResolveQuery{nil, {Entity: "group"}, {Entity: "group", IDs: []uint{0}},
		{Entity: "group", IDs: []uint{1, 1}}, {Entity: "group", IDs: make([]uint, 51)}} {
		_, err := ctx.ResolvePickerEntities(q)
		require.ErrorIs(t, err, query_models.ErrEntityPickerInput)
	}
}
