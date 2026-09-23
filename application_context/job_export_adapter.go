package application_context

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"path/filepath"
	"time"

	"mahresources/archive"
	"mahresources/contracts"
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
	// jobExportScopeManifestVersion marks outputs whose archive manifest was
	// validated at publication and contains the full exported source-ID set.
	jobExportScopeManifestVersion = 1
	// jobExportMaxScopeManifestBytes bounds decompressed manifest parsing while
	// still allowing large exports with hundreds of thousands of entries.
	jobExportMaxScopeManifestBytes = 32 << 20
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

// AuthorizeJobOutput rechecks every entity recorded in the archive manifest
// against the asking principal's current scope. The bounded output reference
// carries only a version marker; the full proof stays with the archive itself.
func (a *groupExportAdapter) AuthorizeJobOutput(_ context.Context, request JobOutputOpenRequest) error {
	if a == nil || a.ctx == nil || request.Principal == nil ||
		request.Snapshot.Kind != a.kind || request.Snapshot.KindVersion != jobExportKindVersion ||
		request.Output.Key != jobExportArtifactOutput || request.Output.Type != jobs.OutputTypeArtifact {
		return ErrJobOutputForbidden
	}
	var summary exportSummary
	if len(request.Snapshot.Summary) == 0 || json.Unmarshal(request.Snapshot.Summary, &summary) != nil || len(summary.RootGroups) == 0 {
		return ErrJobOutputForbidden
	}
	path, err := groupExportScopeManifestPath(request.Output, request.Snapshot.ID, summary.Gzip)
	if err != nil {
		return ErrJobOutputForbidden
	}
	scoped := a.ctx.WithPrincipal(request.Principal)
	manifest, err := readGroupExportScopeManifestFromPath(scoped, path)
	if err != nil || !authorizeGroupExportScope(scoped, manifest, summary.RootGroups) {
		return ErrJobOutputForbidden
	}
	return nil
}

// OpenJobOutput repeats current scope authorization against the same open file
// it returns. The shared output policy has already checked the Job and reference;
// this second pass prevents a local file replacement between authorization and
// file serving from swapping in an archive with different source entities.
func (a *groupExportAdapter) OpenJobOutput(_ context.Context, request JobOutputOpenRequest) (contracts.JobOutputContent, error) {
	if a == nil || a.ctx == nil || request.Principal == nil ||
		request.Snapshot.Kind != a.kind || request.Snapshot.KindVersion != jobExportKindVersion ||
		request.Output.Key != jobExportArtifactOutput || request.Output.Type != jobs.OutputTypeArtifact {
		return contracts.JobOutputContent{}, ErrJobOutputForbidden
	}
	var summary exportSummary
	if len(request.Snapshot.Summary) == 0 || json.Unmarshal(request.Snapshot.Summary, &summary) != nil || len(summary.RootGroups) == 0 {
		return contracts.JobOutputContent{}, ErrJobOutputForbidden
	}
	path, err := groupExportScopeManifestPath(request.Output, request.Snapshot.ID, summary.Gzip)
	if err != nil {
		return contracts.JobOutputContent{}, ErrJobOutputForbidden
	}
	file, err := a.ctx.GetDefaultFs().Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return contracts.JobOutputContent{}, ErrJobOutputUnavailable
		}
		return contracts.JobOutputContent{}, fmt.Errorf("open group export output: %w", err)
	}
	closeOnError := true
	defer func() {
		if closeOnError {
			_ = file.Close()
		}
	}()
	manifest, err := readGroupExportScopeManifest(file)
	if err != nil || !authorizeGroupExportScope(a.ctx.WithPrincipal(request.Principal), manifest, summary.RootGroups) {
		return contracts.JobOutputContent{}, ErrJobOutputForbidden
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return contracts.JobOutputContent{}, ErrJobOutputUnavailable
	}
	info, err := file.Stat()
	if err != nil || info.IsDir() {
		return contracts.JobOutputContent{}, ErrJobOutputUnavailable
	}
	contentType := mime.TypeByExtension(filepath.Ext(path))
	if contentType == "" {
		contentType = "application/x-tar"
	}
	closeOnError = false
	return contracts.JobOutputContent{
		Body: file, ContentType: contentType, Filename: safeJobOutputFilename(request.Output.Label, path),
	}, nil
}

