package application_context

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mahresources/models"
	"mahresources/models/query_models"
)

// A tier value no Cluster can hold is never bound as the request wrote it: Postgres
// refuses a NUL byte in a text parameter, which turned `tier=%00` into a 500. The
// filter still matches nothing.
func TestWithDistinctTiersNeverBindsAnUnknownValue(t *testing.T) {
	for _, raw := range []string{"\x00", "x\x00y", "bogus"} {
		got := withDistinctTiers(&query_models.ResourceReductionReviewQuery{Tier: []string{raw}})
		require.Len(t, got.Tier, 1)
		assert.NotContains(t, got.Tier[0], "\x00", "%q", raw)
		assert.NotEqual(t, models.ReductionTierIdentical, got.Tier[0])
		assert.NotEqual(t, models.ReductionTierNear, got.Tier[0])
	}
	mixed := withDistinctTiers(&query_models.ResourceReductionReviewQuery{Tier: []string{"near", "\x00", "near", "zz"}})
	assert.Equal(t, []string{models.ReductionTierNear, mixed.Tier[1]}, mixed.Tier)
	assert.NotContains(t, mixed.Tier[1], "\x00")
}

func TestStoreReviewTierFilterWithAnUnknownValueMatchesNothingSQLite(t *testing.T) {
	ctx := newReductionSQLiteContext(t)
	r := tableReduction(t, ctx)
	a := makeResource(t, ctx.db, "a", opsRes{Hash: "h"})
	b := makeResource(t, ctx.db, "b", opsRes{Hash: "h"})
	require.NoError(t, ctx.reductionStore().insertCluster(r, "gen", &storedCluster{
		Row: models.ReductionClusterRow{Key: "c1", Tier: "identical", State: "open", WinnerID: a}, Members: memberRows(a, b)}))

	review, _, err := ctx.storeGetReductionReview(r.ID, nil, false, 1, &query_models.ResourceReductionReviewQuery{Tier: []string{"\x00"}})
	require.NoError(t, err)
	assert.Empty(t, review.Clusters)
}
