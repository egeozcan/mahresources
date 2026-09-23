package application_context

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"mahresources/download_queue"
	"mahresources/jobs"

	"github.com/spf13/afero"
)

// This file is the group-import Kind pair: parse and apply.
//
// They are two Kinds rather than one because they are two executions with two
// inputs and two different questions about replay. A parse reads an archive and
// writes a plan; an apply reads a plan and writes the library. A reviewer sits
// between them — that is what the review step in the flow *is* — so an apply is a
// child Job of the parse it decided on, never a continuation of it.
//
// Three decisions are load-bearing:
//
//   - The staged archive is execution-required input for both Kinds. A parse that
//     cannot read its archive cannot run, and an apply reads the archive's blobs as
//     well as the plan, so Retry is advertised only while the archive is still on
//     disk. The alternative — advertising Retry and failing at dispatch — is a
//     button that lies.
//   - An apply's Retry is advertised only on the evidence the executor itself
//     produced: a consumed plan is restored exactly when replay is provably
//     idempotent (`ImportApplyPlanShouldBeRestored`), so the state of that file is
//     the durable answer, and a partially applied archive that is not replay-safe
//     offers no Retry at all — it publishes its report instead.
//   - The apply's authorization comes from the *canonical Job* behind the parse
//     handle, not from the parse's in-memory queue entry. "Clear completed" and the
//     retention sweep remove that entry within an hour while the plan, result and
//     archive it authorised stay on disk, and an authorization that expires while
//     the thing it guarded does not is not an authorization.

const (
	// JobKindGroupImportParse is one archive parse submitted from the UI, the API or
	// the CLI.
	JobKindGroupImportParse = "group-import-parse"
	// JobKindGroupImportApply is one apply of a reviewed plan.
	JobKindGroupImportApply = "group-import-apply"
	// jobImportKindVersion is the version of these two Kinds' input semantics.
	jobImportKindVersion = 1
	// jobImportPlanOutput is the plan a parse publishes. Success depends on it: a
	// parse whose plan cannot be handed over did not parse anything.
	jobImportPlanOutput = "plan"
	// jobImportResultOutput is the report an apply publishes. It is deliberately not
	// required: a partial apply publishes its report *because* it failed, and a
	// failure that produced one is still a failure.
	jobImportResultOutput = "result"
)

// ErrImportPlanConsumed is the refusal a second apply answers with 409: the plan was
// already consumed, and there is nothing left to decide on.
var ErrImportPlanConsumed = errors.New("this import plan was already applied or has expired")

// importArchivePathFor is where one import's staged upload lives, named by the
// parse handle — which is what the delete handler and the legacy routes look for.
func importArchivePathFor(handle string) string {
	return filepath.Join("_imports", handle+".tar")
}

// importPlanPathFor is the plan a parse writes.
func importPlanPathFor(handle string) string {
	return filepath.Join("_imports", handle+".plan.json")
}

// importConsumedPlanPathFor is where one apply's plan goes while that apply is
// deciding from it.
//
// It is named by the apply *Job's* own legacy handle rather than by the parse alone,
// and that is the whole of the ownership: the file a Job finds at this path was put
// there by this Job's own admission, so its presence is evidence about this Job and
// not about whichever apply last renamed the plan. A shared name made the two
// indistinguishable, and a Retry therefore ran the plan another apply had just
// consumed — two executors applying one archive.
//
// A Retry successor carries its ancestor's handle (that is what makes an unchanged
// client id follow the lineage), so one lineage has one plan slot: the successor of a
// failed, restore-safe apply re-consumes the restored plan into the same name its
// ancestor used.
func importConsumedPlanPathFor(handle, applyHandle string) string {
	return filepath.Join("_imports", handle+"."+applyHandle+".plan.applied.json")
}

// importResultPathFor is the report an apply writes.
func importResultPathFor(handle string) string {
	return filepath.Join("_imports", handle+".result.json")
}

// importParseJobInput is what a parse Job is accepted with: the staged upload it
// reads, and the legacy id the plan, result and archive are named by.
type importParseJobInput struct {
	Handle  string `json:"handle"`
	Archive string `json:"archive"`
}

// importApplyJobInput is what an apply Job is accepted with: the plan it decided on
// and the decisions themselves. The decisions are part of the sealed input because
// they are what makes this apply *this* apply, and a Retry that lost them would
// re-run the import with a different set of choices.
//
// Plan names the plan the apply was admitted for — the staged review artifact at its
// *unconsumed* path. It is deliberately not the path the job will read from: which file
// an apply may read is decided by that apply's own admission, which moves the plan to
// the path its Job owns (importConsumedPlanPathFor), and a sealed path that named the
// consumed file would let a Retry read one another apply had taken.
type importApplyJobInput struct {
	ParseHandle string          `json:"parseHandle"`
	Plan        string          `json:"plan"`
	Decisions   ImportDecisions `json:"decisions"`
}

func importParseJobCodec() jobs.ReplayCodec {
	return jobs.ReplayCodec{
		Sanitize: sanitizeImportParseInput,
		Encode:   encodeImportParseInput,
		Decode:   decodeImportParseInput,
		Migrate: func(payload json.RawMessage, fromVersion, toVersion uint) (json.RawMessage, error) {
			if fromVersion != toVersion {
				return nil, fmt.Errorf("jobs: no import parse input migration from v%d to v%d", fromVersion, toVersion)
			}
			return payload, nil
		},
	}
}

func importApplyJobCodec() jobs.ReplayCodec {
	return jobs.ReplayCodec{
		Sanitize: sanitizeImportApplyInput,
		Encode:   encodeImportApplyInput,
		Decode:   decodeImportApplyInput,
		Migrate: func(payload json.RawMessage, fromVersion, toVersion uint) (json.RawMessage, error) {
			if fromVersion != toVersion {
				return nil, fmt.Errorf("jobs: no import apply input migration from v%d to v%d", fromVersion, toVersion)
			}
			return payload, nil
		},
	}
}

// importParseSummary is the bounded, searchable half of a parse's input: the
// archive's own file name, never a path into the deployment's storage.
type importParseSummary struct {
	Handle  string `json:"handle,omitempty"`
	Archive string `json:"archive,omitempty"`
}

// importApplySummary is the bounded, searchable half of an apply's input. The
// decisions themselves are deliberately summarized as counts: which entities a
// person chose to include is their own review, and a summary is searchable text
// anybody with access to the Job may read.
type importApplySummary struct {
	ParseHandle     string `json:"parseHandle,omitempty"`
	ExcludedItems   int    `json:"excludedItems,omitempty"`
	MappingActions  int    `json:"mappingActions,omitempty"`
	DanglingActions int    `json:"danglingActions,omitempty"`
	ShellActions    int    `json:"shellActions,omitempty"`
}

func sanitizeImportParseInput(input json.RawMessage) (json.RawMessage, error) {
	parsed, err := importParseInputOf(input)
	if err != nil {
		return nil, err
	}
	return json.Marshal(importParseSummary{Handle: parsed.Handle, Archive: filepath.Base(parsed.Archive)})
}

