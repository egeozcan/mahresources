package api_tests

import (
	"mahresources/models"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDuplicateGroupCopiesRelationships verifies that DuplicateGroup copies
// the group's outgoing Relationships (GroupRelation records where the group
// is the FromGroup) to the new duplicate group.
//
// Bug: DuplicateGroup loads all associations via clause.Associations (which
// includes Relationships and BackRelations), but the new Group struct only
// copies RelatedResources, RelatedNotes, RelatedGroups, and Tags. The
// Relationships slice is silently dropped, so the duplicate loses all typed
// group relations.
func TestDuplicateGroupCopiesRelationships(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create a category (required for relation types)
	cat := &models.Category{Name: "DupRelCat"}
	tc.DB.Create(cat)

	// Create a relation type
	relType := &models.GroupRelationType{
		Name:           "depends-on",
		FromCategoryId: &cat.ID,
		ToCategoryId:   &cat.ID,
	}
	tc.DB.Create(relType)

	// Create groups: original and a target
	original := &models.Group{Name: "Original Group", CategoryId: &cat.ID}
	tc.DB.Create(original)

	target := &models.Group{Name: "Target Group", CategoryId: &cat.ID}
	tc.DB.Create(target)

	// Create a GroupRelation: original -> target
	relation := &models.GroupRelation{
		FromGroupId:    &original.ID,
		ToGroupId:      &target.ID,
		RelationTypeId: &relType.ID,
		Name:           "critical dependency",
		Description:    "must be resolved first",
	}
	tc.DB.Create(relation)
	require.NotZero(t, relation.ID, "relation should be created")

	// Verify the relation exists on the original
	var origRelations []models.GroupRelation
	tc.DB.Where("from_group_id = ?", original.ID).Find(&origRelations)
	require.Len(t, origRelations, 1, "original should have 1 outgoing relation")

	// Duplicate the group
	duplicate, err := tc.AppCtx.DuplicateGroup(original.ID)
	require.NoError(t, err, "DuplicateGroup should succeed")
	require.NotNil(t, duplicate)
	require.NotEqual(t, original.ID, duplicate.ID, "duplicate should have a new ID")

	// Verify: the duplicate should have an outgoing relationship
	// that mirrors the original's relationship (from duplicate -> target)
	var dupRelations []models.GroupRelation
	tc.DB.Where("from_group_id = ?", duplicate.ID).Find(&dupRelations)

	assert.Len(t, dupRelations, 1,
		"DuplicateGroup should copy outgoing Relationships to the duplicate; "+
			"currently the duplicate has %d relations instead of 1", len(dupRelations))

	if len(dupRelations) == 1 {
		assert.Equal(t, target.ID, *dupRelations[0].ToGroupId,
			"duplicated relation should point to the same target group")
		assert.Equal(t, relType.ID, *dupRelations[0].RelationTypeId,
			"duplicated relation should have the same relation type")
	}
}

// TestDuplicateGroupCopiesBackRelations verifies that DuplicateGroup copies
// incoming BackRelations (GroupRelation records where the group is the ToGroup)
// to the new duplicate group.
func TestDuplicateGroupCopiesBackRelations(t *testing.T) {
	tc := SetupTestEnv(t)

	cat := &models.Category{Name: "DupBackRelCat"}
	tc.DB.Create(cat)

	relType := &models.GroupRelationType{
		Name:           "linked-to",
		FromCategoryId: &cat.ID,
		ToCategoryId:   &cat.ID,
	}
	tc.DB.Create(relType)

	source := &models.Group{Name: "Source Group", CategoryId: &cat.ID}
	tc.DB.Create(source)

	original := &models.Group{Name: "Original Target", CategoryId: &cat.ID}
	tc.DB.Create(original)

	// Create a GroupRelation: source -> original (so original has a back-relation)
	relation := &models.GroupRelation{
		FromGroupId:    &source.ID,
		ToGroupId:      &original.ID,
		RelationTypeId: &relType.ID,
		Name:           "incoming link",
	}
	tc.DB.Create(relation)

	// Verify back-relation exists
	var origBack []models.GroupRelation
	tc.DB.Where("to_group_id = ?", original.ID).Find(&origBack)
	require.Len(t, origBack, 1, "original should have 1 incoming relation")

	// Duplicate
	duplicate, err := tc.AppCtx.DuplicateGroup(original.ID)
	require.NoError(t, err)
	require.NotNil(t, duplicate)

	// The duplicate should also have a back-relation from source
	var dupBack []models.GroupRelation
	tc.DB.Where("to_group_id = ?", duplicate.ID).Find(&dupBack)

	assert.Len(t, dupBack, 1,
		"DuplicateGroup should copy incoming BackRelations to the duplicate; "+
			"currently the duplicate has %d back-relations instead of 1", len(dupBack))

	if len(dupBack) == 1 {
		assert.Equal(t, source.ID, *dupBack[0].FromGroupId,
			"duplicated back-relation should come from the same source group")
	}
}
