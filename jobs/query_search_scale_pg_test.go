//go:build postgres && json1 && fts5

package jobs

import "testing"

func TestJobSearchMillionRowsPostgres(t *testing.T) {
	runJobSearchScaleEvidence(t, "postgres", newPGDeps(t))
}
