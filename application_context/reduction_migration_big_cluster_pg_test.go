//go:build postgres && json1 && fts5

package application_context

import "testing"

func TestMigrationOfALargeClusterIsLinearPG(t *testing.T) {
	testMigrationOfALargeClusterIsLinear(t, newReductionPGContext(t))
}
