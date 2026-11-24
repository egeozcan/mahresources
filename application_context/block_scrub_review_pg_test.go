//go:build postgres && json1 && fts5

package application_context

import "testing"

// A calendar resourceId of 1e-1000 is valid JSON and underflows a float8 cast; it must
// not raise 22003 and roll back an unrelated delete.
func TestScrubNeverFailsOnOddNumbersPG(t *testing.T) {
	testScrubNeverFailsOnOddNumbers(t, newReductionPGContext(t))
}
