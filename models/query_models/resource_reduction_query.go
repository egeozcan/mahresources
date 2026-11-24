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

	// OwnerId selects all currently owned Resources from the group detail page,
	// beyond its preview limit. IncludeDescendants also selects Resources owned
	// by its subgroups. This selection is stored as explicit Resource ids.
	OwnerId            uint `json:"ownerId" schema:"ownerId"`
	IncludeDescendants bool `json:"includeDescendants" schema:"includeDescendants"`

	// All selects the whole library as the creating principal can see it, instead
	// of named Resources and Groups. Stored as a flag, never expanded at rest.
	All bool `json:"all" schema:"all"`

	// MatchingMode is "identical" or "both". Empty means both, which is the
	// default a new Reduction is created with.
	MatchingMode string `json:"matchingMode" schema:"matchingMode"`

	// ExcludeExternalResources limits matching to the Extent. It defaults to
	// false and is only used when creating, not extending, a Reduction.
	ExcludeExternalResources bool `json:"excludeExternalResources" schema:"excludeExternalResources"`

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
// own labels map to: open / reviewed / skipped / applied / stale for Status
// ("open" is every open Cluster, acted on or not; "reviewed" only the acted-on
// ones; "all" no state filter), identical / near for Tier.
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
	// Approval keeps only approved (Checked) or unapproved Clusters: "approved" or
	// "unapproved". Any other value is ignored.
	Approval string `schema:"approval"`
	// Sort orders the filtered Clusters before pagination: "copies" (the default,
	// most members first), "size" (largest total member file size first) or
	// "computed" (the plan's own order). Ties break by Cluster id, so pages are
	// stable across refreshes.
	Sort string `schema:"sort"`
	// PageSize is Clusters per page: 20, 50 or 100. Anything else is 20.
	PageSize int `schema:"pageSize"`
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

	// MembersSeen is how many of the Cluster's copies the reviewer's page had
	// rendered when they acknowledged it. The review page sends it, because a page
	// past its member budget shows only a large Cluster's kept copy until "Show more
	// members" loads the rest. When it is sent, zero included, an acknowledgement is
	// refused unless it equals the Cluster's member count. Nil means the caller did not
	// say, and the acknowledgement alone stands: like AcknowledgeOversized itself this
	// is the caller's own statement, a guard against a page that skipped the copies,
	// not proof against a caller that lies.
	MembersSeen *uint `json:"membersSeen" schema:"membersSeen"`
}

// ReductionApply is a request to apply what is checked.
type ReductionApply struct {
	ID      uint `json:"id" schema:"id"`
	Version uint `json:"version" schema:"version"`
	// ExpectedClusters and ExpectedLosers are the figures the caller confirmed: how
	// many approved Clusters, and how many copies in them, the apply dialog named.
	// When given, the apply is refused with 409 and nothing is merged unless the
	// approved Clusters the caller can see right now add up to exactly those
	// numbers. The version alone cannot say so, because what a caller may see
	// changes without the plan changing (a scope change). Each is checked only when
	// present.
	ExpectedClusters *uint `json:"expectedClusters" schema:"expectedClusters"`
	ExpectedLosers   *uint `json:"expectedLosers" schema:"expectedLosers"`
	// ApplyToken is the review's `applyToken`: a digest of the exact proposals the
	// dialog confirmed (each approved Cluster's id, Winner, Losers and reviewed
	// hashes). Equal totals cannot stand in for it, because a different pair with
	// the same counts is a different merge. When sent it must match exactly, or the
	// apply is refused with 409 before anything is claimed.
	ApplyToken string `json:"applyToken" schema:"applyToken"`
	// RequestID is an opaque client-chosen id (at most 64 characters), stored with
	// the apply record so a client that lost the response can tell its own attempt
	// from another tab's.
	RequestID string `json:"requestId" schema:"requestId"`
	// SettleOnly runs only the reconciliation of Clusters an earlier batch claimed
	// and never finished, and merges nothing.
	SettleOnly bool `json:"settleOnly" schema:"settleOnly"`
}

// ReductionBulkOverride checks or unchecks many Clusters in one write, with exactly
// one way of naming them: the ids the page rendered, or every open Identical
// Cluster.
type ReductionBulkOverride struct {
	ID      uint `json:"id" schema:"id"`
	Version uint `json:"version" schema:"version"`
	// Action is "check" or "uncheck".
	Action string `json:"action" schema:"action"`
	// ClusterIDs are the Clusters to act on, any tier. Exactly one of ClusterIDs
	// and AllIdentical must be given.
	ClusterIDs []string `json:"clusterIds" schema:"clusterIds"`
	// AllIdentical acts on every open Identical Cluster in the whole plan, never on
	// the Near-Identical tier, so approval of perceptual matches across pages the
	// reviewer has not seen stays impossible.
	AllIdentical bool `json:"allIdentical" schema:"allIdentical"`
	// ExpectedCount is how many Clusters the caller believes AllIdentical names (the
	// number on its button). Required with AllIdentical; a different number is
	// refused with 409 so a stale button cannot approve more than it said.
	ExpectedCount *uint `json:"expectedCount" schema:"expectedCount"`
}
