package api_tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"mahresources/application_context"
	"mahresources/models"
	"mahresources/models/query_models"
)

func createWholeLibrary(t *testing.T, tc *TestContext, c *application_context.MahresourcesContext, name string) *models.ResourceReduction {
	t.Helper()
	owner, restricted := asAdmin()
	red, err := c.CreateOrExtendResourceReduction(&query_models.ResourceReductionCreator{Name: name, All: true}, owner, restricted)
	require.NoError(t, err)
	return red
}

// A whole-library Reduction names nothing: the flag is the selection, and the
// resolver iterates every Resource the principal can see.
func TestAWholeLibraryReductionCoversEveryResource(t *testing.T) {
	tc := SetupTestEnv(t)

	a := clusterOf(t, tc, "all-lib-a", 2)
	b := clusterOf(t, tc, "all-lib-b", 3)
	addResourceWithBody(t, tc, "single.txt", "single")

	red := createWholeLibrary(t, tc, tc.AppCtx, "Everything")
	extent, err := application_context.DecodeReductionExtent(red.Extent)
	require.NoError(t, err)
	assert.True(t, extent.All)
	assert.Empty(t, extent.ResourceIDs)
	assert.Empty(t, extent.GroupIDs)

	plan := computeReduction(t, tc, red.ID)
	require.Len(t, plan.Clusters, 2)
	assert.ElementsMatch(t, a, memberIDs(clusterFor(plan, a[0])))
	assert.ElementsMatch(t, b, memberIDs(clusterFor(plan, b[0])))
	assert.Equal(t, 6, plan.Coverage.ExtentSize)

	review := reviewOf(t, tc, red.ID, 1, nil)
	assert.True(t, review.ExtentAll)
}

// Iterating by id keyset means more than one chunk of Resources is walked, and
// every one is visited exactly once.
func TestAWholeLibraryExtentWalksMoreThanOneChunk(t *testing.T) {
	tc := SetupTestEnv(t)
	total := 1234
	rows := make([]models.Resource, total)
	for i := range rows {
		rows[i] = models.Resource{Name: fmt.Sprintf("bulk-%d", i), Hash: fmt.Sprintf("unique-%d", i)}
	}
	require.NoError(t, tc.DB.CreateInBatches(&rows, 200).Error)

	red := createWholeLibrary(t, tc, tc.AppCtx, "Big")
	plan := computeReduction(t, tc, red.ID)
	assert.Empty(t, plan.Clusters)
	assert.Equal(t, total, plan.Coverage.ExtentSize)
	assert.Equal(t, total, plan.Coverage.ContentHashed)
}

// The drift notice still works: a Resource added after the compute is an arrival.
func TestAWholeLibraryReductionReportsArrivalsSinceTheCompute(t *testing.T) {
	tc := SetupTestEnv(t)
	clusterOf(t, tc, "arrive-a", 2)
	red := createWholeLibrary(t, tc, tc.AppCtx, "Arrivals")
	computeReduction(t, tc, red.ID)
	assert.Zero(t, reviewOf(t, tc, red.ID, 1, nil).EnteredSinceCompute)

	time.Sleep(1100 * time.Millisecond)
	addResourceWithBody(t, tc, "late.txt", "late arrival")
	assert.Equal(t, 1, reviewOf(t, tc, red.ID, 1, nil).EnteredSinceCompute)
}

// A subtree-confined reviewer's whole-library Reduction covers their subtree and
// nothing else: the resolver reads through the scoped handle, so a Resource outside
// it is never in the Extent, never a Cluster member, and never destroyed.
func TestAWholeLibraryReductionStopsAtTheReviewersSubtree(t *testing.T) {
	assertWholeLibraryStopsAtTheSubtree(t, SetupTestEnv(t))
}

func assertWholeLibraryStopsAtTheSubtree(t *testing.T, tc *TestContext) {
	t.Helper()
	owner, restricted := asAdmin()

	mine, err := tc.AppCtx.CreateGroup(&query_models.GroupCreator{Name: "Mine"})
	require.NoError(t, err)
	theirs, err := tc.AppCtx.CreateGroup(&query_models.GroupCreator{Name: "Theirs"})
	require.NoError(t, err)

	inside := addWithHash(t, tc, "inside.txt", "inside body", "shared-hash")
	insideToo := addWithHash(t, tc, "inside-too.txt", "inside too body", "shared-hash")
	outside := addWithHash(t, tc, "outside.txt", "outside body", "shared-hash")
	outsideOnly := clusterOf(t, tc, "outside-only", 2)
	require.NoError(t, tc.DB.Model(&models.Resource{}).Where("id IN ?", []uint{inside.ID, insideToo.ID}).Update("owner_id", mine.ID).Error)
	require.NoError(t, tc.DB.Model(&models.Resource{}).Where("id IN ?", append([]uint{outside.ID}, outsideOnly...)).Update("owner_id", theirs.ID).Error)
	setDimensions(t, tc, outside.ID, 900, 900)
	setDimensions(t, tc, inside.ID, 400, 400)
	setDimensions(t, tc, insideToo.ID, 100, 100)

	scoped := scopedTo(t, tc, mine.ID)
	red, err := scoped.CreateOrExtendResourceReduction(&query_models.ResourceReductionCreator{Name: "Confined library", All: true}, owner, restricted)
	require.NoError(t, err)
	fresh, err := scoped.GetResourceReduction(red.ID, owner, restricted)
	require.NoError(t, err)
	_, err = scoped.RequestReductionCompute(red.ID, fresh.Version, owner, restricted, nil)
	require.NoError(t, err)
	plan := awaitReduction(t, tc, red.ID)

	require.Len(t, plan.Clusters, 1, "the Cluster wholly outside the subtree is not proposed")
	assert.ElementsMatch(t, []uint{inside.ID, insideToo.ID}, memberIDs(plan.Clusters[0]),
		"the copy outside the subtree never joins a Cluster")
	assert.Equal(t, 2, plan.Coverage.ExtentSize, "the Extent is the subtree's two Resources")

	// Applying destroys only what is inside.
	current, err := scoped.GetResourceReduction(red.ID, owner, restricted)
	require.NoError(t, err)
	_, err = scoped.ApplyResourceReduction(&query_models.ReductionApply{ID: red.ID, Version: current.Version}, owner, restricted)
	require.NoError(t, err)
	assert.True(t, resourceExists(t, tc, outside.ID))
	for _, id := range outsideOnly {
		assert.True(t, resourceExists(t, tc, id))
	}
	assert.False(t, resourceExists(t, tc, insideToo.ID))
}

// The create route accepts the flag as JSON and as a form field.
func TestCreatingAWholeLibraryReductionOverTheRoutes(t *testing.T) {
	tc := SetupTestEnv(t)

	rec := tc.MakeRequest(http.MethodPost, "/v1/reduction", map[string]any{"name": "JSON all", "all": true})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var created map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))

	form := tc.MakeFormRequest(http.MethodPost, "/v1/reduction", url.Values{"name": {"Form all"}, "all": {"true"}})
	require.Equal(t, http.StatusOK, form.Code, form.Body.String())

	owner, restricted := asAdmin()
	red, err := tc.AppCtx.GetResourceReduction(uint(created["id"].(float64)), owner, restricted)
	require.NoError(t, err)
	extent, err := application_context.DecodeReductionExtent(red.Extent)
	require.NoError(t, err)
	assert.True(t, extent.All)

	// Without the flag an empty selection is still refused.
	rec = tc.MakeRequest(http.MethodPost, "/v1/reduction", map[string]any{"name": "Nothing"})
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}
