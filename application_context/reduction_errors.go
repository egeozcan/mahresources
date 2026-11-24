package application_context

import "errors"

// The overrides a reviewer can make on a Cluster.
const (
	// ReductionActionPromote makes a different member the Winner. The reviewer's
	// judgement beats the rule when the rule is wrong.
	ReductionActionPromote = "promote"
	// ReductionActionEject removes one member from the Cluster and leaves the
	// Resource completely untouched. One bad match should not force abandoning a
	// Cluster that is otherwise right.
	ReductionActionEject = "eject"
	// ReductionActionRestore puts an ejected member back.
	ReductionActionRestore = "restore"
	// ReductionActionSkip moves past a Cluster without deciding about it.
	ReductionActionSkip = "skip"
	// ReductionActionReopen takes a skipped Cluster back.
	ReductionActionReopen = "reopen"
	// ReductionActionCheck and ReductionActionUncheck select what an apply acts
	// on.
	ReductionActionCheck   = "check"
	ReductionActionUncheck = "uncheck"
)

var (
	// ErrReductionClusterNotFound is an id no Cluster in this plan carries.
	ErrReductionClusterNotFound = errors.New("no such Cluster in this Resource Reduction")
	// ErrReductionMemberNotFound is an id no member of that Cluster carries.
	ErrReductionMemberNotFound = errors.New("no such member in this Cluster")
	// ErrReductionNothingToRemove refuses approving a Cluster whose every copy is
	// kept: a merge that removes nothing.
	ErrReductionNothingToRemove = errors.New("every copy in this Cluster is kept, so there is nothing to approve")
	// ErrReductionClusterStale refuses approving a Cluster that was made stale by a
	// Resource that is gone.
	ErrReductionClusterStale = errors.New("this Cluster is stale and cannot be approved: a Resource in it no longer exists. Don't remove that copy, or recompute")
	// ErrReductionClusterSettled refuses an override on a Cluster whose merge has
	// already happened. Its Losers do not exist any more.
	ErrReductionClusterSettled = errors.New("this Cluster has already been applied")
	// ErrReductionEjectWinner refuses ejecting the Winner, which would leave the
	// Cluster with nothing to merge into.
	ErrReductionEjectWinner = errors.New("promote another member first: a Cluster cannot lose its Winner")
	// ErrReductionOversizedUnexpanded refuses checking an unusually large
	// Near-Identical Cluster that has not been acknowledged.
	ErrReductionOversizedUnexpanded = errors.New("expand this Cluster and look at it before checking it")
	// ErrReductionRestoreUnpaired refuses putting back a member that has no stored
	// perceptual pair to the Cluster's current Winner.
	ErrReductionRestoreUnpaired = errors.New("this Resource has no stored match to the Cluster's Winner, so it cannot be put back")
	// ErrReductionRestoreOutsider refuses putting back a member outside the Extent.
	// Restoring one would make it a Loser, and an outsider may never lose.
	ErrReductionRestoreOutsider = errors.New("this Resource is outside the Extent and may never be merged away, so it cannot be put back")
)

var (
	// ErrReductionBulkTarget refuses a bulk request that names its Clusters in none
	// of the two ways, or in both.
	ErrReductionBulkTarget = errors.New("name the Clusters with exactly one of clusterIds and allIdentical, and use action check or uncheck")
	// ErrReductionBulkNeedsCount refuses allIdentical without the count the caller
	// saw. The count is what keeps a stale button from approving more than it said.
	ErrReductionBulkNeedsCount = errors.New("allIdentical needs the expectedCount shown on the button")
	// ErrReductionCountMismatch is the 409 for an expectedCount that no longer
	// matches what allIdentical would act on.
	ErrReductionCountMismatch = errors.New("the number of Identical Clusters changed since you loaded this page; reload and try again")
	// ErrReductionClusterNotOpen refuses a bulk act that names a Cluster which is
	// not open. Skipped, stale and applied Clusters are reopened or settled one at
	// a time.
	ErrReductionClusterNotOpen = errors.New("a Cluster named here is not open; reload and try again")
)

var (
	// ErrReductionIndexesBuilding refuses a compute while the similarity distance
	// indexes the neighbour query reads are still being built or read invalid
	// (design D1): without them that query is a sort over a hub's whole star.
	ErrReductionIndexesBuilding = errors.New("indexes are still being built; try again shortly")
)
