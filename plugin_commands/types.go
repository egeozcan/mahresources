package plugin_commands

import (
	"context"
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
	JobID                 string
	JobExecutionToken     string
	PluginName            string
	CommandName           string
	ParamsJSON            string
	Inputs                []SuppliedInput
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
	ExchangeRemovedAt     *time.Time
}

type RecoveryRun struct {
	RunRecord
	BootSessionID string
}

type RetentionCursor struct {
	FinishedAt time.Time
	RunID      string
}

type RunOutput struct {
	RunID      string
	ArgvJSON   string
	OutputTail string
	CreatedAt  time.Time
}

// SuppliedInput is the durable record of one input file a run was given: the
// declared name and the number of bytes supplied. Contents are deliberately
// absent — they exist only as the file in the run's exchange folder.
type SuppliedInput struct {
	Name  string `json:"name"`
	Bytes int64  `json:"bytes"`
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
	if a.PluginName == "" || a.PluginName != run.PluginName {
		return false
	}
	// Actorless-at-submission is explicit provenance, not an accidental NULL.
	// It is how auth-off command runs remain available to their own plugin while
	// an ordinary actor-owned run nulled by user deletion stays fail-closed.
	if run.ActorlessAtSubmission {
		return true
	}
	if a.ActorUserID == nil || *a.ActorUserID == 0 {
		return false
	}
	return run.CreatedByUserID != nil && *run.CreatedByUserID == *a.ActorUserID
}

type ImportRecord struct {
	ID                  string
	JobID               string
	JobExecutionToken   string
	RunID               string
	FileName            string
	FieldsJSON          string
	PluginGeneration    uint64
	CreatedByUserID     *uint
	Status              string
	Error               string
	SourceDeletePending bool
	CreatedAt           time.Time
	StartedAt           *time.Time
	FinishedAt          *time.Time
}

type ImportMapEntry struct {
	RunID               string
	FileName            string
	ImportID            string
	ResourceID          *uint
	Status              string
	Error               string
	SourceDeletePending bool
}

type ImportClaimRequest struct {
	ImportID         string
	RunID            string
	FileName         string
	FieldsJSON       string
	PluginGeneration uint64
	CreatedByUserID  *uint
	CreatedAt        time.Time
}

type ImportClaimResult struct {
	ImportID   string
	JobID      string
	ResourceID *uint
	Status     string
	Created    bool
	Enqueue    bool
}

type ImportFinish struct {
	Status              string
	Error               string
	ResourceID          *uint
	SourceDeletePending bool
	FinishedAt          time.Time
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

// Settings is the live runtime surface used by command dispatch and execution.
type Settings interface {
	StagingRoot() string
	PendingPerPluginLimit() int
	PerRunQuota() int64
	GlobalStagingQuota() int64
	ExchangeRetention() time.Duration
	OutputRetention() time.Duration
	CommandPath() string
}

type RunJobSpec struct {
	JobID       string
	RunID       string
	PluginName  string
	OwnerUserID *uint
}

type ImportJobSpec struct {
	JobID       string
	ImportID    string
	RunID       string
	FileName    string
	PluginName  string
	OwnerUserID *uint
}

type Progress interface {
	SetPhase(string)
	SetPhaseProgress(int64, int64)
	SetAuthoritativeStatus(string)
}

type Outcome struct {
	// Status classifies the in-memory managed job outcome.
	Status string
	// AuthoritativeStatus is set only when Status (or another returned status)
	// was successfully persisted or successfully reread from the durable store.
	// An empty value means the durable state is unknown.
	AuthoritativeStatus string
	Error               string
}

type LiveJobs interface {
	SubmitCommandJob(RunJobSpec, func(string) error, func(context.Context, Progress) Outcome) (string, error)
	SubmitImportJob(ImportJobSpec, func(context.Context, Progress) Outcome) (string, error)
}

type Result struct {
	OK       bool
	ExitCode *int
	Error    string
	RunID    string
}

type CommandRequest struct {
	PluginName       string
	PluginGeneration uint64
	ActorUserID      *uint
	Declaration      Declaration
	Params           map[string]string
	// Inputs is what the plugin supplied, keyed by declared name. Validation
	// belongs to the host: ValidateInputs turns this into QueuedRun.Inputs
	// before anything durable exists, and the contents are dropped from memory
	// once they are on disk.
	Inputs     map[string]string
	Completion func(Result)
}

type QueuedRun struct {
	RunID               string
	JobID               string
	Request             CommandRequest
	ExchangeDir         string
	Invocation          Invocation
	Inputs              []InputFile
	control             *runControl
	progress            Progress
	completionLifecycle *commandCompletionLifecycle
}

type Executor interface {
	Execute(context.Context, QueuedRun) Outcome
}

type Dependencies struct {
	Store         Store
	BootSessionID string
	Jobs          LiveJobs
	Executor      Executor
	Settings      Settings
	Inspector     ProcessInspector
	Usage         *StagingUsageCache
	Leases        *LeaseManager
	Logf          func(string, ...any)
}
