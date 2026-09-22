package application_context

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"time"

	"mahresources/download_queue"
	"mahresources/jobs"
)

// This file is the group-export Kind adapter: the one place that knows how an
// export joins the durable Job control plane.
//
// The work stays where it was — StreamExport streams the tar and the queue runs it
// — and what is added is durable identity, a typed artifact output with its own
// expiry, and an outcome that follows the artifact. Three things are worth saying
// out loud, because each is a decision rather than a mechanical mapping:
//
//   - The archive is staged under the *canonical Job's* id. Naming it after the
//     legacy handle instead would give an ancestor's artifact and the artifact of a
//     Repeat the same path, and a repeat would rewrite the file the ancestor's
//     output row still names. The handle stays what a client polls with; the file
//     belongs to the execution that produced it.
//   - The archive is written to `<path>.part` and renamed into place once it is
//     whole, so a file at the published path is a complete one. That is what makes
//     reconciliation able to tell "the export finished and nobody recorded it" from
//     "the export died mid-stream", and it is why a crash before publication is
//     never a blind rerun.
//   - Cancel is cooperative. The queue's own cancellation reaches the runFn's
//     context, StreamExport honours it, and the partial file is removed; the Job is
//     cancelled when the entry reaches that terminal status, never when the request
//     was accepted.

const (
	// JobKindGroupExport is one group export submitted from the UI, the API, the
	// CLI or a plugin.
	JobKindGroupExport = "group-export"
	// jobExportKindVersion is the version of this Kind's input semantics.
	jobExportKindVersion = 1
	// jobExportArtifactOutput is the output key a succeeded export publishes. §7
	// makes success depend on it: an export whose archive cannot be handed over did
	// not succeed, whatever the queue's own status says.
	jobExportArtifactOutput = "artifact"
	// jobExportPartialSuffix is appended to the archive's path while it is being
	// written. A file at the published path is therefore a complete one.
	jobExportPartialSuffix = ".part"
)

// exportJobInput is what an export Job is accepted with: the request the caller
// made. Nothing in it is a secret — a set of group ids and a set of scope flags —
// so the sanitized summary is a projection of it rather than a redaction.
type exportJobInput struct {
	Request ExportRequest `json:"request"`
}

func exportJobCodec() jobs.ReplayCodec {
	return jobs.ReplayCodec{
		Sanitize: sanitizeExportInput,
		Encode:   encodeExportInput,
		Decode:   decodeExportInput,
		Migrate: func(payload json.RawMessage, fromVersion, toVersion uint) (json.RawMessage, error) {
			if fromVersion != toVersion {
				return nil, fmt.Errorf("jobs: no export input migration from v%d to v%d", fromVersion, toVersion)
			}
			return payload, nil
		},
	}
}

// exportSummary is the bounded, searchable half of an export's input.
type exportSummary struct {
	RootGroups     []uint   `json:"rootGroups,omitempty"`
	Subtree        bool     `json:"subtree"`
	OwnedResources bool     `json:"ownedResources,omitempty"`
	OwnedNotes     bool     `json:"ownedNotes,omitempty"`
	RelatedM2M     bool     `json:"relatedM2M,omitempty"`
	GroupRelations bool     `json:"groupRelations,omitempty"`
	Gzip           bool     `json:"gzip,omitempty"`
	Fidelity       []string `json:"fidelity,omitempty"`
}

func sanitizeExportInput(input json.RawMessage) (json.RawMessage, error) {
	request, err := exportRequestOf(input)
	if err != nil {
		return nil, err
	}
	return json.Marshal(exportSummary{
		RootGroups:     request.RootGroupIDs,
		Subtree:        request.Scope.Subtree,
		OwnedResources: request.Scope.OwnedResources,
		OwnedNotes:     request.Scope.OwnedNotes,
		RelatedM2M:     request.Scope.RelatedM2M,
		GroupRelations: request.Scope.GroupRelations,
		Gzip:           request.Gzip,
		Fidelity:       exportFidelityWords(request),
	})
}

