//go:build postgres && json1 && fts5

package application_context

import "testing"

func TestReductionLeaseJudgedAfterTheLockPG(t *testing.T) {
	testReductionLeaseJudgedAfterTheLock(t, newReductionPGContext(t))
}
