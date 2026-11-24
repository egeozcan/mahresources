package models

import (
	"time"

	"mahresources/models/types"
)

// The tables below hold a Reduction's plan as rows instead of one JSON
// document (docs/plans/2026-11-24-reduction-plan-tables-design.md, D1). None of
// them has a foreign key: a Cluster naming a Resource that has since been
// deleted is a staleness the apply revalidation reports, not a row the database
// quietly rewrites. Every write to them goes through the application_context
// reductionStore; a guard test keeps it that way.
//
// The Go names carry a Row/Reduction prefix because ReductionCluster and
// ReductionMember are already the JSON plan's types, which the legacy plan and
// the review page still use.

// ReductionClusterRow is one Cluster of one generation of a Reduction's plan.
type ReductionClusterRow struct {
	ID          uint   `gorm:"primaryKey;index:idx_rc_batch,priority:5;index:idx_rc_bulk,priority:4;index:idx_rc_claims,priority:5"`
	ReductionID uint   `gorm:"not null;default:0;uniqueIndex:ux_reduction_clusters_key,priority:1;index:idx_rc_position,priority:1;index:idx_rc_state,priority:1;index:idx_rc_members,priority:1;index:idx_rc_size,priority:1;index:idx_rc_batch,priority:1;index:idx_rc_outcome,priority:1;index:idx_rc_bulk,priority:1;index:idx_rc_claims,priority:1"`
	Generation  string `gorm:"size:64;not null;default:'';uniqueIndex:ux_reduction_clusters_key,priority:2;index:idx_rc_position,priority:2;index:idx_rc_state,priority:2;index:idx_rc_members,priority:2;index:idx_rc_size,priority:2;index:idx_rc_batch,priority:2;index:idx_rc_outcome,priority:2;index:idx_rc_bulk,priority:2;index:idx_rc_claims,priority:2"`
	// Key is the Cluster's address in the API and the DOM. On Postgres it is
	// COLLATE "C" (EnsureReductionKeyCollation), so SQL order is Go's byte order.
	Key string `gorm:"column:key;size:64;not null;default:'';uniqueIndex:ux_reduction_clusters_key,priority:3;index:idx_rc_members,priority:4;index:idx_rc_size,priority:4"`

	// Position is plan order: carried Clusters first in their old order, then build order.
	Position int64 `gorm:"not null;default:0;index:idx_rc_position,priority:3;index:idx_rc_batch,priority:4"`

	Tier         string `gorm:"size:20;not null;default:'';index:idx_rc_state,priority:4"`
	State        string `gorm:"size:20;not null;default:'';index:idx_rc_state,priority:3;index:idx_rc_claims,priority:3"`
	WinnerID     uint   `gorm:"not null;default:0"`
	Checked      bool   `gorm:"not null;default:false;index:idx_rc_state,priority:5"`
	Reviewed     bool   `gorm:"not null;default:false"`
	Oversized    bool   `gorm:"not null;default:false"`
	DecidedBy    string `gorm:"type:text;not null;default:''"`
	Margin       string `gorm:"type:text;not null;default:''"`
	Undecided    bool   `gorm:"not null;default:false"`
	KeepsSmaller bool   `gorm:"not null;default:false"`

	// ReadOnly is history that cannot be acted on (a legacy Cluster over the member cap).
	ReadOnly bool `gorm:"not null;default:false"`
	// Lossy is the list of field names a Loser holds and the Winner lacks.
	Lossy       types.JSON `gorm:"type:json"`
	StaleReason string     `gorm:"type:text;not null;default:''"`
	Merged      bool       `gorm:"not null;default:false;index:idx_rc_claims,priority:4"`
	ClaimID     string     `gorm:"type:text;not null;default:''"`
	AppliedAt   *time.Time

	// Derived columns (deriveClusterColumns, D3). Nothing else writes them except
	// the SQL paths that maintain gone_members.
	MemberCount    int    `gorm:"not null;default:0;index:idx_rc_members,priority:3,sort:desc"`
	LoserCount     int    `gorm:"not null;default:0"`
	NeedsAttention bool   `gorm:"not null;default:false"`
	Fingerprint    string `gorm:"size:64;not null;default:''"`
	TotalSize      int64  `gorm:"not null;default:0;index:idx_rc_size,priority:3,sort:desc"`
	GoneMembers    int    `gorm:"not null;default:0"`

	// Truncated counts eligible members left out by the member cap (D6).
	Truncated int `gorm:"not null;default:0"`
	// RestoreSeq is bumped by promote and restore, the only ways a Loser is added.
	RestoreSeq int `gorm:"not null;default:0"`

	// Apply bookkeeping (D10).
	ApplyBatch     string `gorm:"size:64;not null;default:'';index:idx_rc_batch,priority:3"`
	ApprovedWinner uint   `gorm:"not null;default:0"`
	ApprovedSeq    int    `gorm:"not null;default:0"`
	OutcomeBatch   string `gorm:"size:64;not null;default:'';index:idx_rc_outcome,priority:3"`
	// BulkOp marks the exact counted target set of one bulk write (D3).
	BulkOp string `gorm:"size:64;not null;default:'';index:idx_rc_bulk,priority:3"`

	// SeedResourceID is the seed of a Near Cluster built by this code, and
	// FixedSeed is set when the insider query reached its cap (D5).
	SeedResourceID uint `gorm:"not null;default:0"`
	FixedSeed      bool `gorm:"not null;default:false"`
}