func encodeExportInput(input json.RawMessage) (json.RawMessage, error) {
	if _, err := exportRequestOf(input); err != nil {
		return nil, err
	}
	return input, nil
}

func decodeExportInput(payload json.RawMessage, version uint) (json.RawMessage, error) {
	if version != jobExportKindVersion {
		return nil, fmt.Errorf("%w: export v%d input", jobs.ErrReplayCodecUnregistered, version)
	}
	if _, err := exportRequestOf(payload); err != nil {
		return nil, err
	}
	return payload, nil
}

// exportRequestOf reads one export input, refusing the two shapes no execution can
// use: a payload nothing can decode, and a request naming no root group.
func exportRequestOf(input json.RawMessage) (*ExportRequest, error) {
	if len(input) == 0 || !json.Valid(input) {
		return nil, errors.New("an export Job's input is not valid JSON")
	}
	var decoded exportJobInput
	if err := json.Unmarshal(input, &decoded); err != nil {
		return nil, fmt.Errorf("an export Job's input is not readable: %w", err)
	}
	if len(decoded.Request.RootGroupIDs) == 0 {
		return nil, errors.New("an export Job's input names no root group")
	}
	return &decoded.Request, nil
}

// exportFidelityWords names the parts of an archive the request asked for, so a
// reader can tell a metadata-only export from one carrying bytes without opening
// the sealer.
func exportFidelityWords(request *ExportRequest) []string {
	words := make([]string, 0, 6)
	if request.Fidelity.ResourceBlobs {
		words = append(words, "blobs")
	}
	if request.Fidelity.ResourceVersions {
		words = append(words, "versions")
	}
	if request.Fidelity.ResourcePreviews {
		words = append(words, "previews")
	}
	if request.Fidelity.ResourceSeries {
		words = append(words, "series")
	}
	if request.SchemaDefs.CategoriesAndTypes {
		words = append(words, "schema")
	}
	if request.SchemaDefs.Tags {
		words = append(words, "tags")
	}
	return words
}

// exportJobTitle is the Job's own title: how many groups were asked for, never
// which — a group name is a person's own data and a title is searchable text.
func exportJobTitle(input json.RawMessage) string {
	request, err := exportRequestOf(input)
	if err != nil {
		return "Group export"
	}
	if len(request.RootGroupIDs) == 1 {
		return "Export of one group"
	}
	return fmt.Sprintf("Export of %d groups", len(request.RootGroupIDs))
}

// exportArchivePath is where one execution's archive is staged.
//
// The stem is the canonical Job's id when the queue entry carries one, and the
// entry's own id otherwise — a deployment with no control plane, which is what the
// package tests and the CLI run.
func exportArchivePath(stem string, gzip bool) string {
	ext := ".tar"
	if gzip {
		ext = ".tar.gz"
	}
	return filepath.Join("_exports", stem+ext)
}

// exportArtifactExpiry is when a published archive's bytes are due: the same
// retention window the export sweep removes _exports files on, so an output's
// deadline and the file's own lifetime are one fact rather than two that drift.
func (ctx *MahresourcesContext) exportArtifactExpiry() time.Time {
	retention := time.Duration(0)
	if ctx != nil && ctx.downloadManager != nil {
		retention = ctx.downloadManager.ExportRetention()
	}
	if retention <= 0 {
		retention = ctx.Config.ExportRetention
	}
	if retention <= 0 {
		retention = 24 * time.Hour
	}
	return time.Now().Add(retention)
}

// groupExportAdapter runs one export Kind.
type groupExportAdapter struct {
	ctx  *MahresourcesContext
	kind string
}

// Definition declares an export restorable: the request that produced an archive is
// sealed with the Job, so a Job whose process died is one this Kind can start
// again. The queue owns its own concurrency budget, so the Kind declares none.
func (a *groupExportAdapter) Definition() jobs.Definition {
	return jobs.Definition{
		Kind:        a.kind,
		KindVersion: jobExportKindVersion,
		Restorable:  true,
		Visibility:  jobs.VisibilityOwner,
	}
}

