package models

import "time"

// JobImportCommandFact is the indexed, durable view of the import files that
// determine whether a failed parse or apply can be retried. The paths themselves
// stay in the import filesystem; these flags are invalidated by the application
// paths that publish, consume, restore and remove those files, so Job command
// selectors never walk the filesystem once per history row.
type JobImportCommandFact struct {
	ParseHandle       string    `gorm:"primaryKey;size:120" json:"parseHandle"`
	ArchiveAvailable  bool      `gorm:"not null;default:false;index:idx_job_import_archive_available" json:"archiveAvailable"`
	PlanAvailable     bool      `gorm:"not null;default:false;index:idx_job_import_plan_available" json:"planAvailable"`
	UpdatedAt         time.Time `json:"updatedAt"`
}

func (JobImportCommandFact) TableName() string {
	return "job_import_command_facts"
}
