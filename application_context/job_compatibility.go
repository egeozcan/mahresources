package application_context

import (
	"encoding/json"
	"errors"
	"fmt"

	"mahresources/download_queue"
	"mahresources/jobs"
	"mahresources/models/query_models"
)

// This file is the compatibility seam between the legacy identifier spaces the
// deployed UI, API and CLI already speak and the canonical Jobs that now carry
// the work behind them.
//
// A legacy `id` is a handle, not an identity. It names the current leaf of a
// linear Retry lineage, which is what lets a bookmark or a polling client keep
// one id across successive retries while every attempt keeps its own immutable
// UUID (ADR 0007). The durable half of that mapping lives in the Job control
// plane — it is written with acceptance and moved with a Retry successor, both
// atomically — and what is left here is the only thing the application can add:
// resolving a handle as the asking principal, so a handle grants no rights over
// the Job it names.

// ScheduledDownloadHandleNamespace is the namespace of the scheduled-download
// store's own row ids. It is separate from the download queue's because they are
// different id spaces: a row id and a queue id are both small numbers or short
// strings, and one must never resolve as the other.
const ScheduledDownloadHandleNamespace = "scheduled-download"

// DownloadHandleNamespace is the namespace of the download queue's legacy ids.
//
// The namespace is part of the key rather than decoration: two legacy id spaces
// (a download queue id and a plugin action job id) are both short random strings,
// and one of them must not resolve as the other.
const DownloadHandleNamespace = "download"

// ResolveJobHandle answers the canonical Job one legacy identifier currently
// names, under this context's own principal.
//
// A handle carries no authority, so the Job it resolves to is authorized exactly
// as any other read would be: an asker who may not see it is answered NotFound,
// which is the same answer a missing handle gets. That is deliberate — the
// difference between "there is no such handle" and "there is one and you may not
// have it" is itself information about somebody else's work.
func (ctx *MahresourcesContext) ResolveJobHandle(namespace, handle string) (jobs.Snapshot, error) {
	service, err := ctx.requireJobService()
	if err != nil {
		return jobs.Snapshot{}, err
	}
	jobID, err := service.ResolveLegacyHandle(ctx.jobDeps(), namespace, handle)
	if err != nil {
		return jobs.Snapshot{}, err
	}
	return service.Get(ctx.jobDeps(), ctx.jobAccess(), jobID)
}

// JobHandlesFor lists the legacy identifiers one visible Job currently answers to.
// It is the reverse projection: a surface that has a canonical Job and wants to
// print the id a legacy client would use asks this, and the visibility check is
// the Job read's.
func (ctx *MahresourcesContext) JobHandlesFor(jobID string) ([]jobs.LegacyRef, error) {
	service, err := ctx.requireJobService()
	if err != nil {
		return nil, err
	}
	if _, err := service.Get(ctx.jobDeps(), ctx.jobAccess(), jobID); err != nil {
		return nil, err
	}
	return service.LegacyHandlesFor(ctx.jobDeps(), jobID)
}

// JobHandleNamespaceFor resolves the single handle in one namespace a Job answers
// to, or an empty string when it answers to none.
//
// A Job may carry several handles in one namespace after a migration that met an
// older one, and a projection that picked between them arbitrarily would hand
// different clients different ids for one execution. The first in the stable
// order is what every caller sees.
func (ctx *MahresourcesContext) JobHandleNamespaceFor(jobID, namespace string) (string, error) {
	refs, err := ctx.JobHandlesFor(jobID)
	if err != nil {
		return "", err
	}
	for _, ref := range refs {
		if ref.Namespace == namespace {
			return ref.Handle, nil
		}
	}
	return "", nil
}

// queueBackedHandleNamespaces lists the legacy id spaces this projector resolves, in the
// order they are tried: the download queue's first, because it is the id space every
// deployed client already speaks, and then the queue-backed Kinds that were given their
// own durable namespaces.
//
// They are all resolved here rather than only the download one because the compatibility
// routes are not download-only: `/v1/jobs/get` and `/v1/jobs/cancel` are the two paths a
// CLI or a bookmark uses for *any* background job, and a queue-backed export whose Job was
// accepted but not dispatched has no queue entry in this process to fall back to. A
// namespace left out of this list is a 404 for an id the server itself just answered with.
//
// The Source each one projects onto is the label the jobs panel and the legacy rows have
// always used for that Kind, so a client branching on `source` keeps working.
var queueBackedHandleNamespaces = []struct {
	Namespace string
	Source    string
}{
	{DownloadHandleNamespace, download_queue.JobSourceDownload},
	{GroupExportHandleNamespace, download_queue.JobSourceGroupExport},
	{ImportParseHandleNamespace, download_queue.JobSourceGroupImportParse},
	{ImportApplyHandleNamespace, download_queue.JobSourceGroupImportApply},
	{ReductionComputeHandleNamespace, download_queue.JobSourceResourceReduction},
	{SimilarityRecomputeHandleNamespace, maintenanceJobSource},
}