// forExecution returns this adapter bound to the principal one execution acts as,
// so that what is *authorized* and what is *executed* are one view of the
// subtree.
//
// The binding is the export's whole confinement story: groupio resolves scope
// through the context it is handed (`groupioDeps` reads that context's db and
// principal on every call), so an export run from the singleton context — which
// is what a restart dispatch, a Retry and a Repeat are — follows related groups,
// resources and notes outside the principal's subtree even though the request
// that asked for it was checked against them. Resolving the actor once and using
// that context for the authorization check *and* the worker is what makes the two
// answer the same question; a second resolution, or a check on one context and a
// run on another, is the defect this method exists to prevent.
func (a *groupExportAdapter) forExecution(execution jobs.Execution) *groupExportAdapter {
	if a.ctx == nil || execution.Access.UserID == 0 {
		return a
	}
	principal := a.ctx.principalForPluginActor(execution.Access.UserID)
	if principal == nil {
		return a
	}
	return &groupExportAdapter{ctx: a.ctx.WithPrincipal(principal), kind: a.kind}
}

// Dispatch runs one claimed export: it makes sure this process's queue is running
// it, waits, and publishes the outcome.
func (a *groupExportAdapter) Dispatch(ctx context.Context, execution jobs.Execution) error {
	if a.ctx == nil || a.ctx.downloadManager == nil {
		return errors.New("the download queue is not available")
	}
	input, err := exportRequestOf(execution.Input)
	if err != nil {
		return err
	}
	if execution.KindVersion != jobExportKindVersion {
		return fmt.Errorf("%w: export v%d input", jobs.ErrReplayCodecUnregistered, execution.KindVersion)
	}

	// One binding for both halves: the refusal below and the run function built
	// beneath it read the same context, so an export that passed the check cannot
	// then stream a tree the check would have refused.
	a = a.forExecution(execution)

	if reason := a.refusalReason(execution, input); reason != "" {
		return a.ctx.blockQueueJob(execution.JobID, execution.ExecutionToken, reason)
	}

	entry, found := a.ctx.queueEntryFor(execution.JobID)
	if !found {
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
		return a.publishOutcome(execution, input, snap)
	}
	snap, err := a.ctx.waitForQueueExecution(ctx, execution, entry)
	if err != nil {
		return err
	}
	return a.publishOutcome(execution, input, snap)
}

// refusalReason answers why this execution may not start, or an empty string.
//
// Everything rechecked here was checked when the submission arrived, and none of it
// is a standing permission: a group-limited principal may have lost the group since,
// a retry replays a request made under an older decision, and the group may simply
// be gone.
func (a *groupExportAdapter) refusalReason(execution jobs.Execution, request *ExportRequest) string {
	if execution.Access.UserID == 0 {
		return ""
	}
	scoped := a.ctx.WithPrincipal(a.ctx.principalForPluginActor(execution.Access.UserID))
	if err := scoped.requireWriteRole("run an export"); err != nil {
		return "role-refused"
	}
	for _, id := range request.RootGroupIDs {
		if !scoped.GroupVisible(id) {
			return "group-out-of-scope"
		}
	}
	return ""
}

// start submits the export this execution needs, taking the id from the Job's own
// handle so the panel and the Job Center name one row.
func (a *groupExportAdapter) start(execution jobs.Execution, request *ExportRequest) (*download_queue.DownloadJob, error) {
	legacyID, err := a.ctx.jobHandleFor(execution.JobID, GroupExportHandleNamespace)
	if err != nil {
		return nil, err
	}
	if legacyID == "" {
		legacyID = download_queue.NewJobID()
	}
	return a.ctx.submitQueueJob(
		download_queue.JobOptions{
			Source:       download_queue.JobSourceGroupExport,
			InitialPhase: "queued",
			OwnerUserID:  a.ctx.jobOwnerFor(execution.JobID),
		},
		legacyID,
		jobs.ExecutionRef{JobID: execution.JobID, ExecutionToken: execution.ExecutionToken},
		a.ctx.buildGroupExportRunFn(request),
	)
}

