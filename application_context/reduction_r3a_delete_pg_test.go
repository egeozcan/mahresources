//go:build postgres && json1 && fts5

package application_context

import "testing"

func TestReductionDeleteChunksAreExactPG(t *testing.T) {
	testReductionDeleteChunksAreExact(t, newReductionPGContext(t))
}
