package plugin_commands

import (
	"time"

	"mahresources/models"
)

const (
	RunStatusQueued      = models.PluginCommandRunStatusQueued
	RunStatusRunning     = models.PluginCommandRunStatusRunning
	RunStatusSucceeded   = models.PluginCommandRunStatusSucceeded
	RunStatusFailed      = models.PluginCommandRunStatusFailed
	RunStatusCancelled   = models.PluginCommandRunStatusCancelled
	RunStatusInterrupted = models.PluginCommandRunStatusInterrupted

	ImportStatusPending     = models.PluginCommandImportStatusPending
	ImportStatusRunning     = models.PluginCommandImportStatusRunning
	ImportStatusSucceeded   = models.PluginCommandImportStatusSucceeded
	ImportStatusFailed      = models.PluginCommandImportStatusFailed
	ImportStatusCancelled   = models.PluginCommandImportStatusCancelled
	ImportStatusInterrupted = models.PluginCommandImportStatusInterrupted
)

type RunRecord struct {
	ID                    string
	PluginName            string
	CommandName           string
	ParamsJSON            string
	Status                string
	ExitCode              *int
	Error                 string
	ProcessGroupID        *int
	CancelRequested       bool
	OutputUnverified      bool
	ActorlessAtSubmission bool
	CreatedByUserID       *uint
	CreatedAt             time.Time
	StartedAt             *time.Time
	FinishedAt            *time.Time
}

type RunOutput struct {
	RunID      string
	ArgvJSON   string
	OutputTail string
	CreatedAt  time.Time
}

type RunFinish struct {
	Status           string
	ExitCode         *int
	Error            string
	OutputTail       string
	OutputUnverified bool
	FinishedAt       time.Time
}

type RunView struct {
	RunRecord
	Output  RunOutput
	Imports []ImportMapEntry
}

// Access identifies the plugin and current principal requesting plugin-visible
// history. Administrator bypass is reserved for the dedicated operator UI.
type Access struct {
	PluginName    string
	ActorUserID   *uint
	Administrator bool
}

func (a Access) AllowsRun(run RunRecord) bool {
	if a.Administrator {
		return true
	}
	if a.PluginName == "" || a.PluginName != run.PluginName || a.ActorUserID == nil || *a.ActorUserID == 0 {
		return false
	}
	if run.ActorlessAtSubmission {
		return true
	}
	return run.CreatedByUserID != nil && *run.CreatedByUserID == *a.ActorUserID
}

type ImportRecord struct {
	ID               string
	RunID            string
	FileName         string
	PluginGeneration uint64
	CreatedByUserID  *uint
	Status           string
	Error            string
	CreatedAt        time.Time
	StartedAt        *time.Time
	FinishedAt       *time.Time
}

type ImportMapEntry struct {
	RunID      string
	FileName   string
	ImportID   string
	ResourceID *uint
	Status     string
	Error      string
}

type ImportClaimRequest struct {
	ImportID         string
	RunID            string
	FileName         string
	PluginGeneration uint64
	CreatedByUserID  *uint
	CreatedAt        time.Time
}

type ImportClaimResult struct {
	ImportID   string
	ResourceID *uint
	Status     string
	Created    bool
	Enqueue    bool
}

type ImportFinish struct {
	Status     string
	Error      string
	ResourceID *uint
	FinishedAt time.Time
}

func RunStatusTerminal(status string) bool {
	switch status {
	case RunStatusSucceeded, RunStatusFailed, RunStatusCancelled, RunStatusInterrupted:
		return true
	default:
		return false
	}
}

func ImportStatusTerminal(status string) bool {
	switch status {
	case ImportStatusSucceeded, ImportStatusFailed, ImportStatusCancelled, ImportStatusInterrupted:
		return true
	default:
		return false
	}
}
