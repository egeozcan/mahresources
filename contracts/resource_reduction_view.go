package contracts

import (
	"time"

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
	// DistanceLabel is the stored perceptual distance in words, and empty when
	// there is none — a Winner, or any member of an Identical Cluster. It is a
	// rendered label for the same reason DecidedByLabel is, and for one more: a
	// template cannot ask whether Distance is set. Pongo2 has no nil literal, so
	// `member.Distance != nil` compares a typed nil pointer against an undefined
	// variable and is true, which put a bare "distance" chip on every Winner.
	DistanceLabel string
	// TagCount, NoteCount and GroupCount are how many tags, notes and related
	// Groups this member holds, counted through the caller's scoped handle so a
	// Note or Group outside their subtree is not in the number. Zero for a member
	// with no Resource behind it.
	TagCount   int
	NoteCount  int
	GroupCount int
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
	// NeedsAttention is the plan-level marker a bulk approval refuses to sweep up:
	// Lossy, Oversized or KeepsSmaller. False on a withheld Cluster.
	NeedsAttention bool
	// KeptDimensions and LargerDimensions ("800×533", "1600×1066") are the two
	// resolutions the keeps-smaller warning compares: the kept copy's and the
	// largest copy being removed. LargestMemberID is that larger copy, the one the
	// "Keep the larger copy instead" button promotes. All empty or zero unless
	// KeepsSmaller is set.
	KeptDimensions   string
	LargerDimensions string
	LargestMemberID  uint
}

// ReductionTabCounts is how many Clusters each review tab holds, over the
// Clusters the caller may fully see. A withheld Cluster is in none of them.
type ReductionTabCounts struct {
	Open    int
	Skipped int
	Stale   int
	// Applied counts Clusters whose merge is confirmed. Merging counts the ones an
	// apply has claimed but not confirmed yet: copies still exist.
	Applied int
	Merging int
}

// ReductionTierFigures is what an apply would do to one tier: the approved
// Clusters, the copies they would remove, and whether those copies would be added
// as versions of the kept one (the Reduction's keep-as-version flag for the tier).
type ReductionTierFigures struct {
	Clusters      int
	Losers        int
	KeepAsVersion bool
}

// ReductionApplyFigures are the numbers the apply dialog states. Counted over the
// approved Clusters the caller may fully see, across the whole plan.
type ReductionApplyFigures struct {
	Identical ReductionTierFigures
	Near      ReductionTierFigures
	// KeepsSmaller is how many approved Clusters keep a lower-resolution copy than
	// one they remove.
	KeepsSmaller int
}

// ReductionBulkFigures are the numbers on the bulk buttons. IdenticalApprovable is
// the target of "Approve all Identical": open, fully visible Identical Clusters
// that do not need attention. IdenticalOpen is the target of the matching
// uncheck: every open, fully visible Identical Cluster.
type ReductionBulkFigures struct {
	IdenticalApprovable int
	IdenticalOpen       int
}

// ReductionLastApplyView is the last apply as the page renders it.
type ReductionLastApplyView struct {
	StartedAt  time.Time
	FinishedAt *time.Time
	Applied    int
	Removed    int
	Stale      int
	// RequestID is the client id the apply was sent with, if any.
	RequestID string
	// Running is true while the record has no FinishedAt. Interrupted is true when
	// it has been running for longer than the apply ceiling, which means the
	// process died or the batch was abandoned: Clusters it did not reach are still
	// approved.
	Running     bool
	Interrupted bool
	// CountsHidden is true when Applied, Removed and Stale were withheld (zeroed)
	// because they count Clusters the viewer may no longer see.
	CountsHidden bool
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
	// ExtentAll is true for a whole-library Reduction: everything the caller can
	// access, with no named Resources or Groups.
	ExtentAll bool
	// SelectionGroups and SelectionResources are the first 20 (by id) named Groups
	// and Resources of the selection the caller may see, and MoreGroups and
	// MoreResources how many further visible ones there are. Ones they cannot see
	// are in neither. Empty for a whole-library Reduction.
	SelectionGroups    []ReductionNamedRef
	SelectionResources []ReductionNamedRef
	MoreGroups         int
	MoreResources      int

	// TabCounts is how many visible Clusters sit on each status tab, whatever the
	// current filters are.
	TabCounts ReductionTabCounts
	// ApplyFigures and Bulk feed the apply dialog and the bulk buttons.
	ApplyFigures ReductionApplyFigures
	// ApplyToken binds an apply to the exact proposals the dialog showed.
	ApplyToken string
	Bulk       ReductionBulkFigures
	// SettingsChanged is true when the Reduction's matching mode, Winner Rule or
	// exclude-external setting differs from what its plan was computed with. False
	// for a plan computed before settings were recorded.
	SettingsChanged bool
	// LastApply is nil until the first apply.
	LastApply *ReductionLastApplyView
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

// ReductionBulkResult is what a bulk check or uncheck did.
type ReductionBulkResult struct {
	// Version is the Reduction's version after the write.
	Version uint `json:"version"`
	// Changed is how many Clusters flipped between approved and not approved.
	Changed int `json:"changed"`
	// NeedsReview is how many Clusters a check left alone because they need
	// individual review (lossy, oversized or keeping a lower-resolution copy).
	NeedsReview int `json:"needsReview"`
}

// ReductionProgress is how far along one Reduction is, counted over the Clusters
// the caller may fully see. Computed on read from the plan, never stored: a
// scope-restricted owner's access can change after the plan was written, and a
// stored figure would keep counting Clusters they can no longer see.
type ReductionProgress struct {
	// Clusters is every visible Cluster, whatever its state.
	Clusters int
	// Open is the Clusters still awaiting a decision or an apply, Approved the part
	// of them that is approved. Skipped, Stale and Applied are the other states.
	Open     int
	Approved int
	Skipped  int
	Stale    int
	// Applied is confirmed merges only; Merging is claimed and not yet confirmed.
	Applied int
	Merging int
}

// ReductionNamedRef is an entity's id and name.
type ReductionNamedRef struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
}