func encodeImportParseInput(input json.RawMessage) (json.RawMessage, error) {
	if _, err := importParseInputOf(input); err != nil {
		return nil, err
	}
	return input, nil
}

func decodeImportParseInput(payload json.RawMessage, version uint) (json.RawMessage, error) {
	if version != jobImportKindVersion {
		return nil, fmt.Errorf("%w: import parse v%d input", jobs.ErrReplayCodecUnregistered, version)
	}
	if _, err := importParseInputOf(payload); err != nil {
		return nil, err
	}
	return payload, nil
}

// importParseInputOf reads one parse input. The staged path is validated to be
// inside the imports directory: the input names a file the executor opens, and a
// path outside `_imports` is a reference this Kind does not hand out.
func importParseInputOf(input json.RawMessage) (*importParseJobInput, error) {
	if len(input) == 0 || !json.Valid(input) {
		return nil, errors.New("an import parse Job's input is not valid JSON")
	}
	var decoded importParseJobInput
	if err := json.Unmarshal(input, &decoded); err != nil {
		return nil, fmt.Errorf("an import parse Job's input is not readable: %w", err)
	}
	if strings.TrimSpace(decoded.Handle) == "" {
		return nil, errors.New("an import parse Job's input names no import")
	}
	if err := requireImportStagingPath(decoded.Archive, ".tar"); err != nil {
		return nil, err
	}
	return &decoded, nil
}

func sanitizeImportApplyInput(input json.RawMessage) (json.RawMessage, error) {
	parsed, err := importApplyInputOf(input)
	if err != nil {
		return nil, err
	}
	return json.Marshal(importApplySummary{
		ParseHandle:     parsed.ParseHandle,
		ExcludedItems:   len(parsed.Decisions.ExcludedItems),
		MappingActions:  len(parsed.Decisions.MappingActions),
		DanglingActions: len(parsed.Decisions.DanglingActions),
		ShellActions:    len(parsed.Decisions.ShellGroupActions),
	})
}

func encodeImportApplyInput(input json.RawMessage) (json.RawMessage, error) {
	if _, err := importApplyInputOf(input); err != nil {
		return nil, err
	}
	return input, nil
}

func decodeImportApplyInput(payload json.RawMessage, version uint) (json.RawMessage, error) {
	if version != jobImportKindVersion {
		return nil, fmt.Errorf("%w: import apply v%d input", jobs.ErrReplayCodecUnregistered, version)
	}
	if _, err := importApplyInputOf(payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func importApplyInputOf(input json.RawMessage) (*importApplyJobInput, error) {
	if len(input) == 0 || !json.Valid(input) {
		return nil, errors.New("an import apply Job's input is not valid JSON")
	}
	var decoded importApplyJobInput
	if err := json.Unmarshal(input, &decoded); err != nil {
		return nil, fmt.Errorf("an import apply Job's input is not readable: %w", err)
	}
	if strings.TrimSpace(decoded.ParseHandle) == "" {
		return nil, errors.New("an import apply Job's input names no parse")
	}
	// The plan may be named at either staging path this Kind hands out: the review
	// artifact at its unconsumed path, which is what this release seals, or the
	// consumed file an apply admitted by an earlier release was admitted with. Which
	// one it is decides nothing at dispatch — the plan is bound to the Job there
	// (claimImportPlanForApply) — and refusing the second would refuse an in-flight Job
	// across the upgrade for no safety at all.
	planErr := requireImportStagingPath(decoded.Plan, ".plan.json")
	if planErr != nil {
		planErr = requireImportStagingPath(decoded.Plan, ".plan.applied.json")
	}
	if planErr != nil {
		return nil, planErr
	}
	return &decoded, nil
}

// requireImportStagingPath checks that one sealed path is a file inside the imports
// directory with the suffix the executor expects. The input is written by this
// application rather than by a caller, but a path that is opened by an executor is
// a path worth validating at the boundary that stores it.
func requireImportStagingPath(path, suffix string) error {
	if path == "" {
		return errors.New("an import Job's input names no staged file")
	}
	if filepath.Dir(path) != "_imports" || filepath.Base(path) != path[len("_imports/"):] && filepath.Dir(path) != "_imports" {
		return fmt.Errorf("an import Job's staged file %q is not inside the imports directory", path)
	}
	if !strings.HasSuffix(path, suffix) {
		return fmt.Errorf("an import Job's staged file %q does not end in %s", path, suffix)
	}
	if strings.Contains(filepath.Base(path), "..") {
		return fmt.Errorf("an import Job's staged file %q is not a plain file name", path)
	}
	return nil
}

// importArchiveExists reports whether one import's staged archive is still there.
func (ctx *MahresourcesContext) importArchiveExists(handle string) bool {
	if ctx == nil || handle == "" {
		return false
	}
	exists, err := afero.Exists(ctx.GetDefaultFs(), importArchivePathFor(handle))
	return err == nil && exists
}

// importPlanExists reports whether one import's plan is waiting at its unconsumed
// path — which is the durable evidence that a failed apply is safe to replay.
func (ctx *MahresourcesContext) importPlanExists(handle string) bool {
	if ctx == nil || handle == "" {
		return false
	}
	exists, err := afero.Exists(ctx.GetDefaultFs(), importPlanPathFor(handle))
	return err == nil && exists
}

// importPlanIsComplete verifies the durable file's identity and required header.
// The writer publishes by atomic rename, but reconciliation still validates the
// serialized artifact before treating it as a completed parse.
func (ctx *MahresourcesContext) importPlanIsComplete(handle string) bool {
	if !ctx.importPlanExists(handle) {
		return false
	}
	plan, err := ctx.LoadImportPlan(handle)
	return err == nil && plan != nil && plan.JobID == handle && plan.SchemaVersion > 0
}

// importStagedFileExists reports whether one staging path is readable, which is how
// this Kind asks whether the file it was admitted with is still there.
func (ctx *MahresourcesContext) importStagedFileExists(path string) bool {
	if ctx == nil || path == "" {
		return false
	}
	exists, err := afero.Exists(ctx.GetDefaultFs(), path)
	return err == nil && exists
}

// consumeImportPlan moves one plan into the name one apply owns, refusing when there
// is no plan left to consume.
//
// The rename is the whole arbitration: a plan may be consumed once, and the loser of
// two attempts — a fresh /apply and a Retry of the apply that restored it, which is the
// pair that used to end up sharing one file — is refused rather than handed a path it
// does not own. One definition, used by the submission (so a second /apply is a 409
// rather than a queued Job that fails) and by a dispatch that is consuming the plan its
// Job was admitted for.
func consumeImportPlan(fs afero.Fs, source, consumed string) error {
	if err := fs.Rename(source, consumed); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ErrImportPlanConsumed
		}
		return err
	}
	return nil
}

