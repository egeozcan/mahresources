package contracts

import (
	"mahresources/models"
)

// ReductionMemberView is one Cluster member with the Resource behind it.
//
// The Resource is loaded at render rather than copied into the plan, and that is
// the point: the load goes through the caller's own scoped handle, so a Reduction
// made before somebody's access changed cannot still show them what they may no
// longer see. A member the current principal cannot read comes back with a nil
// Resource and is rendered as withheld rather than silently dropped, so the
// Cluster's size does not quietly change under them.
type ReductionMemberView struct {
	*models.ReductionMember
	Resource *models.Resource
	IsWinner bool
	IsLoser  bool
}

// ReductionClusterView is one Cluster ready to render.
type ReductionClusterView struct {
	*models.ReductionCluster
	Winner  *models.Resource
	Members []ReductionMemberView
	// DecidedByLabel is the deciding criterion in words, and StateLabel the
	// Cluster's state. Both are rendered rather than the raw fields: "open" is not
	// a sentence, and a template-side guard in internal/arch refuses any field
	// named State on the grounds that NoteBlock.State is a types.JSON that renders
	// as "<types.JSON Value>" to a reader.
	DecidedByLabel string
	StateLabel     string
	// Withheld counts members the current principal may not see. A Cluster with
	// any is not actionable: applying it would destroy Resources the reviewer was
	// never shown.
	Withheld int
}

// ReductionReview is a page of a Reduction's plan, hydrated and paginated.
//
// Clusters paginate; members within a Cluster do not. A Cluster is the unit of
// judgement, and splitting one across two pages would mean deciding about half of
// it.
type ReductionReview struct {
	Reduction *models.ResourceReduction
	// Status is the effective status: a run still `computing` past its deadline
	// reads as failed and is recomputable.
	Status   string
	Coverage models.ReductionCoverage
	Clusters []ReductionClusterView

	ClusterCount int
	Page         int
	PageSize     int

	// ExtentSize is how many Resources the Extent holds right now, and
	// EnteredSinceCompute how many of those arrived after the plan was computed.
	// Together they are the drift report: a Group-scoped Reduction is re-scanned
	// only when the reviewer asks, so the page says what it would find rather
	// than surprising them with it.
	ExtentSize          int
	EnteredSinceCompute int
}

// ReductionApplyOutcome is what happened to one Cluster in an apply.
//
// A refused Cluster is named here as well as marked on the row, because the
// reviewer has to be able to find it: "one of your four hundred Clusters went
// stale" is not something anybody can act on.
type ReductionApplyOutcome struct {
	ClusterID string `json:"clusterId"`
	Tier      string `json:"tier,omitempty"`
	WinnerID  uint   `json:"winnerId,omitempty"`
	LoserIDs  []uint `json:"loserIds,omitempty"`
	// Reason is why the Cluster was refused. Empty for one that was applied.
	Reason string `json:"reason,omitempty"`
}

// ReductionApplyResult is the report of one apply.
//
// Applying is partial by design, so this is never "it worked" or "it did not":
// it is what merged and what was refused, and the Reduction keeps everything in
// the second list.
type ReductionApplyResult struct {
	Applied []ReductionApplyOutcome `json:"applied"`
	Stale   []ReductionApplyOutcome `json:"stale"`
}

// DestroyedCount is how many Resources this apply actually merged away.
func (r *ReductionApplyResult) DestroyedCount() int {
	total := 0
	for _, outcome := range r.Applied {
		total += len(outcome.LoserIDs)
	}
	return total
}
