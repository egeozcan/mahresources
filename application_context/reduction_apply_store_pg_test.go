//go:build postgres && json1 && fts5

package application_context

import "testing"

func TestStoreApplyEndToEndPG(t *testing.T) { testStoreApplyEndToEnd(t, newOpsPGContext(t)) }
func TestStoreApplyTokenMismatchRollsBackPG(t *testing.T) {
	testStoreApplyTokenMismatchRollsBack(t, newOpsPGContext(t))
}
func TestStoreApplyRefusalsPG(t *testing.T) { testStoreApplyRefusals(t, newOpsPGContext(t)) }
func TestStoreApplyStampPG(t *testing.T)    { testStoreApplyStamp(t, newOpsPGContext(t)) }
func TestStoreApplyExpiredLeaseDestroysNothingPG(t *testing.T) {
	testStoreApplyExpiredLeaseDestroysNothing(t, newOpsPGContext(t))
}
func TestStoreApplyParkedWorkerCannotMergePG(t *testing.T) {
	testStoreApplyParkedWorkerCannotMerge(t, newOpsPGContext(t))
}
func TestStoreApplyStaleOutcomesPG(t *testing.T) { testStoreApplyStaleOutcomes(t, newOpsPGContext(t)) }
func TestStoreApplyLeftTheSelectionPG(t *testing.T) {
	testStoreApplyLeftTheSelection(t, newOpsPGContext(t))
}
func TestStoreApplyStrandedClaimIsNotInTheBatchPG(t *testing.T) {
	testStoreApplyStrandedClaimIsNotInTheBatch(t, newOpsPGContext(t))
}
func TestStoreApplyCappedResponsePG(t *testing.T) {
	testStoreApplyCappedResponse(t, newOpsPGContext(t))
}
func TestStoreApplyEvidenceHiddenForScopedViewerPG(t *testing.T) {
	testStoreApplyEvidenceHiddenForScopedViewer(t, newOpsPGContext(t))
}