// ImportApplyPlanShouldBeRestored decides whether a failed apply's consumed plan
// should be renamed back so the import can be applied again.
//
// It is the *durable* replay-safety evidence, and the Kind's Retry predicate is
// written in terms of it: the plan is back at its unconsumed path exactly when this
// answered true, so asking about the file is asking this question about the
// execution that actually ran.
//
// Restore in three cases:
//
//   - result == nil: ApplyImport failed before Phase 1 finished, so no DB writes
//     happened (safe to replay regardless of archive format).
//   - result.HasMutations() is false: Phase 1 succeeded but Phase 2 aborted before
//     any row was committed. Still a clean DB, safe to replay.
//   - result.RetrySafe: Phase 2 mutated rows, but the archive carries GUIDs on every
//     group/note and the policy is not "skip", so replay is idempotent via GUID
//     collision handling and schema-def GUID/name lookups.
//
// Otherwise (legacy archive mid-apply, or skip policy mid-apply) the plan stays at
// .plan.applied.json and the user has to re-upload.
func ImportApplyPlanShouldBeRestored(result *ImportApplyResult) bool {
	if result == nil {
		return true
	}
	if !result.HasMutations() {
		return true
	}
	return result.RetrySafe
}

// importParseAdapter runs one archive parse.
type importParseAdapter struct {
	ctx  *MahresourcesContext
	kind string
}

func (a *importParseAdapter) Definition() jobs.Definition {
	return jobs.Definition{
		Kind:        a.kind,
		KindVersion: jobImportKindVersion,
		Restorable:  true,
		Visibility:  jobs.VisibilityOwner,
	}
}

func (a *importParseAdapter) Dispatch(ctx context.Context, execution jobs.Execution) error {
	if a.ctx == nil || a.ctx.downloadManager == nil {
		return errors.New("the download queue is not available")
	}
	input, err := importParseInputOf(execution.Input)
	if err != nil {
		return err
	}
	if execution.KindVersion != jobImportKindVersion {
		return fmt.Errorf("%w: import parse v%d input", jobs.ErrReplayCodecUnregistered, execution.KindVersion)
	}
	parsePrincipal := a.ctx.WithPrincipal(a.ctx.principalForPluginActor(execution.Access.UserID))
	if execution.Access.UserID == 0 {
		// An intentionally actorless execution runs as the host, which is the same
		// permissive branch every role guard takes for a context with no principal.
		parsePrincipal = a.ctx
	}
	if refusal := importWriteRefusal(parsePrincipal, "parse an import"); refusal != "" {
		return a.ctx.blockQueueJob(execution.JobID, execution.ExecutionToken, refusal)
	}
	entry, found := a.ctx.queueEntryFor(execution.JobID)
	if !found {
		if !a.ctx.importArchiveExists(input.Handle) {
			// The archive this parse was accepted for is gone, and there is no entry
			// left to publish from. There is nothing to parse and no Retry that could
			// bring the input back, so the Job is blocked for a person to decide about
			// rather than run against nothing.
			return a.ctx.blockQueueJob(execution.JobID, execution.ExecutionToken, "staged-archive-missing")
		}
		entry, err = a.start(execution, input)
		if err != nil {
			return err
		}
	} else if ref, ok := entry.CanonicalExecution(); !ok || ref.ExecutionToken != execution.ExecutionToken {
		if !entry.AttachCanonical(download_queue.CanonicalRef{
			JobID: execution.JobID, ExecutionToken: execution.ExecutionToken,
		}) {
			return fmt.Errorf("the queue entry %s publishes into another Job", entry.ID)
		}
	}

	if snap := entry.Snapshot(); queueJobTerminal(snap.Status) {
		return a.ctx.finishQueueExecution(execution, snap, func(finished *download_queue.DownloadJob) error {
			return a.publishOutcome(execution, input, finished)
		})
	}
	snap, err := a.ctx.waitForQueueExecution(ctx, execution, entry)
	if err != nil {
		return err
	}
	return a.ctx.finishQueueExecution(execution, snap, func(finished *download_queue.DownloadJob) error {
		return a.publishOutcome(execution, input, finished)
	})
}

// start submits the parse this execution needs. The archive stays where the
// submission staged it: the handle names it, and the executor normalizes any
// pre-rename staging path itself.
func (a *importParseAdapter) start(execution jobs.Execution, input *importParseJobInput) (*download_queue.DownloadJob, error) {
	handle, err := a.ctx.jobHandleFor(execution.JobID, ImportParseHandleNamespace)
	if err != nil {
		return nil, err
	}
	return a.ctx.submitQueueJob(
		download_queue.JobOptions{
			Source:       download_queue.JobSourceGroupImportParse,
			InitialPhase: "queued",
			URL:          input.Archive,
			OwnerUserID:  a.ctx.jobOwnerFor(execution.JobID),
		},
		handle,
		jobs.ExecutionRef{JobID: execution.JobID, ExecutionToken: execution.ExecutionToken},
		a.ctx.buildImportParseRunFn(input),
	)
}

func (a *importParseAdapter) publishOutcome(execution jobs.Execution, input *importParseJobInput, snap *download_queue.DownloadJob) error {
	switch snap.Status {
	case download_queue.JobStatusCompleted:
		planPath := importPlanPathFor(input.Handle)
		if err := a.ctx.publishQueueReport(execution, jobImportPlanOutput, "Import plan", planPath, true); err != nil {
			if !errors.Is(err, errQueueStagedOutputMissing) {
				// A refused publication is not a parse without a plan: the plan is on
				// disk and the write that would have recorded it is what failed. The
				// execution's owner retries the publication; ending the Job here would
				// report a parse that produced nothing.
				return err
			}
			return a.ctx.finishQueueJob(execution, jobs.StateFailed,
				&jobs.Failure{
					Code:    "import-plan-missing",
					Class:   jobs.FailureClassInternal,
					Message: "the import was parsed without leaving a plan to review",
				},
				[]string{jobImportPlanOutput})
		}
		return a.ctx.finishQueueJob(execution, jobs.StateSucceeded, nil, []string{jobImportPlanOutput})
	case download_queue.JobStatusCancelled:
		return a.ctx.finishQueueJob(execution, jobs.StateCancelled, nil, nil)
	default:
		return a.ctx.finishQueueJob(execution, jobs.StateFailed,
			&jobs.Failure{
				Code:    "import-parse-failed",
				Class:   jobs.FailureClassInternal,
				Message: "the archive could not be read",
			},
			nil)
	}
}