func (ReductionClusterRow) TableName() string { return "reduction_clusters" }

// ReductionMemberRow is one Resource in a Cluster.
type ReductionMemberRow struct {
	ID          uint   `gorm:"primaryKey"`
	ClusterID   uint   `gorm:"not null;default:0;index:idx_rm_cluster,priority:1"`
	ReductionID uint   `gorm:"not null;default:0;index:idx_rm_resource,priority:1"`
	Generation  string `gorm:"size:64;not null;default:'';index:idx_rm_resource,priority:2"`
	ResourceID  uint   `gorm:"not null;default:0;index:idx_rm_resource,priority:3;index:idx_rm_resource_only"`
	Position    int    `gorm:"not null;default:0;index:idx_rm_cluster,priority:2"`

	Hash string `gorm:"not null;default:''"`
	// FileSize is the Resource's size at compute, so a Cluster's total derives from its members.
	FileSize int64 `gorm:"not null;default:0"`

	InExtent      bool   `gorm:"not null;default:false"`
	Ejected       bool   `gorm:"not null;default:false"`
	EjectedReason string `gorm:"type:text;not null;default:''"`
	Distance      *int   // near tier; nil for the Winner and for Identical

	// OwnerAtMerge is the Group that owned a merged Loser, 0 for unknown.
	OwnerAtMerge uint `gorm:"not null;default:0"`
	// Gone says the Resource no longer exists (D9), for every member, ejected or not.
	Gone bool `gorm:"not null;default:false"`
	// Changed says the member failed the apply's revalidation.
	Changed bool `gorm:"not null;default:false"`
}

func (ReductionMemberRow) TableName() string { return "reduction_members" }

// ReductionExtentResource is one explicitly selected Resource of a Reduction.
type ReductionExtentResource struct {
	ReductionID uint `gorm:"primaryKey;autoIncrement:false"`
	ResourceID  uint `gorm:"primaryKey;autoIncrement:false"`
}

func (ReductionExtentResource) TableName() string { return "reduction_extent_resources" }

// ReductionExtentGroup is one selected Group of a Reduction.
type ReductionExtentGroup struct {
	ReductionID uint `gorm:"primaryKey;autoIncrement:false"`
	GroupID     uint `gorm:"primaryKey;autoIncrement:false"`
}

func (ReductionExtentGroup) TableName() string { return "reduction_extent_groups" }

// ReductionExtentRow is a Resource staged into one compute generation's Extent.
type ReductionExtentRow struct {
	ReductionID uint   `gorm:"primaryKey;autoIncrement:false;index:idx_re_hash,priority:1"`
	Generation  string `gorm:"primaryKey;size:64;index:idx_re_hash,priority:2"`
	ResourceID  uint   `gorm:"primaryKey;autoIncrement:false"`
	Hash        string `gorm:"not null;default:'';index:idx_re_hash,priority:3"`
}

func (ReductionExtentRow) TableName() string { return "reduction_extent" }

// ReductionSeed is a Near-tier seed candidate staged for one compute generation,
// ordered by the Winner Rule's byte key. KeyPrefix is the first 512 bytes of
// SortKey (a Postgres btree row must stay under about 2.7 KB).
type ReductionSeed struct {
	ReductionID uint   `gorm:"primaryKey;autoIncrement:false;index:idx_rseed_order,priority:1"`
	Generation  string `gorm:"primaryKey;size:64;index:idx_rseed_order,priority:2"`
	ResourceID  uint   `gorm:"primaryKey;autoIncrement:false"`
	KeyPrefix   []byte `gorm:"not null;index:idx_rseed_order,priority:3"`
	SortKey     []byte `gorm:"not null"`
}

func (ReductionSeed) TableName() string { return "reduction_seeds" }

// ReductionDeletedID stages the Resource ids one deleting transaction removes
// (D9), under one operation id per transaction.
type ReductionDeletedID struct {
	Op         string `gorm:"primaryKey;size:64"`
	ResourceID uint   `gorm:"primaryKey;autoIncrement:false"`
}