// publishOutcome records what the export reached, if nobody has recorded it yet.
func (a *groupExportAdapter) publishOutcome(execution jobs.Execution, request *ExportRequest, snap *download_queue.DownloadJob) error {
	switch snap.Status {
	case download_queue.JobStatusCompleted:
		path := snap.ResultPath
		if path == "" {
			path = exportArchivePath(execution.JobID, request.Gzip)
		}
		if err := a.ctx.publishQueueArtifact(execution, jobExportArtifactOutput, "Exported archive",
			path, a.ctx.exportArtifactExpiry()); err != nil {
			// The queue says the tar was written and the artifact is not there: that
			// is not a success, whatever the queue's own status says, and it is the
			// one failure a reader has to be able to tell from "the export failed".
			return a.ctx.finishQueueJob(execution, jobs.StateFailed,
				&jobs.Failure{
					Code:    "export-artifact-missing",
					Class:   jobs.FailureClassInternal,
					Message: "the export finished without leaving an archive to hand over",
				},
				[]string{jobExportArtifactOutput})
		}
		return a.ctx.finishQueueJob(execution, jobs.StateSucceeded, nil, []string{jobExportArtifactOutput})
	case download_queue.JobStatusCancelled:
		return a.ctx.finishQueueJob(execution, jobs.StateCancelled, nil, nil)
	default:
		return a.ctx.finishQueueJob(execution, jobs.StateFailed,
			&jobs.Failure{
				Code:    "export-failed",
				Class:   jobs.FailureClassInternal,
				Message: "the export did not complete",
			},
			nil)
	}
}

// Reconcile answers what should happen to one export whose claim expired.
//
// The archive on disk is the evidence, and it is decisive because a file at the
// published path is a complete one: a run that died mid-stream left a `.part` file
// and nothing else. Three answers follow, and the one this Kind never gives is
// "run it again" while an archive is sitting there unpublished — that is the blind
// rerun §3 forbids, and it would replace a finished archive with a second one.
func (a *groupExportAdapter) Reconcile(_ context.Context, request jobs.ReconcileRequest) (jobs.ReconcileDecision, error) {
	if a.ctx == nil || a.ctx.downloadManager == nil {
		return jobs.ReconcileExternalWorkUnproven, nil
	}
	if entry, found := a.ctx.queueEntryFor(request.Snapshot.ID); found {
		if !queueJobTerminal(entry.GetStatus()) {
			// Still running in this process: the Job keeps running under a fresh
			// token, and the runtime re-attaches to the entry.
			return jobs.ReconcileResume, nil
		}
		// Finished here while nobody owned the Job: the fresh dispatch publishes the
		// outcome the expired execution never got to.
		return jobs.ReconcileQueue, nil
	}

	parsed, err := exportRequestOf(request.Input)
	if err != nil {
		return jobs.ReconcileBlock, nil
	}
	outputs, err := a.ctx.jobOutputsFor(request.Snapshot.ID)
	if err != nil {
		return jobs.ReconcileExternalWorkUnproven, nil
	}
	path := exportArchivePath(request.Snapshot.ID, parsed.Gzip)
	if artifact, published := findJobOutput(outputs, jobExportArtifactOutput); published {
		if artifact.Availability != jobs.OutputAvailable {
			return jobs.ReconcileFail, nil
		}
		if _, err := a.ctx.GetDefaultFs().Stat(artifactPathOr(artifact, path)); err != nil {
			return jobs.ReconcileFail, nil
		}
		return jobs.ReconcileSucceed, nil
	}
	if _, err := a.ctx.GetDefaultFs().Stat(path); err == nil {
		if err := a.ctx.publishQueueArtifact(request.Execution, jobExportArtifactOutput, "Exported archive",
			path, a.ctx.exportArtifactExpiry()); err != nil {
			// Publishing under a token that no longer owns the Job is the fence
			// working: somebody else owns the outcome now, and this pass decides
			// nothing on its behalf.
			return jobs.ReconcileExternalWorkUnproven, nil
		}
		return jobs.ReconcileSucceed, nil
	}
	// Nothing was produced here. Running the export again is the only way it can
	// succeed — and it is only safe once the runtime that claimed it is proved
	// gone, because a second export started over a live one writes the same tree
	// twice.
	return a.ctx.queueOnlyIfTheRuntimeIsProvedGone(request), nil
}

