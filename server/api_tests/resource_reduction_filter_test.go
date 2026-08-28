package api_tests

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mahresources/application_context"
	"mahresources/models"
	"mahresources/models/query_models"
)

// The review page filters its Clusters by state, server-side, exactly as the
// list pages filter their rows: the count, the pagination and the page shown all
// describe what matches. The headline case is hiding the Clusters a reviewer has
// already moved past.
func TestReductionReviewPageFiltersByClusterStatus(t *testing.T) {
	tc := SetupTestEnv(t)

	a := addWithHash(t, tc, "a.txt", "a body", "h1")
	b := addWithHash(t, tc, "b.txt", "b body", "h1")
	c := addWithHash(t, tc, "c.txt", "c body", "h2")
	d := addWithHash(t, tc, "d.txt", "d body", "h2")

	red := createReduction(t, tc, "Filtered", []uint{a.ID, b.ID, c.ID, d.ID})
	plan := computeReduction(t, tc, red.ID)
	require.Len(t, plan.Clusters, 2)

	// Skip one Cluster; the other stays open.
	override(t, tc, red.ID, plan.Clusters[0].ID, application_context.ReductionActionSkip, 0)

	// Unfiltered: both render.
	body := tc.MakeRequest(http.MethodGet, reductionPath(red.ID), nil).Body.String()
	assert.Equal(t, 2, strings.Count(body, `data-testid="reduction-cluster"`), "both Clusters render unfiltered")

	// Filtered to skipped: only the frozen one, which offers Reopen.
	body = tc.MakeRequest(http.MethodGet, reductionPath(red.ID)+"&Status=skipped", nil).Body.String()
	assert.Equal(t, 1, strings.Count(body, `data-testid="reduction-cluster"`), "the status filter is server-side")
	assert.Contains(t, body, `data-testid="cluster-reopen"`)
	assert.NotContains(t, body, `data-testid="cluster-skip"`)
	assert.Contains(t, body, ">(1)<", "the heading counts what is shown, like a filtered list")

	// Filtered to open: only the one that has not been acted on, which still
	// offers Skip.
	body = tc.MakeRequest(http.MethodGet, reductionPath(red.ID)+"&Status=open", nil).Body.String()
	assert.Equal(t, 1, strings.Count(body, `data-testid="reduction-cluster"`))
	assert.Contains(t, body, `data-testid="cluster-skip"`)
	assert.NotContains(t, body, `data-testid="cluster-reopen"`)

	// A status with no matches renders the empty state rather than a wrong page,
	// and the empty state says the filter matched nothing — not that the Extent
	// has no repeats, which is a claim about the whole selection.
	body = tc.MakeRequest(http.MethodGet, reductionPath(red.ID)+"&Status=applied", nil).Body.String()
	assert.Equal(t, 0, strings.Count(body, `data-testid="reduction-cluster"`))
	assert.Contains(t, body, "No Clusters match these filters", "an empty filtered review says so")
}

// "Reviewed" is a judgement recorded on an open Cluster, and the filter offers it
// under that name — the same label the cards show.
func TestReductionReviewPageFiltersByReviewed(t *testing.T) {
	tc := SetupTestEnv(t)

	a := addWithHash(t, tc, "a.txt", "a body", "h1")
	b := addWithHash(t, tc, "b.txt", "b body", "h1")

	red := createReduction(t, tc, "Reviewed filter", []uint{a.ID, b.ID})
	plan := computeReduction(t, tc, red.ID)
	require.Len(t, plan.Clusters, 1)

	// An Identical Cluster arrives checked; acting on it (unchecking) marks it
	// Reviewed.
	override(t, tc, red.ID, plan.Clusters[0].ID, application_context.ReductionActionUncheck, 0)

	body := tc.MakeRequest(http.MethodGet, reductionPath(red.ID)+"&Status=reviewed", nil).Body.String()
	assert.Equal(t, 1, strings.Count(body, `data-testid="reduction-cluster"`))
	assert.Contains(t, body, "Reviewed", "the card still reads Reviewed")

	body = tc.MakeRequest(http.MethodGet, reductionPath(red.ID)+"&Status=open", nil).Body.String()
	assert.Equal(t, 0, strings.Count(body, `data-testid="reduction-cluster"`),
		"an acted-on Cluster is no longer Open")
}

// The tier filter separates the two match kinds, which is the distinction the
// whole feature is graded on: byte-identity is a fact, perceptual similarity is a
// guess.
func TestReductionReviewPageFiltersByTier(t *testing.T) {
	tc := SetupTestEnv(t)

	a := addWithHash(t, tc, "a.txt", "a body", "h1")
	b := addWithHash(t, tc, "b.txt", "b body", "h1")
	best := addImage(t, tc, "best.jpg", 800, 800)
	worse := addImage(t, tc, "worse.jpg", 400, 400)
	pairThem(t, tc, best, worse, 4)

	red := createReduction(t, tc, "Tier filter", []uint{a.ID, b.ID, best.ID, worse.ID})
	plan := computeReduction(t, tc, red.ID)
	require.Len(t, plan.Clusters, 2)

	body := tc.MakeRequest(http.MethodGet, reductionPath(red.ID)+"&Tier=identical", nil).Body.String()
	assert.Equal(t, 1, strings.Count(body, `data-testid="reduction-cluster"`))
	assert.Contains(t, body, "Identical Resources")

	body = tc.MakeRequest(http.MethodGet, reductionPath(red.ID)+"&Tier=near", nil).Body.String()
	assert.Equal(t, 1, strings.Count(body, `data-testid="reduction-cluster"`))
	assert.Contains(t, body, "Near-Identical Resources")
	assert.Contains(t, body, "distance 4", "the Near-Identical justification is on the filtered page")
}

