//go:build postgres && json1 && fts5

package application_context

import "testing"

func TestReductionReadStoreMatchesJSONReaderPG(t *testing.T) {
	testReductionReadStoreMatchesJSONReader(t, newReductionPGContext, []int64{1, 2, 3, 4, 5, 6, 7, 8})
}

func TestStoreReviewRefusesALegacyPlanPG(t *testing.T) {
	testStoreReviewRefusesALegacyPlan(t, newReductionPGContext(t))
}

func TestStoreReviewBoundsDisplayTextPG(t *testing.T) {
	testStoreReviewBoundsDisplayText(t, newReductionPGContext(t))
}

func TestStoreReviewPageBudgetPG(t *testing.T) {
	testStoreReviewPageBudget(t, newReductionPGContext(t))
}

func TestStoreReviewWinnerBeyondTheFirstMembersPG(t *testing.T) {
	testStoreReviewWinnerBeyondTheFirstMembers(t, newReductionPGContext(t))
}

func TestStoreReviewHidesTruncationFromAScopedViewerPG(t *testing.T) {
	testStoreReviewHidesTruncationFromAScopedViewer(t, newReductionPGContext(t))
}

func TestStoreAssociationCountsUseTheLiveSubtreePG(t *testing.T) {
	testStoreAssociationCountsUseTheLiveSubtree(t, newReductionPGContext(t))
}

func TestStoreAssociationCountsOnAHugeSubtreePG(t *testing.T) {
	testStoreAssociationCountsOnAHugeSubtree(t, newReductionPGContext(t))
}

func TestStoreLastApplyRunningFollowsTheLeasePG(t *testing.T) {
	testStoreLastApplyRunningFollowsTheLease(t, newReductionPGContext(t))
}

func TestStoreLastApplyCountsNeedEvidencePG(t *testing.T) {
	testStoreLastApplyCountsNeedEvidence(t, newReductionPGContext(t))
}

func TestStoreWidenedExtentCountsTheSelectionPG(t *testing.T) {
	testStoreWidenedExtentCountsTheSelection(t, newReductionPGContext(t))
}

func TestStoreLeftOutCountsDoNotScanMembersPG(t *testing.T) {
	testStoreLeftOutCountsDoNotScanMembers(t, newReductionPGContext(t))
}

func TestStoreReductionNameIsBoundedPG(t *testing.T) {
	testStoreReductionNameIsBounded(t, newReductionPGContext(t))
}

func TestStoreListSamplesTheClockOncePG(t *testing.T) {
	testStoreListSamplesTheClockOnce(t, newReductionPGContext(t))
}

func TestStoreTierFilterIsDeduplicatedPG(t *testing.T) {
	testStoreTierFilterIsDeduplicated(t, newReductionPGContext(t))
}

func TestStoreApplyTokenFollowsTheEligibleSetPG(t *testing.T) {
	testStoreApplyTokenFollowsTheEligibleSet(t, newReductionPGContext(t))
}
