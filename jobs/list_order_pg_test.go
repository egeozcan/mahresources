//go:build postgres && json1 && fts5

package jobs

import "testing"

// The state-entered listing on PostgreSQL: its per-state branches merged in
// one statement, and its keyset over state_entered_at.
func TestStateEnteredOrderOnPostgres(t *testing.T) {
	fixture := func(t *testing.T) (*Service, Deps, Snapshot, []Snapshot) {
		return stateEnteredFixtureOn(t, newPGDeps(t))
	}
	requireStateEnteredOrder(t, fixture)
	requireEveryJobOnceWithoutAState(t, fixture)
}
