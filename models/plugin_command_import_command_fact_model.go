package models

import "time"

// PluginCommandImportCommandFact is the bounded, indexed projection used by
// retry-import command selectors. The accepted fields may be sealed inside the
// Job replay envelope, and the exchange file lives outside the database, so
// selectors cannot derive either fact from the source rows at list time.
//
// The selector also joins the current claim, map, parent run, actor, and runtime
// fence. Those live predicates make a missing, mismatched, or obsolete fact
// fail closed.
type PluginCommandImportCommandFact struct {
	JobID                 string    `gorm:"primaryKey;size:36"`
	ImportID              string    `gorm:"uniqueIndex;size:32;not null"`
	RunID                 string    `gorm:"size:32;not null;index"`
	FileName              string    `gorm:"size:255;not null"`
	FieldsValidated       bool      `gorm:"not null;default:false;index:idx_plugin_command_import_retry_fact,priority:1"`
	ExchangeFileAvailable bool      `gorm:"not null;default:false;index:idx_plugin_command_import_retry_fact,priority:2"`
	SeriesID              uint      `gorm:"not null;default:0"`
	GroupCount            int       `gorm:"not null;default:0"`
	UpdatedAt             time.Time `gorm:"not null"`
}

func (PluginCommandImportCommandFact) TableName() string {
	return "plugin_command_import_command_facts"
}

// PluginCommandImportCommandFactGroup stores only validated group references
// from sealed import fields. Keeping these normalized lets the selector check
// that every referenced group still exists with indexed SQL on both engines.
type PluginCommandImportCommandFactGroup struct {
	ImportID string `gorm:"primaryKey;size:32;index:idx_plugin_command_import_fact_group,priority:1"`
	GroupID  uint   `gorm:"primaryKey;index:idx_plugin_command_import_fact_group,priority:2"`
}

func (PluginCommandImportCommandFactGroup) TableName() string {
	return "plugin_command_import_command_fact_groups"
}
