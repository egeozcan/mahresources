//go:build postgres && json1 && fts5

package application_context

import "testing"

func TestMigrationBoundsAClusterScalarInTheSpoolPG(t *testing.T) {
	testMigrationBoundsAClusterScalarInTheSpool(t, newReductionPGContext(t))
}