// CleanupArtifacts removes one expired export's archive, or confirms it is gone.
//
// A missing file is reported removed: §7 treats it as removed rather than as an
// error, and a second pass must not be refused because the first one worked.
func (a *groupExportAdapter) CleanupArtifacts(_ context.Context, request jobs.ArtifactCleanupRequest) (jobs.ArtifactCleanupResult, error) {
	result := jobs.ArtifactCleanupResult{}
	for _, artifact := range request.Artifacts {
		removed, err := a.ctx.removeArtifact(artifact.Reference)
		if err != nil {
			// A reference this Kind cannot read is one it cannot account for, and an
			// unaccounted artifact keeps its row: the sweep has to be able to say
			// "still there" rather than "removed" and be believed.
			if errors.Is(err, fs.ErrNotExist) {
				result.Removed = append(result.Removed, artifact.Key)
				continue
			}
			return jobs.ArtifactCleanupResult{}, err
		}
		if removed {
			result.Removed = append(result.Removed, artifact.Key)
		}
	}
	return result, nil
}

// Commands reports the controls one export offers right now.
//
// Cancel always (the host narrows it on finished work), Retry on an unsuccessful
// outcome and Repeat on a successful one — a second export of the same groups is a
// second archive, which is what "export again" means, and unlike a download it
// duplicates nothing in the library.
func (a *groupExportAdapter) Commands(_ context.Context, commandContext jobs.CommandContext) ([]jobs.Command, error) {
	commands := []jobs.Command{{
		Key:          jobs.CommandCancel,
		Label:        "Cancel",
		Destructive:  true,
		Confirmation: "Stop this export? Nothing is added to the library by it.",
	}}
	switch commandContext.Snapshot.State {
	case jobs.StateFailed, jobs.StateCancelled, jobs.StateInterrupted:
		commands = append(commands, jobs.Command{Key: jobs.CommandRetry, Label: "Retry"})
	case jobs.StateSucceeded:
		commands = append(commands, jobs.Command{Key: jobs.CommandRepeat, Label: "Export again"})
	}
	return commands, nil
}

// ExecuteCommand runs one control the host decided this Kind owns. Only cancel
// reaches here: Retry and Repeat are the control plane's own lineage work.
func (a *groupExportAdapter) ExecuteCommand(_ context.Context, execution jobs.CommandExecution) (jobs.CommandOutcome, error) {
	if a.ctx == nil || a.ctx.downloadManager == nil {
		return jobs.CommandOutcome{}, errors.New("the download queue is not available")
	}
	if execution.Key != jobs.CommandCancel {
		return jobs.CommandOutcome{}, fmt.Errorf("%w: %s", jobs.ErrCommandNotAdvertised, execution.Key)
	}
	legacyID, err := a.ctx.jobHandleFor(execution.JobID, GroupExportHandleNamespace)
	if err != nil {
		return jobs.CommandOutcome{}, err
	}
	entry, found := a.ctx.downloadManager.GetJob(legacyID)
	if !found || legacyID == "" {
		// Nothing in this process is running it, so there is nothing to stop: the
		// work is a Job the queue will not pick up again, and the host records the
		// cancellation on the Job itself.
		return jobs.CommandOutcome{Status: jobs.CommandStatusSucceeded, Message: "the export is no longer running"}, nil
	}
	if err := a.ctx.downloadManager.Cancel(entry.ID); err != nil {
		var conflict *download_queue.StateConflictError
		if errors.As(err, &conflict) {
			return jobs.CommandOutcome{Status: jobs.CommandStatusSucceeded, Message: "the export had already finished"}, nil
		}
		return jobs.CommandOutcome{}, err
	}
	return jobs.CommandOutcome{Status: jobs.CommandStatusSucceeded, Message: "cancelling"}, nil
}

