//go:build postgres && json1 && fts5

package application_context

import "testing"

func TestStoreFailedComputesLeaveNoOrphansPG(t *testing.T) {
	testStoreFailedComputesLeaveNoOrphans(t, newOpsPGContext(t))
}
func TestStoreComputeClaimIsOwnerQualifiedPG(t *testing.T) {
	testStoreComputeClaimIsOwnerQualified(t, newOpsPGContext(t))
}
