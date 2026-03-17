package api_tests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"mahresources/models"
)

// TestMergeGroupsNoDanglingRelatedGroupEntries verifies that when two losers
// have a group_related_groups entry between them (loserA -> loserB), merging
// both into a winner does not leave dangling rows in group_related_groups
// pointing to the now-deleted losers.
//
// The merge transfers loserA's outgoing relation (loserA -> loserB) as
// (winner -> loserB). When loserB is subsequently deleted, the reverse
// direction (related_group_id = loserB) must also be cleaned up. On SQLite
// inside a transaction, FK cascades don't fire, so explicit cleanup is needed.
func TestMergeGroupsNoDanglingRelatedGroupEntries(t *testing.T) {
	tc := SetupTestEnv(t)
	requireJsonPatch(t, tc.DB)

	// Create winner and two losers
	winner := &models.Group{Name: "Winner", Meta: []byte(`{}`)}
	tc.DB.Create(winner)
	loserA := &models.Group{Name: "Loser A", Meta: []byte(`{}`)}
	tc.DB.Create(loserA)
	loserB := &models.Group{Name: "Loser B", Meta: []byte(`{}`)}
	tc.DB.Create(loserB)

	// Create a group_related_groups entry: loserA -> loserB
	tc.DB.Exec("INSERT INTO group_related_groups (group_id, related_group_id) VALUES (?, ?)",
		loserA.ID, loserB.ID)

	// Verify the entry exists
	var countBefore int64
	tc.DB.Raw("SELECT COUNT(*) FROM group_related_groups WHERE group_id = ? AND related_group_id = ?",
		loserA.ID, loserB.ID).Scan(&countBefore)
	assert.Equal(t, int64(1), countBefore, "setup: loserA -> loserB relation should exist")

	// Merge both losers into winner
	err := tc.AppCtx.MergeGroups(winner.ID, []uint{loserA.ID, loserB.ID})
	assert.NoError(t, err)

	// Both losers should be deleted
	var loserACheck, loserBCheck models.Group
	assert.Error(t, tc.DB.First(&loserACheck, loserA.ID).Error, "loserA should be deleted")
	assert.Error(t, tc.DB.First(&loserBCheck, loserB.ID).Error, "loserB should be deleted")

	// There should be NO dangling entries in group_related_groups pointing to deleted losers
	var danglingCount int64
	tc.DB.Raw(`
		SELECT COUNT(*) FROM group_related_groups
		WHERE related_group_id NOT IN (SELECT id FROM groups)
		   OR group_id NOT IN (SELECT id FROM groups)
	`).Scan(&danglingCount)

	assert.Equal(t, int64(0), danglingCount,
		"After merging groups where losers are related to each other, there should be no dangling "+
			"entries in group_related_groups pointing to deleted groups. The merge transfers "+
			"(loserA -> loserB) as (winner -> loserB), but when loserB is deleted, "+
			"(winner -> loserB) must also be removed.")

	// Also verify winner's related groups are all valid (exist in the groups table)
	var winnerRelatedCount int64
	tc.DB.Raw("SELECT COUNT(*) FROM group_related_groups WHERE group_id = ?", winner.ID).Scan(&winnerRelatedCount)

	if winnerRelatedCount > 0 {
		// Every related_group_id for the winner should reference an existing group
		var invalidRelated int64
		tc.DB.Raw(`
			SELECT COUNT(*) FROM group_related_groups
			WHERE group_id = ? AND related_group_id NOT IN (SELECT id FROM groups)
		`, winner.ID).Scan(&invalidRelated)
		assert.Equal(t, int64(0), invalidRelated,
			"Winner should not have any related_group_id entries pointing to non-existent groups")
	}
}