// buildGroupExportRunFn is the export's executor body: it streams the archive to
// its staging path and renames it into place.
//
// The rename is what makes a file at the published path a complete one, and it is
// also why the export is a two-write artifact rather than a stream a reader could
// fetch halves of.
func (ctx *MahresourcesContext) buildGroupExportRunFn(request *ExportRequest) download_queue.JobRunFn {
	return func(jobCtx context.Context, j *download_queue.DownloadJob, sink download_queue.ProgressSink) error {
		return ctx.runGroupExport(jobCtx, j, sink, request)
	}
}

// runGroupExport is one export execution.
func (ctx *MahresourcesContext) runGroupExport(jobCtx context.Context, j *download_queue.DownloadJob, sink download_queue.ProgressSink, request *ExportRequest) error {
	fs := ctx.GetDefaultFs()
	// fs is already rooted at FileSavePath (BasePathFs in disk mode), so the tar path
	// stays root-relative — matching resource_upload_context.
	if err := fs.MkdirAll("_exports", 0755); err != nil {
		return fmt.Errorf("mkdir _exports: %w", err)
	}
	stem := j.ID
	if j.CanonicalJobID != "" {
		stem = j.CanonicalJobID
	}
	finalPath := exportArchivePath(stem, request.Gzip)
	partialPath := finalPath + jobExportPartialSuffix

	f, err := fs.Create(partialPath)
	if err != nil {
		return fmt.Errorf("create tar: %w", err)
	}

	// Estimate first so TotalSize (bytes) is seeded for the UI's bytes-written bar.
	// EstimateExport walks the scope without reading blob bytes, so it is cheap even
	// for large tars. If it fails we still stream — the bar just stays open-ended.
	var estimatedBytes int64 = -1
	if est, estErr := ctx.EstimateExport(request); estErr == nil && est != nil {
		estimatedBytes = est.EstimatedBytes
		sink.UpdateProgress(0, estimatedBytes)
	}

	sink.SetPhase("preparing")

	// Each incoming event may carry any combination of phase, item count, bytes and
	// warning; route each to the matching sink method so every change broadcasts
	// independently.
	report := func(ev ProgressEvent) {
		if ev.Phase != "" {
			sink.SetPhase(ev.Phase)
		}
		if ev.PhaseTotal > 0 || ev.PhaseCurrent > 0 {
			sink.SetPhaseProgress(int64(ev.PhaseCurrent), int64(ev.PhaseTotal))
		}
		if ev.BytesWritten > 0 {
			sink.UpdateProgress(ev.BytesWritten, estimatedBytes)
		}
		if ev.Warning != "" {
			sink.AppendWarning(ev.Warning)
		}
	}

	streamErr := ctx.StreamExport(jobCtx, request, f, report)
	closeErr := f.Close()
	if streamErr != nil {
		_ = fs.Remove(partialPath)
		return streamErr
	}
	if closeErr != nil {
		_ = fs.Remove(partialPath)
		return closeErr
	}
	if err := fs.Rename(partialPath, finalPath); err != nil {
		_ = fs.Remove(partialPath)
		return fmt.Errorf("finalize tar: %w", err)
	}

	sink.SetResultPath(finalPath)
	sink.SetPhase("completed")
	return nil
}

