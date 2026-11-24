//go:build postgres && json1 && fts5

package application_context

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mahresources/models"
)

func TestStoreArrivalsDoNotReadRelationshipHistoryPG(t *testing.T) {
	testStoreArrivalsDoNotReadRelationshipHistory(t, newReductionPGContext(t))
}

func TestStoreMemberPageSeeksOnTheWholeKeyPG(t *testing.T) {
	testStoreMemberPageSeeksOnTheWholeKey(t, newReductionPGContext(t))
}

func TestStoreLargestLoserIsExactPastInt64PG(t *testing.T) {
	testStoreLargestLoserIsExactPastInt64(t, newReductionPGContext(t))
}

func TestStoreMembersRequestReadsTheBoundedNamePG(t *testing.T) {
	testStoreMembersRequestReadsTheBoundedName(t, newReductionPGContext(t))
}

func TestStoreArrivalsWalkAcrossTiesPG(t *testing.T) {
	testStoreArrivalsWalkAcrossTies(t, newReductionPGContext(t))
}

func TestStoreArrivalMembershipIsCorrelatedPG(t *testing.T) {
	testStoreArrivalMembershipIsCorrelated(t, newReductionPGContext(t))
}

// Postgres seeks a (created_at, id) cursor only through an index on both columns.
func TestStoreArrivalsCursorHasAnIndexPG(t *testing.T) {
	ctx := newReductionPGContext(t)
	require.NoError(t, models.EnsureResourceListIndexes(ctx.db))
	var n int64
	require.NoError(t, ctx.db.Raw(`SELECT COUNT(*) FROM pg_indexes WHERE tablename = 'resources' AND indexdef LIKE '%(created_at, id)%'`).Scan(&n).Error)
	assert.Equal(t, int64(1), n)
}

func TestStoreLargerCopyWarningSurvivesUint64WrapPG(t *testing.T) {
	testStoreLargerCopyWarningSurvivesUint64Wrap(t, newReductionPGContext(t))
}