// The filter checkboxes render their request's own state, so a filtered page
// arrives with the right boxes checked — the list-page contract.
func TestReductionReviewPageFilterControlsReflectTheQuery(t *testing.T) {
	tc := SetupTestEnv(t)

	a := addWithHash(t, tc, "a.txt", "a body", "h1")
	b := addWithHash(t, tc, "b.txt", "b body", "h1")
	red := createReduction(t, tc, "Filter state", []uint{a.ID, b.ID})
	computeReduction(t, tc, red.ID)

	body := tc.MakeRequest(http.MethodGet, reductionPath(red.ID)+"&Status=skipped&Status=stale&Tier=near&Attention=1", nil).Body.String()
	assert.Contains(t, body, `name="Status" value="skipped" checked`, "an active status renders checked")
	assert.Contains(t, body, `name="Status" value="stale" checked`)
	assert.Contains(t, body, `name="Tier" value="near" checked`)
	assert.Contains(t, body, `name="Attention" value="1" checked`, "the attention toggle reflects the query")
	assert.NotContains(t, body, `name="Tier" value="identical" checked`)
}

// The filter applies before pagination, exactly as a list query's WHERE runs
// before its LIMIT: a status matching only five of twenty-two Clusters shows all
// five on one page, and the count in the review is over what matches.
func TestReductionReviewFilterAppliesBeforePagination(t *testing.T) {
	tc := SetupTestEnv(t)

	// 22 Identical Clusters need 44 Resources sharing 22 hashes.
	var ids []uint
	for i := 0; i < 22; i++ {
		a := addWithHash(t, tc, fmt.Sprintf("a%d.txt", i), "a body", fmt.Sprintf("h%d", i))
		b := addWithHash(t, tc, fmt.Sprintf("b%d.txt", i), "b body", fmt.Sprintf("h%d", i))
		ids = append(ids, a.ID, b.ID)
	}
	red := createReduction(t, tc, "Paginated", ids)
	plan := computeReduction(t, tc, red.ID)
	require.Len(t, plan.Clusters, 22)

	// Five Clusters skipped; seventeen stay open.
	for i := 0; i < 5; i++ {
		override(t, tc, red.ID, plan.Clusters[i].ID, application_context.ReductionActionSkip, 0)
	}

	owner, restricted := asAdmin()

	unfiltered, err := tc.AppCtx.GetReductionReview(red.ID, owner, restricted, 1, nil)
	require.NoError(t, err)
	assert.Equal(t, 22, unfiltered.ClusterCount, "the count is over the whole plan, not the page")
	require.Len(t, unfiltered.Clusters, 20, "a full page of 20 renders")

	secondPage, err := tc.AppCtx.GetReductionReview(red.ID, owner, restricted, 2, nil)
	require.NoError(t, err)
	require.Len(t, secondPage.Clusters, 2, "the remaining two Clusters are page two")

	filtered, err := tc.AppCtx.GetReductionReview(red.ID, owner, restricted, 1, &query_models.ResourceReductionReviewQuery{
		Status: []string{models.ReductionClusterSkipped},
	})
	require.NoError(t, err)
	assert.Equal(t, 5, filtered.ClusterCount, "the count is over what matches")
	require.Len(t, filtered.Clusters, 5,
		"the five matching Clusters all fit one page: the filter ran before the slice")
}

// The checked counts describe what Apply would do — the whole plan, not what the
// filter shows. A filter that hides a checked Cluster must not change them.
func TestReductionReviewCheckedCountIsWholePlanNotFiltered(t *testing.T) {
	tc := SetupTestEnv(t)

	a := addWithHash(t, tc, "a.txt", "a body", "h1")
	b := addWithHash(t, tc, "b.txt", "b body", "h1")
	c := addWithHash(t, tc, "c.txt", "c body", "h2")
	d := addWithHash(t, tc, "d.txt", "d body", "h2")

	red := createReduction(t, tc, "Whole plan", []uint{a.ID, b.ID, c.ID, d.ID})
	plan := computeReduction(t, tc, red.ID)
	require.Len(t, plan.Clusters, 2)

	// Skip Cluster 1. Cluster 2 stays open and — being Identical — checked.
	override(t, tc, red.ID, plan.Clusters[0].ID, application_context.ReductionActionSkip, 0)

	owner, restricted := asAdmin()
	review, err := tc.AppCtx.GetReductionReview(red.ID, owner, restricted, 1, &query_models.ResourceReductionReviewQuery{
		Status: []string{models.ReductionClusterSkipped},
	})
	require.NoError(t, err)
	require.Len(t, review.Clusters, 1, "only the skipped Cluster shows under the filter")
	assert.Equal(t, 1, review.CheckedCount,
		"the hidden checked Cluster still counts — Apply acts on the whole plan")
	assert.Equal(t, 1, review.CheckedLoserCount)
}
