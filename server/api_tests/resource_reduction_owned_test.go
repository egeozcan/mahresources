package api_tests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"mahresources/application_context"
	"mahresources/models"
	"mahresources/models/query_models"
)

func TestResourceReductionOwnedSelection(t *testing.T) {
	tc := SetupTestEnv(t)
	root := &models.Group{Name: "Root"}
	require.NoError(t, tc.DB.Create(root).Error)
	child := &models.Group{Name: "Child", OwnerId: &root.ID}
	require.NoError(t, tc.DB.Create(child).Error)
	grandchild := &models.Group{Name: "Grandchild", OwnerId: &child.ID}
	require.NoError(t, tc.DB.Create(grandchild).Error)
	outside := &models.Group{Name: "Outside"}
	require.NoError(t, tc.DB.Create(outside).Error)

	// More than the detail preview or a standard list page can hold.
	var direct []uint
	for i := 0; i < 70; i++ {
		r := &models.Resource{Name: "Direct", OwnerId: &root.ID}
		require.NoError(t, tc.DB.Create(r).Error)
		direct = append(direct, r.ID)
	}
	var descendants []uint
	for _, group := range []*models.Group{child, grandchild} {
		r := &models.Resource{Name: "Descendant", OwnerId: &group.ID}
		require.NoError(t, tc.DB.Create(r).Error)
		descendants = append(descendants, r.ID)
	}
	related := &models.Resource{Name: "Related only", OwnerId: &outside.ID}
	require.NoError(t, tc.DB.Create(related).Error)
	require.NoError(t, tc.DB.Model(root).Association("RelatedResources").Append(related))

	ctx := scopedTo(t, tc, root.ID)
	red, err := ctx.CreateOrExtendResourceReduction(&query_models.ResourceReductionCreator{OwnerId: root.ID}, nil, false)
	require.NoError(t, err)
	extent, err := application_context.DecodeReductionExtent(red.Extent)
	require.NoError(t, err)
	assert.ElementsMatch(t, direct, extent.ResourceIDs)
	assert.Empty(t, extent.GroupIDs)

	// The same control can widen an existing reduction; overlapping selections
	// remain a set and related resources owned elsewhere stay outside it.
	widened, err := ctx.CreateOrExtendResourceReduction(&query_models.ResourceReductionCreator{
		ID: red.ID, OwnerId: root.ID, IncludeDescendants: true,
	}, nil, false)
	require.NoError(t, err)
	assert.Equal(t, red.ID, widened.ID)
	extent, err = application_context.DecodeReductionExtent(widened.Extent)
	require.NoError(t, err)
	assert.ElementsMatch(t, append(direct, descendants...), extent.ResourceIDs)
	assert.Empty(t, extent.GroupIDs)

	for _, subtree := range []bool{false, true} {
		_, err := ctx.CreateOrExtendResourceReduction(&query_models.ResourceReductionCreator{
			OwnerId: outside.ID, IncludeDescendants: subtree,
		}, nil, false)
		require.Error(t, err, "an inaccessible root must never be traversed")
	}
}

func TestResourceReductionOwnedSelectionEmptyParent(t *testing.T) {
	tc := SetupTestEnv(t)
	root := &models.Group{Name: "Empty parent"}
	require.NoError(t, tc.DB.Create(root).Error)
	child := &models.Group{Name: "Child", OwnerId: &root.ID}
	require.NoError(t, tc.DB.Create(child).Error)
	r := &models.Resource{Name: "Child resource", OwnerId: &child.ID}
	require.NoError(t, tc.DB.Create(r).Error)

	_, err := tc.AppCtx.CreateOrExtendResourceReduction(&query_models.ResourceReductionCreator{OwnerId: root.ID}, nil, false)
	require.ErrorContains(t, err, "no owned Resources")
	red, err := tc.AppCtx.CreateOrExtendResourceReduction(&query_models.ResourceReductionCreator{
		OwnerId: root.ID, IncludeDescendants: true,
	}, nil, false)
	require.NoError(t, err)
	extent, err := application_context.DecodeReductionExtent(red.Extent)
	require.NoError(t, err)
	assert.Equal(t, []uint{r.ID}, extent.ResourceIDs)
}