// resolveQueueBackedHandle answers the canonical Job one legacy identifier currently
// names in any of the queue-backed id spaces, together with the source label that id
// space projects onto.
//
// The namespaces are tried in order and the first hit wins. Each is a space of random
// ids, so two spaces holding one string is not a case that arises; trying the download
// space first is what keeps every already-deployed client on exactly the path it had.
func (ctx *MahresourcesContext) resolveQueueBackedHandle(id string) (*jobs.Snapshot, string, string, error) {
	service, err := ctx.requireJobService()
	if err != nil {
		return nil, "", "", err
	}
	for _, candidate := range queueBackedHandleNamespaces {
		jobID, err := service.ResolveLegacyHandle(ctx.jobDeps(), candidate.Namespace, id)
		if err != nil {
			if errors.Is(err, jobs.ErrNotFound) {
				// Not a handle in this space. It may be one in the next, or a raw queue id.
				continue
			}
			return nil, "", "", err
		}
		// The handle exists, so the Job it *currently* names is the answer, and this
		// principal's authorization for that Job is the next question — not a reason to
		// look somewhere else. A Retry may have moved the handle onto a successor somebody
		// else owns: falling back to this process's queue entry would then publish the
		// ancestor under the successor's handle, and a control on that row would act on
		// work the asker was just refused.
		resolved, err := service.Get(ctx.jobDeps(), ctx.jobAccess(), jobID)
		if err != nil {
			return nil, "", "", err
		}
		return &resolved, candidate.Source, candidate.Namespace, nil
	}
	return nil, "", "", nil
}

// ProjectDownloadJob answers the legacy row one download identifier currently names.
//
// It resolves in the order the compatibility contract implies, and each step is
// skipped when nothing is there:
//
//  1. the durable handle table, which is what makes an unchanged legacy id follow
//     successive retries to the execution it now means;
//  2. the queue entry this process holds for that Job, if it holds one;
//  3. otherwise a projection of the durable Job itself, so a client polling after a
//     restart — or before the transfer was dispatched — still gets a row rather
//     than a 404 for work that is plainly still going to happen.
//
// A raw queue id that no handle names is still resolved as an entry, which is what
// keeps every legacy client that has an id from before this release working.
func (ctx *MahresourcesContext) ProjectDownloadJob(id string) (download_queue.DownloadProjection, error) {
	projection := download_queue.DownloadProjection{ID: id}
	if ctx == nil {
		return projection, fmt.Errorf("%w: download %s", jobs.ErrNotFound, id)
	}

	var canonical *jobs.Snapshot
	source := download_queue.JobSourceDownload
	legacyNamespace := ""
	if ctx.JobService() != nil {
		resolved, resolvedSource, resolvedNamespace, err := ctx.resolveQueueBackedHandle(id)
		switch {
		case err != nil:
			return projection, err
		case resolved != nil:
			canonical, source = resolved, resolvedSource
			legacyNamespace = resolvedNamespace
		}
	}

	if canonical != nil {
		projection.CanonicalJobID = canonical.ID
		projection.CanonicalVersion = canonical.Version
		projection.CanonicalState = string(canonical.State)
		projection.LegacyNamespace = legacyNamespace
		if entry, ok := ctx.downloadManager.GetJobByCanonicalJobID(canonical.ID); ok {
			projection.Entry = entry
			// The live entry wins while it is not behind the record. It is the thing
			// actually transferring, and its progress is finer than a Job's snapshot.
			// It does not win when the record has already ended the work and the entry
			// has not caught up — a cancellation the queue has yet to unwind, a mirror
			// still in flight — because reporting a transfer as running after its Job
			// is finished is the one direction that misleads.
			if !canonical.Terminal() || downloadRowTerminal(entry) {
				projection.Row = downloadRowFromEntry(entry, id, canonical.ID)
				return projection, nil
			}
			projection.Row = downloadRowFromJob(*canonical, id, source)
			return projection, nil
		}
		// No queue entry in this process. The durable Job is what answers, whatever
		// state it reached: a handle onto finished work still names that work, and the
		// queue this process happens to hold is not the record of it. A restart, an
		// eviction and the panel's own dismissal all look exactly like this from here,
		// and none of them is evidence that the work never existed or that somebody
		// cleared it — the Job Center's dismissal is a per-viewer preference, not a
		// deletion, and a client that kept one id may still read its outcome and ask
		// for a Retry.
		projection.Row = downloadRowFromJob(*canonical, id, source)
		return projection, nil
	}

	entry, ok := ctx.downloadManager.GetJob(id)
	if !ok {
		return projection, fmt.Errorf("%w: download %s", jobs.ErrNotFound, id)
	}
	// Visibility is the queue's own rule for a row that has no Job behind it: a
	// non-administrator sees only what they submitted, and an ownerless row is
	// nobody's.
	if !ctx.downloadRowVisible(entry) {
		return projection, fmt.Errorf("%w: download %s", jobs.ErrNotFound, id)
	}
	projection.Entry = entry
	projection.Row = downloadRowFromEntry(entry, id, entry.CanonicalJobID)
	return projection, nil
}

