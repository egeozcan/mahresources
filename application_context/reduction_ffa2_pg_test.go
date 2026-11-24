//go:build postgres && json1 && fts5

package application_context

import "testing"

func TestReductionDeleteRemovesItsSpoolPG(t *testing.T) {
	testReductionDeleteRemovesItsSpool(t, newReductionPGContext(t))
}
