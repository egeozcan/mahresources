package api_tests

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"mahresources/application_context"
	"mahresources/models"
	"mahresources/models/query_models"
)

func bulk(t *testing.T, tc *TestContext, id uint, version uint, req query_models.ReductionBulkOverride) error {
	t.Helper()
	owner, restricted := asAdmin()
	req.ID, req.Version = id, version
	_, err := tc.AppCtx.BulkOverrideReductionClusters(&req, owner, restricted)
	return err
}

func versionOf(t *testing.T, tc *TestContext, id uint) uint {
	t.Helper()
	owner, restricted := asAdmin()
	red, err := tc.AppCtx.GetResourceReduction(id, owner, restricted)
	require.NoError(t, err)
	return red.Version
}

func uintPtr(v uint) *uint { return &v }

// Approving a page never sweeps up a Cluster that needs individual review: it is
// left exactly as it was and counted, so the reviewer is told rather than
// surprised. Every Cluster that is approved is marked Reviewed, so it freezes.
func TestBulkCheckSkipsClustersThatNeedAttention(t *testing.T) {
	tc := SetupTestEnv(t)
	owner, restricted := asAdmin()

	plain := clusterOf(t, tc, "bulk-plain", 2)
	curated := clusterOf(t, tc, "bulk-curated", 2)
	require.NoError(t, tc.DB.Model(&models.Resource{}).Where("id = ?", curated[1]).
		Update("description", "the curated copy").Error)
	setDimensions(t, tc, curated[0], 800, 600)
	setDimensions(t, tc, curated[1], 400, 300)

	red := createReduction(t, tc, "Bulk check", append(append([]uint{}, plain...), curated...))
	plan := computeReduction(t, tc, red.ID)
	plainID, curatedID := clusterFor(plan, plain[0]).ID, clusterFor(plan, curated[0]).ID
	require.NotEmpty(t, clusterFor(plan, curated[0]).Lossy, "the fixture is lossy")

	// Both arrive checked (Identical); clear them so the bulk check has work to do.
	require.NoError(t, bulk(t, tc, red.ID, versionOf(t, tc, red.ID), query_models.ReductionBulkOverride{
		Action: "uncheck", ClusterIDs: []string{plainID, curatedID},
	}))

	result, err := tc.AppCtx.BulkOverrideReductionClusters(&query_models.ReductionBulkOverride{
		ID: red.ID, Version: versionOf(t, tc, red.ID), Action: "check", ClusterIDs: []string{plainID, curatedID},
	}, owner, restricted)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Changed)
	assert.Equal(t, 1, result.NeedsReview)
	assert.Equal(t, versionOf(t, tc, red.ID), result.Version)

	after := planOf(t, tc, red.ID)
	assert.True(t, findClusterByID(after, plainID).Checked)
	assert.True(t, findClusterByID(after, plainID).Reviewed)
	assert.False(t, findClusterByID(after, curatedID).Checked, "a lossy Cluster is never approved in bulk")
}

// allIdentical is the Identical tier only, carries the count its button showed, and
// refuses with 409 when that count is out of date.
func TestBulkAllIdenticalNeverTouchesTheNearTierAndChecksTheCount(t *testing.T) {
	tc := SetupTestEnv(t)

	a := clusterOf(t, tc, "all-a", 2)
	b := clusterOf(t, tc, "all-b", 2)
	best := addImage(t, tc, "best.jpg", 800, 800)
	worse := addImage(t, tc, "worse.jpg", 400, 400)
	pairThem(t, tc, best, worse, 3)

	red := createReduction(t, tc, "All identical", append(append(append([]uint{}, a...), b...), best.ID, worse.ID))
	plan := computeReduction(t, tc, red.ID)
	near := clusterFor(plan, best.ID)
	require.Equal(t, models.ReductionTierNear, near.Tier)
	require.False(t, near.Checked)

	require.NoError(t, bulk(t, tc, red.ID, versionOf(t, tc, red.ID), query_models.ReductionBulkOverride{
		Action: "uncheck", AllIdentical: true, ExpectedCount: uintPtr(2),
	}))
	plan = planOf(t, tc, red.ID)
	assert.False(t, clusterFor(plan, a[0]).Checked)
	assert.False(t, clusterFor(plan, b[0]).Checked)

	err := bulk(t, tc, red.ID, versionOf(t, tc, red.ID), query_models.ReductionBulkOverride{
		Action: "check", AllIdentical: true, ExpectedCount: uintPtr(5),
	})
	assert.ErrorIs(t, err, application_context.ErrReductionCountMismatch)
	assert.False(t, clusterFor(planOf(t, tc, red.ID), a[0]).Checked, "a refused request changes nothing")

	err = bulk(t, tc, red.ID, versionOf(t, tc, red.ID), query_models.ReductionBulkOverride{
		Action: "check", AllIdentical: true,
	})
	assert.ErrorIs(t, err, application_context.ErrReductionBulkNeedsCount)

	require.NoError(t, bulk(t, tc, red.ID, versionOf(t, tc, red.ID), query_models.ReductionBulkOverride{
		Action: "check", AllIdentical: true, ExpectedCount: uintPtr(2),
	}))
	plan = planOf(t, tc, red.ID)
	assert.True(t, clusterFor(plan, a[0]).Checked)
	assert.True(t, clusterFor(plan, b[0]).Checked)
	assert.False(t, clusterFor(plan, best.ID).Checked, "the Near-Identical tier is never approved wholesale")
	assert.False(t, clusterFor(plan, best.ID).Reviewed, "and is not even marked as acted on")
}

