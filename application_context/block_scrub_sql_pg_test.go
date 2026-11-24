//go:build postgres && json1 && fts5

package application_context

import "testing"

func TestSQLScrubMatchesGoScrubPG(t *testing.T) {
	testSQLScrubMatchesGoScrub(t, newReductionPGContext(t))
}

func TestSQLScrubEdgesPG(t *testing.T) {
	testSQLScrubEdges(t, newReductionPGContext(t))
}

func TestDeletionHookMarksMembersGonePG(t *testing.T) {
	testDeletionHookMarksMembersGone(t, newReductionPGContext(t))
}

func TestDeletionHookWalksClustersAscendingPG(t *testing.T) {
	testDeletionHookWalksClustersAscending(t, newReductionPGContext(t))
}