// Reconcile answers what should happen to one parse whose claim expired.
func (a *importParseAdapter) Reconcile(_ context.Context, request jobs.ReconcileRequest) (jobs.ReconcileDecision, error) {
	if a.ctx == nil || a.ctx.downloadManager == nil {
		return jobs.ReconcileExternalWorkUnproven, nil
	}
	if entry, found := a.ctx.queueEntryFor(request.Snapshot.ID); found {
		if !queueJobTerminal(entry.GetStatus()) {
			return jobs.ReconcileResume, nil
		}
		return jobs.ReconcileQueue, nil
	}
	input, err := importParseInputOf(request.Input)
	if err != nil {
		// The input names the handle every piece of evidence a parse leaves behind is
		// derived from, so a reconciler that cannot read it can check none of them. What
		// it must not do is decide the work is not running: the process that holds the key
		// may be parsing right now. Blocking here released the claim and the capacity of a
		// live parse, so the answer is the one that needs no input — proof the runtime is
		// gone.
		return a.ctx.queueOnlyIfTheRuntimeIsProvedGone(request), nil
	}
	outputs, err := a.ctx.jobOutputsFor(request.Snapshot.ID)
	if err != nil {
		return jobs.ReconcileExternalWorkUnproven, nil
	}
	if _, published := findJobOutput(outputs, jobImportPlanOutput); published {
		if !a.ctx.importPlanIsComplete(input.Handle) {
			return jobs.ReconcileFail, nil
		}
		return jobs.ReconcileSucceed, nil
	}
	if a.ctx.importPlanIsComplete(input.Handle) {
		// A complete plan can be renamed into place while ParseImport is still
		// returning. Its existence is not proof the queue executor has ended; only
		// a dead owning runtime or the output published by its terminal handoff is.
		if !runtimeIsProvedGone(request) {
			return jobs.ReconcileExternalWorkUnproven, nil
		}
		// The owner is proved gone, so publish this complete plan under the expired
		// claim's token and settle the execution that produced it.
		if err := a.ctx.publishQueueReport(request.Execution, jobImportPlanOutput, "Import plan",
			importPlanPathFor(input.Handle), true); err != nil {
			return jobs.ReconcileExternalWorkUnproven, nil
		}
		return jobs.ReconcileSucceed, nil
	}
	if !runtimeIsProvedGone(request) {
		return jobs.ReconcileExternalWorkUnproven, nil
	}
	if a.ctx.importArchiveExists(input.Handle) {
		// Nothing was produced and the archive is still there. Running the parse
		// again is safe once the runtime that claimed it is proved gone: a second
		// parse over a live one writes the same plan twice.
		return a.ctx.queueOnlyIfTheRuntimeIsProvedGone(request), nil
	}
	// Neither the plan nor the archive: there is no input left and nothing that
	// could produce one.
	return jobs.ReconcileFail, nil
}

// CleanupArtifacts accounts for one parse's outputs. A parse publishes no
// artifact: its plan is a report the delete handler and the import's own retention
// own, and claiming to have removed bytes this Kind never staged would be a lie a
// reader could act on.
func (a *importParseAdapter) CleanupArtifacts(_ context.Context, _ jobs.ArtifactCleanupRequest) (jobs.ArtifactCleanupResult, error) {
	return jobs.ArtifactCleanupResult{}, nil
}

// Commands reports what one parse offers. Retry is advertised only while the
// staged archive is still there, because the archive is the input a Retry needs and
// a button that dispatches a Job which cannot read its own input is not a control.
func (a *importParseAdapter) Commands(_ context.Context, commandContext jobs.CommandContext) ([]jobs.Command, error) {
	// §8: a principal demoted below "may write" keeps the history and loses the
	// controls over it.
	if a.ctx.commandActorRefusal(commandContext.Deps, commandContext.Access, "") != "" {
		return nil, nil
	}
	commands := []jobs.Command{{
		Key:          jobs.CommandCancel,
		Label:        "Cancel",
		Destructive:  true,
		Confirmation: "Stop reading this archive? Nothing is added to the library by a parse.",
	}}
	state := commandContext.Snapshot.State
	if state == jobs.StateFailed || state == jobs.StateCancelled || state == jobs.StateInterrupted {
		handle, err := a.ctx.jobHandleForDeps(commandContext.Deps, commandContext.Snapshot.ID, ImportParseHandleNamespace)
		if err != nil {
			return nil, err
		}
		if a.ctx.importArchiveExists(handle) {
			commands = append(commands, jobs.Command{Key: jobs.CommandRetry, Label: "Retry"})
		}
	}
	return commands, nil
}

func (a *importParseAdapter) ExecuteCommand(_ context.Context, execution jobs.CommandExecution) (jobs.CommandOutcome, error) {
	if a.ctx == nil || a.ctx.downloadManager == nil {
		return jobs.CommandOutcome{}, errors.New("the download queue is not available")
	}
	if execution.Key != jobs.CommandCancel {
		return jobs.CommandOutcome{}, fmt.Errorf("%w: %s", jobs.ErrCommandNotAdvertised, execution.Key)
	}
	handle, err := a.ctx.jobHandleFor(execution.JobID, ImportParseHandleNamespace)
	if err != nil {
		return jobs.CommandOutcome{}, err
	}
	if handle == "" {
		return jobs.CommandOutcome{Status: jobs.CommandStatusSucceeded, Message: "the parse is no longer running"}, nil
	}
	if _, found := a.ctx.downloadManager.GetJob(handle); !found {
		return jobs.CommandOutcome{Status: jobs.CommandStatusSucceeded, Message: "the parse is no longer running"}, nil
	}
	if err := a.ctx.downloadManager.Cancel(handle); err != nil {
		var conflict *download_queue.StateConflictError
		if errors.As(err, &conflict) {
			return jobs.CommandOutcome{Status: jobs.CommandStatusSucceeded, Message: "the parse had already finished"}, nil
		}
		return jobs.CommandOutcome{}, err
	}
	return jobs.CommandOutcome{Status: jobs.CommandStatusSucceeded, Message: "cancelling"}, nil
}

// importApplyAdapter runs one apply of a reviewed plan.
type importApplyAdapter struct {
	ctx  *MahresourcesContext
	kind string
}

func (a *importApplyAdapter) Definition() jobs.Definition {
	return jobs.Definition{
		Kind:        a.kind,
		KindVersion: jobImportKindVersion,
		Restorable:  true,
		Visibility:  jobs.VisibilityOwner,
	}
}

// Dispatch runs one claimed apply, binding the Job's actor as the importing
// principal so every row the apply creates is attributed to whoever asked for it.
//
// The binding is what the handler does at submission and what a Retry has no
// request for: the Job records the actor, and `principalForPluginActor` resolves it
// to the stored account — deny-all when that account is gone, never unscoped.
func (a *importApplyAdapter) Dispatch(ctx context.Context, execution jobs.Execution) error {
	if a.ctx == nil || a.ctx.downloadManager == nil {
		return errors.New("the download queue is not available")
	}
	input, err := importApplyInputOf(execution.Input)
	if err != nil {
		return err
	}
	if execution.KindVersion != jobImportKindVersion {
		return fmt.Errorf("%w: import apply v%d input", jobs.ErrReplayCodecUnregistered, execution.KindVersion)
	}
	bound := a.ctx.WithPrincipal(a.ctx.principalForPluginActor(execution.Access.UserID))
	if execution.Access.UserID == 0 {
		bound = a.ctx
	}
	if refusal := importWriteRefusal(bound, "apply an import"); refusal != "" {
		return a.ctx.blockQueueJob(execution.JobID, execution.ExecutionToken, refusal)
	}

	entry, found := a.ctx.queueEntryFor(execution.JobID)
	if !found {
		entry, err = a.start(bound, execution, input)
		if err != nil {
			return err
		}
	} else if ref, ok := entry.CanonicalExecution(); !ok || ref.ExecutionToken != execution.ExecutionToken {
		if !entry.AttachCanonical(download_queue.CanonicalRef{
			JobID: execution.JobID, ExecutionToken: execution.ExecutionToken,
		}) {
			return fmt.Errorf("the queue entry %s publishes into another Job", entry.ID)
		}
	}

	if snap := entry.Snapshot(); queueJobTerminal(snap.Status) {
		return a.ctx.finishQueueExecution(execution, snap, func(finished *download_queue.DownloadJob) error {
			return a.publishOutcome(execution, input, finished)
		})
	}
	snap, err := a.ctx.waitForQueueExecution(ctx, execution, entry)
	if err != nil {
		return err
	}
	return a.ctx.finishQueueExecution(execution, snap, func(finished *download_queue.DownloadJob) error {
		return a.publishOutcome(execution, input, finished)
	})
}

