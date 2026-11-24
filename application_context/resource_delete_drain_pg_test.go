//go:build postgres && json1 && fts5

package application_context

import "testing"

func TestDrainSurvivesRequestCancellationPG(t *testing.T) {
	testDrainSurvivesRequestCancellation(t, newReductionPGContext(t))
}
