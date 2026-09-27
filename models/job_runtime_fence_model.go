package models

import "time"

// JobRuntimeFence is a database-local ownership marker for runtimes whose
// external work is rooted in a separately leased staging directory.
type JobRuntimeFence struct {
	Key         string    `gorm:"primaryKey;size:128"`
	Token       string    `gorm:"size:36;not null"`
	StagingRoot string    `gorm:"size:2048;not null"`
	AcquiredAt  time.Time `gorm:"not null"`
	// Owner is the runtime identity of the process that holds Token, recorded
	// so a successor can prove that process stopped. Empty in a row written
	// before it was recorded, which proves nothing.
	Owner string `gorm:"size:160;not null;default:''"`
	// StagingTemporary marks a StagingRoot private to its owning process, which
	// deletes it on exit, so the binding to it ends with that process.
	StagingTemporary bool `gorm:"not null;default:false"`
}
