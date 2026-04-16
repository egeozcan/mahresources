package api_tests

import (
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"mahresources/models"
	"mahresources/models/query_models"
	template_context_providers "mahresources/server/template_handlers/template_context_providers"
)

func TestGroupCompareContextProvider_MissingG1(t *testing.T) {
	tc := SetupTestEnv(t)
	provider := template_context_providers.GroupCompareContextProvider(tc.AppCtx)

	req := httptest.NewRequest("GET", "/group/compare", nil)
	ctx := provider(req)

	assert.Equal(t, "Compare Groups", ctx["pageTitle"])
	assert.Equal(t, "Group 1 ID (g1) is required", ctx["errorMessage"])
}

func TestGroupCompareContextProvider_DefaultsG2ToG1(t *testing.T) {
	tc := SetupTestEnv(t)
	group := tc.CreateDummyGroup("Compare Me")
	provider := template_context_providers.GroupCompareContextProvider(tc.AppCtx)

	req := httptest.NewRequest("GET", fmt.Sprintf("/group/compare?g1=%d", group.ID), nil)
	ctx := provider(req)

	query, ok := ctx["query"].(query_models.CrossGroupCompareQuery)
	require.True(t, ok, "query should be present in template context")
	assert.Equal(t, group.ID, query.Group1ID)
	assert.Equal(t, group.ID, query.Group2ID)

	comparison, ok := ctx["comparison"].(*models.GroupComparison)
	require.True(t, ok, "comparison should be present in template context")
	assert.True(t, comparison.SameGroup)
	assert.False(t, comparison.HasDifferences)
}

