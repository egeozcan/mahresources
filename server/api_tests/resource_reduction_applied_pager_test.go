package api_tests

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"mahresources/application_context"
)

// An applied Cluster of more copies than one card shows is history, and its card must
// still page: a summary line with no way to list the rest leaves most of the record
// unreadable.
func TestAnAppliedClusterPastTheCardCapStillPagesItsMembers(t *testing.T) {
	tc := SetupTestEnv(t)
	ids := make([]uint, 0, 15)
	for i := 0; i < 15; i++ {
		res := addWithHash(t, tc, fmt.Sprintf("copy-%d.txt", i), fmt.Sprintf("body %d", i), "shared-hash")
		setDimensions(t, tc, res.ID, uint(100+i), 100)
		ids = append(ids, res.ID)
	}
	red := createReduction(t, tc, "History", ids)
	computeReduction(t, tc, red.ID)
	applyReduction(t, tc, red.ID)
	// A budget of one cuts a 15-copy Cluster to its kept copy and a cursor, which is what a
	// migrated 340-copy Cluster looks like at the default budget.
	defer application_context.SetReductionPageMemberBudgetForTest(1)()

	response := tc.MakeRequest(http.MethodGet, reductionPath(red.ID)+"&status=applied", nil)
	require.Equal(t, http.StatusOK, response.Code)
	body := response.Body.String()
	require.Contains(t, body, `data-cluster-state="applied"`)
	assert.Contains(t, body, `data-testid="cluster-more-members"`, "the applied card pages the rest of its members")
	assert.Contains(t, body, `data-member-count="15"`, "the pager reads the total from the card")
	assert.Equal(t, 1, strings.Count(body, `class="reduction-cluster-members"`), "one member list for the applied card")
}
