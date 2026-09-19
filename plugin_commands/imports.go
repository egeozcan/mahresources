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
type ImportSource struct {
	Path     string
	FileName string
	RunID    string
	ImportID string
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
	ImportID   string
	ResourceID *uint
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
	defer source.Close()

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

	claimDir := filepath.Join(d.deps.Settings.StagingRoot(), "import_tmp", claim.ImportID)
	if err := resetPrivateImportDir(claimDir); err != nil {
		d.finishImportAdmissionFailure(claim.ImportID, err)
		return ImportSubmitResult{}, err
	}
	cleanupOnce := sync.OnceFunc(func() {
		if err := os.RemoveAll(claimDir); err != nil {
			d.deps.Logf("remove plugin command import temp %s: %v", claim.ImportID, err)
		}
	})
	snapshot, err := os.CreateTemp(claimDir, "source-")
	if err == nil {
		_ = snapshot.Chmod(0o600)
		_, err = io.Copy(snapshot, source)
		if closeErr := snapshot.Close(); err == nil {
			err = closeErr
		}
	}
	if err != nil {
		cleanupOnce()
		d.finishImportAdmissionFailure(claim.ImportID, err)
		return ImportSubmitResult{}, fmt.Errorf("snapshot plugin command import: %w", err)
	}

	request := cloneImportSubmission(submission)
	spec := ImportJobSpec{ImportID: claim.ImportID, RunID: run.ID, PluginName: run.PluginName, OwnerUserID: actor}
	queued := queuedImport{
		spec: spec, release: releaseOnce, cleanup: cleanupOnce,
		completion: request.Completion,
	}
	queued.run = func(ctx context.Context, progress Progress) Outcome {
		return d.runImport(ctx, progress, run, request, claim.ImportID, snapshot.Name(), claimDir)
	}
	if err := d.submitClaimedImport(queued); err != nil {
		cleanupOnce()
		d.finishImportAdmissionFailure(claim.ImportID, err)
		return ImportSubmitResult{}, err
	}
	owned = true
	return ImportSubmitResult{ImportID: claim.ImportID}, nil
}

func importMapShortCircuit(mapped ImportMapEntry) (ImportSubmitResult, bool, error) {
	switch mapped.Status {
	case ImportStatusSucceeded:
		if mapped.ResourceID == nil || *mapped.ResourceID == 0 {
			return ImportSubmitResult{}, true, fmt.Errorf("successful plugin command import has no resource id")
		}
		return ImportSubmitResult{ImportID: mapped.ImportID, ResourceID: copyUint(mapped.ResourceID)}, true, nil
	case ImportStatusPending, ImportStatusRunning:
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

func resetPrivateImportDir(path string) error {
	if err := os.RemoveAll(path); err != nil {
		return fmt.Errorf("remove stale import temp: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create import temp root: %w", err)
	}
	if err := os.Mkdir(path, 0o700); err != nil {
		return fmt.Errorf("create import temp: %w", err)
	}
	return nil
}

func (d *Dispatcher) finishImportAdmissionFailure(importID string, cause error) {
	_, finishErr := d.deps.Store.FinishImport(importID, ImportFinish{
		Status: ImportStatusFailed, Error: cause.Error(), FinishedAt: time.Now().UTC(),
	})
	if finishErr != nil {
		d.deps.Logf("finish plugin command import %s admission failure: %v", importID, finishErr)
	}
}

func (d *Dispatcher) runImport(ctx context.Context, progress Progress, run RunRecord, submission ImportSubmission, importID, snapshotPath, claimDir string) Outcome {
	result := ImportResult{ImportID: importID}
	durableSuccess := false
	finish := func(status, message string, resourceID *uint) Outcome {
		won, err := d.deps.Store.FinishImport(importID, ImportFinish{
			Status: status, Error: message, ResourceID: resourceID, FinishedAt: time.Now().UTC(),
		})
		if err != nil {
			message = appendImportError(message, "persist terminal import", err)
		} else if !won {
			mapped, found, readErr := d.deps.Store.ImportMap(run.ID, submission.Name)
			switch {
			case readErr != nil:
				err = readErr
				message = appendImportError(message, "read terminal import", readErr)
			case !found || mapped.ImportID != importID || !ImportStatusTerminal(mapped.Status):
				err = fmt.Errorf("plugin command import terminal transition was lost")
				message = appendImportError(message, "persist terminal import", err)
			default:
				status, message, resourceID = mapped.Status, mapped.Error, copyUint(mapped.ResourceID)
			}
		}
		durableSuccess = status == ImportStatusSucceeded && err == nil
		result.OK = durableSuccess
		result.Error = message
		result.ResourceID = copyUint(resourceID)
		deliverImportCompletion(submission.Completion, result)
		return Outcome{Status: status, Error: message}
	}

	if won, err := d.deps.Store.MarkImportRunning(importID, time.Now().UTC()); err != nil {
		return finish(ImportStatusFailed, err.Error(), nil)
	} else if !won {
		return finish(ImportStatusCancelled, "import is no longer pending", nil)
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
	if err := importer.ValidateImport(validation); err != nil {
		return finish(ImportStatusCancelled, err.Error(), nil)
	}
	if progress != nil {
		progress.SetPhase("importing resource")
	}
	resourceID, err := importer.ImportResource(ctx, ImportSource{
		Path: snapshotPath, FileName: submission.Name, RunID: run.ID, ImportID: importID,
	}, cloneResourceFields(submission.Fields), claimDir)
	if err != nil {
		status := ImportStatusFailed
		if errors.Is(context.Cause(ctx), errDispatcherShutdown) || errors.Is(err, context.Canceled) {
			status = ImportStatusInterrupted
		}
		return finish(status, err.Error(), nil)
	}

	resource := resourceID
	outcome := finish(ImportStatusSucceeded, "", &resource)
	// Durable success is authoritative. Source cleanup is best-effort bookkeeping
	// and can be retried/swept without creating another resource.
	if durableSuccess {
		if err := d.deleteImportedSource(run, submission.Name); err != nil {
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

func (d *Dispatcher) deleteImportedSource(run RunRecord, name string) error {
	dir, err := openExchangeRunDir(d.deps.Settings.StagingRoot(), run.PluginName, run.ID)
	if err != nil {
		return classifyRunDirError(err)
	}
	defer dir.Close()
	if err := unlinkExchangeRegularAt(dir, name, nil); err != nil {
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