// SubmitGroupExport is the one door a group export is submitted through.
//
// The order is the design's and it is not negotiable: the durable Job is accepted
// first, and only then is the export dispatched. A deployment with no control plane
// submits exactly as it always has and answers no canonical id.
func (ctx *MahresourcesContext) SubmitGroupExport(request *ExportRequest, origin string) QueueJobSubmission {
	result := QueueJobSubmission{}
	if ctx == nil || ctx.downloadManager == nil {
		result.Err = errors.New("the download queue is not available")
		return result
	}
	if request == nil || len(request.RootGroupIDs) == 0 {
		result.Err = errors.New("an export needs at least one root group")
		return result
	}

	owner := ctx.queueSubmitterOwner()
	service := ctx.JobService()
	if service == nil {
		job, err := ctx.downloadManager.SubmitJobWithOptions(download_queue.JobOptions{
			Source:       download_queue.JobSourceGroupExport,
			InitialPhase: "queued",
			OwnerUserID:  owner,
		}, ctx.buildGroupExportRunFn(request))
		if err != nil {
			result.Err = err
			return result
		}
		result.QueueJobID = job.ID
		return result
	}

	input, err := json.Marshal(exportJobInput{Request: *request})
	if err != nil {
		result.Err = err
		return result
	}
	legacyID := download_queue.NewJobID()
	accepted, err := ctx.acceptQueueJob(jobs.Acceptance{
		Kind:        JobKindGroupExport,
		KindVersion: jobExportKindVersion,
		State:       jobs.StateQueued,
		OwnerUserID: owner,
		ActorUserID: owner,
		Origin:      origin,
		Title:       exportJobTitle(input),
		Replay:      jobs.ReplayInput{Input: input},
		LegacyRefs:  []jobs.LegacyRef{{Namespace: GroupExportHandleNamespace, Handle: legacyID}},
	})
	if err != nil {
		result.Err = err
		return result
	}
	result.CanonicalJobID = accepted.ID
	result.QueueJobID = legacyID

	entry, err := ctx.submitQueueJob(
		download_queue.JobOptions{
			Source:       download_queue.JobSourceGroupExport,
			InitialPhase: "queued",
			OwnerUserID:  owner,
		},
		legacyID,
		jobs.ExecutionRef{JobID: accepted.ID},
		ctx.buildGroupExportRunFn(request),
	)
	if err != nil {
		ctx.failUndispatchedQueueJob(accepted, err)
		result.Err = err
		return result
	}
	result.QueueJobID = entry.ID
	return result
}

// ExportArchive is one export's downloadable archive, as the compatibility route
// reads it from the durable Job rather than from the in-memory queue entry.
//
// The handler maps the State and Availability onto its own status codes, because
// which status a refusal is spelled with is the HTTP surface's business and not this
// layer's.
//
// It is answered by a *route* rather than by a Job adapter because it is the one read
// on an export that is not about the Job's lifecycle: what a client wants is the
// bytes, and whether the artifact that promised them is still there.
type ExportArchive struct {
	// Path is the archive's path in the deployment's storage, empty when the Job has
	// published no artifact to fetch (still queued, failed, or a Job from before this
	// Kind existed).
	Path string
	// State is the Job's normalized state.
	State jobs.State
	// Availability is the artifact's own availability: available, expired or removed.
	Availability jobs.OutputAvailability
	// Published reports whether an artifact output exists at all, which is what tells
	// "not finished yet" apart from "finished and its artifact is gone".
	Published bool
}

// Available reports whether there is an archive to serve right now: a succeeded Job whose
// artifact is published, still available and has a path.
func (a ExportArchive) Available() bool {
	return a.State == jobs.StateSucceeded && a.Published &&
		a.Availability == jobs.OutputAvailable && a.Path != ""
}

