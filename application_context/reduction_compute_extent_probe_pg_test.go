//go:build postgres && json1 && fts5

package application_context

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"mahresources/models"
)

// The in-Extent test of a neighbour or holder statement is one probe of the staged
// Extent per row it returns, never a read of the staged Extent. Found by the D14
// scale proof: written as EXISTS (SELECT 1 FROM reduction_extent ...), Postgres
// turned it into a hashed subplan that read the whole generation's Extent once per
// statement whenever the statement was expected to return many rows (a hub with
// thousands of pairs, a hash with thousands of holders), so a 2 million row
// Extent was scanned for every 400-row page of a hub's neighbours.
func TestComputeNeighbourStatementDoesNotReadTheWholeExtentPG(t *testing.T) {
	ctx := newOpsPGContext(t)
	const extent, spokes = 60_000, 4_000

	first := seedResources(t, ctx, extent, `'x' || n`, `'xh' || n`, `NULL`)
	hub := first
	require.NoError(t, ctx.db.Exec(`INSERT INTO image_hashes (resource_id, p_hash_int, status) SELECT id, id, 'ok' FROM resources WHERE id <= ?`, hub+spokes).Error)
	require.NoError(t, ctx.db.Exec(seedSeries(spokes)+`INSERT INTO resource_similarities (resource_id1, resource_id2, hamming_distance, p_distance, a_distance)
		SELECT ?, ? + n, 1 + n % 4, 1 + n % 4, 1 + n % 4 FROM seq`, hub, hub).Error)
	buildSimilarityIndexes(t, ctx)

	r := mkStreamReduction(t, ctx, streamSel{All: true})
	gen := "gprobe"
	require.NoError(t, ctx.db.Exec(`INSERT INTO reduction_extent (reduction_id, generation, resource_id, hash) SELECT ?, ?, id, hash FROM resources WHERE id >= ? AND id < ?`, r.ID, gen, first, first+extent).Error)
	analyzeAll(t, ctx)

	rec := installScaleRecorder(t, ctx)
	rec.capture("neighbours", `FROM resource_similarities s WHERE s\.resource_id1 = \?`, fmt.Sprint(hub))
	run := &reductionStream{ctx: ctx, s: ctx.reductionStore(), r: r, generation: gen, rule: models.DefaultWinnerRule(), threshold: 10, jobCtx: context.Background()}
	insiders, _, err := run.starNeighbours(hub)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(insiders), models.MaxClusterMembers-1)

	plan := rec.explain(t, ctx, "neighbours")
	require.NotRegexp(t, `Seq Scan on reduction_extent\b`, plan, "the Extent is probed by key, not scanned")
	require.NotRegexp(t, `Index (Only )?Scan using \S+ on reduction_extent[^\n]*\(actual rows=\d{3,}`, plan, "no probe of the Extent reads more than a handful of rows")
}
