//go:build postgres && json1 && fts5

package application_context

import "testing"

func TestSQLScrubMatchesGoScrubPG(t *testing.T) {
	testSQLScrubMatchesGoScrub(t, newReductionPGContext(t))
}

func TestSQLScrubEdgesPG(t *testing.T) {
	testSQLScrubEdges(t, newReductionPGContext(t))
}
