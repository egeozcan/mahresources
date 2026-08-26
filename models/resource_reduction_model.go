package models

import (
	"mahresources/models/types"
	"time"
)

// ResourceReduction is a named, durable proposal to collapse Clusters of
// Identical and Near-Identical Resources down to one Winner each.
//
// The whole feature lives in this one row: the Extent it considers, the plan its
// clustering job computed, and every decision the reviewer has made about that
// plan. ADR 0003 records why, in one line: group import's file-backed plan keeps
// its decisions in an Alpine object that a reload destroys, and derives its
// authorization from an in-memory job record that is swept an hour after the job
// finishes. A Reduction is named, accumulated over several sittings and applied
// in parts, so both of those properties are disqualifying.
//
// It never expires. There is no retention sweep on this table, deliberately —
// this is domain data someone named and curated, not an artifact like an export
// tar.
type ResourceReduction struct {
	ID        uint      `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `gorm:"index" json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`

	// Name is what tells two in-progress Reductions apart. Deliberately not
	// unique — the list shows creation date and time, which is what actually
	// distinguishes two called "holiday photos".
	Name string `gorm:"size:1000;not null" json:"name"`

	// CreatedByUserId is the owner. Named to match the other stamped models so
	// the user-deletion sweep covers this table for free. Visibility is
	// owner-restricted with admins seeing all, so a NULL owner — which is what a
	// deleted user leaves behind — belongs to nobody.
	CreatedByUserId *uint `gorm:"index" json:"createdByUserId,omitempty"`

	// Status is one of the ReductionStatus* constants below.
	Status string `gorm:"size:20;not null;index" json:"status"`

	// MatchingMode selects which tiers the clustering job computes.
	MatchingMode string `gorm:"size:20;not null" json:"matchingMode"`

	// KeepAsVersionIdentical / KeepAsVersionNear are two flags rather than one
	// because the default matching mode contains both tiers and they need
	// opposite values: a byte-identical Loser has nothing to preserve, while a
	// Near-Identical one holds pixels the reviewer decided against and may want
	// back. Neither flag governs whether the file is retained — see the note on
	// MergeResources; it governs whether a further version is created.
	KeepAsVersionIdentical bool `gorm:"not null;default:false" json:"keepAsVersionIdentical"`
	KeepAsVersionNear      bool `gorm:"not null;default:true" json:"keepAsVersionNear"`

	// WinnerRule is the ordered list of criteria that picks each Cluster's
	// Winner, stored as a JSON array of criterion tokens.
	WinnerRule types.JSON `gorm:"type:json" json:"-"`

	// Extent is the ResourceReductionExtent: an explicit Resource set, a Group
	// set expanded through descendants at compute time, or both. Never flattened
	// into a URL, and never expanded at rest — a Group's descendants change.
	Extent types.JSON `gorm:"type:json" json:"-"`

	// Plan is the ResourceReductionPlan: the computed Clusters together with
	// every decision made about them. One document, three independent writers
	// (recompute, override, apply), which is what Version is for.
	Plan types.JSON `gorm:"type:json" json:"-"`

	// ComputedAt is when the plan in this row was produced. Nil before the first
	// successful compute.
	ComputedAt *time.Time `json:"computedAt,omitempty"`

	// ComputingStartedAt stamps the start of the current clustering job, and
	// ComputeDeadline is when a run still holding `computing` should be read as
	// failed. Generic queue jobs are not drained at shutdown, so this deadline —
	// not the queue — is what guarantees a Reduction cannot be stranded at
	// `computing` forever on a table that never expires.
	ComputingStartedAt *time.Time `json:"computingStartedAt,omitempty"`
	ComputeDeadline    *time.Time `json:"computeDeadline,omitempty"`

	// ComputeJobID is the queue job computing this Reduction, so the page can
	// follow its progress.
	ComputeJobID string `gorm:"size:64" json:"computeJobId,omitempty"`

	// ComputeError is the message from the last failed compute, shown on the page
	// so a failure is diagnosable rather than merely visible.
	ComputeError string `gorm:"size:2000" json:"computeError,omitempty"`

	// Version is the optimistic-concurrency counter. Recompute, each override and
	// apply are three independent read-modify-write writers on the Plan document,
	// so every write is a compare-and-set on this integer — the shape
	// ClaimDownloadHistoryRetry already uses. A stale write is refused, never
	// merged.
	Version uint `gorm:"not null;default:0" json:"version"`
}

func (r ResourceReduction) GetId() uint {
	return r.ID
}

// Reduction lifecycle statuses.
const (
	// ReductionStatusDraft is a Reduction that has never been computed.
	ReductionStatusDraft = "draft"
	// ReductionStatusComputing is a Reduction with a clustering job in flight.
	ReductionStatusComputing = "computing"
	// ReductionStatusReady is a Reduction holding a computed plan.
	ReductionStatusReady = "ready"
	// ReductionStatusFailed is a Reduction whose last compute failed or whose
	// compute deadline passed while it was still `computing`.
	ReductionStatusFailed = "failed"
)

// Matching modes. Identical-only is an index-supported GROUP BY over the content
// hash: it covers video, PDF and audio, which have no perceptual hash at all, and
// it is the only mode that is tractable on a very large library.
const (
	MatchingModeIdenticalOnly = "identical"
	MatchingModeBothTiers     = "both"
)

// Cluster tiers.
const (
	// ReductionTierIdentical is byte-identity: same content hash, any content
	// type. A fact.
	ReductionTierIdentical = "identical"
	// ReductionTierNear is perceptual distance within threshold, images only. A
	// guess, and defaulted accordingly.
	ReductionTierNear = "near"
)

// Cluster states. A Cluster is open until something terminal happens to it.
const (
	// ReductionClusterOpen is awaiting review, or reviewed and awaiting apply.
	ReductionClusterOpen = "open"
	// ReductionClusterSkipped is one the reviewer moved past. Frozen, never
	// applied, and excluded from re-clustering while it stays that way.
	ReductionClusterSkipped = "skipped"
	// ReductionClusterApplied is one whose merge has been performed. Its Losers
	// no longer exist; its Winner returns to the pool as an ordinary candidate.
	ReductionClusterApplied = "applied"
	// ReductionClusterStale is one that failed revalidation at apply. It stays in
	// the Reduction so the reviewer can look at it rather than hunt for it.
	ReductionClusterStale = "stale"
)

// ResourceReductionExtent is the set of Resources a Reduction considers. Both
// halves may be populated: a Reduction created from a Resource selection and then
// widened with a Group selection carries each.
//
// Group ids are stored, never their expansion. A Group's descendants and its
// contents both change, and D23 makes re-scanning an explicit act.
type ResourceReductionExtent struct {
	ResourceIDs []uint `json:"resourceIds,omitempty"`
	GroupIDs    []uint `json:"groupIds,omitempty"`
}

// ResourceReductionPlan is the computed proposal plus the review made of it.
type ResourceReductionPlan struct {
	Clusters []*ReductionCluster `json:"clusters"`
	Coverage ReductionCoverage   `json:"coverage"`
}

// ReductionCoverage reports how much of the Extent the clustering could actually
// examine, so "no repeats found" stays distinguishable from "nothing was hashed".
type ReductionCoverage struct {
	// ExtentSize is how many Resources the Extent expanded to.
	ExtentSize int `json:"extentSize"`
	// ContentHashed is how many of them carry a content hash. The rest cannot be
	// matched byte-identically and are excluded from the Identical tier, because
	// the empty string is a live value in this schema and a GROUP BY would
	// otherwise collapse every one of them into a single Cluster.
	ContentHashed int `json:"contentHashed"`
	// PerceptualHashed is how many carry a perceptual hash. Only four raster
	// image formats ever do, so a library of video and PDFs reads as zero here
	// and that is correct, not a fault.
	PerceptualHashed int `json:"perceptualHashed"`
	// PerceptualEligible is how many are of a content type a perceptual hash
	// exists for. PerceptualHashed below this is the number worth recomputing.
	PerceptualEligible int `json:"perceptualEligible"`
}

// ReductionCluster is one proposed merge: a set of Resources, exactly one of
// which is the Winner, together with the justification for that choice and every
// decision the reviewer has made about it.
type ReductionCluster struct {
	// ID is stable across recomputes for the same membership, so a frozen
	// Cluster keeps its identity and the page's per-Cluster controls address the
	// same thing after a reload.
	ID string `json:"id"`

	Tier string `json:"tier"`

	// WinnerID is the surviving Resource. Every other non-ejected member is a
	// Loser.
	WinnerID uint `json:"winnerId"`

	Members []*ReductionMember `json:"members"`

	// DecidedBy is the Winner Rule criterion that actually discriminated, and
	// Margin is that criterion's margin in words — "4.0x the pixels" reads
	// differently from "1.01x the pixels", which is the whole point of showing
	// it. Undecided is set when every criterion tied and the Winner fell to
	// lowest id, which is a tiebreaker of last resort rather than a decision.
	DecidedBy string `json:"decidedBy,omitempty"`
	Margin    string `json:"margin,omitempty"`
	Undecided bool   `json:"undecided,omitempty"`

	// Lossy names the fields a Loser holds and the Winner does not — merge drops
	// all three, so this is the warning that stops a curated copy being discarded
	// silently. Recomputed on every change of Winner.
	Lossy []string `json:"lossy,omitempty"`

	State string `json:"state"`

	// Checked is what apply acts on. Identical Clusters arrive checked because
	// byte-identity is a fact; Near-Identical ones arrive unchecked because
	// perceptual similarity is a guess.
	Checked bool `json:"checked"`

	// Reviewed marks a Cluster explicitly acted on. It freezes: its members are
	// held out of re-clustering so growing the Extent can add Clusters but never
	// rearrange a judgement already made. Arriving checked by default is not
	// "acted on".
	Reviewed bool `json:"reviewed,omitempty"`

	// Oversized marks a Near-Identical Cluster above the size threshold. It
	// arrives unchecked whatever the tier default says and must be expanded
	// before it can be acted on, so a chained match cannot delete three hundred
	// files behind one checkbox. It never applies to Identical, where a large
	// Cluster is merely numerous.
	Oversized bool `json:"oversized,omitempty"`

	// StaleReason names what failed revalidation at apply.
	StaleReason string     `json:"staleReason,omitempty"`
	AppliedAt   *time.Time `json:"appliedAt,omitempty"`
}

// ReductionMember is one Resource in a Cluster.
//
// It carries ids and facts about the match, never a copy of the Resource's own
// fields: the page hydrates members from the database at render, which is also
// where the current principal's subtree is re-checked. A Reduction made before
// someone's access changed must not still show them what they may no longer see.
type ReductionMember struct {
	ResourceID uint `json:"resourceId"`

	// Hash is the member's content hash at compute time, and it is what makes
	// staleness detectable at all. A version upload rewrites resources.hash and
	// leaves the similarity pairs untouched, so the pair table cannot report that
	// the reviewed bytes are gone. Apply refuses any Cluster where a member's
	// current hash no longer matches this.
	Hash string `json:"hash"`

	// InExtent is false for a Resource the Reduction reached but was not asked to
	// consider. Such a member may win, absorbing associations, and may never
	// lose. A Cluster holds at most one.
	InExtent bool `json:"inExtent,omitempty"`

	// Ejected members are left completely untouched. Ejection is a safe action,
	// which is what makes it usable freely.
	Ejected bool `json:"ejected,omitempty"`

	// EjectedReason distinguishes the reviewer's own ejection from the automatic
	// one that follows a promote, so "I did this" and "this happened because of
	// what I did" read differently.
	EjectedReason string `json:"ejectedReason,omitempty"`

	// Distance is the stored perceptual distance between this member and the
	// Winner. Nil on the Identical tier, and nil for the Winner itself.
	Distance *uint8 `json:"distance,omitempty"`
}

// Ejection reasons.
const (
	EjectReasonManual      = "manual"
	EjectReasonNoPairToWinner = "no-pair-to-winner"
)

// IsLoser reports whether a member would be merged away and deleted by an apply.
func (m *ReductionMember) IsLoser(winnerID uint) bool {
	return !m.Ejected && m.ResourceID != winnerID
}

// LoserIDs returns the Resources this Cluster would destroy, in member order.
func (c *ReductionCluster) LoserIDs() []uint {
	ids := make([]uint, 0, len(c.Members))
	for _, m := range c.Members {
		if m.IsLoser(c.WinnerID) {
			ids = append(ids, m.ResourceID)
		}
	}
	return ids
}

// Frozen reports whether a Cluster's members are held out of re-clustering.
//
// Skipped and stale Clusters freeze so a recompute cannot rearrange a judgement
// already made. An applied one does not: its Losers no longer exist and its
// Winner returns to the pool as an ordinary candidate, because what freezing
// protects is the judgement, not the Winner's future eligibility. A duplicate
// arriving next month has to be catchable.
func (c *ReductionCluster) Frozen() bool {
	if c.State == ReductionClusterApplied {
		return false
	}
	return c.Reviewed || c.State == ReductionClusterSkipped || c.State == ReductionClusterStale
}