func TestCompareGroupsCross_BuildsStructuredDiffs(t *testing.T) {
	tc := SetupTestEnv(t)

	group1 := tc.CreateDummyGroup("Group Left")
	group2 := tc.CreateDummyGroup("Group Right")

	tagLeft := &models.Tag{Name: "Tag Left"}
	tagShared := &models.Tag{Name: "Tag Shared"}
	tagRight := &models.Tag{Name: "Tag Right"}
	require.NoError(t, tc.DB.Create([]*models.Tag{tagLeft, tagShared, tagRight}).Error)
	require.NoError(t, tc.DB.Model(group1).Association("Tags").Append([]*models.Tag{tagLeft, tagShared}))
	require.NoError(t, tc.DB.Model(group2).Association("Tags").Append([]*models.Tag{tagShared, tagRight}))

	sharedRelated := tc.CreateDummyGroup("Related Shared")
	leftRelated := tc.CreateDummyGroup("Related Left")
	rightRelated := tc.CreateDummyGroup("Related Right")
	require.NoError(t, tc.DB.Model(group1).Association("RelatedGroups").Append([]*models.Group{sharedRelated, leftRelated}))
	require.NoError(t, tc.DB.Model(group2).Association("RelatedGroups").Append([]*models.Group{sharedRelated, rightRelated}))

	relType := &models.GroupRelationType{Name: "Depends On"}
	require.NoError(t, tc.DB.Create(relType).Error)

	sharedTarget := tc.CreateDummyGroup("Target Shared")
	leftTarget := tc.CreateDummyGroup("Target Left")
	rightTarget := tc.CreateDummyGroup("Target Right")
	require.NoError(t, tc.DB.Create([]*models.GroupRelation{
		{FromGroupId: UintPtr(group1.ID), ToGroupId: UintPtr(sharedTarget.ID), RelationTypeId: UintPtr(relType.ID), Name: "shared forward"},
		{FromGroupId: UintPtr(group2.ID), ToGroupId: UintPtr(sharedTarget.ID), RelationTypeId: UintPtr(relType.ID), Name: "shared forward"},
		{FromGroupId: UintPtr(group1.ID), ToGroupId: UintPtr(leftTarget.ID), RelationTypeId: UintPtr(relType.ID), Name: "left forward"},
		{FromGroupId: UintPtr(group2.ID), ToGroupId: UintPtr(rightTarget.ID), RelationTypeId: UintPtr(relType.ID), Name: "right forward"},
	}).Error)

	sharedSource := tc.CreateDummyGroup("Source Shared")
	leftSource := tc.CreateDummyGroup("Source Left")
	rightSource := tc.CreateDummyGroup("Source Right")
	require.NoError(t, tc.DB.Create([]*models.GroupRelation{
		{FromGroupId: UintPtr(sharedSource.ID), ToGroupId: UintPtr(group1.ID), RelationTypeId: UintPtr(relType.ID), Name: "shared reverse"},
		{FromGroupId: UintPtr(sharedSource.ID), ToGroupId: UintPtr(group2.ID), RelationTypeId: UintPtr(relType.ID), Name: "shared reverse"},
		{FromGroupId: UintPtr(leftSource.ID), ToGroupId: UintPtr(group1.ID), RelationTypeId: UintPtr(relType.ID), Name: "left reverse"},
		{FromGroupId: UintPtr(rightSource.ID), ToGroupId: UintPtr(group2.ID), RelationTypeId: UintPtr(relType.ID), Name: "right reverse"},
	}).Error)

	comparison, err := tc.AppCtx.CompareGroupsCross(group1.ID, group2.ID)
	require.NoError(t, err)

	assert.Equal(t, 1, comparison.Tags.SharedCount)
	assert.Equal(t, 1, comparison.Tags.OnlyLeftCount)
	assert.Equal(t, 1, comparison.Tags.OnlyRightCount)
	assert.Equal(t, "Tag Left", comparison.Tags.OnlyLeft[0].Label)
	assert.Equal(t, "Tag Right", comparison.Tags.OnlyRight[0].Label)

	assert.Equal(t, 1, comparison.RelatedGroups.SharedCount)
	assert.Equal(t, 1, comparison.RelatedGroups.OnlyLeftCount)
	assert.Equal(t, 1, comparison.RelatedGroups.OnlyRightCount)
	assert.Equal(t, "Related Left", comparison.RelatedGroups.OnlyLeft[0].Label)
	assert.Equal(t, "Related Right", comparison.RelatedGroups.OnlyRight[0].Label)

	assert.Equal(t, 1, comparison.ForwardRelations.SharedCount)
	assert.Equal(t, 1, comparison.ForwardRelations.OnlyLeftCount)
	assert.Equal(t, 1, comparison.ForwardRelations.OnlyRightCount)
	assert.Equal(t, "Depends On: Target Left", comparison.ForwardRelations.OnlyLeft[0].Label)
	assert.Equal(t, "Depends On: Target Right", comparison.ForwardRelations.OnlyRight[0].Label)

	assert.Equal(t, 1, comparison.ReverseRelations.SharedCount)
	assert.Equal(t, 1, comparison.ReverseRelations.OnlyLeftCount)
	assert.Equal(t, 1, comparison.ReverseRelations.OnlyRightCount)
	assert.Equal(t, "Depends On: Source Left", comparison.ReverseRelations.OnlyLeft[0].Label)
	assert.Equal(t, "Depends On: Source Right", comparison.ReverseRelations.OnlyRight[0].Label)
}

func TestCompareGroupsCross_NotTruncatedByDetailPreloadLimits(t *testing.T) {
	tc := SetupTestEnv(t)

	group1 := tc.CreateDummyGroup("Base Left")
	group2 := tc.CreateDummyGroup("Base Right")

	relatedGroups := make([]*models.Group, 0, 60)
	for i := 0; i < 60; i++ {
		related := tc.CreateDummyGroup(fmt.Sprintf("Related %02d", i))
		relatedGroups = append(relatedGroups, related)
	}

	require.NoError(t, tc.DB.Model(group1).Association("RelatedGroups").Append(relatedGroups))

	comparison, err := tc.AppCtx.CompareGroupsCross(group1.ID, group2.ID)
	require.NoError(t, err)

	assert.Equal(t, 60, comparison.RelatedGroups.TotalCount, "compare loader should not be limited to MaxResultsPerPage")
	assert.Equal(t, 60, comparison.RelatedGroups.OnlyLeftCount)
	assert.Equal(t, 0, comparison.RelatedGroups.OnlyRightCount)
}
