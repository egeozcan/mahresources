//go:build postgres && json1 && fts5

package application_context

import (
	"context"
	"regexp"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"mahresources/models"
)

// The availability anti-join of a holder statement is a probe per candidate row,
// not a read of the generation's members. Found by the D14 scale proof: written as
// a plain NOT EXISTS, Postgres planned the holders of a dense hash as a merge anti
// join that walked every member row of the generation (and looked up its Cluster)
// for each 500-row page, so a library with many Clusters paid for all of them on
// every page of every dense hash.
func TestComputeHolderStatementDoesNotReadTheGenerationsMembersPG(t *testing.T) {
	ctx := newOpsPGContext(t)
	const resources, holders, clusters = 60_000, 3_000, 25_000

	first := seedResources(t, ctx, resources, `'a' || n`, `CASE WHEN n <= 3000 THEN 'dense' ELSE 'u' || n END`, `NULL`)
	r := mkStreamReduction(t, ctx, streamSel{All: true, Identical: true})
	gen := "gavail"
	require.NoError(t, ctx.db.Exec(`INSERT INTO reduction_extent (reduction_id, generation, resource_id, hash) SELECT ?, ?, id, hash FROM resources WHERE id >= ? AND id < ?`, r.ID, gen, first, first+resources).Error)
	// Clusters already built in this generation, over Resources the dense hash does not hold.
	require.NoError(t, ctx.db.Exec(seedSeries(clusters)+`INSERT INTO reduction_clusters (reduction_id, generation, key, position, tier, state, winner_id) SELECT ?, ?, 'k' || n, n, 'identical', 'open', ? + 3000 + 2 * n FROM seq`,
		r.ID, gen, first).Error)
	require.NoError(t, ctx.db.Exec(`INSERT INTO reduction_members (cluster_id, reduction_id, generation, resource_id, position)
		SELECT c.id, c.reduction_id, c.generation, c.winner_id + k.o, k.o FROM reduction_clusters c CROSS JOIN (SELECT 0 AS o UNION ALL SELECT 1) k WHERE c.reduction_id = ? AND c.generation = ?`, r.ID, gen).Error)
	analyzeAll(t, ctx)

	rec := installScaleRecorder(t, ctx)
	rec.capture("holders", `FROM resources r WHERE r.hash = \? AND r.id > \?`, "dense")
	run := &reductionStream{ctx: ctx, s: ctx.reductionStore(), r: r, generation: gen, rule: models.DefaultWinnerRule(), jobCtx: context.Background()}
	pending, err := run.buildIdentical("dense")
	require.NoError(t, err)
	require.NotNil(t, pending)
	require.Equal(t, holders-models.MaxClusterMembers, pending.stored.Row.Truncated)

	plan := rec.explain(t, ctx, "holders")
	for _, m := range regexp.MustCompile(`on reduction_members[^\n]*\(actual rows=(\d+)`).FindAllStringSubmatch(plan, -1) {
		n, _ := strconv.Atoi(m[1])
		require.LessOrEqual(t, n, 2*reductionRowChunk, "the page reads the members of its candidate rows only:\n%s", plan)
	}
}
