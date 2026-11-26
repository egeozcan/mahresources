//go:build postgres && json1 && fts5

package application_context

import "testing"

func TestReductionSelectionSkipsResourcesOutsideScopePG(t *testing.T) {
	testReductionSelectionSkipsResourcesOutsideScope(t, newReductionPGContext(t))
}

func TestReductionSelectionRefusesGroupsOutsideScopePG(t *testing.T) {
	testReductionSelectionRefusesGroupsOutsideScope(t, newReductionPGContext(t))
}

func TestReductionPreviewCountsOnlyVisiblePG(t *testing.T) {
	testReductionPreviewCountsOnlyVisible(t, newReductionPGContext(t))
}

func TestReductionSelectFromGroupCapHoldsUnderConcurrentInsertPG(t *testing.T) {
	testReductionSelectFromGroupCapHoldsUnderConcurrentInsert(t, newReductionPGContext(t))
}