func TestBulkRefusesMalformedRequestsAndUnknownIds(t *testing.T) {
	tc := SetupTestEnv(t)
	a := clusterOf(t, tc, "refuse-a", 2)
	red := createReduction(t, tc, "Refusals", a)
	plan := computeReduction(t, tc, red.ID)
	id := plan.Clusters[0].ID
	v := versionOf(t, tc, red.ID)

	assert.ErrorIs(t, bulk(t, tc, red.ID, v, query_models.ReductionBulkOverride{Action: "check"}), application_context.ErrReductionBulkTarget)
	assert.ErrorIs(t, bulk(t, tc, red.ID, v, query_models.ReductionBulkOverride{
		Action: "check", ClusterIDs: []string{id}, AllIdentical: true, ExpectedCount: uintPtr(1),
	}), application_context.ErrReductionBulkTarget)
	assert.ErrorIs(t, bulk(t, tc, red.ID, v, query_models.ReductionBulkOverride{Action: "promote", ClusterIDs: []string{id}}), application_context.ErrReductionBulkTarget)

	// One unknown id refuses the whole request, and the known one is not touched.
	assert.ErrorIs(t, bulk(t, tc, red.ID, v, query_models.ReductionBulkOverride{
		Action: "uncheck", ClusterIDs: []string{id, "identical-nope"},
	}), application_context.ErrReductionClusterNotFound)
	assert.True(t, planOf(t, tc, red.ID).Clusters[0].Checked)

	// A stale version is refused like any write.
	assert.ErrorIs(t, bulk(t, tc, red.ID, v+7, query_models.ReductionBulkOverride{
		Action: "uncheck", ClusterIDs: []string{id},
	}), application_context.ErrReductionConflict)

	// A Cluster that is not open is refused, not silently skipped.
	override(t, tc, red.ID, id, application_context.ReductionActionSkip, 0)
	assert.ErrorIs(t, bulk(t, tc, red.ID, versionOf(t, tc, red.ID), query_models.ReductionBulkOverride{
		Action: "check", ClusterIDs: []string{id},
	}), application_context.ErrReductionClusterNotOpen)
}