// downloadRowVisible applies the queue's visibility rule to one entry, at the
// principal this context carries.
func (ctx *MahresourcesContext) downloadRowVisible(entry *download_queue.DownloadJob) bool {
	principal := ctx.Principal()
	if principal == nil || principal.IsAdmin() {
		return true
	}
	owner := entry.GetOwnerUserID()
	return owner != nil && *owner == principal.UserID
}

// downloadRowTerminal reports whether a queue entry has reached one of the queue's
// own terminal statuses.
func downloadRowTerminal(entry *download_queue.DownloadJob) bool {
	if entry == nil {
		return false
	}
	switch entry.GetStatus() {
	case download_queue.JobStatusCompleted, download_queue.JobStatusFailed, download_queue.JobStatusCancelled:
		return true
	default:
		return false
	}
}

// downloadRowFromEntry is the legacy row for a queue entry: the entry's own
// snapshot, reporting the handle the caller asked for rather than the entry's own
// id, because the two differ exactly when a retry has moved the handle.
func downloadRowFromEntry(entry *download_queue.DownloadJob, handle, canonicalJobID string) *download_queue.DownloadJob {
	snap := entry.Snapshot()
	snap.ID = handle
	snap.CanonicalJobID = canonicalJobID
	return snap
}

// downloadRowFromJob projects a durable Job into the legacy row shape, for a download
// this process's queue does not hold.
//
// It is a projection and nothing more: the queue's own statuses are finer than a
// Job's states, so a state maps onto the queue's closest word for it, and no
// progress is invented. A client that needs the authoritative view has the
// canonical surfaces for exactly that.
//
// source is the label the id's own namespace uses, so an export projected here is not
// relabelled a download: the jobs panel branches on it, and a client that kept an export
// id from before this release still recognizes its own row.
func downloadRowFromJob(projected jobs.Snapshot, handle, source string) *download_queue.DownloadJob {
	row := &download_queue.DownloadJob{
		ID:             handle,
		URL:            downloadURLFromSummary(projected.Summary),
		Status:         downloadStatusFromState(projected.State),
		Progress:       progressCompleted(projected.Progress),
		TotalSize:      progressTotal(projected.Progress),
		CreatedAt:      projected.AcceptedAt,
		Source:         source,
		CanonicalJobID: projected.ID,
		Phase:          projected.Phase,
	}
	if projected.Progress.Completed != nil {
		row.Progress = *projected.Progress.Completed
	}
	if projected.Progress.Total != nil {
		row.TotalSize = *projected.Progress.Total
	}
	if projected.Progress.Total != nil && *projected.Progress.Total > 0 {
		row.ProgressPercent = float64(row.Progress) * 100 / float64(*projected.Progress.Total)
	} else {
		row.ProgressPercent = -1
	}
	row.StartedAt = projected.StartedAt
	row.CompletedAt = projected.FinishedAt
	if projected.Failure != nil {
		row.Error = projected.Failure.Message
	}
	return row
}

// downloadStatusFromState maps a normalized Job state onto the queue's own status
// vocabulary, which is what every legacy consumer switches on.
func downloadStatusFromState(state jobs.State) download_queue.JobStatus {
	switch state {
	case jobs.StateScheduled, jobs.StateQueued:
		return download_queue.JobStatusPending
	case jobs.StateRunning:
		return download_queue.JobStatusDownloading
	case jobs.StatePaused, jobs.StateBlocked:
		return download_queue.JobStatusPaused
	case jobs.StateSucceeded:
		return download_queue.JobStatusCompleted
	case jobs.StateCancelled:
		return download_queue.JobStatusCancelled
	default:
		return download_queue.JobStatusFailed
	}
}

