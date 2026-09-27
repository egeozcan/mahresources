package application_context

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"mahresources/models"
	"mahresources/models/query_models"
)

// TestEditRelationType_ChangeCategory_LeavesInconsistentRelations verifies that
// changing a RelationType's FromCategoryId or ToCategoryId leaves existing
// GroupRelation records in an inconsistent state: the groups' categories no
// longer match the relation type's category constraints.
//
// Bug: EditRelationType allows changing FromCategoryId/ToCategoryId without
// checking or updating existing GroupRelation records that use this type.
// After the edit, AddRelation would reject new relations of the same type
// for the same groups (category mismatch), yet old relations remain with
// mismatched categories.
func TestEditRelationType_ChangeCategory_LeavesInconsistentRelations(t *testing.T) {
	ctx := createTestContext(t)

	// Create two categories
	catA := &models.Category{Name: "Category A"}
	catB := &models.Category{Name: "Category B"}
	catC := &models.Category{Name: "Category C"}
	ctx.db.Create(catA)
	ctx.db.Create(catB)
	ctx.db.Create(catC)

	// Create two groups: groupFrom is in catA, groupTo is in catB
	groupFrom := &models.Group{Name: "From Group", CategoryId: &catA.ID}
	groupTo := &models.Group{Name: "To Group", CategoryId: &catB.ID}
	ctx.db.Create(groupFrom)
	ctx.db.Create(groupTo)

	// Create a relation type: catA -> catB
	relType, err := ctx.AddRelationType(&query_models.RelationshipTypeEditorQuery{
		Name:         "test-relation",
		FromCategory: catA.ID,
		ToCategory:   catB.ID,
	})
	if err != nil {
		t.Fatalf("AddRelationType failed: %v", err)
	}

	// Add a relation from groupFrom (catA) to groupTo (catB) — this should succeed
	relation, err := ctx.AddRelation(groupFrom.ID, groupTo.ID, relType.ID, "test", "test relation")
	if err != nil {
		t.Fatalf("AddRelation failed: %v", err)
	}

	// Verify the relation exists
	if relation.ID == 0 {
		t.Fatal("Expected relation to be created with non-zero ID")
	}

	// Now change the relation type's FromCategory from catA to catC
	_, err = ctx.EditRelationType(&query_models.RelationshipTypeEditorQuery{
		Id:           relType.ID,
		FromCategory: catC.ID, // Changed from catA to catC
	})
	if err != nil {
		t.Fatalf("EditRelationType failed: %v", err)
	}

	// Reload the relation type to verify the change
	updatedRelType, err := ctx.GetRelationType(relType.ID)
	if err != nil {
		t.Fatalf("GetRelationType failed: %v", err)
	}
	if *updatedRelType.FromCategoryId != catC.ID {
		t.Fatalf("Expected FromCategoryId to be %d (catC), got %d", catC.ID, *updatedRelType.FromCategoryId)
	}

	// The existing relation pointed to groups in catA -> catB,
	// but the relation type now requires catC -> catB.
	// EditRelationType should have cleaned up the inconsistent relation.

	// The relation should have been cascade-deleted because its FromGroup's
	// category (catA) no longer matches the relation type's FromCategory (catC).
	_, err = ctx.GetRelation(relation.ID)
	if err == nil {
		t.Errorf("BUG: GroupRelation %d still exists after EditRelationType changed "+
			"FromCategory from catA to catC. The relation's FromGroup is in catA, "+
			"which no longer matches. EditRelationType should clean up inconsistent relations.",
			relation.ID)
	}
}

