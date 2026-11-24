//go:build postgres && json1 && fts5

package application_context

import "testing"

func TestMergeRefusesDuplicateLosersPG(t *testing.T) {
	testMergeRefusesDuplicateLosers(t, newReductionPGContext(t))
}