// A Cluster the caller may not fully see answers exactly as an unknown id does,
// and is in no allIdentical count: otherwise the bulk endpoint is an oracle for
// the Cluster ids the render refuses to print.
func TestBulkTreatsAWithheldClusterLikeAnUnknownOne(t *testing.T) {
	tc := SetupTestEnv(t)
	owner, restricted := asAdmin()

	wide, err := tc.AppCtx.CreateGroup(&query_models.GroupCreator{Name: "Wide"})
	require.NoError(t, err)
	narrow, err := tc.AppCtx.CreateGroup(&query_models.GroupCreator{Name: "Narrow", OwnerId: wide.ID})
	require.NoError(t, err)

	mine := clusterOf(t, tc, "withheld-mine", 2)
	hidden := clusterOf(t, tc, "withheld-hidden", 2)
	for _, id := range mine {
		require.NoError(t, tc.DB.Model(&models.Resource{}).Where("id = ?", id).Update("owner_id", narrow.ID).Error)
	}
	require.NoError(t, tc.DB.Model(&models.Resource{}).Where("id = ?", hidden[0]).Update("owner_id", narrow.ID).Error)
	require.NoError(t, tc.DB.Model(&models.Resource{}).Where("id = ?", hidden[1]).Update("owner_id", wide.ID).Error)

	wideCtx := scopedTo(t, tc, wide.ID)
	red, err := wideCtx.CreateOrExtendResourceReduction(&query_models.ResourceReductionCreator{
		Name: "Bulk withheld", ResourceIds: append(append([]uint{}, mine...), hidden...),
	}, owner, restricted)
	require.NoError(t, err)
	fresh, err := wideCtx.GetResourceReduction(red.ID, owner, restricted)
	require.NoError(t, err)
	_, err = wideCtx.RequestReductionCompute(red.ID, fresh.Version, owner, restricted, nil)
	require.NoError(t, err)
	plan := awaitReduction(t, tc, red.ID)
	hiddenID, mineID := clusterFor(plan, hidden[0]).ID, clusterFor(plan, mine[0]).ID

	narrowCtx := scopedTo(t, tc, narrow.ID)
	_, err = narrowCtx.BulkOverrideReductionClusters(&query_models.ReductionBulkOverride{
		ID: red.ID, Version: versionOf(t, tc, red.ID), Action: "uncheck", ClusterIDs: []string{mineID, hiddenID},
	}, owner, restricted)
	assert.ErrorIs(t, err, application_context.ErrReductionClusterNotFound)

	// allIdentical counts only what they can see: one Cluster, not two.
	_, err = narrowCtx.BulkOverrideReductionClusters(&query_models.ReductionBulkOverride{
		ID: red.ID, Version: versionOf(t, tc, red.ID), Action: "uncheck", AllIdentical: true, ExpectedCount: uintPtr(2),
	}, owner, restricted)
	assert.ErrorIs(t, err, application_context.ErrReductionCountMismatch)
	result, err := narrowCtx.BulkOverrideReductionClusters(&query_models.ReductionBulkOverride{
		ID: red.ID, Version: versionOf(t, tc, red.ID), Action: "uncheck", AllIdentical: true, ExpectedCount: uintPtr(1),
	}, owner, restricted)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Changed)

	after := planOf(t, tc, red.ID)
	assert.True(t, findClusterByID(after, hiddenID).Checked, "the hidden Cluster was never touched")
	assert.False(t, findClusterByID(after, hiddenID).Reviewed)
}

// The route, the JSON shape and its status codes.
func TestBulkRoute(t *testing.T) {
	tc := SetupTestEnv(t)
	a := clusterOf(t, tc, "route-a", 2)
	red := createReduction(t, tc, "Route", a)
	plan := computeReduction(t, tc, red.ID)
	id := plan.Clusters[0].ID

	post := func(body map[string]any) (*http.Response, map[string]any) {
		rec := tc.MakeRequest(http.MethodPost, "/v1/reduction/clusters", body)
		out := map[string]any{}
		_ = json.Unmarshal(rec.Body.Bytes(), &out)
		return rec.Result(), out
	}

	resp, out := post(map[string]any{"id": red.ID, "version": versionOf(t, tc, red.ID), "action": "uncheck", "clusterIds": []string{id}})
	require.Equal(t, http.StatusOK, resp.StatusCode)
	assert.EqualValues(t, 1, out["changed"])
	assert.EqualValues(t, 0, out["needsReview"])
	assert.EqualValues(t, versionOf(t, tc, red.ID), out["version"])

	resp, _ = post(map[string]any{"id": red.ID, "version": 1, "action": "check", "clusterIds": []string{id}})
	assert.Equal(t, http.StatusConflict, resp.StatusCode, "a stale version")

	resp, _ = post(map[string]any{"id": red.ID, "version": versionOf(t, tc, red.ID), "action": "check", "allIdentical": true, "expectedCount": 9})
	assert.Equal(t, http.StatusConflict, resp.StatusCode, "a stale count")

	resp, _ = post(map[string]any{"id": red.ID, "version": versionOf(t, tc, red.ID), "action": "check"})
	assert.Equal(t, http.StatusBadRequest, resp.StatusCode, "no target")
}
