package models

import "time"

// JobSourceMapping is the durable bridge between one pre-Job source record and
// the canonical Job that now owns its execution history. SourceHash is computed
// from the execution-required legacy fields before they are scrubbed. It is
// retained afterwards so a resumed migration can distinguish a verified copy
// from an unexamined or changed source.
type JobSourceMapping struct {
	SourceKind string `gorm:"primaryKey;size:48" json:"sourceKind"`
	SourceID   string `gorm:"primaryKey;size:128" json:"sourceId"`

	JobID          string     `gorm:"size:36;not null;index:idx_job_source_mapping_job" json:"jobId"`
	SourceRevision uint64     `gorm:"not null;default:1" json:"sourceRevision"`
	SourceHash     string     `gorm:"size:64;not null" json:"sourceHash"`
	PostScrubHash  string     `gorm:"size:64" json:"postScrubHash,omitempty"`
	Status         string     `gorm:"size:20;not null;index:idx_job_source_mapping_status" json:"status"`
	BlockerCode    string     `gorm:"size:64" json:"blockerCode,omitempty"`
	Origin         string     `gorm:"size:24;not null" json:"origin"`
	CopiedAt       time.Time  `gorm:"not null" json:"copiedAt"`
	VerifiedAt     *time.Time `json:"verifiedAt,omitempty"`
	ScrubbedAt     *time.Time `json:"scrubbedAt,omitempty"`
	PurgedAt       *time.Time `json:"purgedAt,omitempty"`
	PurgeReason    string     `gorm:"size:80" json:"purgeReason,omitempty"`
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`
}

func (JobSourceMapping) TableName() string { return "job_source_mappings" }

const (
	JobSourceMappingCopied      = "copied"
	JobSourceMappingVerified    = "verified"
	JobSourceMappingScrubbed    = "scrubbed"
	JobSourceMappingPurged      = "purged"
	JobSourceMappingQuarantined = "quarantined"

	JobSourceOriginBackfilled    = "backfilled"
	JobSourceOriginDualPublished = "dual-published"

	JobMigrationCheckpointRowID = 1
)

// JobMigrationCheckpoint makes the startup backfill bounded and resumable. A
// checkpoint advances only after the preceding source batch commits, so a crash
// either repeats a verified idempotent batch or resumes at the next cursor.
type JobMigrationCheckpoint struct {
	ID                 uint       `gorm:"primaryKey" json:"id"`
	Phase              string     `gorm:"size:24;not null;index" json:"phase"`
	SourceKind         string     `gorm:"size:48" json:"sourceKind,omitempty"`
	CursorID           string     `gorm:"size:128" json:"cursorId,omitempty"`
	LastError          string     `gorm:"type:text" json:"lastError,omitempty"`
	WritersDrainAt     *time.Time `json:"writersDrainAt,omitempty"`
	BarrierInstalledAt *time.Time `json:"barrierInstalledAt,omitempty"`
	CompletedAt        *time.Time `json:"completedAt,omitempty"`
	UpdatedAt          time.Time  `json:"updatedAt"`
}

func (JobMigrationCheckpoint) TableName() string { return "job_migration_checkpoints" }

const (
	JobMigrationPhaseCopy       = "copy"
	JobMigrationPhaseVerify     = "verify"
	JobMigrationPhaseDrainFence = "drain-fence"
	JobMigrationPhaseScrub      = "scrub"
	JobMigrationPhaseComplete   = "complete"
)
