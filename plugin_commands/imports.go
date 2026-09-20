package plugin_commands

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ResourceFields is the intentionally small resource-create surface exposed to
// command imports. Paths and storage locations are host-owned.
type ResourceFields struct {
	Name        string
	Description string
	TagIDs      []uint
	GroupIDs    []uint
	Meta        map[string]any
}

// ImportValidation is re-evaluated when a queued import actually owns a worker
// slot. A durable request is not a standing permission.
type ImportValidation struct {
	PluginName       string
	PluginGeneration uint64
	ActorUserID      *uint
	Fields           ResourceFields
}

// ImportSource names the host-managed snapshot consumed by the application
// adapter. Path is never supplied by Lua or by a plugin.
type ScratchFileFactory func() (*os.File, func() error, error)

type ImportSource struct {
	File          *os.File
	CreateScratch ScratchFileFactory
	FileName      string
	RunID         string
	ImportID      string
}

type Importer interface {
	ValidateImport(ImportValidation) error
	ImportResource(context.Context, ImportSource, ResourceFields, string) (uint, error)
}

type ImportSubmission struct {
	Access           Access
	RunID            string
	Name             string
	Fields           ResourceFields
	PluginGeneration uint64
	ActorUserID      *uint
	Completion       func(ImportResult)
}

type ImportSubmitResult struct {
	ImportID             string
	ResourceID           *uint
	CompletionRegistered bool
}

type ImportResult struct {
	OK         bool
	Error      string
	ImportID   string
	ResourceID *uint
}

type importDeleteFailureRecorder interface {
	RecordImportDeleteFailure(importID, message string) error
}

// SetImporter installs the application-layer adapter. It is a process-lifetime
// dependency and is read afresh for every worker so request-scoped db handles
// are never captured here.
func (d *Dispatcher) SetImporter(importer Importer) {
	d.importerMu.Lock()
	d.importer = importer
	d.importerMu.Unlock()
}

func (d *Dispatcher) currentImporter() Importer {
	d.importerMu.RLock()
	defer d.importerMu.RUnlock()
	return d.importer
}

