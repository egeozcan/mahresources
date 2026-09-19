package plugin_commands

import (
	"errors"
	"time"
)

var (
	ErrRunNotFound       = errors.New("command run not found")
	ErrRunNotCancellable = errors.New("command run is not cancellable")
)

type Store interface {
	CreateRun(RunRecord, RunOutput) error
	MarkRunRunning(id string, started time.Time) (bool, error)
	SetRunProcessGroup(id string, pgid int) error
	RequestRunCancel(id, reason string) error
	FinishRun(id string, finish RunFinish) (bool, error)
	Run(id string) (RunRecord, RunOutput, error)
	Runs(Access) ([]RunView, error)
	NonterminalRuns() ([]RunRecord, error)
	ExpiredTerminalRuns(before time.Time) ([]RunRecord, error)
	PruneRunOutputs(before time.Time) (int64, error)

	ImportMap(runID, name string) (ImportMapEntry, bool, error)
	ClaimImport(ImportClaimRequest) (ImportClaimResult, error)
	MarkImportRunning(importID string, started time.Time) (bool, error)
	FinishImport(importID string, finish ImportFinish) (bool, error)
	InterruptNonterminalImports(time.Time) error
	NonterminalImports() ([]ImportRecord, error)
	HasNonterminalImports(runID string) (bool, error)
}
