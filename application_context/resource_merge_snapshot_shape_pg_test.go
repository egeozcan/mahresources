//go:build postgres && json1 && fts5

package application_context

import "testing"

func TestMergeSnapshotShapesPG(t *testing.T) {
	testMergeSnapshotShapes(t, newReductionPGContext(t))
}