func groupExportScopeManifestPath(output jobs.Output, jobID string, gzip bool) (string, error) {
	var reference queueArtifactReference
	if len(output.Reference) == 0 || json.Unmarshal(output.Reference, &reference) != nil ||
		reference.ScopeManifestVersion != jobExportScopeManifestVersion || reference.Path == "" {
		return "", ErrJobOutputForbidden
	}
	path, err := rootedJobOutputPath(reference.Path)
	if err != nil || path != exportArchivePath(jobID, gzip) {
		return "", ErrJobOutputForbidden
	}
	return path, nil
}

func readGroupExportScopeManifestFromPath(ctx *MahresourcesContext, path string) (*archive.Manifest, error) {
	if ctx == nil {
		return nil, errors.New("application context is unavailable")
	}
	clean, err := rootedJobOutputPath(path)
	if err != nil {
		return nil, err
	}
	file, err := ctx.GetDefaultFs().Open(clean)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return readGroupExportScopeManifest(file)
}

func readGroupExportScopeManifest(source io.Reader) (*archive.Manifest, error) {
	reader, err := archive.NewReaderWithManifestLimit(source, jobExportMaxScopeManifestBytes)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	manifest, err := reader.ReadManifest()
	if err != nil {
		return nil, err
	}
	if err := validateGroupExportScopeManifest(manifest, nil); err != nil {
		return nil, err
	}
	return manifest, nil
}

func validateGroupExportScopeManifest(manifest *archive.Manifest, input json.RawMessage) error {
	if manifest == nil || len(manifest.Entries.Groups) == 0 ||
		manifest.Counts.Groups != len(manifest.Entries.Groups) || manifest.Counts.ShellGroups > manifest.Counts.Groups ||
		manifest.Counts.Resources != len(manifest.Entries.Resources) ||
		manifest.Counts.Notes != len(manifest.Entries.Notes) ||
		manifest.Counts.Series != len(manifest.Entries.Series) {
		return errors.New("manifest entries do not prove the complete export scope")
	}
	if _, err := uniqueGroupExportSourceIDs("group", manifest.Entries.Groups, func(entry archive.GroupEntry) uint { return entry.SourceID }); err != nil {
		return err
	}
	if _, err := uniqueGroupExportSourceIDs("resource", manifest.Entries.Resources, func(entry archive.ResourceEntry) uint { return entry.SourceID }); err != nil {
		return err
	}
	if _, err := uniqueGroupExportSourceIDs("note", manifest.Entries.Notes, func(entry archive.NoteEntry) uint { return entry.SourceID }); err != nil {
		return err
	}
	if _, err := uniqueGroupExportSourceIDs("series", manifest.Entries.Series, func(entry archive.SeriesEntry) uint { return entry.SourceID }); err != nil {
		return err
	}
	if len(input) > 0 {
		request, err := exportRequestOf(input)
		if err != nil {
			return err
		}
		for _, root := range request.RootGroupIDs {
			if !manifestHasExportedRoot(manifest, root) {
				return errors.New("manifest omits an accepted root group")
			}
		}
	}
	return nil
}

func manifestHasExportedRoot(manifest *archive.Manifest, sourceID uint) bool {
	for _, group := range manifest.Entries.Groups {
		if group.SourceID != sourceID {
			continue
		}
		for _, root := range manifest.Roots {
			if root == group.ExportID {
				return true
			}
		}
	}
	return false
}

func uniqueGroupExportSourceIDs[T any](kind string, entries []T, sourceID func(T) uint) (map[uint]bool, error) {
	ids := make(map[uint]bool, len(entries))
	for _, entry := range entries {
		id := sourceID(entry)
		if id == 0 || ids[id] {
			return nil, fmt.Errorf("manifest has a missing or duplicate %s source id", kind)
		}
		ids[id] = true
	}
	return ids, nil
}