// SubmitImport makes the source durable before entering the private dispatcher
// queue. A successful historical mapping returns synchronously even when the
// exchange file has already been deleted, and deliberately does not invoke the
// asynchronous completion callback.
func (d *Dispatcher) SubmitImport(submission ImportSubmission) (ImportSubmitResult, error) {
	if d == nil || d.deps.Store == nil || d.deps.Settings == nil {
		return ImportSubmitResult{}, fmt.Errorf("plugin_commands: import dependencies are incomplete")
	}
	if d.currentImporter() == nil {
		return ImportSubmitResult{}, fmt.Errorf("plugin command importer is unavailable")
	}
	if submission.RunID == "" {
		return ImportSubmitResult{}, fmt.Errorf("plugin command import requires run id")
	}
	if err := validateExchangeName(submission.Name); err != nil {
		return ImportSubmitResult{}, err
	}
	if submission.ActorUserID == nil || *submission.ActorUserID == 0 {
		return ImportSubmitResult{}, fmt.Errorf("plugin command import requires an acting user")
	}
	if submission.Access.ActorUserID == nil || *submission.Access.ActorUserID != *submission.ActorUserID {
		return ImportSubmitResult{}, ErrExchangeRunNotFound
	}

	exchange := &exchangeService{store: d.deps.Store, settings: d.deps.Settings, leases: d.deps.Leases}
	run, err := exchange.authorizeRun(submission.Access, submission.RunID, false)
	if err != nil {
		return ImportSubmitResult{}, err
	}
	if mapped, found, err := d.deps.Store.ImportMap(run.ID, submission.Name); err != nil {
		return ImportSubmitResult{}, err
	} else if found {
		if result, done, err := importMapShortCircuit(mapped); done || err != nil {
			return result, err
		}
	}

	// Pin before opening the exchange path. The second authorization and map
	// lookup close the gaps between the first probes and lease acquisition.
	release, err := d.deps.Leases.Acquire(run.ID)
	if err != nil {
		return ImportSubmitResult{}, err
	}
	releaseOnce := sync.OnceFunc(release)
	owned := false
	defer func() {
		if !owned {
			releaseOnce()
		}
	}()
	if run, err = exchange.authorizeRun(submission.Access, submission.RunID, false); err != nil {
		return ImportSubmitResult{}, err
	}
	if mapped, found, err := d.deps.Store.ImportMap(run.ID, submission.Name); err != nil {
		return ImportSubmitResult{}, err
	} else if found {
		if result, done, err := importMapShortCircuit(mapped); done || err != nil {
			return result, err
		}
	}

	runDir, err := openExchangeRunDir(d.deps.Settings.StagingRoot(), run.PluginName, run.ID)
	if err != nil {
		return ImportSubmitResult{}, classifyRunDirError(err)
	}
	source, err := openExchangeRegularAt(runDir, submission.Name, nil)
	runDir.Close()
	if err != nil {
		return ImportSubmitResult{}, classifyFileError(err)
	}
	sourceOwned := false
	defer func() {
		if !sourceOwned {
			_ = source.Close()
		}
	}()
	sourceInfo, err := source.Stat()
	if err != nil {
		return ImportSubmitResult{}, classifyFileError(err)
	}

	requestedID, err := newRunID()
	if err != nil {
		return ImportSubmitResult{}, err
	}
	actor := copyUint(submission.ActorUserID)
	claim, err := d.deps.Store.ClaimImport(ImportClaimRequest{
		ImportID: requestedID, RunID: run.ID, FileName: submission.Name,
		PluginGeneration: submission.PluginGeneration, CreatedByUserID: actor,
		CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		return ImportSubmitResult{}, err
	}
	if !claim.Enqueue {
		return importClaimShortCircuit(claim)
	}

	request := cloneImportSubmission(submission)
	cleanupOnce := sync.OnceFunc(func() { _ = source.Close() })
	spec := ImportJobSpec{ImportID: claim.ImportID, RunID: run.ID, PluginName: run.PluginName, OwnerUserID: actor}
	queued := queuedImport{
		spec: spec, release: releaseOnce, cleanup: cleanupOnce,
		completion: request.Completion, retainOnReject: true,
	}
	queued.run = func(ctx context.Context, progress Progress) Outcome {
		return d.runImport(ctx, progress, run, request, claim.ImportID, source, sourceInfo.Size())
	}
	if err := d.submitClaimedImport(queued); err != nil {
		if finishErr := d.finishImportAdmissionFailure(claim.ImportID, run.ID, submission.Name, err); finishErr != nil {
			cleanupOnce()
			return ImportSubmitResult{}, errors.Join(err, finishErr)
		}
		cleanupOnce()
		return ImportSubmitResult{}, err
	}
	sourceOwned = true
	owned = true
	return ImportSubmitResult{ImportID: claim.ImportID, CompletionRegistered: true}, nil
}

func importMapShortCircuit(mapped ImportMapEntry) (ImportSubmitResult, bool, error) {
	switch mapped.Status {
	case ImportStatusSucceeded:
		if mapped.ResourceID == nil || *mapped.ResourceID == 0 {
			return ImportSubmitResult{}, true, fmt.Errorf("successful plugin command import has no resource id")
		}
		return ImportSubmitResult{ImportID: mapped.ImportID, ResourceID: copyUint(mapped.ResourceID)}, true, nil
	case ImportStatusPending, ImportStatusRunning:
		// Submission is idempotent, but the existing worker owns the only
		// completion callback. Callers must not wait for a callback this request
		// did not register.
		return ImportSubmitResult{ImportID: mapped.ImportID}, true, nil
	default:
		return ImportSubmitResult{}, false, nil
	}
}

func importClaimShortCircuit(claim ImportClaimResult) (ImportSubmitResult, error) {
	if claim.Status == ImportStatusSucceeded {
		if claim.ResourceID == nil || *claim.ResourceID == 0 {
			return ImportSubmitResult{}, fmt.Errorf("successful plugin command import has no resource id")
		}
		return ImportSubmitResult{ImportID: claim.ImportID, ResourceID: copyUint(claim.ResourceID)}, nil
	}
	return ImportSubmitResult{ImportID: claim.ImportID}, nil
}

func (d *Dispatcher) finishImportAdmissionFailure(importID, runID, name string, cause error) error {
	_, err := d.persistImportTerminal(importID, runID, name, ImportFinish{
		Status: ImportStatusFailed, Error: cause.Error(), FinishedAt: time.Now().UTC(),
	})
	return err
}

// persistImportTerminal does not publish a live terminal result until the
// durable transition exists. Transient store failures retain the worker, its
// lease and its source descriptor, so a later submission can never observe a
// pending/running row whose replay state was already discarded.
func (d *Dispatcher) persistImportTerminal(importID, runID, name string, finish ImportFinish) (ImportFinish, error) {
	for {
		won, err := d.deps.Store.FinishImport(importID, finish)
		if err == nil && won {
			return finish, nil
		}
		if err == nil && runID != "" {
			mapped, found, readErr := d.deps.Store.ImportMap(runID, name)
			if readErr == nil && found && mapped.ImportID == importID && ImportStatusTerminal(mapped.Status) {
				return ImportFinish{
					Status: mapped.Status, Error: mapped.Error,
					ResourceID: copyUint(mapped.ResourceID), FinishedAt: finish.FinishedAt,
				}, nil
			}
			if readErr != nil {
				err = readErr
			} else {
				err = errors.New("plugin command import terminal transition was lost")
			}
		}
		if err == nil {
			err = errors.New("plugin command import terminal transition was refused")
		}
		d.deps.Logf("persist plugin command import %s terminal state: %v", importID, err)
		time.Sleep(dispatchFailureRetryDelay)
	}
}

func (d *Dispatcher) runImport(ctx context.Context, progress Progress, run RunRecord, submission ImportSubmission, importID string, source *os.File, sourceSize int64) Outcome {
	result := ImportResult{ImportID: importID}
	var temp *importTempDir
	finish := func(status, message string, resourceID *uint) Outcome {
		if temp != nil {
			if err := temp.Cleanup(); err != nil {
				d.deps.Logf("remove plugin command import temp %s: %v", importID, err)
			}
			temp = nil
		}
		persisted, err := d.persistImportTerminal(importID, run.ID, submission.Name, ImportFinish{
			Status: status, Error: message, ResourceID: resourceID, FinishedAt: time.Now().UTC(),
		})
		if err != nil {
			// persistImportTerminal currently retries until it has an authoritative
			// result; keep the branch explicit if that contract ever changes.
			return Outcome{Status: ImportStatusRunning, Error: err.Error()}
		}
		result.OK = persisted.Status == ImportStatusSucceeded
		result.Error = persisted.Error
		result.ResourceID = copyUint(persisted.ResourceID)
		deliverImportCompletion(submission.Completion, result)
		return Outcome{Status: persisted.Status, Error: persisted.Error}
	}

	if err := ctx.Err(); err != nil {
		return finish(ImportStatusInterrupted, "server interrupted", nil)
	}
	importer := d.currentImporter()
	if importer == nil {
		return finish(ImportStatusFailed, "plugin command importer is unavailable", nil)
	}
	validation := ImportValidation{
		PluginName: run.PluginName, PluginGeneration: submission.PluginGeneration,
		ActorUserID: copyUint(submission.ActorUserID), Fields: cloneResourceFields(submission.Fields),
	}
	// Permission and generation validation belongs while the claim is pending.
	// Once MarkImportRunning wins, disable deliberately leaves this commit lane
	// alone so it can finish atomically.
	if err := importer.ValidateImport(validation); err != nil {
		return finish(ImportStatusCancelled, err.Error(), nil)
	}
	quotaRelease, err := d.reserveImportQuota(run, sourceSize)
	if err != nil {
		return finish(ImportStatusFailed, err.Error(), nil)
	}
	defer quotaRelease()
	if won, err := d.deps.Store.MarkImportRunning(importID, time.Now().UTC()); err != nil {
		return finish(ImportStatusFailed, err.Error(), nil)
	} else if !won {
		return finish(ImportStatusCancelled, "import is no longer pending", nil)
	}
	if err := ctx.Err(); err != nil {
		return finish(ImportStatusInterrupted, "server interrupted", nil)
	}

	temp, err = createImportTempDir(d.deps.Settings.StagingRoot(), importID)
	if err != nil {
		return finish(ImportStatusFailed, err.Error(), nil)
	}
	snapshot, snapshotCleanup, err := temp.Create("source-")
	if err != nil {
		return finish(ImportStatusFailed, fmt.Sprintf("create plugin command import snapshot: %v", err), nil)
	}
	if err := copyImportSnapshot(ctx, snapshot, source, sourceSize); err != nil {
		_ = snapshotCleanup()
		status := ImportStatusFailed
		if errors.Is(context.Cause(ctx), errDispatcherShutdown) || errors.Is(err, context.Canceled) {
			status = ImportStatusInterrupted
		}
		return finish(status, fmt.Sprintf("snapshot plugin command import: %v", err), nil)
	}
	if progress != nil {
		progress.SetPhase("importing resource")
	}
	resourceID, err := importer.ImportResource(ctx, ImportSource{
		File: snapshot, CreateScratch: func() (*os.File, func() error, error) {
			return temp.Create("upload-")
		},
		FileName: submission.Name, RunID: run.ID, ImportID: importID,
	}, cloneResourceFields(submission.Fields), "")
	_ = snapshotCleanup()
	if err != nil {
		status := ImportStatusFailed
		if errors.Is(context.Cause(ctx), errDispatcherShutdown) || errors.Is(err, context.Canceled) {
			status = ImportStatusInterrupted
		}
		return finish(status, err.Error(), nil)
	}

	resource := resourceID
	outcome := finish(ImportStatusSucceeded, "", &resource)
	// Durable success is authoritative. Source cleanup is best-effort and is
	// tied to the admitted descriptor, never merely to the current pathname.
	if outcome.Status == ImportStatusSucceeded {
		if err := d.deleteImportedSource(run, submission.Name, source); err != nil {
			message := "imported-pending-delete: " + err.Error()
			if recorder, ok := d.deps.Store.(importDeleteFailureRecorder); ok {
				if recordErr := recorder.RecordImportDeleteFailure(importID, message); recordErr != nil {
					d.deps.Logf("record plugin command import %s source cleanup failure: %v", importID, recordErr)
				}
			}
			d.deps.Logf("plugin command import %s succeeded but source cleanup failed: %v", importID, err)
		}
	}
	return outcome
}

func copyImportSnapshot(ctx context.Context, destination, source *os.File, expected int64) error {
	if expected < 0 {
		return errors.New("plugin command import source has invalid size")
	}
	if _, err := source.Seek(0, io.SeekStart); err != nil {
		return err
	}
	reader := &contextImportReader{ctx: ctx, reader: io.LimitReader(source, expected+1)}
	written, err := io.Copy(destination, reader)
	if err != nil {
		return err
	}
	if written != expected {
		return fmt.Errorf("plugin command import source changed during snapshot (expected %d bytes, copied %d)", expected, written)
	}
	_, err = destination.Seek(0, io.SeekStart)
	return err
}

type contextImportReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *contextImportReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

func (d *Dispatcher) reserveImportQuota(run RunRecord, sourceSize int64) (func(), error) {
	if sourceSize < 0 || sourceSize >= (1<<62) {
		return nil, errors.New("plugin command import source has invalid size")
	}
	reservation := sourceSize * 2 // outer snapshot plus AddResource scratch copy
	limit := effectiveQuota(d.deps.Settings.PerRunQuota(), defaultPerRunQuota)
	d.importQuotaMu.Lock()
	defer d.importQuotaMu.Unlock()
	exchangeDir := filepath.Join(d.deps.Settings.StagingRoot(), "plugin_exchange", run.PluginName, run.ID)
	usage, err := pathUsageNoSymlinks(exchangeDir)
	if err != nil {
		return nil, fmt.Errorf("measure plugin command import quota: %w", err)
	}
	reserved := d.importQuotaReserved[run.ID]
	if usage > limit || reservation > limit-usage || reserved > limit-usage-reservation {
		return nil, fmt.Errorf("plugin command per-run quota exceeded (%d bytes)", limit)
	}
	d.importQuotaReserved[run.ID] = reserved + reservation
	var once sync.Once
	return func() {
		once.Do(func() {
			d.importQuotaMu.Lock()
			remaining := d.importQuotaReserved[run.ID] - reservation
			if remaining <= 0 {
				delete(d.importQuotaReserved, run.ID)
			} else {
				d.importQuotaReserved[run.ID] = remaining
			}
			d.importQuotaMu.Unlock()
		})
	}, nil
}

func (d *Dispatcher) deleteImportedSource(run RunRecord, name string, source *os.File) error {
	dir, err := openExchangeRunDir(d.deps.Settings.StagingRoot(), run.PluginName, run.ID)
	if err != nil {
		return classifyRunDirError(err)
	}
	defer dir.Close()
	if err := unlinkExchangeOpenedRegularAt(dir, name, source); err != nil {
		return classifyFileError(err)
	}
	return nil
}

func appendImportError(message, prefix string, err error) string {
	if message == "" {
		return prefix + ": " + err.Error()
	}
	return message + "; " + prefix + ": " + err.Error()
}

func cloneImportSubmission(submission ImportSubmission) ImportSubmission {
	submission.ActorUserID = copyUint(submission.ActorUserID)
	submission.Access.ActorUserID = copyUint(submission.Access.ActorUserID)
	submission.Fields = cloneResourceFields(submission.Fields)
	return submission
}

func cloneResourceFields(fields ResourceFields) ResourceFields {
	fields.TagIDs = append([]uint(nil), fields.TagIDs...)
	fields.GroupIDs = append([]uint(nil), fields.GroupIDs...)
	if fields.Meta != nil {
		fields.Meta = cloneAnyMap(fields.Meta)
	}
	return fields
}

func cloneAnyMap(source map[string]any) map[string]any {
	clone := make(map[string]any, len(source))
	for key, value := range source {
		clone[key] = value
	}
	return clone
}

func deliverImportCompletion(completion func(ImportResult), result ImportResult) {
	if completion != nil {
		go completion(result)
	}
}
