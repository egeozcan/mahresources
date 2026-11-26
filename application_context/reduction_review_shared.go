package application_context

import (
	"mahresources/contracts"
	"mahresources/models"
	"strconv"
	"time"
)

// ReductionClustersPerPage is how many Clusters one page of the review shows.
//
// Small, because a Cluster is several thumbnails, a justification and a row of
// controls, and the reviewer is meant to look at each one. A Reduction with
// thousands of Clusters still has to load.
const ReductionClustersPerPage = 20

const reductionSelectionNamesShown = 20

// reductionSettingsChanged reports whether the row's settings differ from the ones
// the plan was computed with. A plan with none recorded has nothing to compare.
// Keep-as-version flags are not compared: they act at apply, not at compute.
func reductionSettingsChanged(reduction *models.ResourceReduction, ran *models.ReductionSettings) bool {
	if ran == nil {
		return false
	}
	if ran.MatchingMode != normalizeMatchingMode(reduction.MatchingMode) ||
		ran.ExcludeExternal != reduction.ExcludeExternalResources {
		return true
	}
	rule := DecodeWinnerRule(reduction.WinnerRule)
	if len(rule) != len(ran.WinnerRule) {
		return true
	}
	for i := range rule {
		if rule[i] != ran.WinnerRule[i] {
			return true
		}
	}
	return false
}

// describeLastApply is the stored apply record as the page shows it, with the
// interruption reading derived from the clock: a record still running past the
// ceiling is one whose process died or whose batch was abandoned.
func describeLastApply(last *models.ReductionLastApply) *contracts.ReductionLastApplyView {
	if last == nil {
		return nil
	}
	running := last.FinishedAt == nil
	return &contracts.ReductionLastApplyView{
		StartedAt:   last.StartedAt,
		FinishedAt:  last.FinishedAt,
		Applied:     last.Applied,
		Removed:     last.Removed,
		Stale:       last.Stale,
		RequestID:   last.RequestID,
		Running:     running,
		Interrupted: running && time.Since(last.StartedAt) > ReductionApplyCeiling,
	}
}

// ReductionPageSizes are the page sizes the review offers.
var ReductionPageSizes = []int{20, 50, 100}

func normalizeReviewPageSize(asked int) int {
	for _, size := range ReductionPageSizes {
		if asked == size {
			return size
		}
	}
	return ReductionClustersPerPage
}

func resourceIDsOf(resources map[uint]*models.Resource) []uint {
	ids := make([]uint, 0, len(resources))
	for id := range resources {
		ids = append(ids, id)
	}
	return ids
}

// reductionDistanceLabel renders a member's stored perceptual distance, and
// nothing at all when it has none. A Winner has no distance to itself and the
// Identical tier has no distances anywhere, so the empty string is the common
// case rather than an error one.
func reductionDistanceLabel(distance *uint8) string {
	if distance == nil {
		return ""
	}
	return "distance " + strconv.FormatUint(uint64(*distance), 10)
}

func containsUint(list []uint, v uint) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// loadResourcesByID reads the named Resources through the caller's scoped handle.
// An id that is missing from the result is one that was deleted or that this
// principal may not see; the two are deliberately not distinguished.
func (ctx *MahresourcesContext) loadResourcesByID(ids []uint) (map[uint]*models.Resource, error) {
	out := map[uint]*models.Resource{}
	for _, chunk := range chunkUints(dedupeUints(ids), idChunk) {
		var batch []*models.Resource
		if err := ctx.db.
			Preload("Series").
			Preload("ResourceCategory").
			Where("resources.id IN ?", chunk).
			Find(&batch).Error; err != nil {
			return nil, err
		}
		for _, r := range batch {
			out[r.ID] = r
		}
	}
	return out, nil
}