func progressCompleted(progress jobs.Progress) int64 {
	if progress.Completed != nil {
		return *progress.Completed
	}
	return 0
}

func progressTotal(progress jobs.Progress) int64 {
	if progress.Total != nil {
		return *progress.Total
	}
	return -1
}

// downloadURLFromSummary reads the sanitized host back out of a Job's summary.
//
// It is deliberately not the URL: a summary never carries one, and a legacy row
// built from a Job this process never queued has no URL to show. What it shows is
// the sanitized host, which is what a person needs to recognize the row.
func downloadURLFromSummary(summary json.RawMessage) string {
	var decoded downloadSummary
	if len(summary) == 0 {
		return ""
	}
	if err := json.Unmarshal(summary, &decoded); err != nil {
		return ""
	}
	if decoded.Host == "" {
		return ""
	}
	if decoded.Scheme == "" {
		return decoded.Host
	}
	return decoded.Scheme + "://" + decoded.Host
}

// DownloadRestartPayload returns the submission a stored download Job was accepted
// with, so a caller can re-validate it against the principal doing the restarting.
//
// The stored payload is a record of what was once asked for, not a standing
// permission: a user whose confinement changed after submitting — or whose scope
// group moved in the tree — must not be able to press a button and have the old
// targets honoured. The validation itself belongs to the HTTP layer, which is where
// the group-visibility question is asked, so this hands back the payload rather than
// deciding about it.
//
// It opens sealed input, so it is a read for a purpose rather than a casual one: the
// caller must already have resolved the Job under its own visibility.
func (ctx *MahresourcesContext) DownloadRestartPayload(canonicalJobID string) (*query_models.ResourceFromRemoteCreator, error) {
	service, err := ctx.requireJobService()
	if err != nil {
		return nil, err
	}
	opened, err := service.OpenReplay(ctx.jobDeps(), ctx.jobAccess(), canonicalJobID)
	if err != nil {
		return nil, err
	}
	var decoded downloadJobInput
	if err := json.Unmarshal(opened.Input, &decoded); err != nil {
		return nil, fmt.Errorf("the stored download input is not readable: %w", err)
	}
	if decoded.Creator == nil {
		return nil, errors.New("the stored download input names no submission")
	}
	return decoded.Creator, nil
}

// ProjectDownloadQueue answers every legacy row this principal may see for the
// queue-backed Kinds: the entries this process's queue holds, and the durable Jobs that
// have no entry anywhere yet.
//
// The second half is the point. A submission accepted with no capacity to run it starts
// no queue entry — it waits, durably, for a runtime with room — and a legacy listing
// that read only the queue's own memory therefore omitted work the server had just
// accepted and answered an id for. The same is true of a Job this process is not the one
// running: the queue is per-process memory, the Job is not.
//
// Merging them is the part that has to be careful, and the rule is the single-Job
// projection's: the live entry wins while it is not behind the record, because it is the
// thing actually running and its progress is finer than a Job's snapshot — and it does
// not win when the record has already ended the work, or when the handle has moved on to
// a successor, because reporting an ancestor's row under an id that now names the
// successor is how a Retry's stale row came back to life.
func (ctx *MahresourcesContext) ProjectDownloadQueue() ([]*download_queue.DownloadJob, error) {
	if ctx == nil || ctx.downloadManager == nil {
		return nil, nil
	}

	rows := make([]*download_queue.DownloadJob, 0, 8)
	// Where each canonical Job's row sits, so a live entry can replace it in place
	// rather than appearing twice.
	positionOfJob := map[string]int{}
	seenHandle := map[string]bool{}

	if service := ctx.JobService(); service != nil {
		page, err := service.List(ctx.jobDeps(), ctx.jobAccess(), jobs.Filter{
			Kinds:  queueBackedHandleKindNames,
			States: nonterminalJobStates(),
		}, jobs.Cursor{}, maxLegacyQueueRows)
		if err != nil {
			return nil, err
		}
		for _, projected := range page.Jobs {
			handle, err := ctx.JobHandleNamespaceFor(projected.ID, namespaceForKind(projected.Kind))
			if err != nil || handle == "" {
				// A Job with no handle in its own namespace is one no legacy client can
				// name, so there is nothing to project: the canonical surfaces are where
				// it is read.
				continue
			}
			if seenHandle[handle] {
				continue
			}
			seenHandle[handle] = true
			positionOfJob[projected.ID] = len(rows)
			rows = append(rows, downloadRowFromJob(projected, handle, sourceForKind(projected.Kind)))
		}
	}

	for _, entry := range ctx.downloadManager.GetJobs() {
		if !ctx.downloadRowVisible(entry) {
			continue
		}
		canonical := entry.CanonicalJobID
		if canonical != "" {
			// One question decides whether this entry may be published at all: does the
			// handle still name *this* execution? After a Retry it does not — the
			// ancestor's entry keeps its id while the id now belongs to the successor —
			// and forwarding it would show a finished attempt under live work's name.
			current, err := ctx.JobHandleNamespaceFor(canonical, downloadNamespaceForEntry(entry))
			if err != nil || current != entry.ID {
				continue
			}
			if position, known := positionOfJob[canonical]; known {
				// The live entry wins over the projection of the same Job: it is the
				// thing actually running, its progress is finer than a snapshot, and the
				// queue's own status vocabulary is what every legacy consumer switches
				// on. It replaces the durable row rather than being skipped by it — the
				// two are one row, and taking the projected one would report a running
				// transfer as a Job state no panel row has ever carried.
				rows[position] = downloadRowFromEntry(entry, entry.ID, canonical)
				continue
			}
			// No durable row: the Job behind this entry is terminal — the queue
			// remembers finished work until it is evicted, and the panel has always
			// shown it — or it is older than the durable listing's bound.
		}
		if seenHandle[entry.ID] {
			continue
		}
		seenHandle[entry.ID] = true
		rows = append(rows, entry.Snapshot())
	}
	return rows, nil
}

