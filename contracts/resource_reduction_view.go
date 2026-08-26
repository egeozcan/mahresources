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
	// Position is this Cluster's place on the rendered page, 1-based. It exists so
	// a withheld Cluster can be labelled without its own id: that id is a hash of
	// the tier and the member ids, and ids in this schema are small integers, so
	// publishing it next to one visible member is enough to recover the hidden one
	// by enumeration.
	Position int
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
	// CoverageTrusted is false when this principal can reach less of the selection
	// than the plan was computed over. The coverage figures are a measurement made
	// under whatever access the computing reviewer had, so showing them to someone
	// whose subtree has since shrunk states how much is out there — which is the
	// thing every other surface here refuses to state.
	CoverageTrusted bool
	Clusters        []ReductionClusterView

	ClusterCount int
	// CheckedCount is how many Clusters an apply would act on across the WHOLE
	// plan, not this page's share of it. The confirm dialog names it, and naming
	// what the current page can see would understate the blast radius by however
	// many pages the reviewer has not opened.
	CheckedCount int
	// CheckedLoserCount is how many Resources those Clusters would destroy.
	CheckedLoserCount int
	Page              int
	PageSize          int

	// SelectedResources and SelectedGroups are what the reviewer picked, counted
	// as the *current* principal sees it. The stored Extent is not filtered, so
	// publishing its raw lengths would state exactly how many Resources and Groups
	// somebody whose subtree has since shrunk may no longer open.
	SelectedResources int
	SelectedGroups    int

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
	// Withheld marks an outcome that carries no identifiers at all, because the
	// Cluster reached a Resource the caller may not see. Naming it would publish
	// the very facts the render refuses to: the Cluster id is derived from the
	// member ids, so it recovers them, and the Winner and Loser ids are those ids.
	Withheld bool `json:"withheld,omitempty"`
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