// TestAddingARelationIsNotRefusedByACommitBeforeItsWrite pins AddRelation's write
// order: every lookup the insert depends on happens before its transaction, whose
// first statement is the insert, so another connection committing in between cannot
// fail it with "database is locked". See commitBeforeEveryWriteTo.
func TestAddingARelationIsNotRefusedByACommitBeforeItsWrite(t *testing.T) {
	ctx := newWALTestContext(t, 0)
	from, to := &models.Category{Name: "From"}, &models.Category{Name: "To"}
	require.NoError(t, ctx.db.Create(from).Error)
	require.NoError(t, ctx.db.Create(to).Error)
	fromGroup := &models.Group{Name: "from", CategoryId: &from.ID}
	toGroup := &models.Group{Name: "to", CategoryId: &to.ID}
	require.NoError(t, ctx.db.Create(fromGroup).Error)
	require.NoError(t, ctx.db.Create(toGroup).Error)
	back := &models.GroupRelationType{Name: "is linked from", FromCategoryId: &to.ID, ToCategoryId: &from.ID}
	require.NoError(t, ctx.db.Create(back).Error)
	relationType := &models.GroupRelationType{Name: "links to", FromCategoryId: &from.ID, ToCategoryId: &to.ID, BackRelationId: &back.ID}
	require.NoError(t, ctx.db.Create(relationType).Error)

	fired := commitBeforeEveryWriteTo(t, ctx, "group_relations")
	relation, err := ctx.AddRelation(fromGroup.ID, toGroup.ID, relationType.ID, "linked", "")
	require.NoError(t, err)
	require.NotZero(t, fired.Load(), "the relation wrote nothing, so the interleave never happened")
	require.NotZero(t, relation.ID)

	var edges []models.GroupRelation
	require.NoError(t, ctx.db.Order("id").Find(&edges).Error)
	require.Len(t, edges, 2, "the relation and its back relation, once each")
	assert.Equal(t, relation.ID, edges[0].ID)
	assert.Equal(t, back.ID, *edges[1].RelationTypeId)
}

// TestARelationIsNotCreatedOverACategoryChangeThatLandsBeforeItsInsert pins that the
// category check holds for the rows the insert commits with, not only for the rows read
// before the transaction: a group moved to another category between those reads and the
// insert must refuse the relation rather than store an edge its type does not allow.
//
// The change commits right after the last of those reads. It cannot land any later:
// the insert's transaction holds the writer lock from its BEGIN.
func TestARelationIsNotCreatedOverACategoryChangeThatLandsBeforeItsInsert(t *testing.T) {
	ctx := newWALTestContext(t, 0)
	from, to, elsewhere := &models.Category{Name: "From"}, &models.Category{Name: "To"}, &models.Category{Name: "Elsewhere"}
	for _, category := range []*models.Category{from, to, elsewhere} {
		require.NoError(t, ctx.db.Create(category).Error)
	}
	fromGroup := &models.Group{Name: "from", CategoryId: &from.ID}
	toGroup := &models.Group{Name: "to", CategoryId: &to.ID}
	require.NoError(t, ctx.db.Create(fromGroup).Error)
	require.NoError(t, ctx.db.Create(toGroup).Error)
	relationType := &models.GroupRelationType{Name: "links to", FromCategoryId: &from.ID, ToCategoryId: &to.ID}
	require.NoError(t, ctx.db.Create(relationType).Error)

	sqlDB, err := ctx.db.DB()
	require.NoError(t, err)
	var once sync.Once
	const name = "test:recategorize_before_relation_insert"
	require.NoError(t, ctx.db.Callback().Query().After("gorm:query").Register(name, func(tx *gorm.DB) {
		// The relation type is the last row the check before the transaction reads.
		if _, inTransaction := tx.Statement.ConnPool.(gorm.TxCommitter); inTransaction || tx.Statement.Table != "group_relation_types" {
			return
		}
		once.Do(func() {
			if _, err := sqlDB.Exec(`UPDATE groups SET category_id = ? WHERE id = ?`, elsewhere.ID, fromGroup.ID); err != nil {
				t.Errorf("competing category change: %v", err)
			}
		})
	}))
	t.Cleanup(func() { _ = ctx.db.Callback().Query().Remove(name) })

	_, err = ctx.AddRelation(fromGroup.ID, toGroup.ID, relationType.ID, "linked", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "category mismatch")

	var edges int64
	require.NoError(t, ctx.db.Model(&models.GroupRelation{}).Count(&edges).Error)
	assert.Zero(t, edges, "no edge may survive the refused check")
}
