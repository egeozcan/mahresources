package api_tests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"mahresources/application_context"
	"mahresources/models"
	"mahresources/models/query_models"
)

// createReductionWithRule is createReduction with an explicit Winner Rule.
func createReductionWithRule(t *testing.T, tc *TestContext, name string, ids []uint, rule []string) *models.ResourceReduction {
	t.Helper()
	owner, restricted := asAdmin()
	red, err := tc.AppCtx.CreateOrExtendResourceReduction(&query_models.ResourceReductionCreator{
		Name: name, ResourceIds: ids, WinnerRule: rule,
	}, owner, restricted)
	require.NoError(t, err)
	return red
}

func setFileSize(t *testing.T, tc *TestContext, id uint, size int64) {
	t.Helper()
	require.NoError(t, tc.DB.Model(&models.Resource{}).Where("id = ?", id).Update("file_size", size).Error)
}

// The default rule keeps the highest resolution, so nothing is flagged until the
// reviewer promotes a lower-resolution copy by hand. That is the audit's P0: a
// mis-click on "Make Winner" turned the 1600x1066 original into "Will be deleted".
// The flag follows every change of Winner, and a manual promotion is never
// suppressed.
func TestKeepsSmallerFollowsPromotionEjectionAndRestore(t *testing.T) {
	tc := SetupTestEnv(t)

	large := addWithHash(t, tc, "large.txt", "large body", "shared-hash")
	mid := addWithHash(t, tc, "mid.txt", "mid body", "shared-hash")
	small := addWithHash(t, tc, "small.txt", "small body", "shared-hash")
	setDimensions(t, tc, large.ID, 1600, 1066)
	setDimensions(t, tc, mid.ID, 800, 533)
	setDimensions(t, tc, small.ID, 400, 266)

	red := createReduction(t, tc, "Keeps smaller", []uint{large.ID, mid.ID, small.ID})
	plan := computeReduction(t, tc, red.ID)
	cluster := plan.Clusters[0]
	require.Equal(t, large.ID, cluster.WinnerID)
	assert.False(t, cluster.KeepsSmaller, "the highest resolution is kept")

	plan = override(t, tc, red.ID, cluster.ID, application_context.ReductionActionPromote, small.ID)
	cluster = plan.Clusters[0]
	assert.True(t, cluster.KeepsSmaller, "the kept copy has fewer pixels than the copies being removed")
	assert.Equal(t, models.ManualCriterion, cluster.DecidedBy, "a promotion is the reviewer's decision, not a rule's")
	assert.Empty(t, cluster.Margin)
	assert.False(t, cluster.Undecided)
	assert.Equal(t, "you chose it", models.WinnerCriterionLabel(cluster.DecidedBy))

	// Ejecting the larger copies leaves nothing being removed, so nothing to warn about.
	plan = override(t, tc, red.ID, cluster.ID, application_context.ReductionActionEject, large.ID)
	assert.True(t, plan.Clusters[0].KeepsSmaller, "the mid copy is still removed and is larger")
	plan = override(t, tc, red.ID, cluster.ID, application_context.ReductionActionEject, mid.ID)
	assert.False(t, plan.Clusters[0].KeepsSmaller, "no copy is being removed any more")

	plan = override(t, tc, red.ID, cluster.ID, application_context.ReductionActionRestore, large.ID)
	assert.True(t, plan.Clusters[0].KeepsSmaller, "restoring the larger copy re-arms the warning")

	plan = override(t, tc, red.ID, cluster.ID, application_context.ReductionActionPromote, large.ID)
	assert.False(t, plan.Clusters[0].KeepsSmaller, "promoting the larger copy clears it")
}

// Pixels only. A smaller file at the same resolution is usually the better copy,
// and comparing sizes would flag every Cluster under the "Smallest file" preset.
func TestKeepsSmallerComparesPixelsNotFileSize(t *testing.T) {
	tc := SetupTestEnv(t)

	heavy := addWithHash(t, tc, "heavy.txt", "heavy body", "shared-hash")
	light := addWithHash(t, tc, "light.txt", "light body", "shared-hash")
	setDimensions(t, tc, heavy.ID, 800, 600)
	setDimensions(t, tc, light.ID, 800, 600)
	setFileSize(t, tc, heavy.ID, 9000)
	setFileSize(t, tc, light.ID, 1000)

	red := createReductionWithRule(t, tc, "Smallest file", []uint{heavy.ID, light.ID},
		[]string{models.WinnerCriterionSizeAsc})
	plan := computeReduction(t, tc, red.ID)
	require.Equal(t, light.ID, plan.Clusters[0].WinnerID)
	assert.False(t, plan.Clusters[0].KeepsSmaller)
}

// A rule that picks by size can keep the lower resolution, and that is flagged at
// compute. A rule whose first criterion is lowest resolution asked for exactly
// that, so it is suppressed.
func TestKeepsSmallerAtComputeAndSuppressedForPixelsAsc(t *testing.T) {
	tc := SetupTestEnv(t)

	a := addWithHash(t, tc, "a.txt", "a body", "hash-a")
	b := addWithHash(t, tc, "b.txt", "b body", "hash-a")
	setDimensions(t, tc, a.ID, 400, 400)
	setDimensions(t, tc, b.ID, 1200, 1200)
	setFileSize(t, tc, a.ID, 9000)
	setFileSize(t, tc, b.ID, 1000)

	bySize := createReductionWithRule(t, tc, "By size", []uint{a.ID, b.ID},
		[]string{models.WinnerCriterionSizeDesc})
	plan := computeReduction(t, tc, bySize.ID)
	require.Equal(t, a.ID, plan.Clusters[0].WinnerID)
	assert.True(t, plan.Clusters[0].KeepsSmaller, "the biggest file here is the lower resolution")

	asked := createReductionWithRule(t, tc, "Asked for it", []uint{a.ID, b.ID},
		[]string{models.WinnerCriterionPixelsAsc, models.WinnerCriterionSizeDesc})
	plan = computeReduction(t, tc, asked.ID)
	require.Equal(t, a.ID, plan.Clusters[0].WinnerID)
	assert.False(t, plan.Clusters[0].KeepsSmaller, "the reviewer's rule asked for the lowest resolution")

	// Not suppressed after a manual promotion, even under that rule.
	plan = override(t, tc, asked.ID, plan.Clusters[0].ID, application_context.ReductionActionPromote, b.ID)
	assert.False(t, plan.Clusters[0].KeepsSmaller, "promoting the larger one keeps the larger copy")
	plan = override(t, tc, asked.ID, plan.Clusters[0].ID, application_context.ReductionActionPromote, a.ID)
	assert.True(t, plan.Clusters[0].KeepsSmaller, "a manual promotion of the smaller copy is never suppressed")
}
