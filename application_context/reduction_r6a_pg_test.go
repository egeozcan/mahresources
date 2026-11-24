//go:build postgres && json1 && fts5

package application_context

import (
	"testing"

	"mahresources/models"
)

func TestR6ReplacedFileIsNotStoredUnderTheOldPairPG(t *testing.T) {
	testR6ReplacedFileIsNotStoredUnderTheOldPair(t, newOpsPGContext(t))
}
func TestR6ChangedHashIsRebuiltNotCarriedPG(t *testing.T) {
	testR6ChangedHashIsRebuiltNotCarried(t, newOpsPGContext(t))
}
func TestR6VanishedPairRefusesTheClusterPG(t *testing.T) {
	testR6VanishedPairRefusesTheCluster(t, newOpsPGContext(t))
}
func TestR6UnlinkedMemberLeavesTheRestAsAClusterPG(t *testing.T) {
	testR6UnlinkedMemberLeavesTheRestAsACluster(t, newOpsPGContext(t))
}
func TestR6OnlyManualEjectionReservesPG(t *testing.T) {
	testR6OnlyManualEjectionReserves(t, newOpsPGContext(t))
}
func TestR6TieDiscoveryIsIncrementalPG(t *testing.T) {
	testR6TieDiscoveryIsIncremental(t, newOpsPGContext(t))
}
// The summary predicate is dialect-specific (jsonb ->> on Postgres): the held-run
// read must narrow to the Reduction there too.
func TestPublicationReadsOnlyThatReductionsHeldComputesPG(t *testing.T) {
	ctx := newPostgresJobStatesContext(t)
	if err := ctx.db.AutoMigrate(&models.ResourceSimilarity{}); err != nil {
		t.Fatal(err)
	}
	if err := models.EnsureSimilarityDistanceIndexes(ctx.db); err != nil {
		t.Fatal(err)
	}
	reductions, held := heldComputesFor(t, ctx, reductionHoldMigrationPending, reductionHoldMigrationPending)
	ctx.resumeBlockedReductionComputes(reductions[0].ID)
	if got := jobStateOf(t, ctx, held[0]); got != "queued" {
		t.Fatalf("the Reduction's own held run: %s", got)
	}
	if got := jobStateOf(t, ctx, held[1]); got != "blocked" {
		t.Fatalf("another Reduction's held run: %s", got)
	}
}

func TestR6ExpiredComputeGenerationIsSweptPG(t *testing.T) {
	testR6ExpiredComputeGenerationIsSwept(t, newOpsPGContext(t))
}
