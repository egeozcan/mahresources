package application_context

import (
	"testing"

	"github.com/stretchr/testify/require"
	"mahresources/models"
	"mahresources/models/query_models"
	"mahresources/models/types"
)

// Shared resource cards need their carrier's visibility and custom templates,
// including cards reached through series and either similarity query path.
func TestCardCarrierResourceReaders(t *testing.T) {
	ctx := createTestContext(t)
	category := models.ResourceCategory{
		Name: "card resources", SectionConfig: types.JSON(`{"description":false}`),
		CustomSummary: "<p>Resource replacement</p>",
	}
	series := models.Series{Name: "card series", Slug: "card-series"}
	require.NoError(t, ctx.db.Create(&category).Error)
	require.NoError(t, ctx.db.Create(&series).Error)
	root := models.Resource{Name: "series root", ResourceCategoryId: category.ID, SeriesID: &series.ID}
	neighbor := models.Resource{Name: "series neighbor", ResourceCategoryId: category.ID, SeriesID: &series.ID}
	fallbackRoot := models.Resource{Name: "fallback root", ResourceCategoryId: category.ID}
	fallbackNeighbor := models.Resource{Name: "fallback neighbor", ResourceCategoryId: category.ID}
	for _, resource := range []*models.Resource{&root, &neighbor, &fallbackRoot, &fallbackNeighbor} {
		require.NoError(t, ctx.db.Create(resource).Error)
	}
	require.NoError(t, ctx.db.Create(&models.ResourceSimilarity{
		ResourceID1: root.ID, ResourceID2: neighbor.ID, HammingDistance: 1,
	}).Error)
	for _, id := range []uint{fallbackRoot.ID, fallbackNeighbor.ID} {
		require.NoError(t, ctx.db.Create(&models.ImageHash{ResourceId: &id, DHash: "abcdef"}).Error)
	}
	check := func(t *testing.T, resources []*models.Resource, want ...uint) {
		t.Helper()
		ids := make([]uint, len(resources))
		for i, resource := range resources {
			ids[i] = resource.ID
		}
		require.ElementsMatch(t, want, ids, "reader must return the seeded resources")
		for _, resource := range resources {
			require.NotNil(t, resource.ResourceCategory, "resource %d has no card carrier", resource.ID)
			require.Equal(t, category.ID, resource.ResourceCategory.ID)
			require.Equal(t, category.SectionConfig, resource.ResourceCategory.SectionConfig)
			require.Equal(t, category.CustomSummary, resource.ResourceCategory.CustomSummary)
		}
	}
	t.Run("series", func(t *testing.T) {
		got, err := ctx.GetSeries(series.ID)
		require.NoError(t, err)
		require.Equal(t, series.ID, got.ID)
		check(t, got.Resources, root.ID, neighbor.ID)
	})
	t.Run("series siblings", func(t *testing.T) {
		got, err := ctx.GetSeriesSiblings(root.ID, series.ID)
		require.NoError(t, err)
		check(t, got, neighbor.ID)
	})
	t.Run("precomputed similarity", func(t *testing.T) {
		got, err := ctx.GetSimilarResources(root.ID)
		require.NoError(t, err)
		check(t, got, neighbor.ID)
	})
	t.Run("same hash fallback", func(t *testing.T) {
		got, err := ctx.GetSimilarResources(fallbackRoot.ID)
		require.NoError(t, err)
		check(t, got, fallbackNeighbor.ID)
	})
}

// Owner and relation cards must use the endpoint group's own category rather
// than resolving visible defaults because only the group row was preloaded.
func TestCardCarrierGroupReaders(t *testing.T) {
	ctx := createTestContext(t)
	fromCategory := models.Category{
		Name: "from card category", SectionConfig: types.JSON(`{"timestamps":false}`),
		CustomSummary: "<p>From replacement</p>",
	}
	toCategory := models.Category{
		Name: "to card category", SectionConfig: types.JSON(`{"description":false}`),
		CustomSummary: "<p>To replacement</p>",
	}
	require.NoError(t, ctx.db.Create(&fromCategory).Error)
	require.NoError(t, ctx.db.Create(&toCategory).Error)
	from := models.Group{Name: "from card", CategoryId: &fromCategory.ID}
	to := models.Group{Name: "to card", CategoryId: &toCategory.ID}
	require.NoError(t, ctx.db.Create(&from).Error)
	require.NoError(t, ctx.db.Create(&to).Error)
	resource := models.Resource{Name: "owned card", OwnerId: &from.ID}
	require.NoError(t, ctx.db.Create(&resource).Error)
	relationType := models.GroupRelationType{Name: "card relation type"}
	require.NoError(t, ctx.db.Create(&relationType).Error)
	relation := models.GroupRelation{
		FromGroupId: &from.ID, ToGroupId: &to.ID, RelationTypeId: &relationType.ID,
	}
	require.NoError(t, ctx.db.Create(&relation).Error)
	check := func(t *testing.T, group *models.Group, wantID uint, category *models.Category) {
		t.Helper()
		require.NotNil(t, group, "reader must return the seeded group")
		require.Equal(t, wantID, group.ID)
		require.NotNil(t, group.Category, "group %d has no card carrier", group.ID)
		require.Equal(t, category.ID, group.Category.ID)
		require.Equal(t, category.SectionConfig, group.Category.SectionConfig)
		require.Equal(t, category.CustomSummary, group.Category.CustomSummary)
	}
	t.Run("resource owner", func(t *testing.T) {
		got, err := ctx.GetResource(resource.ID)
		require.NoError(t, err)
		require.Equal(t, resource.ID, got.ID)
		check(t, got.Owner, from.ID, &fromCategory)
	})
	for _, endpoint := range []string{"from", "to"} {
		t.Run("relation "+endpoint, func(t *testing.T) {
			got, err := ctx.GetRelations(0, 10, &query_models.GroupRelationshipQuery{FromGroupId: from.ID})
			require.NoError(t, err)
			require.Len(t, got, 1)
			require.Equal(t, relation.ID, got[0].ID)
			if endpoint == "from" {
				check(t, got[0].FromGroup, from.ID, &fromCategory)
			} else {
				check(t, got[0].ToGroup, to.ID, &toCategory)
			}
		})
	}
}
