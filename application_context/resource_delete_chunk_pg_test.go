//go:build postgres && json1 && fts5

package application_context

import "testing"

func TestBulkDeleteStatementsStayWithinAChunkPG(t *testing.T) {
	testBulkDeleteStatementsStayWithinAChunk(t, newReductionPGContext(t))
}
