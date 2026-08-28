package query_models

// ResourceReductionCreator is the body the bulk bar POSTs to start a Resource
// Reduction, and the same body with ID set adds a further selection to one that
// already exists.
//
// The selection travels in the body, never in the URL: an Extent is thousands of
// ids, and the existing bulk bar already POSTs its selection everywhere else.
type ResourceReductionCreator struct {
	// ID names an existing Reduction to widen. Zero creates a new one.
	ID uint `json:"id" schema:"id"`

	Name string `json:"name" schema:"name"`

	// ResourceIds and GroupIds are the selection. Groups are expanded through
	// their descendants at compute time, not here.
	ResourceIds []uint `json:"resourceIds" schema:"resourceIds"`
	GroupIds    []uint `json:"groupIds" schema:"groupIds"`

	// MatchingMode is "identical" or "both". Empty means both, which is the
	// default a new Reduction is created with.
	MatchingMode string `json:"matchingMode" schema:"matchingMode"`

	// WinnerRule is the ordered criterion list. Empty means the default rule.
	WinnerRule []string `json:"winnerRule" schema:"winnerRule"`

	// KeepAsVersion flags, per tier. Pointers because a Reduction's stored values
	// default on for Near-Identical and off for Identical, and an absent field
	// must keep those rather than reading as false.
	KeepAsVersionIdentical *bool `json:"keepAsVersionIdentical" schema:"keepAsVersionIdentical"`
	KeepAsVersionNear      *bool `json:"keepAsVersionNear" schema:"keepAsVersionNear"`
}

// ResourceReductionQuery filters the Reduction list.
//
// OwnerUserID and OwnerRestricted are not request fields — the handler sets them
// from the authenticated principal after decoding, exactly as the download
// history does, so a caller cannot widen its own visibility by sending them.
type ResourceReductionQuery struct {
	Name   string   `schema:"name"`
	Status []string `schema:"status"`
	SortBy []string `schema:"sortBy"`

	OwnerUserID     *uint `schema:"-"`
	OwnerRestricted bool  `schema:"-"`
}

// ResourceReductionReviewQuery filters the Clusters of one Reduction's review.
//
// Unlike the list query it is applied in memory: the plan is one decoded JSON
// document, so there is no SQL to scope. The values are the tokens the page's
// own labels map to: open / reviewed / skipped / applied / stale for Status,
// identical / near for Tier.
type ResourceReductionReviewQuery struct {
	// Status is an OR over the Cluster states the page shows. "reviewed" is not
	// a state of its own — it is a judgement recorded on an open Cluster — so it
	// selects `open` Clusters that have been acted on, and "open" selects the
	// rest.
	Status []string `schema:"status"`
	// Tier is an OR over the matching tiers.
	Tier []string `schema:"tier"`
	// NeedsAttention keeps only the Clusters that ask for a closer look: a Loser
	// holding something the Winner does not, or an unusually large Near-Identical
	// Cluster. Both are facts of the plan, which is what makes the filter
	// server-side at all — withheld Clusters are decided at render time and are
	// deliberately not part of it.
	NeedsAttention bool `schema:"attention"`
}

// ResourceReductionEditor updates a Reduction's own settings, as opposed to its
// Extent or its plan.
type ResourceReductionEditor struct {
	ID                     uint     `json:"id" schema:"id"`
	Name                   string   `json:"name" schema:"name"`
	MatchingMode           string   `json:"matchingMode" schema:"matchingMode"`
	WinnerRule             []string `json:"winnerRule" schema:"winnerRule"`
	KeepAsVersionIdentical *bool    `json:"keepAsVersionIdentical" schema:"keepAsVersionIdentical"`
	KeepAsVersionNear      *bool    `json:"keepAsVersionNear" schema:"keepAsVersionNear"`

	// Version is the optimistic-concurrency counter the caller last saw. A write
	// carrying a stale one is refused rather than merged.
	Version uint `json:"version" schema:"version"`
}

// ReductionOverride is one review decision about one Cluster.
//
// Version is what the caller last saw. Every override, the recompute and the
// apply all write the same JSON document, so a decision taken from a stale page
// is refused rather than merged — merging two judgements about which files to
// delete is not something that can be done safely.
type ReductionOverride struct {
	ID        uint   `json:"id" schema:"id"`
	Version   uint   `json:"version" schema:"version"`
	ClusterID string `json:"clusterId" schema:"clusterId"`
	Action    string `json:"action" schema:"action"`
	// ResourceID names the member a promote, eject or restore acts on. Unused by
	// the whole-Cluster actions.
	ResourceID uint `json:"resourceId" schema:"resourceId"`

	// AcknowledgeOversized is required to check an unusually large Near-Identical
	// Cluster. The browser sets it once the reviewer has expanded the Cluster;
	// the server refuses without it, so the guard means something to a caller
	// that is not the page — which is the only kind of caller that could check
	// three hundred files in one request without having looked at any of them.
	AcknowledgeOversized bool `json:"acknowledgeOversized" schema:"acknowledgeOversized"`
}

// ReductionApply is a request to apply what is checked.
type ReductionApply struct {
	ID      uint `json:"id" schema:"id"`
	Version uint `json:"version" schema:"version"`
}
