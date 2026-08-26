package api_tests

import (
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The review page renders the Clusters with their justification, which is the
// whole product: reviewing is meant to be reading a reason rather than squinting
// at thumbnails.
func TestReductionPageRendersClustersAndTheirJustification(t *testing.T) {
	tc := SetupTestEnv(t)

	small := addWithHash(t, tc, "small.txt", "small body", "shared-hash")
	large := addWithHash(t, tc, "large.txt", "large body", "shared-hash")
	setDimensions(t, tc, small.ID, 100, 100)
	setDimensions(t, tc, large.ID, 400, 100)

	red := createReduction(t, tc, "Rendered", []uint{small.ID, large.ID})
	computeReduction(t, tc, red.ID)

	response := tc.MakeRequest(http.MethodGet, reductionPath(red.ID), nil)
	require.Equal(t, http.StatusOK, response.Code)
	body := response.Body.String()

	assert.Contains(t, body, `data-testid="reduction-cluster"`)
	assert.Contains(t, body, "Identical Resources")
	assert.Contains(t, body, "highest resolution", "the deciding criterion is on the page")
	assert.Contains(t, body, "4x the pixels", "and so is the margin that decided it")
	assert.Contains(t, body, "Will be deleted", "a Loser says what will happen to it")
	assert.Contains(t, body, "cannot be undone", "and the page says there is no way back")
}

// A Reduction that has never been computed says so rather than rendering an empty
// list that reads as "nothing repeats".
func TestReductionPageDistinguishesNotComputedFromNothingFound(t *testing.T) {
	tc := SetupTestEnv(t)

	a := addResourceWithBody(t, tc, "a.txt", "aaa")
	red := createReduction(t, tc, "Fresh", []uint{a.ID})

	body := tc.MakeRequest(http.MethodGet, reductionPath(red.ID), nil).Body.String()
	assert.Contains(t, body, "Not computed yet")

	computeReduction(t, tc, red.ID)

	body = tc.MakeRequest(http.MethodGet, reductionPath(red.ID), nil).Body.String()
	assert.Contains(t, body, "Nothing in this Extent repeats")
	assert.Contains(t, body, "carry a content hash", "and the coverage line says what could be examined")
}

// The list page shows every Reduction the caller may see, with the creation date
// and time that tells two similarly named ones apart.
func TestReductionListPageRendersEachReduction(t *testing.T) {
	tc := SetupTestEnv(t)

	a := addResourceWithBody(t, tc, "a.txt", "aaa")
	createReduction(t, tc, "First sweep", []uint{a.ID})
	createReduction(t, tc, "Second sweep", []uint{a.ID})

	body := tc.MakeRequest(http.MethodGet, "/reductions", nil).Body.String()
	assert.Contains(t, body, "First sweep")
	assert.Contains(t, body, "Second sweep")
	assert.Equal(t, 2, strings.Count(body, `data-testid="reduction-card"`))
}

func reductionPath(id uint) string {
	return "/reduction?id=" + uintToPath(id)
}

func uintToPath(id uint) string {
	digits := ""
	if id == 0 {
		return "0"
	}
	for id > 0 {
		digits = string(rune('0'+id%10)) + digits
		id /= 10
	}
	return digits
}
