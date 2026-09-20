package plugin_commands

import (
	"errors"
	"time"
)

var (
	ErrRunNotFound       = errors.New("command run not found")
	ErrRunNotCancellable = errors.New("command run is not cancellable")
)

// PendingImportCanceller is the narrow compare-and-set used by plugin disable.
// It deliberately competes only with pending -> running so whichever durable
// transition wins defines the disable boundary: pending work is cancelled,
// while an import already marked running is allowed to finish.
type PendingImportCanceller interface {
	CancelPendingImport(importID, reason string, finished time.Time) (bool, error)
}

type Store interface {
	CreateRun(RunRecord, RunOutput) error
	MarkRunRunning(id string, started time.Time) (bool, error)
	SetRunProcessGroup(id string, pgid int) error
	RequestRunCancel(id, reason string) error
	FinishRun(id string, finish RunFinish) (bool, error)
	Run(id string) (RunRecord, RunOutput, error)
	Runs(Access) ([]RunView, error)
	NonterminalRuns() ([]RunRecord, error)
	ExpiredTerminalRuns(before time.Time, limit int) ([]RunRecord, error)
	MarkRunExchangeRemoved(runID string, removedAt time.Time) error
	PruneRunOutputs(before time.Time) (int64, error)

	ImportMap(runID, name string) (ImportMapEntry, bool, error)
	ClaimImport(ImportClaimRequest) (ImportClaimResult, error)
	MarkImportRunning(importID string, started time.Time) (bool, error)
	FinishImport(importID string, finish ImportFinish) (bool, error)
	SetImportSourceDeletePending(importID string, pending bool) error
	InterruptNonterminalImports(time.Time) error
	NonterminalImports() ([]ImportRecord, error)
	HasNonterminalImports(runID string) (bool, error)
}
