//go:build postgres && json1 && fts5

package application_context

import "testing"

func TestStoreArrivalsDoNotReadRelationshipHistoryPG(t *testing.T) {
	testStoreArrivalsDoNotReadRelationshipHistory(t, newReductionPGContext(t))
}

func TestStoreMemberPageSeeksOnTheWholeKeyPG(t *testing.T) {
	testStoreMemberPageSeeksOnTheWholeKey(t, newReductionPGContext(t))
}

func TestStoreLargestLoserIsExactPastInt64PG(t *testing.T) {
	testStoreLargestLoserIsExactPastInt64(t, newReductionPGContext(t))
}

func TestStoreMembersRequestReadsTheBoundedNamePG(t *testing.T) {
	testStoreMembersRequestReadsTheBoundedName(t, newReductionPGContext(t))
}
