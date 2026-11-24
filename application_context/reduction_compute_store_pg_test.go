//go:build postgres && json1 && fts5

package application_context

import "testing"

func TestStoreLandCarriesEverythingPG(t *testing.T) {
	testStoreLandCarriesEverythingAndKeepsDecisions(t, newOpsPGContext(t))
}
func TestStoreLandCarryIsChunkedPG(t *testing.T) { testStoreLandCarryIsChunked(t, newOpsPGContext(t)) }
func TestStoreLandRefusalsPG(t *testing.T)       { testStoreLandRefusals(t, newOpsPGContext(t)) }
func TestStoreLandStopsWhenTheReductionIsDeletedPG(t *testing.T) {
	testStoreLandStopsWhenTheReductionIsDeleted(t, newOpsPGContext(t))
}
func TestStoreBeginComputeRefusalsPG(t *testing.T) {
	testStoreBeginComputeRefusals(t, newOpsPGContext(t))
}
func TestStoreSweepPG(t *testing.T) { testStoreSweep(t, newOpsPGContext(t)) }
func TestStoreSweepVersusComputeStartPG(t *testing.T) {
	testStoreSweepVersusComputeStart(t, newOpsPGContext(t))
}
func TestStoreSweepFirstComputeFailsPG(t *testing.T) {
	testStoreSweepFirstComputeFails(t, newOpsPGContext(t))
}
func TestStoreDeleteReductionPG(t *testing.T) { testStoreDeleteReduction(t, newOpsPGContext(t)) }
func TestStoreComputeSeamsPG(t *testing.T)    { testStoreComputeSeams(t, newOpsPGContext(t)) }

func TestStoreSweepExaminesOnlyStalePG(t *testing.T) {
	testStoreSweepExaminesOnlyStale(t, newOpsPGContext(t))
}

func TestMigrationRunnerHonoursACancelledContextPG(t *testing.T) {
	testMigrationRunnerHonoursACancelledContext(t, newOpsPGContext(t))
}
