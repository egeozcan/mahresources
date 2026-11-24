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