// maxLegacyQueueRows bounds the durable half of one legacy queue listing. The in-memory
// queue the legacy surfaces were built around caps itself, so a projection that returned
// every nonterminal Job of every kind without a bound would be a different surface with
// a different cost; this keeps the listing the size it always was.
const maxLegacyQueueRows = 200

// downloadNamespaceForEntry answers the legacy id space one queue entry belongs to.
func downloadNamespaceForEntry(entry *download_queue.DownloadJob) string {
	if entry == nil {
		return DownloadHandleNamespace
	}
	for _, candidate := range queueBackedHandleNamespaces {
		if candidate.Source == entry.Source {
			return candidate.Namespace
		}
	}
	return DownloadHandleNamespace
}

// namespaceForKind answers the legacy id space one canonical Kind is named by.
func namespaceForKind(kind string) string {
	switch kind {
	case JobKindRemoteDownload, JobKindDeferredDownload:
		return DownloadHandleNamespace
	case JobKindGroupExport:
		return GroupExportHandleNamespace
	case JobKindGroupImportParse:
		return ImportParseHandleNamespace
	case JobKindGroupImportApply:
		return ImportApplyHandleNamespace
	case JobKindReductionCompute:
		return ReductionComputeHandleNamespace
	case JobKindSimilarityRecompute:
		return SimilarityRecomputeHandleNamespace
	default:
		return ""
	}
}

// sourceForKind answers the legacy source label one canonical Kind projects onto: the
// word the jobs panel and the legacy rows have always used for that work.
func sourceForKind(kind string) string {
	switch kind {
	case JobKindRemoteDownload, JobKindDeferredDownload:
		return download_queue.JobSourceDownload
	case JobKindGroupExport:
		return download_queue.JobSourceGroupExport
	case JobKindGroupImportParse:
		return download_queue.JobSourceGroupImportParse
	case JobKindGroupImportApply:
		return download_queue.JobSourceGroupImportApply
	case JobKindReductionCompute:
		return download_queue.JobSourceResourceReduction
	case JobKindSimilarityRecompute:
		return maintenanceJobSource
	default:
		return kind
	}
}

// queueBackedHandleKindNames is every canonical Kind the legacy queue surfaces project,
// in the order the id spaces were listed above.
var queueBackedHandleKindNames = []string{
	JobKindRemoteDownload,
	JobKindDeferredDownload,
	JobKindGroupExport,
	JobKindGroupImportParse,
	JobKindGroupImportApply,
	JobKindReductionCompute,
	JobKindSimilarityRecompute,
}

// nonterminalJobStates is the vocabulary a legacy listing includes: the states in which
// work is still going to happen. Finished work is deliberately absent — the legacy panel
// shows the entries its own queue still holds, and resurrecting a month of terminal
// history into it would be a different listing from the one every client reads.
func nonterminalJobStates() []string {
	states := make([]string, 0, len(jobs.AllStates))
	for _, state := range jobs.AllStates {
		if !state.Terminal() {
			states = append(states, string(state))
		}
	}
	return states
}
