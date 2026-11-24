package application_context

import "testing"

// REDUCTION_PAGE_MEMBER_BUDGET lets a test server cut Clusters at a budget small enough to
// reach by hand; anything that is not a positive number leaves the default.
func TestPageMemberBudgetFromEnv(t *testing.T) {
	for raw, want := range map[string]int{"": 2000, "40": 40, "1": 1, "0": 2000, "-5": 2000, "many": 2000} {
		if got := pageMemberBudgetFromEnv(raw); got != want {
			t.Errorf("pageMemberBudgetFromEnv(%q) = %d, want %d", raw, got, want)
		}
	}
}
