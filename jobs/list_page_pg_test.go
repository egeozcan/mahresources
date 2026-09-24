//go:build postgres && json1 && fts5

package jobs

import "testing"

// The keyset walk back and the unwindowed state count, on PostgreSQL: the
// backward predicate compares a timestamp column the way the forward one does,
// and the grouped count runs through the same filter and command selector.

func TestListBeforeWalksBackToTheFirstPageOnPostgres(t *testing.T) {
	testListBeforeWalksBackToTheFirstPage(t, newPGDeps(t))
}

func TestListBeforeReturnsAFullFirstPageWhenFewerNewerJobsRemainOnPostgres(t *testing.T) {
	testListBeforeReturnsAFullFirstPageWhenFewerNewerJobsRemain(t, newPGDeps(t))
}

func TestListBeforeKeepsTheFilterAndVisibilityOnPostgres(t *testing.T) {
	testListBeforeKeepsTheFilterAndVisibility(t, newPGDeps(t))
}

func TestCountByStateHasNoWindowAndSharesTheListingsFilterOnPostgres(t *testing.T) {
	testCountByStateHasNoWindowAndSharesTheListingsFilter(t, newPGDeps(t))
}
func TestAnEmptiedPageStillLeadsBackOnPostgres(t *testing.T) { testAnEmptiedPageStillLeadsBack(t, newPGDeps(t)) }
