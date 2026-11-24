//go:build postgres && json1 && fts5

package application_context

import "testing"

func TestReductionOwnerSelectionInAHugeScopedSubtreePG(t *testing.T) {
	testReductionOwnerSelectionInAHugeScopedSubtree(t, newReductionPGContext(t))
}

func TestReductionSelectionCapCountsBothListsPG(t *testing.T) {
	testReductionSelectionCapCountsBothLists(t, newReductionPGContext(t))
}

func TestReductionStaleReasonIsBoundedPG(t *testing.T) {
	testReductionStaleReasonIsBounded(t, newReductionPGContext(t))
}

func TestReductionDeleteRemovesItsSpoolPG(t *testing.T) {
	testReductionDeleteRemovesItsSpool(t, newReductionPGContext(t))
}