func groupExportManifestSourceIDs(manifest *archive.Manifest) (groups, resources, notes, series map[uint]bool) {
	groups, _ = uniqueGroupExportSourceIDs("group", manifest.Entries.Groups, func(entry archive.GroupEntry) uint { return entry.SourceID })
	resources, _ = uniqueGroupExportSourceIDs("resource", manifest.Entries.Resources, func(entry archive.ResourceEntry) uint { return entry.SourceID })
	notes, _ = uniqueGroupExportSourceIDs("note", manifest.Entries.Notes, func(entry archive.NoteEntry) uint { return entry.SourceID })
	series, _ = uniqueGroupExportSourceIDs("series", manifest.Entries.Series, func(entry archive.SeriesEntry) uint { return entry.SourceID })
	return
}

func authorizeGroupExportScope(ctx *MahresourcesContext, manifest *archive.Manifest, roots []uint) bool {
	if ctx == nil || validateGroupExportScopeManifest(manifest, nil) != nil {
		return false
	}
	groups, resources, notes, series := groupExportManifestSourceIDs(manifest)
	for _, id := range roots {
		if id == 0 || !groups[id] || !manifestHasExportedRoot(manifest, id) {
			return false
		}
	}
	return exportSourceIDsVisible(ctx, "groups", mapKeys(groups)) &&
		exportSourceIDsVisible(ctx, "resources", mapKeys(resources)) &&
		exportSourceIDsVisible(ctx, "notes", mapKeys(notes)) &&
		exportSeriesIDsVisible(ctx, mapKeys(series))
}

// exportSourceIDsVisible checks entity existence in bounded batches. Raw reads
// avoid GORM adding the full group allow-list as a second large IN clause; the
// allow-list is then applied in memory with the same containment rule.
func exportSourceIDsVisible(ctx *MahresourcesContext, table string, ids []uint) bool {
	if ctx == nil || len(ids) == 0 {
		return ctx != nil
	}
	scoped := ctx.isScopedPrincipal()
	for _, chunk := range chunkUints(ids, 500) {
		if table == "groups" {
			var rows []struct {
				ID uint `gorm:"column:id"`
			}
			if err := ctx.db.Raw("SELECT id FROM groups WHERE id IN ?", chunk).Scan(&rows).Error; err != nil || len(rows) != len(chunk) {
				return false
			}
			if scoped {
				allowed := ctx.visibleGroupIDs(chunk)
				for _, id := range chunk {
					if !allowed[id] {
						return false
					}
				}
			}
			continue
		}
		if table != "resources" && table != "notes" {
			return false
		}
		var rows []struct {
			ID      uint  `gorm:"column:id"`
			OwnerID *uint `gorm:"column:owner_id"`
		}
		query := "SELECT id, owner_id FROM " + table + " WHERE id IN ?"
		if err := ctx.db.Raw(query, chunk).Scan(&rows).Error; err != nil || len(rows) != len(chunk) {
			return false
		}
		if scoped {
			ownerIDs := make([]uint, 0, len(rows))
			for _, row := range rows {
				if row.OwnerID == nil {
					return false
				}
				ownerIDs = append(ownerIDs, *row.OwnerID)
			}
			allowed := ctx.visibleGroupIDs(ownerIDs)
			for _, ownerID := range ownerIDs {
				if !allowed[ownerID] {
					return false
				}
			}
		}
	}
	return true
}