// start binds the plan to this execution's Job and submits the apply it needs.
//
// Two plans could be in play, and telling them apart is what makes an
// accepted-but-undispatched apply runnable at all:
//
//   - The plan is at the path *this Job owns* (`importConsumedPlanPathFor`, named by
//     this apply's own handle): this apply consumed it and never ran it. The queue
//     entry it was going to get either never existed (the process stopped between
//     acceptance and submission) or was claimed away by the very runtime dispatching
//     it now. Reading that file applies the import exactly once.
//   - The plan is at its *unconsumed* path, which is what this Job's input names: the
//     executor restored it as its own replay-safety evidence, or the Job is a Retry of
//     one that did, so this consumes it now exactly as the submission would have. The
//     rename is atomic, so of a Retry and a fresh /apply for one review exactly one
//     takes the plan and the other is refused; neither may read the other's copy.
//
// The archive is required either way: the plan names rows, the archive has their
// bytes.
func (a *importApplyAdapter) start(bound *MahresourcesContext, execution jobs.Execution, input *importApplyJobInput) (*download_queue.DownloadJob, error) {
	plan, err := a.claimPlanForApply(execution, input)
	if err != nil {
		return nil, err
	}
	if !a.ctx.importArchiveExists(input.ParseHandle) {
		return nil, fmt.Errorf("the archive for import %s is no longer there", input.ParseHandle)
	}
	opts := download_queue.JobOptions{
		Source:       download_queue.JobSourceGroupImportApply,
		InitialPhase: "queued",
		OwnerUserID:  a.ctx.jobOwnerFor(execution.JobID),
	}
	handle, err := a.ctx.jobHandleFor(execution.JobID, ImportApplyHandleNamespace)
	if err != nil {
		return nil, err
	}
	return bound.submitQueueJob(
		opts,
		handle,
		jobs.ExecutionRef{JobID: execution.JobID, ExecutionToken: execution.ExecutionToken},
		bound.buildImportApplyRunFn(input.ParseHandle, plan, &input.Decisions),
	)
}

// claimPlanForApply answers the plan path one apply may read, binding the plan to that
// apply's Job if its admission did not already.
//
// The binding is the file's *name*: the plan is consumed into a path derived from this
// Job's own handle, so finding one there is evidence about this Job — its admission put
// it there — and never about whichever apply consumed a shared name last. That is the
// difference between the two executors the old scheme could not tell apart: a Retry
// whose plan had been consumed by a fresh /apply found the consumed file, read it as
// "my admission consumed this and never ran it", and applied the same archive a second
// time beside the apply already running it.
//
// The plan the input names is the *source* it is consumed from, so an apply that has not
// consumed anything yet (a Retry, whose input is its ancestor's) takes it exactly once,
// and a refusal means somebody else has. A refusal is a refusal to *admit* the work —
// nothing can run it — rather than a failure of the work, which is why the caller blocks
// the Job for a person instead of ending it.
func (a *importApplyAdapter) claimPlanForApply(execution jobs.Execution, input *importApplyJobInput) (string, error) {
	handle, err := a.ctx.jobHandleFor(execution.JobID, ImportApplyHandleNamespace)
	if err != nil {
		return "", err
	}
	if handle == "" {
		// A Job accepted without the handle the client is answered with (a test, or a
		// migration): the canonical id is unique per Job and names the same lineage.
		handle = execution.JobID
	}
	mine := importConsumedPlanPathFor(input.ParseHandle, handle)
	if a.ctx.importStagedFileExists(mine) {
		return mine, nil
	}
	source := strings.TrimSpace(input.Plan)
	if err := consumeImportPlan(a.ctx.GetDefaultFs(), source, mine); err != nil {
		if errors.Is(err, ErrImportPlanConsumed) {
			return "", fmt.Errorf("%w: the plan for import %s is no longer there",
				ErrImportPlanConsumed, input.ParseHandle)
		}
		return "", err
	}
	return mine, nil
}

func (a *importApplyAdapter) publishOutcome(execution jobs.Execution, input *importApplyJobInput, snap *download_queue.DownloadJob) error {
	// The report goes first, and on every outcome: a partial apply's result is the
	// only thing that names the rows it created, which is exactly what a failure
	// leaves behind.
	resultPath := importResultPathFor(input.ParseHandle)
	if _, err := a.ctx.GetDefaultFs().Stat(resultPath); err == nil {
		if publishErr := a.ctx.publishQueueReport(execution, jobImportResultOutput, "Import report", resultPath, false); publishErr != nil &&
			!mirrorRefusalIsSilent(publishErr) {
			// The report is optional to success, but this execution owns its publication
			// snapshot. A transient write refusal must be retried alongside the terminal
			// outcome; otherwise the first attempt can succeed without ever retaining the
			// partial-apply report.
			return publishErr
		}
	}
	switch snap.Status {
	case download_queue.JobStatusCompleted:
		return a.ctx.finishQueueJob(execution, jobs.StateSucceeded, nil, nil)
	case download_queue.JobStatusCancelled:
		return a.ctx.finishQueueJob(execution, jobs.StateCancelled, nil, nil)
	default:
		return a.ctx.finishQueueJob(execution, jobs.StateFailed,
			&jobs.Failure{
				Code:    "import-apply-failed",
				Class:   jobs.FailureClassInternal,
				Message: "the import could not be applied",
			},
			nil)
	}
}

