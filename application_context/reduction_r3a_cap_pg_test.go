//go:build postgres && json1 && fts5

package application_context

import "testing"

func TestOwnerExpansionCountsAgainstTheRequestCapPG(t *testing.T) {
	testOwnerExpansionCountsAgainstTheRequestCap(t, newReductionPGContext(t))
}