// exportSeriesIDsVisible treats a series as visible when it still belongs to
// at least one current resource the principal can see. Series have no direct
// group owner, so the resource membership is their scope boundary.
func exportSeriesIDsVisible(ctx *MahresourcesContext, ids []uint) bool {
	if ctx == nil || len(ids) == 0 {
		return ctx != nil
	}
	scoped := ctx.isScopedPrincipal()
	visible := make(map[uint]bool, len(ids))
	for _, chunk := range chunkUints(ids, 500) {
		var rows []struct {
			SeriesID uint  `gorm:"column:series_id"`
			OwnerID  *uint `gorm:"column:owner_id"`
		}
		if err := ctx.db.Raw("SELECT series_id, owner_id FROM resources WHERE series_id IN ? AND series_id IS NOT NULL", chunk).Scan(&rows).Error; err != nil {
			return false
		}
		ownerIDs := make([]uint, 0, len(rows))
		if scoped {
			for _, row := range rows {
				if row.OwnerID == nil {
					continue
				}
				ownerIDs = append(ownerIDs, *row.OwnerID)
			}
		}
		allowed := ctx.visibleGroupIDs(ownerIDs)
		for _, row := range rows {
			if scoped && (row.OwnerID == nil || !allowed[*row.OwnerID]) {
				continue
			}
			visible[row.SeriesID] = true
		}
	}
	for _, id := range ids {
		if !visible[id] {
			return false
		}
	}
	return true
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
			if errors.Is(err, errQueueStagedOutputMissing) {
				// The queue says the tar was written and there is no file: that is not
				// a success, whatever the queue's own status says, and it is the one
				// failure a reader has to be able to tell from "the export failed".
				return a.ctx.finishQueueJob(execution, jobs.StateFailed,
					&jobs.Failure{
						Code:    "export-artifact-missing",
						Class:   jobs.FailureClassInternal,
						Message: "the export finished without leaving an archive to hand over",
					},
					[]string{jobExportArtifactOutput})
			}
			// The publication itself was refused — a locked database, a pool briefly
			// exhausted, a version that moved. The archive is on disk and the outcome is
			// not the executor's to lose: the owner of this execution offers the whole
			// publication again. Ending the Job here is how a finished export became an
			// immutable `export-artifact-missing`.
			return err
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
		// The input is what this Kind's reconciliation is made of, and a reconciler that
		// cannot read it — no key for the envelope, a payload from a newer Kind version —
		// has decided nothing about the work. Blocking here released the claim, the token
		// and the capacity of an export that may still be running in the process that
		// holds the key. The one answer that needs no input is proof that the runtime
		// which started the work is gone.
		return a.ctx.queueOnlyIfTheRuntimeIsProvedGone(request), nil
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
	// §8: a principal demoted below "may write" keeps the history and loses the
	// controls over it.
	if a.ctx.commandActorRefusal(commandContext.Deps, commandContext.Access, "") != "" {
		return nil, nil
	}
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
	admission, err := ctx.admitQueueJob(jobs.Acceptance{
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
	result.CanonicalJobID = admission.Accepted.ID
	result.QueueJobID = legacyID
	if !admission.Owned() {
		// The deployment's budget is full: the Job is durable, answers the id the
		// client was handed, and starts no executor here. A runtime with a free slot
		// takes it — see admitQueueJob.
		return result
	}

	entry, err := ctx.submitQueueJob(
		download_queue.JobOptions{
			Source:       download_queue.JobSourceGroupExport,
			InitialPhase: "queued",
			OwnerUserID:  owner,
		},
		legacyID,
		jobs.ExecutionRef{JobID: admission.Execution.JobID, ExecutionToken: admission.Execution.ExecutionToken},
		ctx.buildGroupExportRunFn(request),
	)
	if err != nil {
		ctx.failUndispatchedQueueJob(admission, err)
		result.Err = err
		return result
	}
	result.QueueJobID = entry.ID
	ctx.ownQueueExecution(admission, entry, func(snap *download_queue.DownloadJob) error {
		return (&groupExportAdapter{ctx: ctx, kind: JobKindGroupExport}).publishOutcome(admission.Execution, request, snap)
	})
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
	storedPath, pathErr := artifactPathOf(artifact.Reference)
	if pathErr == nil {
		archive.Path, pathErr = rootedJobOutputPath(storedPath)
	}
	if pathErr != nil {
		archive.Path = ""
	}
	if artifact.Availability != jobs.OutputAvailable || archive.Path == "" {
		return archive, true, nil
	}
	if _, err := ctx.GetDefaultFs().Stat(archive.Path); err != nil {
		// Keep the legacy download route's established 410 answer for an artifact
		// whose bytes have already been removed by retention or external cleanup.
		return archive, true, nil
	}
	request := JobOutputOpenRequest{Snapshot: snap, Output: artifact, Principal: ctx.Principal()}
	if err := ctx.authorizeJobOutput(context.Background(), service, request); err != nil {
		if errors.Is(err, ErrJobOutputForbidden) {
			return ExportArchive{}, true, jobs.ErrNotFound
		}
		return ExportArchive{}, true, err
	}
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
		{JobKindSummaryExport, jobSummaryExportVersion, jobSummaryExportCodec(), &jobSummaryExportAdapter{ctx: ctx}},
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
