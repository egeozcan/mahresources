//go:build postgres && json1 && fts5

package jobs

import "testing"

func TestJobQueryPlansMillionRowsPostgres(t *testing.T) {
	deps := newPGDeps(t)
	runMillionJobQueryPlanEvidence(t, "postgres", deps)
}
