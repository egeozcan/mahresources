//go:build postgres && json1 && fts5

package application_context

import "testing"

func TestStreamMatchesInMemoryComputePG(t *testing.T) {
	for seed := int64(1); seed <= 4; seed++ {
		for _, chunk := range []int{500, 1} {
			testStreamMatchesInMemoryCompute(t, func() *MahresourcesContext { return newOpsPGContext(t) }, seed, chunk)
		}
	}
}
func TestStreamIdenticalCapPG(t *testing.T) { testStreamIdenticalCap(t, newOpsPGContext(t)) }
func TestStreamFullStar250PG(t *testing.T)  { testStreamFullStar(t, newOpsPGContext(t), 250, true) }
func TestStreamFullStar199PG(t *testing.T)  { testStreamFullStar(t, newOpsPGContext(t), 199, true) }
func TestStreamStar198PG(t *testing.T)      { testStreamFullStar(t, newOpsPGContext(t), 198, false) }
func TestStreamStarWithOutsiderPG(t *testing.T) {
	testStreamFullStar(t, newOpsPGContext(t), 30, false)
}
func TestStreamVersionUploadGroupingPG(t *testing.T) {
	testStreamVersionUploadBetweenGroupingAndLoad(t, newOpsPGContext(t), false)
}
func TestStreamVersionUploadInsertPG(t *testing.T) {
	testStreamVersionUploadBetweenGroupingAndLoad(t, newOpsPGContext(t), true)
}
func TestStreamScopedPrincipalPG(t *testing.T) { testStreamScopedPrincipal(t, newOpsPGContext(t)) }
func TestStreamSeedOrderPG(t *testing.T) {
	testStreamSeedOrderIsTheWinnerRuleOrder(t, newOpsPGContext(t))
}
func TestStreamStatementsAreBoundedPG(t *testing.T) {
	testStreamStatementsAreBounded(t, newOpsPGContext(t))
}
func TestStreamAvailabilityPG(t *testing.T) {
	testStreamAvailabilityAgainstCarriedClusters(t, newOpsPGContext(t))
}
