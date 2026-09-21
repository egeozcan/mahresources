package models

import "time"

const (
	PluginCommandRunStatusQueued      = "queued"
	PluginCommandRunStatusRunning     = "running"
	PluginCommandRunStatusSucceeded   = "succeeded"
	PluginCommandRunStatusFailed      = "failed"
	PluginCommandRunStatusCancelled   = "cancelled"
	PluginCommandRunStatusInterrupted = "interrupted"
)

// PluginCommandRun is the durable authority for one declared plugin command.
// ActorlessAtSubmission is provenance, not a derivation from CreatedByUserId:
// deleting a user nulls the latter and must not turn their run into an
// intentionally actorless one.
type PluginCommandRun struct {
	ID          string `gorm:"primaryKey;size:32;index:idx_plugin_command_run_sweep,priority:3"`
	PluginName  string `gorm:"index;size:50;not null"`
	CommandName string `gorm:"size:50;not null"`
	ParamsJSON  string `gorm:"type:text;not null"`
	// InputsJSON records the input files the run was given as an ordered
	// [{"name":…,"bytes":…}] array, or empty when it was given none. Contents
	// are never stored anywhere.
	InputsJSON            string `gorm:"type:text"`
	Status                string `gorm:"index;size:16;not null"`
	ExitCode              *int
	Error                 string `gorm:"type:text"`
	ProcessGroupID        *int
	BootSessionID         string `gorm:"size:64"`
	CancelRequested       bool
	OutputUnverified      bool
	ActorlessAtSubmission bool
	CreatedByUserId       *uint     `gorm:"index"`
	CreatedAt             time.Time `gorm:"index"`
	StartedAt             *time.Time
	FinishedAt            *time.Time `gorm:"index;index:idx_plugin_command_run_sweep,priority:2"`
	ExchangeRemovedAt     *time.Time `gorm:"index:idx_plugin_command_run_sweep,priority:1"`
}

type PluginCommandRunOutput struct {
	RunID      string    `gorm:"primaryKey;size:32"`
	ArgvJSON   string    `gorm:"type:text;not null"`
	OutputTail string    `gorm:"type:text"`
	CreatedAt  time.Time `gorm:"index"`
}
