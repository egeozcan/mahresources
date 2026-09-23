package models

import "time"

// JobRuntimeFence is a database-local ownership marker for runtimes whose
// external work is rooted in a separately leased staging directory.
type JobRuntimeFence struct {
	Key         string    `gorm:"primaryKey;size:128"`
	Token       string    `gorm:"size:36;not null"`
	StagingRoot string    `gorm:"size:2048;not null"`
	AcquiredAt  time.Time `gorm:"not null"`
}
