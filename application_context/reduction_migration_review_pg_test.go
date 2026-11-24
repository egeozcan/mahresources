//go:build postgres && json1 && fts5

package application_context

import "testing"

func TestReductionMigrationJSONNullDocumentsPG(t *testing.T) {
	testReductionMigrationJSONNullDocuments(t, newReductionPGContext(t))
}
func TestReductionMigrationKeepsLongLegacyTextPG(t *testing.T) {
	testReductionMigrationKeepsLongLegacyText(t, newReductionPGContext(t))
}
func TestReductionMigrationSkipsOutOfRangeIDsPG(t *testing.T) {
	testReductionMigrationSkipsOutOfRangeIDs(t, newReductionPGContext(t))
}
func TestReductionMigrationPublicationCountsMemberRowsPG(t *testing.T) {
	testReductionMigrationPublicationCountsMemberRows(t, newReductionPGContext(t))
}
func TestReductionMigrationShareLocksTheResourcesItReadsPG(t *testing.T) {
	testReductionMigrationShareLocksTheResourcesItReads(t, newReductionPGContext(t))
}
func TestReductionMigrationOverCapClusterResumesMidClusterPG(t *testing.T) {
	testReductionMigrationOverCapClusterResumesMidCluster(t, newReductionPGContext(t))
}
func TestReductionMigrationSkipsAReductionPublishedMeanwhilePG(t *testing.T) {
	testReductionMigrationSkipsAReductionPublishedMeanwhile(t, newReductionPGContext(t))
}
func TestReductionMigrationReadsSettingsOncePerReductionPG(t *testing.T) {
	testReductionMigrationReadsSettingsOncePerReduction(t, newReductionPGContext(t))
}
func TestReductionMigrationOverCapLossyNeedsAttentionPG(t *testing.T) {
	testReductionMigrationOverCapLossyNeedsAttention(t, newReductionPGContext(t))
}
func TestReductionDeleteSpoolInsideATransactionPG(t *testing.T) {
	testReductionDeleteSpoolInsideATransaction(t, newReductionPGContext(t))
}