// Reconcile answers what should happen to one apply whose claim expired.
//
// The two files are the evidence, and they answer the design's question — but only the
// first arm of it is positive. A plan back at its unconsumed path is produced by one
// thing only: the executor's own replay-safety gate, which restores it exactly when a
// replay is provably idempotent and never before the run has ended. That is why it may
// be queued again without proving anything about the process that wrote it.
//
// A *consumed* plan is the opposite kind of answer. Consuming the plan is what the
// submission does before the executor exists, so it is equally the state of an apply
// another process is walking right now — and "this process has no queue entry"
// distinguishes nothing, because the queue is memory and the process is another one.
// Reading absence there as "nothing can continue" terminated live work and released its
// ownership, which is the one thing §3 forbids: a terminal outcome on external work
// nobody has proved quiescent. So the terminal classification is taken only on positive
// evidence that the runtime is gone, and otherwise the claim, the capacity and the Job
// stay exactly where they are for a person to resolve.
func (a *importApplyAdapter) Reconcile(_ context.Context, request jobs.ReconcileRequest) (jobs.ReconcileDecision, error) {
	if a.ctx == nil || a.ctx.downloadManager == nil {
		return jobs.ReconcileExternalWorkUnproven, nil
	}
	if entry, found := a.ctx.queueEntryFor(request.Snapshot.ID); found {
		if !queueJobTerminal(entry.GetStatus()) {
			return jobs.ReconcileResume, nil
		}
		return jobs.ReconcileQueue, nil
	}
	input, err := importApplyInputOf(request.Input)
	if err != nil {
		// Every piece of evidence an apply is judged by — the plan restored to its
		// unconsumed name, the archive it reads blobs from — is named by this input, so a
		// reconciler that cannot read it can judge nothing. The process holding the key may
		// be applying the plan at this instant, and a block here released its claim, its
		// token and its capacity: the replacement would then be refused by the plan's own
		// consumption while the first apply kept writing. What is left is the question that
		// needs no input, positive proof the runtime is gone.
		return a.ctx.queueOnlyIfTheRuntimeIsProvedGone(request), nil
	}
	if a.ctx.importPlanExists(input.ParseHandle) && a.ctx.importArchiveExists(input.ParseHandle) {
		return jobs.ReconcileQueue, nil
	}
	if !runtimeIsProvedGone(request) {
		// An executor in another process may be applying this plan at this instant, and
		// nothing here can tell that from a process that died mid-apply. The Job keeps
		// its claim and its capacity, no terminal outcome is recorded, and a person
		// decides — which is recoverable, where a duplicated import is not.
		return jobs.ReconcileExternalWorkUnproven, nil
	}
	// The runtime is proved gone and the plan is consumed without having been restored:
	// the apply reached the one state that cannot be replayed. It stays failed, with
	// whatever report the executor left for a reader.
	return jobs.ReconcileFail, nil
}

// CleanupArtifacts accounts for one apply's outputs: an apply stages no artifact of
// its own, and its report is owned by the import's delete handler.
func (a *importApplyAdapter) CleanupArtifacts(_ context.Context, _ jobs.ArtifactCleanupRequest) (jobs.ArtifactCleanupResult, error) {
	return jobs.ArtifactCleanupResult{}, nil
}

// Commands reports what one apply offers.
//
// Retry is advertised only when the executor's own replay-safety evidence is still
// on disk: the plan restored to its unconsumed path *and* the archive it reads
// blobs from. A partial apply that is not replay-safe offers none — it publishes its
// report and stays failed, which is the honest answer, and re-running it would
// duplicate the rows it already committed.
func (a *importApplyAdapter) Commands(_ context.Context, commandContext jobs.CommandContext) ([]jobs.Command, error) {
	// §8: a principal demoted below "may write" keeps the history and loses the
	// controls over it.
	if a.ctx.commandActorRefusal(commandContext.Deps, commandContext.Access, "") != "" {
		return nil, nil
	}
	commands := make([]jobs.Command, 0, 2)
	state := commandContext.Snapshot.State
	if !state.Terminal() {
		commands = append(commands, jobs.Command{
			Key:          jobs.CommandCancel,
			Label:        "Cancel",
			Destructive:  true,
			Confirmation: "Stop applying this import? Rows it has already created stay in the library.",
		})
	}
	if state == jobs.StateFailed || state == jobs.StateCancelled || state == jobs.StateInterrupted {
		// The Job's own sealed input names the plan that would be replayed, so the
		// evidence is read from the same input the Retry would use rather than from a
		// handle that may have moved. It is read on the handle this advertisement was
		// asked on: the command plane re-asks this question inside the transaction
		// that would create the successor, and a second connection there deadlocks a
		// pool of one.
		input, err := a.inputOf(commandContext.Deps, commandContext.Snapshot.ID)
		if err != nil {
			return commands, nil
		}
		if a.ctx.importPlanExists(input.ParseHandle) && a.ctx.importArchiveExists(input.ParseHandle) {
			commands = append(commands, jobs.Command{Key: jobs.CommandRetry, Label: "Retry"})
		}
	}
	return commands, nil
}

// inputOf opens one Job's sealed input as this Kind reads it, on the caller's own
// handle. An input this process cannot open answers an error, and every caller
// treats that as "nothing can be promised about a re-run" rather than as a failure
// to answer.
func (a *importApplyAdapter) inputOf(deps jobs.Deps, jobID string) (*importApplyJobInput, error) {
	service := a.ctx.JobService()
	if service == nil {
		return nil, errors.New("this context has no job control plane installed")
	}
	opened, err := service.OpenReplay(deps, jobs.Access{Administrator: true}, jobID)
	if err != nil {
		return nil, err
	}
	return importApplyInputOf(opened.Input)
}

func (a *importApplyAdapter) ExecuteCommand(_ context.Context, execution jobs.CommandExecution) (jobs.CommandOutcome, error) {
	if a.ctx == nil || a.ctx.downloadManager == nil {
		return jobs.CommandOutcome{}, errors.New("the download queue is not available")
	}
	if execution.Key != jobs.CommandCancel {
		return jobs.CommandOutcome{}, fmt.Errorf("%w: %s", jobs.ErrCommandNotAdvertised, execution.Key)
	}
	handle, err := a.ctx.jobHandleFor(execution.JobID, ImportApplyHandleNamespace)
	if err != nil {
		return jobs.CommandOutcome{}, err
	}
	if handle == "" {
		return jobs.CommandOutcome{Status: jobs.CommandStatusSucceeded, Message: "the import is no longer running"}, nil
	}
	if _, found := a.ctx.downloadManager.GetJob(handle); !found {
		return jobs.CommandOutcome{Status: jobs.CommandStatusSucceeded, Message: "the import is no longer running"}, nil
	}
	if err := a.ctx.downloadManager.Cancel(handle); err != nil {
		var conflict *download_queue.StateConflictError
		if errors.As(err, &conflict) {
			return jobs.CommandOutcome{Status: jobs.CommandStatusSucceeded, Message: "the import had already finished"}, nil
		}
		return jobs.CommandOutcome{}, err
	}
	return jobs.CommandOutcome{Status: jobs.CommandStatusSucceeded, Message: "cancelling"}, nil
}

func (ctx *MahresourcesContext) buildImportParseRunFn(input *importParseJobInput) download_queue.JobRunFn {
	return func(jobCtx context.Context, j *download_queue.DownloadJob, sink download_queue.ProgressSink) error {
		return ctx.runImportParseJob(jobCtx, j, sink, input)
	}
}

// runImportParseJob reads one staged archive into a plan.
func (ctx *MahresourcesContext) runImportParseJob(jobCtx context.Context, j *download_queue.DownloadJob, sink download_queue.ProgressSink, input *importParseJobInput) error {
	sink.SetPhase("parsing")

	fs := ctx.GetDefaultFs()
	canonicalPath := importArchivePathFor(input.Handle)
	// Normalize to _imports/<handle>.tar so DeleteImportFiles can find it by handle.
	// On first run the file may still be at the handler's staging path; on a retry it
	// is already at the canonical path, so skip the rename if the staging path is
	// gone.
	if input.Archive != canonicalPath {
		if exists, _ := afero.Exists(fs, input.Archive); exists {
			if err := fs.Rename(input.Archive, canonicalPath); err != nil {
				return fmt.Errorf("rename staged tar: %w", err)
			}
		}
	}

	plan, err := ctx.ParseImport(jobCtx, input.Handle, canonicalPath)
	if err != nil {
		return err
	}

	planPath := importPlanPathFor(input.Handle)
	sink.SetResultPath(planPath)
	sink.SetPhase("completed")

	for _, warning := range plan.Warnings {
		sink.AppendWarning(warning)
	}
	return nil
}