// ExportArchiveFor answers one legacy export id's archive from its durable Job.
//
// The answer is a pair rather than a value because the *absence* of a control plane
// is not a refusal: a deployment that runs the queue without one has no durable Jobs
// to consult, and its export download route keeps answering from the queue entry
// exactly as it always has. `durable` is therefore "a Job answered this", and an
// error is returned only when a Job answered with a refusal of its own.
func (ctx *MahresourcesContext) ExportArchiveFor(legacyID string) (ExportArchive, bool, error) {
	service := ctx.JobService()
	if service == nil || legacyID == "" {
		return ExportArchive{}, false, nil
	}
	// Resolved without a viewer, then read with one, exactly as the import
	// authorization is: which Job a handle names is a property of the handle table, and
	// whether this principal may fetch from it is the same question every Job read asks.
	jobID, err := service.ResolveLegacyHandle(ctx.jobDeps(), GroupExportHandleNamespace, legacyID)
	if err != nil {
		if errors.Is(err, jobs.ErrNotFound) {
			// Not a handle: it may still be a queue entry from before this release, or
			// one this process queued without a control plane.
			return ExportArchive{}, false, nil
		}
		return ExportArchive{}, true, err
	}
	snap, err := service.Get(ctx.jobDeps(), ctx.jobAccess(), jobID)
	if err != nil {
		return ExportArchive{}, true, err
	}
	archive := ExportArchive{State: snap.State}
	outputs, err := service.Outputs(ctx.jobDeps(), ctx.jobAccess(), snap.ID)
	if err != nil {
		return ExportArchive{}, true, err
	}
	artifact, published := findJobOutput(outputs, jobExportArtifactOutput)
	if !published {
		return archive, true, nil
	}
	archive.Published = true
	archive.Availability = artifact.Availability
	archive.Path, _ = artifactPathOf(artifact.Reference)
	return archive, true, nil
}

// queueSubmitterOwner is the owner a queue entry records for this context's own
// submission: the principal's id, or nil for the auth-off super-user and for a
// process with no request behind it.
func (ctx *MahresourcesContext) queueSubmitterOwner() *uint {
	principal := ctx.Principal()
	if principal == nil || principal.SuperUser || principal.UserID == 0 {
		return nil
	}
	id := principal.UserID
	return &id
}

// registerWorkflowJobKinds teaches one control plane to run the queue-backed Kinds
// this context's executors own — the exports, imports, reductions and maintenance
// work of Task 8.
//
// It is idempotent for the reason the download registration is: a context may be
// handed a service that already has them, and the second registration of one
// (Kind, version) is refused on purpose — that refusal exists to stop two executors
// for one Kind, not to make wiring order matter.
func (ctx *MahresourcesContext) registerWorkflowJobKinds(service *jobs.Service) error {
	if ctx == nil || service == nil {
		return nil
	}
	registrations := []struct {
		kind    string
		version uint
		codec   jobs.ReplayCodec
		adapter jobs.Adapter
	}{
		{JobKindGroupExport, jobExportKindVersion, exportJobCodec(), &groupExportAdapter{ctx: ctx, kind: JobKindGroupExport}},
		{JobKindGroupImportParse, jobImportKindVersion, importParseJobCodec(), &importParseAdapter{ctx: ctx, kind: JobKindGroupImportParse}},
		{JobKindGroupImportApply, jobImportKindVersion, importApplyJobCodec(), &importApplyAdapter{ctx: ctx, kind: JobKindGroupImportApply}},
		{JobKindReductionCompute, jobReductionKindVersion, reductionComputeJobCodec(), &reductionComputeAdapter{ctx: ctx, kind: JobKindReductionCompute}},
		{JobKindSimilarityRecompute, jobMaintenanceKindVersion, similarityRecomputeJobCodec(), &similarityRecomputeAdapter{ctx: ctx, kind: JobKindSimilarityRecompute}},
	}
	for _, registration := range registrations {
		if !jobs.HasReplayCodec(service, registration.kind, registration.version) {
			if err := service.RegisterReplayCodec(registration.kind, registration.version, registration.codec); err != nil {
				return err
			}
		}
		if _, registered := service.AdapterFor(registration.kind, registration.version); registered {
			continue
		}
		if err := service.RegisterAdapter(registration.adapter); err != nil {
			return err
		}
	}
	return nil
}