func (ReductionDeletedID) TableName() string { return "reduction_deleted_ids" }

// ResourceCleanupManifest records what a deleting transaction must clean up
// after it commits (files, thumbnails by hash and storage location), so nothing
// collected for the whole request is held in Go (D9).
type ResourceCleanupManifest struct {
	ID              uint   `gorm:"primaryKey"`
	Op              string `gorm:"size:64;not null;default:'';index"`
	ResourceID      uint   `gorm:"not null;default:0"`
	Hash            string `gorm:"not null;default:''"`
	Location        string `gorm:"not null;default:''"`
	StorageLocation *string
	// Name, OwnerID and ResourceCategoryID are what the after-delete hook and the
	// backup path need, so the drain after commit reads them back from here
	// instead of holding the whole request's Resources in Go.
	Name               string `gorm:"not null;default:''"`
	OwnerID            *uint
	ResourceCategoryID uint `gorm:"not null;default:0"`
}

func (ResourceCleanupManifest) TableName() string { return "resource_cleanup_manifest" }

// ReductionMigrationRow is the migration's spool (D12): the legacy plan parsed
// once, server side, one row per member carrying its Cluster's scalar fields.
// Kind says what the row is: "member", "extent_resource", "extent_group" or
// "apply_cluster" (a legacy lastApply.clusterIds entry).
type ReductionMigrationRow struct {
	ReductionID uint   `gorm:"primaryKey;autoIncrement:false"`
	Generation  string `gorm:"primaryKey;size:64"`
	Kind        string `gorm:"primaryKey;size:20"`
	ClusterOrd  int    `gorm:"primaryKey;autoIncrement:false"`
	MemberOrd   int    `gorm:"primaryKey;autoIncrement:false"`

	ClusterKey   string     `gorm:"size:64;not null;default:''"`
	Tier         string     `gorm:"size:20;not null;default:''"`
	State        string     `gorm:"size:20;not null;default:''"`
	WinnerID     uint       `gorm:"not null;default:0"`
	Checked      bool       `gorm:"not null;default:false"`
	Reviewed     bool       `gorm:"not null;default:false"`
	Oversized    bool       `gorm:"not null;default:false"`
	DecidedBy    string     `gorm:"type:text;not null;default:''"`
	Margin       string     `gorm:"type:text;not null;default:''"`
	Undecided    bool       `gorm:"not null;default:false"`
	KeepsSmaller bool       `gorm:"not null;default:false"`
	Lossy        types.JSON `gorm:"type:json"`
	StaleReason  string     `gorm:"type:text;not null;default:''"`
	Merged       bool       `gorm:"not null;default:false"`
	ClaimID      string     `gorm:"type:text;not null;default:''"`
	AppliedAt    *time.Time

	ResourceID    uint   `gorm:"not null;default:0"`
	Hash          string `gorm:"not null;default:''"`
	InExtent      bool   `gorm:"not null;default:false"`
	Ejected       bool   `gorm:"not null;default:false"`
	EjectedReason string `gorm:"type:text;not null;default:''"`
	Distance      *int
	OwnerAtMerge  uint `gorm:"not null;default:0"`
	Changed       bool `gorm:"not null;default:false"`
	// Rejected marks a member or extent entry whose id the spool refused (not a
	// positive id that fits an int64): it is counted and logged, never clamped.
	Rejected bool `gorm:"not null;default:false"`
}

func (ReductionMigrationRow) TableName() string { return "reduction_migration_rows" }

// ReductionMigration is one migration run's own state (D12): its generation,
// writer token, lease and checkpoint live here, never on the Reduction row that
// still carries the legacy blob.
type ReductionMigration struct {
	ReductionID uint   `gorm:"primaryKey;autoIncrement:false"`
	Generation  string `gorm:"size:64;not null;default:''"`
	WriterToken string `gorm:"size:64;not null;default:''"`
	LeaseUntil  int64  `gorm:"not null;default:0"`
	Checkpoint  int64  `gorm:"not null;default:0"`
	// MemberCheckpoint is the next member_ord to write of the Cluster at Checkpoint,
	// nonzero only inside a Cluster over the member cap, whose members are written
	// in fenced chunks of their own.
	MemberCheckpoint int64 `gorm:"not null;default:0"`
	// PassClusters and PassMembers are the counts the spool pass read, which
	// publication compares with what was written.
	PassClusters int64 `gorm:"not null;default:0"`
	PassMembers  int64 `gorm:"not null;default:0"`
}

func (ReductionMigration) TableName() string { return "reduction_migrations" }