// buildImportApplyRunFn is the apply executor's body.
func (ctx *MahresourcesContext) buildImportApplyRunFn(parseHandle, consumedPlanPath string, decisions *ImportDecisions) download_queue.JobRunFn {
	return func(jobCtx context.Context, j *download_queue.DownloadJob, sink download_queue.ProgressSink) error {
		return ctx.runImportApplyJob(jobCtx, sink, parseHandle, consumedPlanPath, decisions)
	}
}

// runImportApplyJob applies one reviewed plan.
//
// ctx is principal-bound by the caller — the submission handler or the adapter's
// dispatch — so every `db.Create` inside ApplyImport inherits the acting-user
// context and stamps CreatedByUserId. One binding covers every entity the import
// creates.
func (ctx *MahresourcesContext) runImportApplyJob(jobCtx context.Context, sink download_queue.ProgressSink, parseHandle, consumedPlanPath string, decisions *ImportDecisions) error {
	result, err := ctx.ApplyImport(jobCtx, parseHandle, consumedPlanPath, decisions, sink)

	// The result is persisted even on failure: a partial-failure result lists the
	// IDs it created for manual cleanup, and it is the report a Job publishes.
	if result != nil {
		resultPath := importResultPathFor(parseHandle)
		if data, marshalErr := json.Marshal(result); marshalErr == nil {
			_ = afero.WriteFile(ctx.GetDefaultFs(), resultPath, data, 0644)
			sink.SetResultPath(resultPath)
		}
	}

	if err != nil {
		if ImportApplyPlanShouldBeRestored(result) {
			if renameErr := ctx.GetDefaultFs().Rename(consumedPlanPath, importPlanPathFor(parseHandle)); renameErr != nil {
				sink.AppendWarning(fmt.Sprintf("could not restore plan for retry: %v", renameErr))
			}
		}
		return err
	}

	for _, warning := range result.Warnings {
		sink.AppendWarning(warning)
	}
	// Clean up the consumed plan on success.
	_ = ctx.GetDefaultFs().Remove(consumedPlanPath)

	sink.SetPhase("completed")
	return nil
}

// SubmitImportParse is the one door an archive parse is submitted through.
//
// The archive is renamed into its final path *before* the Job is accepted, because
// the input names the file the executor reads: accepting a Job whose input names a
// staging path that may be renamed underneath it would be an input that describes
// something other than what runs.
func (ctx *MahresourcesContext) SubmitImportParse(handle, stagingTarPath string, origin string) QueueJobSubmission {
	result := QueueJobSubmission{QueueJobID: handle}
	if ctx == nil || ctx.downloadManager == nil {
		result.Err = errors.New("the download queue is not available")
		return result
	}
	fs := ctx.GetDefaultFs()
	if err := fs.MkdirAll("_imports", 0755); err != nil {
		result.Err = fmt.Errorf("failed to create imports dir: %w", err)
		return result
	}
	archivePath := importArchivePathFor(handle)
	if stagingTarPath != archivePath {
		if err := fs.Rename(stagingTarPath, archivePath); err != nil {
			result.Err = fmt.Errorf("failed to finalize upload: %w", err)
			return result
		}
	}

	owner := ctx.queueSubmitterOwner()
	input, err := json.Marshal(importParseJobInput{Handle: handle, Archive: archivePath})
	if err != nil {
		result.Err = err
		return result
	}
	opts := download_queue.JobOptions{
		Source:       download_queue.JobSourceGroupImportParse,
		InitialPhase: "queued",
		URL:          archivePath,
		OwnerUserID:  owner,
	}
	service := ctx.JobService()
	if service == nil {
		job, err := ctx.downloadManager.SubmitJobWithOptions(opts, ctx.buildImportParseRunFn(&importParseJobInput{
			Handle: handle, Archive: archivePath,
		}))
		if err != nil {
			result.Err = err
			return result
		}
		result.QueueJobID = job.ID
		return result
	}

	admission, err := ctx.admitQueueJob(jobs.Acceptance{
		Kind:        JobKindGroupImportParse,
		KindVersion: jobImportKindVersion,
		State:       jobs.StateQueued,
		OwnerUserID: owner,
		ActorUserID: owner,
		Origin:      origin,
		Title:       "Group import",
		Replay:      jobs.ReplayInput{Input: input},
		LegacyRefs:  []jobs.LegacyRef{{Namespace: ImportParseHandleNamespace, Handle: handle}},
	})
	if err != nil {
		result.Err = err
		return result
	}
	result.CanonicalJobID = admission.Accepted.ID
	if !admission.Owned() {
		// The deployment's budget is full: the Job is durable, answers the handle the
		// client was handed, and starts no executor here. A runtime with a free slot
		// takes it, and the archive its input names is still staged. See admitQueueJob.
		return result
	}

	parseInput := &importParseJobInput{Handle: handle, Archive: archivePath}
	entry, err := ctx.submitQueueJob(opts, handle,
		jobs.ExecutionRef{JobID: admission.Execution.JobID, ExecutionToken: admission.Execution.ExecutionToken},
		ctx.buildImportParseRunFn(parseInput))
	if err != nil {
		ctx.failUndispatchedQueueJob(admission, err)
		result.Err = err
		return result
	}
	result.QueueJobID = entry.ID
	ctx.ownQueueExecution(admission, entry, func(snap *download_queue.DownloadJob) error {
		return (&importParseAdapter{ctx: ctx, kind: JobKindGroupImportParse}).publishOutcome(admission.Execution, parseInput, snap)
	})
	return result
}

// SubmitImportApply is the one door an apply is submitted through.
//
// The plan is bound to this apply *before* the Job is accepted — the review artifact is
// renamed to the name this apply's Job owns — which is what makes a second /apply on the
// same review a refusal rather than a queued Job that fails, and what makes the file a
// Job finds at that path evidence about its own admission. The Job is then accepted,
// claimed, and linked to the parse it decided on in **one** transaction: the link is
// what carries the parse's handle — and so the protection of the archive and plan it
// staged — to a live descendant, so an apply that committed without it would be durable
// work whose input the startup sweep is entitled to delete. The parse is therefore
// resolved first, and a handle that names no durable Job is a refusal rather than an
// unlinked acceptance. The executor starts only after that transaction commits.
func (ctx *MahresourcesContext) SubmitImportApply(parseHandle string, decisions *ImportDecisions, origin string) QueueJobSubmission {
	result := QueueJobSubmission{}
	if ctx == nil || ctx.downloadManager == nil {
		result.Err = errors.New("the download queue is not available")
		return result
	}
	if decisions == nil {
		result.Err = errors.New("applying an import needs the decisions it was reviewed with")
		return result
	}

	owner := ctx.queueSubmitterOwner()
	opts := download_queue.JobOptions{
		Source:       download_queue.JobSourceGroupImportApply,
		InitialPhase: "queued",
		OwnerUserID:  owner,
	}
	service := ctx.JobService()
	if service == nil {
		consumedPath := importConsumedPlanPathFor(parseHandle, download_queue.NewJobID())
		if err := consumeImportPlan(ctx.GetDefaultFs(), importPlanPathFor(parseHandle), consumedPath); err != nil {
			result.Err = err
			return result
		}
		job, err := ctx.downloadManager.SubmitJobWithOptions(opts,
			ctx.buildImportApplyRunFn(parseHandle, consumedPath, decisions))
		if err != nil {
			_ = ctx.GetDefaultFs().Rename(consumedPath, importPlanPathFor(parseHandle))
			result.Err = err
			return result
		}
		result.QueueJobID = job.ID
		return result
	}

	// The parse whose files this apply reads. A handle that names no canonical Job — an
	// import from before this release, or a deployment whose handle row is gone — cannot
	// be linked, and an unlinked apply is work the sweep may delete the input of.
	parentID, err := service.ResolveLegacyHandle(ctx.jobDeps(), ImportParseHandleNamespace, parseHandle)
	if err != nil {
		result.Err = fmt.Errorf("%w: the import %s this apply belongs to has no durable record", err, parseHandle)
		return result
	}

	legacyID := download_queue.NewJobID()
	consumedPath := importConsumedPlanPathFor(parseHandle, legacyID)
	if err := consumeImportPlan(ctx.GetDefaultFs(), importPlanPathFor(parseHandle), consumedPath); err != nil {
		result.Err = err
		return result
	}

	input, err := json.Marshal(importApplyJobInput{
		ParseHandle: parseHandle,
		Plan:        importPlanPathFor(parseHandle),
		Decisions:   *decisions,
	})
	if err != nil {
		ctx.restoreImportPlan(consumedPath, parseHandle)
		result.Err = err
		return result
	}
	// Answered before the capacity question, because it is the id the caller is
	// handed whatever happens next: the Job is durable and answers this handle
	// whether this process runs its executor or a runtime with a free slot does.
	result.QueueJobID = legacyID
	admission, err := ctx.admitQueueJob(jobs.Acceptance{
		Kind:        JobKindGroupImportApply,
		KindVersion: jobImportKindVersion,
		State:       jobs.StateQueued,
		OwnerUserID: owner,
		ActorUserID: owner,
		Origin:      origin,
		Title:       "Apply import",
		Replay:      jobs.ReplayInput{Input: input},
		LegacyRefs:  []jobs.LegacyRef{{Namespace: ImportApplyHandleNamespace, Handle: legacyID}},
		Parents:     []string{parentID},
	})
	if err != nil {
		ctx.restoreImportPlan(consumedPath, parseHandle)
		result.Err = err
		return result
	}
	result.CanonicalJobID = admission.Accepted.ID
	if !admission.Owned() {
		// The deployment's budget is full: the Job is durable, answers the id the
		// client was handed, and starts no executor here. A runtime with a free slot
		// takes it and consumes the plan its input names. See admitQueueJob.
		return result
	}

	applyInput := &importApplyJobInput{ParseHandle: parseHandle, Plan: importPlanPathFor(parseHandle), Decisions: *decisions}
	entry, err := ctx.submitQueueJob(opts, legacyID,
		jobs.ExecutionRef{JobID: admission.Execution.JobID, ExecutionToken: admission.Execution.ExecutionToken},
		ctx.buildImportApplyRunFn(parseHandle, consumedPath, decisions))
	if err != nil {
		ctx.failUndispatchedQueueJob(admission, err)
		result.Err = err
		return result
	}
	result.QueueJobID = entry.ID
	ctx.ownQueueExecution(admission, entry, func(snap *download_queue.DownloadJob) error {
		return (&importApplyAdapter{ctx: ctx, kind: JobKindGroupImportApply}).publishOutcome(admission.Execution, applyInput, snap)
	})
	return result
}

// restoreImportPlan hands a consumed plan back to its review path after an admission
// that did not commit. The consumption is part of the submission, so a refused one must
// leave the review exactly as it found it — the client's next /apply has to work.
func (ctx *MahresourcesContext) restoreImportPlan(consumedPath, parseHandle string) {
	_ = ctx.GetDefaultFs().Rename(consumedPath, importPlanPathFor(parseHandle))
}

// ImportJobAuthorized answers whether this context's principal may act on the import

// ClaimImportPlanForJob is the submission's own consumption rule, exposed for the one
// caller that has to answer 409 rather than queue an apply with nothing to decide on,
// and for tests that need the exact state a running apply is in.
func ClaimImportPlanForJob(fs afero.Fs, parseHandle, applyHandle string) (string, error) {
	consumed := importConsumedPlanPathFor(parseHandle, applyHandle)
	if err := consumeImportPlan(fs, importPlanPathFor(parseHandle), consumed); err != nil {
		return "", err
	}
	return consumed, nil
}

// ImportJobAuthorized answers whether this context's principal may act on the import
// named by one parse handle.
//
// `answered` reports whether a durable Job answered at all: false means the handle
// names no canonical Job — this deployment has no control plane, or the id belongs to
// an import from before this release — and the caller then applies the legacy rule
// itself. Hidden and missing are one answer for a reader, but only the second one sends
// a caller back to the queue's own record.
func (ctx *MahresourcesContext) ImportJobAuthorized(parseHandle string) (bool, bool) {
	service := ctx.JobService()
	if service == nil {
		return false, false
	}
	// Resolved without a viewer, then read with one: which Job a handle names is a
	// property of the handle table, and whether this principal may act on it is a second
	// question, asked through the same predicate every Job read uses. Treating the two as
	// one question would send a caller back to the queue's own record for a Job the
	// durable table already knows about and this principal simply may not see.
	jobID, err := service.ResolveLegacyHandle(ctx.jobDeps(), ImportParseHandleNamespace, parseHandle)
	if err != nil {
		if errors.Is(err, jobs.ErrNotFound) {
			// Not a handle at all: it may still be a queue entry from before this
			// release, or a parse queued by a deployment that had no control plane.
			return false, false
		}
		return false, true
	}
	if _, err := service.Get(ctx.jobDeps(), ctx.jobAccess(), jobID); err != nil {
		return false, true
	}
	return true, true
}

// importWriteRefusal answers why one import execution may not run as the principal it
// was bound to, or an empty string.
//
// The binding answered *where* a write would land; this answers whether the account may
// write at all. Both are needed, and the second is the one nothing else supplies: an
// apply admitted while the deployment was busy is dispatched by whichever runtime has
// room, so the account that asked for the import may have been demoted in between, and
// a runtime that only bound its subtree would import on behalf of somebody who may no
// longer write anything.
//
// It is asked before the plan is read or any staged file is touched, so a refusal costs
// a filesystem and a database nothing.
func importWriteRefusal(bound *MahresourcesContext, op string) string {
	if bound == nil {
		return ""
	}
	if err := bound.requireWriteRole(op); err != nil {
		return "role-refused"
	}
	return ""
}
